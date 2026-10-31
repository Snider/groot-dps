// SPDX-License-Identifier: EUPL-1.2

package meter

// The world: what spawns, dies and leaves.

import "groot/dps/wire"

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
		delete(m.npcs, d.Entity)
		if _, summon := m.owners[d.Entity]; summon { // only a summon's ticks linger
			m.gone[d.Entity] = true
		}
	}
}
