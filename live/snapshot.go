// SPDX-License-Identifier: EUPL-1.2

package live

import (
	"fmt"

	"groot/dps/meter"
)

// Snapshot is what the window shows, as JSON.
type Snapshot struct {
	Fight   *FightView `json:"fight"`
	You     bool       `json:"you"` // we know which entity is you
	Packets int        `json:"packets"`
}

type FightView struct {
	Target  uint32    `json:"target"`
	NPC     uint32    `json:"npc"`
	Boss    bool      `json:"boss"`
	Killed  bool      `json:"killed"`
	MaxHP   uint32    `json:"maxHp"`
	Seconds float64   `json:"seconds"`
	Total   uint64    `json:"total"`
	Rows    []RowView `json:"rows"`
	Heals   []RowView `json:"heals"`
}

type RowView struct {
	Name     string  `json:"name"`
	Class    string  `json:"class"`
	Amount   uint64  `json:"amount"`
	PerSec   float64 `json:"perSec"`
	Share    float64 `json:"share"`
	CritRate float64 `json:"critRate"`
	Deaths   int     `json:"deaths"`
	Self     bool    `json:"self"`
	Party    bool    `json:"party"`
}

func (p *Pipeline) Snapshot() Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := Snapshot{You: p.m.Me() != 0, Packets: p.packets}
	if f := p.m.Current(); f != nil {
		v := view(p.m, f)
		s.Fight = &v
	}
	return s
}

func view(m *meter.Meter, f *meter.Fight) FightView {
	v := FightView{Target: f.Target, NPC: f.NPC, Boss: f.Boss, Killed: f.Killed, MaxHP: f.MaxHP,
		Seconds: float64(f.Duration()) / 1000, Total: f.Total}
	for _, r := range m.Rows(f.Target) {
		name := label(m, r.Actor)
		var crit float64
		if r.Hits > 0 {
			crit = float64(r.Crits) / float64(r.Hits) * 100
		}
		v.Rows = append(v.Rows, RowView{Name: name, Class: r.Class.String(), Amount: r.Amount, PerSec: r.DPS,
			Share: r.Share, CritRate: crit, Deaths: r.Deaths, Self: r.Self, Party: r.InParty})
	}
	for _, h := range m.HealRows(f.Target) {
		v.Heals = append(v.Heals, RowView{Name: label(m, h.Actor), Self: h.Actor == m.Me(), Amount: h.Amount, PerSec: h.PerSecond})
	}
	return v
}

func label(m *meter.Meter, id uint32) string {
	if n := m.Name(id); n != "" {
		return n
	}
	if id == m.Me() {
		return "You"
	}
	return fmt.Sprintf("#%d", id)
}
