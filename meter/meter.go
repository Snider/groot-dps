// SPDX-License-Identifier: EUPL-1.2

// Package meter turns decoded hits into fights and DPS rows. Pure: no IO, no clock.
// Rules from A2Tools-DPS-Meter (combat/data_storage.rs, combat/dps_calculator.rs).
package meter

import "groot/dps/wire"

const (
	IdleResetMS = 30_000 // longer than this without damage on a target: next hit is a new fight
	MinFightMS  = 5_000  // shorter fights aren't kept
	minDivideMS = 1_000  // DPS never divides by less than a second
	maxOwnerHop = 16
)

// Hit is one hit, tick or heal, with a time in ms (capture or server time).
type Hit struct {
	At                   int64
	Actor, Target, Skill uint32
	Amount               uint64
	Crit                 bool
	DoT                  bool // a 05 38 damage tick
	Heal                 bool // a 05 38 heal or HoT tick
}

func FromDamage(at int64, d wire.Damage) Hit {
	skill := d.Skill
	if skill >= 3_000_000 && skill <= 3_099_999 { // a theostone proc, sent as its item id
		skill = skill*10 + 1
	}
	return Hit{At: at, Actor: d.Actor, Target: d.Target, Skill: skill, Amount: uint64(d.Amount), Crit: d.Crit()}
}

func FromTick(at int64, t wire.Tick) Hit {
	return Hit{At: at, Actor: t.Actor, Target: t.Target, Skill: t.Skill(), Amount: uint64(t.Amount), DoT: t.Damage(), Heal: t.Heal()}
}

// Fight is the damage on one target, actors as the game sent them (summons unresolved).
type Fight struct {
	Target     uint32
	Start, End int64
	Total      uint64
	Actors     map[uint32]*actorStats
	Taken      map[uint32]uint64 // player -> damage this target dealt them
	// From the target's spawn, when we saw it:
	NPC      uint32
	MaxHP    uint32
	Boss     bool // spawned with a stagger gauge: a boss or miniboss
	Killed   bool
	KilledAt int64
}

type actorStats struct {
	amount, hits, crits uint64
	skills              map[uint32]*SkillRow
}

func (f *Fight) Duration() int64 { return f.End - f.Start }
func (f *Fight) Counts() bool    { return f.Duration() >= MinFightMS }

type heal struct {
	at     int64
	healer uint32
	amount uint64
}

type Meter struct {
	dotSkills map[uint32]bool
	fights    map[uint32]*Fight
	owners    map[uint32]uint32
	players   map[uint32]bool
	heals     []heal
	names     map[uint32]string
	me        uint32 // 0 until we know which entity is you
	meFrom    source
	streak    struct{ id, n uint32 }
	scope     map[uint32]uint32 // 06 38 records per entity
	myClass   wire.Class
	party     map[string]wire.Member // by name: rosters carry account ids, not entity ids
	partyKey  uint32                 // your party's, once a roster with you in it said
	npcs      map[uint32]wire.Spawn
	gone      map[uint32]bool // left the world (a spirit unsummoned)
	deaths    []death
}

type death struct {
	at     int64
	entity uint32
}

// New takes the DoT skills whose damage ticks count. The rest of 05 38 damage
// repeats what 04 38 already sent.
func New(dotSkills map[uint32]bool) *Meter {
	return &Meter{dotSkills: dotSkills, fights: map[uint32]*Fight{}, owners: map[uint32]uint32{}, players: map[uint32]bool{}, names: map[uint32]string{}, party: map[string]wire.Member{}, scope: map[uint32]uint32{}, npcs: map[uint32]wire.Spawn{}, gone: map[uint32]bool{}}
}

func (m *Meter) Add(h Hit) {
	if h.DoT && m.gone[h.Actor] { // the game doesn't count ticks from a spirit that left
		return
	}
	if !h.DoT && !h.Heal {
		delete(m.gone, h.Actor)
	}
	switch {
	case h.Heal:
		m.heals = append(m.heals, heal{h.At, m.Owner(h.Actor), h.Amount})
		return
	case h.DoT && !m.dotSkills[h.Skill]:
		return
	case npcSkill(h.Skill):
		if target := m.Owner(h.Target); m.players[target] {
			if f := m.fights[h.Actor]; f != nil {
				f.Taken[target] += h.Amount
			}
		}
		return
	}
	if playerSkill(h.Skill) && !h.DoT {
		if _, summon := m.owners[h.Actor]; !summon {
			m.players[h.Actor] = true
		}
	}
	if m.players[m.Owner(h.Actor)] && m.players[m.Owner(h.Target)] {
		m.heals = append(m.heals, heal{h.At, m.Owner(h.Actor), h.Amount})
		return
	}

	f := m.fights[h.Target]
	if f == nil || h.At-f.End > IdleResetMS {
		f = &Fight{Target: h.Target, Start: h.At, End: h.At, Actors: map[uint32]*actorStats{}, Taken: map[uint32]uint64{}}
		if sp, ok := m.npcs[h.Target]; ok {
			f.NPC, f.MaxHP, f.Boss = sp.NPC, sp.MaxHP, sp.Boss()
		}
		m.fights[h.Target] = f
	}
	f.Start, f.End = min(f.Start, h.At), max(f.End, h.At)
	f.Total += h.Amount
	a := f.Actors[h.Actor]
	if a == nil {
		a = &actorStats{skills: map[uint32]*SkillRow{}}
		f.Actors[h.Actor] = a
	}
	s := a.skills[h.Skill]
	if s == nil {
		s = &SkillRow{Skill: h.Skill}
		a.skills[h.Skill] = s
	}
	a.amount, s.Amount = a.amount+h.Amount, s.Amount+h.Amount
	a.hits, s.Hits = a.hits+1, s.Hits+1
	if h.Crit {
		a.crits, s.Crits = a.crits+1, s.Crits+1
	}
}

// Fight is the current fight on target, nil if nobody has hit it.
func (m *Meter) Fight(target uint32) *Fight { return m.fights[target] }

// Reset clears fights, heals and deaths. Who's who (names, party, pets, you) stays.
func (m *Meter) Reset() {
	m.fights = map[uint32]*Fight{}
	m.heals, m.deaths = nil, nil
}
