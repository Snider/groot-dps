// SPDX-License-Identifier: EUPL-1.2

package meter

import (
	"encoding/hex"
	"math"
	"testing"

	"groot/dps/wire"
)

// The rules come from A2Tools-DPS-Meter (c55788c): combat/data_storage.rs
// append_damage/apply_damage and combat/dps_calculator.rs get_dps.

const (
	boss    = 50001
	groot   = 2737 // a Templar
	cleric  = 5041
	spirit  = 25676 // a summon
	monster = 47573
)

func hit(at int64, actor, target, skill uint32, amount uint64) Hit {
	return Hit{At: at, Actor: actor, Target: target, Skill: skill, Amount: amount}
}

func TestDPSIsDamageOverTheFightOnThatTarget(t *testing.T) {
	m := New(nil)
	m.Add(hit(10_000, groot, boss, 11_020_100, 3000))
	m.Add(hit(12_000, cleric, boss, 17_050_240, 1000))
	m.Add(hit(20_000, groot, boss, 11_020_100, 2000))

	f := m.Fight(boss)
	if f.Start != 10_000 || f.End != 20_000 || f.Duration() != 10_000 || f.Total != 6000 {
		t.Fatalf("fight %+v", f)
	}
	rows := m.Rows(boss)
	if len(rows) != 2 || rows[0].Actor != groot || rows[1].Actor != cleric {
		t.Fatalf("rows %+v", rows)
	}
	if rows[0].Amount != 5000 || rows[0].DPS != 500 || math.Abs(rows[0].Share-100*5.0/6) > 1e-9 || rows[0].Hits != 2 {
		t.Errorf("groot %+v", rows[0])
	}
	if rows[1].DPS != 100 {
		t.Errorf("cleric %+v", rows[1])
	}
}

// A single hit (or a 0.2s fight) divides by 1s, not by ~0.
func TestShortFightsDivideByAtLeastOneSecond(t *testing.T) {
	m := New(nil)
	m.Add(hit(10_000, groot, boss, 11_020_100, 3000))
	m.Add(hit(10_200, groot, boss, 11_020_100, 1000))
	if r := m.Rows(boss)[0]; r.DPS != 4000 {
		t.Fatalf("dps %v, want 4000", r.DPS)
	}
	if m.Fight(boss).Counts() {
		t.Error("a 0.2s fight is not a fight (A2Tools keeps those of 5s or more)")
	}
}

// 30s without damage on a target and the next hit starts a new fight.
func TestIdleGapStartsANewFight(t *testing.T) {
	m := New(nil)
	m.Add(hit(10_000, groot, boss, 11_020_100, 3000))
	m.Add(hit(40_000, groot, boss, 11_020_100, 1000)) // exactly 30s: same fight
	m.Add(hit(70_001, groot, boss, 11_020_100, 7))
	f := m.Fight(boss)
	if f.Start != 70_001 || f.Total != 7 {
		t.Fatalf("fight %+v", f)
	}
}

func TestCritsAndSkillsAreCounted(t *testing.T) {
	m := New(nil)
	a := hit(1000, groot, boss, 11_020_100, 100)
	a.Crit = true
	m.Add(a)
	m.Add(hit(2000, groot, boss, 11_020_100, 50))
	m.Add(hit(3000, groot, boss, 11_030_000, 10))
	r := m.Rows(boss)[0]
	if r.Hits != 3 || r.Crits != 1 {
		t.Errorf("row %+v", r)
	}
	s := r.Skills
	if len(s) != 2 || s[0].Skill != 11_020_100 || s[0].Amount != 150 || s[0].Hits != 2 || s[0].Crits != 1 || s[1].Amount != 10 {
		t.Errorf("skills %+v", s)
	}
}

// A summon's damage is its owner's, through chains, and a loop can't hang us.
func TestSummonsCountForTheirOwner(t *testing.T) {
	m := New(nil)
	m.Link(spirit, cleric)
	m.Link(99, spirit) // a spirit's own effect entity
	m.Add(hit(1000, cleric, boss, 17_050_240, 100))
	m.Add(hit(2000, spirit, boss, 16_001_109, 50))
	m.Add(hit(3000, 99, boss, 16_001_109, 25))
	rows := m.Rows(boss)
	if len(rows) != 1 || rows[0].Actor != cleric || rows[0].Amount != 175 {
		t.Fatalf("rows %+v", rows)
	}
	m.Link(cleric, 99) // a loop
	m.Rows(boss)
}

