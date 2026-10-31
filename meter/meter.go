// SPDX-License-Identifier: EUPL-1.2

// Package meter turns decoded hits into fights and DPS rows. Pure: no IO, no clock.
// Rules from A2Tools-DPS-Meter (combat/data_storage.rs, combat/dps_calculator.rs).
package meter

import (
	"cmp"
	"slices"

	"groot/dps/wire"
)

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

type Row struct {
	Actor       uint32 // the owner, for summons
	Name        string // "" until an identity record names them
	Class       wire.Class
	CombatPower uint64 // from the party roster
	Self        bool
	InParty     bool
	Amount      uint64
	DPS, Share  float64 // Share in %
	Hits, Crits uint64
	Deaths      int        // during this fight
	Skills      []SkillRow // biggest first
}

type SkillRow struct {
	Skill               uint32
	Amount, Hits, Crits uint64
}

type HealRow struct {
	Actor     uint32
	Amount    uint64
	PerSecond float64
}

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

// Link records that summon belongs to owner (from a spawn or a spirit's link record).
func (m *Meter) Link(summon, owner uint32) {
	if summon != owner {
		m.owners[summon] = owner
		delete(m.players, summon)
	}
}

// Identify binds an entity to a character (33 36 you, 45 36 another player).
// The latest name wins: entity ids get reissued on zone changes.
func (m *Meter) Identify(id wire.Identity) {
	m.players[id.Entity] = true
	delete(m.owners, id.Entity)
	if id.Self {
		m.me, m.meFrom, m.myClass = id.Entity, fromRecord, id.Class
	}
	if id.Name != "" {
		m.names[id.Entity] = id.Name
	}
}

// How we know which entity is you, weakest first.
type source uint8

const (
	unknown source = iota
	fromScope
	fromRecord
	fromStats
)

// Me is your entity id, 0 until something told us.
func (m *Meter) Me() uint32 { return m.me }

// SelfStats: a 4A 36 record, about your own stats only. Three in a row naming
// one entity make it you, even over a self record (which goes stale when a
// zone load's copy is missed).
func (m *Meter) SelfStats(id uint32) {
	if id == m.me {
		m.streak.n = 0
		return
	}
	if m.streak.id != id {
		m.streak.id, m.streak.n = id, 0
	}
	if m.streak.n++; m.streak.n >= 3 {
		m.me, m.meFrom, m.streak.n = id, fromStats, 0
	}
}

// PartyScope: a 06 38 record. Until a self record or own stats say otherwise,
// you are the player they name far more than anyone (24+, and 4x the next).
func (m *Meter) PartyScope(id uint32) {
	if id < 100 || len(m.scope) >= 10_000 && m.scope[id] == 0 {
		return
	}
	m.scope[id]++
	if m.scope[id]%8 != 0 || m.meFrom > fromScope {
		return
	}
	var best, top, second uint32
	for e, n := range m.scope {
		if _, summon := m.owners[e]; !m.players[e] || summon {
			continue
		}
		if n > top {
			best, top, second = e, n, top
		} else {
			second = max(second, n)
		}
	}
	if top >= 24 && top >= 4*max(second, 1) {
		m.me, m.meFrom = best, fromScope
	}
}

// Spawn records an NPC, summon or effect entering the world.
func (m *Meter) Spawn(s wire.Spawn) {
	m.npcs[s.Entity] = s
	delete(m.gone, s.Entity)
	delete(m.owners, s.Entity) // ids get reissued: whatever it was, it's this now
	delete(m.players, s.Entity)
	if s.Kind != wire.KindSummon {
		return
	}
	if s.Owner != 0 {
		m.Link(s.Entity, s.Owner)
		return
	}
	if s.Name != "" { // the owner's name, when there's no parent_key
		for id, name := range m.names {
			if name == s.Name && id != s.Entity {
				m.Link(s.Entity, id)
				return
			}
		}
	}
}

// Death: killed (flag 3) ends a fight; gone (flag 7) stops a spirit's ticks.
func (m *Meter) Death(at int64, d wire.Death) {
	switch d.Flag {
	case wire.DeathKilled:
		m.deaths = append(m.deaths, death{at, d.Entity})
		if f := m.fights[d.Entity]; f != nil {
			f.Killed, f.KilledAt = true, at
		}
	case wire.DeathGone:
		m.gone[d.Entity] = true
	}
}

// SetParty replaces the party with this roster's members.
func (m *Meter) SetParty(r wire.Roster) {
	m.party = map[string]wire.Member{}
	for _, mem := range r.Members {
		m.party[mem.Name] = mem
	}
}

func (m *Meter) inParty(id uint32) bool {
	if id == m.me && m.me != 0 {
		return true
	}
	_, ok := m.party[m.names[id]]
	return ok && m.names[id] != ""
}

// Ours: you or your party hit this target (a fight only strangers had isn't).
func (m *Meter) Ours(f *Fight) bool {
	if f == nil {
		return false
	}
	for id := range f.Actors {
		if m.inParty(m.Owner(id)) {
			return true
		}
	}
	return false
}

