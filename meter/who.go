// SPDX-License-Identifier: EUPL-1.2

package meter

// Who's who: you, your party, players, and which summon belongs to whom.

import "groot/dps/wire"

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

// Name is the character name for an entity, "" if nobody has said.
func (m *Meter) Name(id uint32) string { return m.names[id] }