// A 7-digit skill is an NPC's: a monster hitting a player is damage taken,
// never damage dealt (Gatekeeper Pinopi, 2026-10-15).
func TestMonsterHitsAreDamageTaken(t *testing.T) {
	m := New(nil)
	m.Add(hit(500, groot, monster, 16_190_020, 100)) // groot is a player now: they used a class skill
	m.Add(hit(1000, monster, groot, 1_607_550, 1258))
	f := m.Fight(monster)
	if f.Total != 100 || f.Taken[groot] != 1258 {
		t.Fatalf("fight %+v", f)
	}
	if m.Fight(groot) != nil {
		t.Error("a monster's hit started a fight on the player")
	}
}

// Player-on-player "damage" is healing or a buff.
func TestPlayerOnPlayerIsNotDamage(t *testing.T) {
	m := New(nil)
	m.Add(hit(500, groot, boss, 11_020_100, 100))
	m.Add(hit(600, cleric, boss, 17_050_240, 100))
	m.Add(hit(550, cleric, groot, 17_090_000, 400))
	if m.Fight(groot) != nil {
		t.Fatal("cleric 'damaged' groot")
	}
	if h := m.HealRows(boss); len(h) != 1 || h[0].Actor != cleric || h[0].Amount != 400 {
		t.Fatalf("heals %+v", h)
	}
}

// DoT damage ticks mostly repeat damage already sent as hits: only the
// allowlisted DoT skills count.
func TestOnlyAllowlistedDoTTicksCount(t *testing.T) {
	m := New(map[uint32]bool{16_001_109: true})
	tick := func(at int64, skill uint32) Hit {
		h := hit(at, spirit, boss, skill, 375)
		h.DoT = true
		return h
	}
	m.Add(tick(1000, 16_001_109))
	m.Add(tick(2000, 12_090_000))
	if f := m.Fight(boss); f == nil || f.Total != 375 {
		t.Fatalf("fight %+v", f)
	}
}

// Heals count for the fight they fall inside: HPS over the same duration.
func TestHealsInsideTheFight(t *testing.T) {
	m := New(nil)
	heal := func(at int64, amount uint64) Hit {
		h := hit(at, cleric, groot, 1_900_001, amount)
		h.Heal = true
		return h
	}
	m.Add(heal(500, 999)) // before the fight
	m.Add(hit(1000, groot, boss, 11_020_100, 100))
	m.Add(heal(3000, 600))
	m.Add(hit(5000, groot, boss, 11_020_100, 100))
	m.Add(heal(9000, 999)) // after
	h := m.HealRows(boss)
	if len(h) != 1 || h[0].Amount != 600 || h[0].PerSecond != 150 {
		t.Fatalf("heals %+v", h)
	}
}

// Theostone procs arrive as raw item ids (3,000,000-3,099,999); A2Tools
// files them as id*10+1, the skill table's code for them.
func TestTheostoneProcsGetTheirSkillCode(t *testing.T) {
	h := FromDamage(1000, wire.Damage{Target: boss, Actor: groot, Skill: 3_000_123, Amount: 9})
	if h.Skill != 30_001_231 {
		t.Fatalf("skill %d", h.Skill)
	}
}

// End to end on golden records: a Wind Spirit's two ticks count for its owner.
func TestGoldenSpiritTicksForTheOwner(t *testing.T) {
	b, _ := hex.DecodeString("19053888fd020accc801881241c15f5ff7025828f400")
	tk, err := wire.ParseTick(b)
	if err != nil {
		t.Fatal(err)
	}
	const owner = 15740
	m := New(map[uint32]bool{16_001_109: true})
	m.Link(tk.Actor, owner)
	m.Add(FromTick(1000, tk))
	m.Add(FromTick(2000, tk))
	rows := m.Rows(tk.Target)
	if len(rows) != 1 || rows[0].Actor != owner || rows[0].Amount != 750 {
		t.Fatalf("rows %+v", rows)
	}
}

func TestFightsListsOnlyRealFightsBiggestFirst(t *testing.T) {
	m := New(nil)
	m.Add(hit(0, groot, boss, 11_020_100, 10))
	m.Add(hit(6_000, groot, boss, 11_020_100, 10))
	m.Add(hit(0, groot, monster, 11_020_100, 500))
	m.Add(hit(9_000, groot, monster, 11_020_100, 500))
	m.Add(hit(0, groot, 77777, 11_020_100, 9999)) // one hit: not a fight
	f := m.Fights()
	if len(f) != 2 || f[0].Target != monster || f[1].Target != boss {
		t.Fatalf("fights %+v", f)
	}
}