func (m *Meter) describe(r *Row) {
	r.Name = m.names[r.Actor]
	r.Self = r.Actor == m.me && m.me != 0
	r.InParty = m.inParty(r.Actor)
	if r.Self {
		r.Class = m.myClass
	}
	if mem, ok := m.party[r.Name]; ok && r.Name != "" {
		r.Class, r.CombatPower = mem.Class, mem.CombatPower
	}
}

// Owner walks summon links up to whoever started the chain.
func (m *Meter) Owner(id uint32) uint32 {
	for range maxOwnerHop {
		o, ok := m.owners[id]
		if !ok {
			break
		}
		id = o
	}
	return id
}

func npcSkill(s uint32) bool { return s >= 1_000_000 && s <= 9_999_999 }

func playerSkill(s uint32) bool {
	return s >= 11_000_000 && s <= 19_999_999 || s >= 3_000_000 && s <= 3_999_999 || s >= 100_000 && s <= 199_999
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

// Rows: damage on target per player (summons folded into owners), biggest first.
func (m *Meter) Rows(target uint32) []Row {
	f := m.fights[target]
	if f == nil {
		return nil
	}
	secs := float64(max(f.Duration(), minDivideMS)) / 1000
	byOwner := map[uint32]*Row{}
	skills := map[uint32]map[uint32]*SkillRow{}
	for id, a := range f.Actors {
		o := m.Owner(id)
		r := byOwner[o]
		if r == nil {
			r = &Row{Actor: o}
			m.describe(r)
			byOwner[o], skills[o] = r, map[uint32]*SkillRow{}
		}
		r.Amount, r.Hits, r.Crits = r.Amount+a.amount, r.Hits+a.hits, r.Crits+a.crits
		for k, s := range a.skills {
			t := skills[o][k]
			if t == nil {
				t = &SkillRow{Skill: k}
				skills[o][k] = t
			}
			t.Amount, t.Hits, t.Crits = t.Amount+s.Amount, t.Hits+s.Hits, t.Crits+s.Crits
		}
	}
	rows := make([]Row, 0, len(byOwner))
	for o, r := range byOwner {
		r.DPS = float64(r.Amount) / secs
		for _, d := range m.deaths {
			if d.entity == o && d.at >= f.Start && d.at <= max(f.End, f.KilledAt) {
				r.Deaths++
			}
		}
		if f.Total > 0 {
			r.Share = float64(r.Amount) / float64(f.Total) * 100
		}
		for _, s := range skills[o] {
			r.Skills = append(r.Skills, *s)
		}
		slices.SortFunc(r.Skills, func(a, b SkillRow) int { return cmp.Or(cmp.Compare(b.Amount, a.Amount), cmp.Compare(a.Skill, b.Skill)) })
		rows = append(rows, *r)
	}
	slices.SortFunc(rows, func(a, b Row) int { return cmp.Or(cmp.Compare(b.Amount, a.Amount), cmp.Compare(a.Actor, b.Actor)) })
	return rows
}

// HealRows: healing done during the fight on target, per healer, biggest first.
func (m *Meter) HealRows(target uint32) []HealRow {
	f := m.fights[target]
	if f == nil {
		return nil
	}
	secs := float64(max(f.Duration(), minDivideMS)) / 1000
	by := map[uint32]uint64{}
	for _, h := range m.heals {
		if h.at >= f.Start && h.at <= f.End {
			by[h.healer] += h.amount
		}
	}
	rows := make([]HealRow, 0, len(by))
	for id, amt := range by {
		rows = append(rows, HealRow{Actor: id, Amount: amt, PerSecond: float64(amt) / secs})
	}
	slices.SortFunc(rows, func(a, b HealRow) int { return cmp.Or(cmp.Compare(b.Amount, a.Amount), cmp.Compare(a.Actor, b.Actor)) })
	return rows
}

// Fights: every current fight long enough to count, bosses first, then biggest.
func (m *Meter) Fights() []*Fight {
	var out []*Fight
	for _, f := range m.fights {
		if f.Counts() {
			out = append(out, f)
		}
	}
	slices.SortFunc(out, func(a, b *Fight) int {
		return cmp.Or(boolCmp(b.Boss, a.Boss), cmp.Compare(b.Total, a.Total), cmp.Compare(a.Target, b.Target))
	})
	return out
}

func boolCmp(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// Current is the fight to show: your latest (anyone's until we know you), but
// a boss over the trash around it while the boss fight is live.
func (m *Meter) Current() *Fight {
	var cands []*Fight
	for _, f := range m.fights {
		if m.Ours(f) {
			cands = append(cands, f)
		}
	}
	if len(cands) == 0 {
		for _, f := range m.fights {
			cands = append(cands, f)
		}
	}
	var latest, boss *Fight
	for _, f := range cands {
		if latest == nil || f.End > latest.End || f.End == latest.End && f.Target < latest.Target {
			latest = f
		}
		if f.Boss && (boss == nil || f.End > boss.End) {
			boss = f
		}
	}
	if boss != nil && boss.End >= latest.End-IdleResetMS {
		return boss
	}
	return latest
}

// Reset clears fights, heals and deaths. Who's who (names, party, pets, you) stays.
func (m *Meter) Reset() {
	m.fights = map[uint32]*Fight{}
	m.heals, m.deaths = nil, nil
}

// Name is the character name for an entity, "" if nobody has said.
func (m *Meter) Name(id uint32) string { return m.names[id] }
