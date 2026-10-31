// SPDX-License-Identifier: EUPL-1.2

package meter

import (
	"testing"

	"groot/dps/wire"
)

// Hours of open-world farming: old fights, heals and deaths go, so memory stays flat.
func TestPruneDropsOldFightsHealsAndDeaths(t *testing.T) {
	m := partyMeter()
	const min = 60_000
	for i := range 100 { // 100 trash fights, one a minute
		at := int64(i * min)
		m.Add(hit(at, groot, uint32(60_000+i), 12_020_100, 10))
		m.Add(Hit{At: at, Actor: cleric, Target: groot, Skill: 1_900_001, Amount: 5, Heal: true})
		m.Death(at, wire.Death{Entity: uint32(60_000 + i), Flag: wire.DeathKilled})
	}
	now := int64(99 * min)
	m.Prune(now)
	if n := len(m.fights); n != KeepMS/min+1 {
		t.Errorf("%d fights kept, want %d", n, KeepMS/min+1)
	}
	if len(m.heals) > KeepMS/min+1 || len(m.deaths) > KeepMS/min+1 {
		t.Errorf("heals %d deaths %d", len(m.heals), len(m.deaths))
	}
	if f := m.Current(); f == nil || f.Target != 60_099 {
		t.Errorf("current %+v", f)
	}
}

// The fight on screen stays, however old.
func TestPruneKeepsTheCurrentFight(t *testing.T) {
	m := partyMeter()
	m.Add(hit(0, groot, boss, 12_020_100, 10))
	m.Prune(int64(KeepMS) * 5)
	if m.Fight(boss) == nil {
		t.Fatal("pruned the fight on screen")
	}
}

// A spawn is forgotten once it leaves the world.
func TestGoneSpawnsAreForgotten(t *testing.T) {
	m := New(nil)
	m.Spawn(wire.Spawn{Entity: monster, Kind: wire.KindNPC, NPC: 1})
	m.Death(0, wire.Death{Entity: monster, Flag: wire.DeathGone})
	if _, ok := m.npcs[monster]; ok {
		t.Fatal("still remembered")
	}
}
