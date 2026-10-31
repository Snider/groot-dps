// SPDX-License-Identifier: EUPL-1.2

package meter

import (
	"testing"

	"groot/dps/wire"
)

func TestFightsKnowTheirBoss(t *testing.T) {
	m := partyMeter()
	m.Spawn(wire.Spawn{Entity: boss, Kind: wire.KindNPC, NPC: 2310471, MaxHP: 600_000, StaggerMax: 40})
	m.Spawn(wire.Spawn{Entity: monster, Kind: wire.KindNPC, NPC: 2920174, MaxHP: 5000})
	m.Add(hit(0, groot, boss, 12_020_100, 10))
	m.Add(hit(0, groot, monster, 12_020_100, 10))
	if f := m.Fight(boss); !f.Boss || f.NPC != 2310471 || f.MaxHP != 600_000 {
		t.Errorf("boss fight %+v", f)
	}
	if f := m.Fight(monster); f.Boss || f.NPC != 2920174 {
		t.Errorf("trash fight %+v", f)
	}
}

// A pet's spawn names its owner (parent_key), or failing that carries the owner's name.
func TestPetSpawnsLinkTheirOwner(t *testing.T) {
	m := partyMeter()
	m.Spawn(wire.Spawn{Entity: spirit, Kind: wire.KindSummon, Owner: cleric})
	m.Spawn(wire.Spawn{Entity: 77, Kind: wire.KindSummon, Name: "Groot"})
	m.Add(hit(0, spirit, boss, 16_001_109, 10))
	m.Add(hit(0, 77, boss, 12_001_109, 5))
	got := map[uint32]uint64{}
	for _, r := range m.Rows(boss) {
		got[r.Actor] = r.Amount
	}
	if got[cleric] != 10 || got[groot] != 5 || len(got) != 2 {
		t.Fatalf("rows %v", got)
	}
}

// An id the game reissues to an NPC isn't a summon or a player any more.
func TestAnNPCSpawnClearsWhatTheIdWas(t *testing.T) {
	m := partyMeter()
	m.Link(spirit, cleric)
	m.Spawn(wire.Spawn{Entity: spirit, Kind: wire.KindNPC, NPC: 2100077})
	if m.Owner(spirit) != spirit {
		t.Fatal("still linked")
	}
}

func TestKillsAndPartyDeaths(t *testing.T) {
	m := partyMeter()
	m.Add(hit(0, groot, boss, 12_020_100, 10))
	m.Add(hit(0, cleric, boss, 17_050_240, 10))
	m.Death(4000, wire.Death{Entity: cleric, Flag: wire.DeathKilled})
	m.Add(hit(8000, groot, boss, 12_020_100, 10))
	m.Death(9000, wire.Death{Entity: boss, Flag: wire.DeathKilled})
	f := m.Fight(boss)
	if !f.Killed || f.KilledAt != 9000 {
		t.Errorf("fight %+v", f)
	}
	for _, r := range m.Rows(boss) {
		if want := map[uint32]int{groot: 0, cleric: 1}[r.Actor]; r.Deaths != want {
			t.Errorf("%s deaths %d, want %d", r.Name, r.Deaths, want)
		}
	}
}

// A spirit's ticks stop counting once it leaves the world; a direct hit means
// the id is back (the game's Damage Analyzer counts the same way).
func TestGoneSpiritsTicksDontCount(t *testing.T) {
	m := New(map[uint32]bool{16_001_109: true})
	m.Link(spirit, cleric)
	tick := func(at int64) Hit {
		h := hit(at, spirit, boss, 16_001_109, 375)
		h.DoT = true
		return h
	}
	m.Add(tick(1000))
	m.Add(tick(2000))
	m.Death(2500, wire.Death{Entity: spirit, Flag: wire.DeathGone})
	m.Add(tick(3000))
	m.Add(tick(4000))
	if f := m.Fight(boss); f.Total != 750 {
		t.Fatalf("total %d", f.Total)
	}
	m.Add(hit(5000, spirit, boss, 16_001_100, 100))
	m.Add(tick(6000))
	if f := m.Fight(boss); f.Total != 1225 {
		t.Fatalf("total %d", f.Total)
	}
}

func TestBossFightsListFirst(t *testing.T) {
	m := New(nil)
	m.Spawn(wire.Spawn{Entity: boss, Kind: wire.KindNPC, StaggerMax: 40})
	m.Add(hit(0, groot, boss, 12_020_100, 1))
	m.Add(hit(6000, groot, boss, 12_020_100, 1))
	m.Add(hit(0, groot, monster, 12_020_100, 500))
	m.Add(hit(6000, groot, monster, 12_020_100, 500))
	if f := m.Fights(); f[0].Target != boss {
		t.Fatalf("first %d", f[0].Target)
	}
}
