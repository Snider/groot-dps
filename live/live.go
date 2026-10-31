// SPDX-License-Identifier: EUPL-1.2

// Package live runs captured TCP segments through the decoders into a meter.
// Pure: no capture, no clock. Safe to feed from one goroutine and read from others.
package live

import (
	"sync"
	"time"

	"groot/dps/meter"
	"groot/dps/tcp"
	"groot/dps/wire"
)

const (
	maxFlows    = 64
	pruneEvery  = 30 * time.Second
	finishEvery = 2 * time.Second
)

type flow struct {
	stream tcp.Stream
	split  wire.Splitter
	seen   time.Time
}

// Pipeline: server → client TCP segments in, meter snapshots out.
type Pipeline struct {
	mu      sync.Mutex
	m       *meter.Meter
	flows   map[string]*flow
	packets int
	pruned  time.Time

	// OnFinished, if set, gets each fight you were in once it's over (killed,
	// or quiet for 30s) and long enough to count. Called with the lock held:
	// don't call back into the pipeline.
	OnFinished func(at time.Time, v FightView)
	checked    time.Time
	handedOver map[fightKey]bool
}

type fightKey struct {
	target uint32
	start  int64
}

func New(dotSkills map[uint32]bool) *Pipeline {
	return &Pipeline{m: meter.New(dotSkills), flows: map[string]*flow{}, handedOver: map[fightKey]bool{}}
}

// Segment takes one server → client TCP segment. flow names the connection
// (any stable string, e.g. "1.2.3.4:5555>10.0.0.2:50123").
func (p *Pipeline) Segment(at time.Time, flowKey string, seq uint32, payload []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f := p.flows[flowKey]
	if f == nil {
		if len(p.flows) >= maxFlows {
			p.dropOldest()
		}
		f = &flow{}
		p.flows[flowKey] = f
	}
	f.seen = at
	data, gap := f.stream.Push(seq, payload)
	if gap {
		f.split = wire.Splitter{} // re-sync on the next keepalive
	}
	for _, raw := range f.split.Feed(data) {
		for _, pk := range wire.Unpack(raw) {
			p.packets++
			Dispatch(p.m, at.UnixMilli(), pk)
		}
	}
	if p.OnFinished != nil && at.Sub(p.checked) >= finishEvery {
		p.handOver(at)
		p.checked = at
	}
	if at.Sub(p.pruned) >= pruneEvery {
		p.m.Prune(at.UnixMilli())
		p.pruned = at
	}
}

func (p *Pipeline) dropOldest() {
	var oldest string
	var t time.Time
	for k, f := range p.flows {
		if oldest == "" || f.seen.Before(t) {
			oldest, t = k, f.seen
		}
	}
	delete(p.flows, oldest)
}

// handOver passes on fights that just finished, before Prune can drop them.
func (p *Pipeline) handOver(at time.Time) {
	now := at.UnixMilli()
	live := map[fightKey]bool{}
	for _, f := range p.m.Fights() {
		k := fightKey{f.Target, f.Start}
		live[k] = true
		over := f.Killed || f.End < now-meter.IdleResetMS
		if over && !p.handedOver[k] && p.m.Ours(f) {
			p.handedOver[k] = true
			p.OnFinished(at, view(p.m, f))
		}
	}
	for k := range p.handedOver {
		if !live[k] {
			delete(p.handedOver, k)
		}
	}
}

// Dispatch hands one game packet to the meter, by opcode.
func Dispatch(m *meter.Meter, at int64, p []byte) {
	op, ok := wire.Opcode(p)
	if !ok {
		return
	}
	switch op {
	case wire.OpDamage:
		if d, err := wire.ParseDamage(p); err == nil && !d.NoValue {
			m.Add(meter.FromDamage(at, d))
		}
	case wire.OpDotDamage:
		if t, err := wire.ParseTick(p); err == nil {
			m.Add(meter.FromTick(at, t))
		}
	case wire.OpPlayerInfo, wire.OpOtherPlayers, wire.OpPlayerSpawnOld:
		if id, err := wire.ParseIdentity(p); err == nil {
			m.Identify(id)
		}
	case wire.OpSelfStats:
		if id, ok := wire.EntityOf(p); ok {
			m.SelfStats(id)
		}
	case wire.OpPartyScope:
		if id, ok := wire.EntityOf(p); ok {
			m.PartyScope(id)
		}
	case wire.OpPartyInfo:
		if r, err := wire.ParseRoster(p); err == nil {
			m.SetParty(r)
		}
	case wire.OpPartyLeft, wire.OpPartyLeftInstance:
		if wire.PartyLeft(p) {
			m.LeaveParty()
		}
	case wire.OpMobSpawn, wire.OpMobSpawnOld:
		if sp, err := wire.ParseSpawn(p); err == nil {
			m.Spawn(sp)
		}
	case wire.OpDeath:
		if d, err := wire.ParseDeath(p); err == nil {
			m.Death(at, d)
		}
	}
}

// Reset clears the fights; who's who stays.
func (p *Pipeline) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.m.Reset()
}

// With runs fn on the meter under the lock (for grootdump's end-of-file report).
func (p *Pipeline) With(fn func(*meter.Meter)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fn(p.m)
}

// Flush hands over every fight of yours long enough to count that hasn't
// been yet, finished or not (for closing the meter mid-fight).
func (p *Pipeline) Flush(at time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.OnFinished == nil {
		return
	}
	for _, f := range p.m.Fights() {
		k := fightKey{f.Target, f.Start}
		if !p.handedOver[k] && p.m.Ours(f) {
			p.handedOver[k] = true
			p.OnFinished(at, view(p.m, f))
		}
	}
}
