// SPDX-License-Identifier: EUPL-1.2

package meter

import (
	"testing"

	"groot/dps/wire"
)

const stranger = 8888

func partyMeter() *Meter {
	m := New(nil)
	m.Identify(wire.Identity{Entity: groot, Self: true, Name: "Groot", Server: 1301, Class: wire.Templar})
	m.Identify(wire.Identity{Entity: cleric, Name: "Pinky"})
	m.Identify(wire.Identity{Entity: stranger, Name: "Rando"})
	m.SetParty(wire.Roster{Members: []wire.Member{
		{Name: "Groot", Class: wire.Templar, CombatPower: 90000},
		{Name: "Pinky", Class: wire.Cleric, CombatPower: 80000},
	}})
	return m
}

func TestRowsCarryNamesAndParty(t *testing.T) {
	m := partyMeter()
	m.Link(spirit, cleric)
	m.Add(hit(0, groot, boss, 12_020_100, 300))
	m.Add(hit(1000, spirit, boss, 16_001_109, 200))
	m.Add(hit(2000, stranger, boss, 11_020_100, 100))
	m.Add(hit(3000, 4242, boss, 11_020_100, 50)) // nobody we know
	rows := m.Rows(boss)
	got := map[string]Row{}
	for _, r := range rows {
		got[r.Name] = r
	}
	if g := got["Groot"]; !g.Self || !g.InParty || g.Class != wire.Templar || g.CombatPower != 90000 {
		t.Errorf("groot %+v", g)
	}
	if p := got["Pinky"]; p.Self || !p.InParty || p.Class != wire.Cleric || p.Amount != 200 {
		t.Errorf("pinky (with her spirit) %+v", p)
	}
	if r := got["Rando"]; r.InParty || r.Amount != 100 {
		t.Errorf("stranger %+v", r)
	}
	if r := got[""]; r.Actor != 4242 {
		t.Errorf("unnamed %+v", r)
	}
}

// A fight only strangers had is not ours.
func TestOursIsAFightYouOrYourPartyWasIn(t *testing.T) {
	m := partyMeter()
	m.Add(hit(0, stranger, monster, 11_020_100, 100))
	m.Add(hit(0, cleric, boss, 17_050_240, 100))
	if m.Ours(m.Fight(monster)) || !m.Ours(m.Fight(boss)) {
		t.Fatal("ours wrong")
	}
}

// A new name for an entity wins (ids get reissued on zone changes).
func TestLatestNameWins(t *testing.T) {
	m := partyMeter()
	m.Identify(wire.Identity{Entity: stranger, Name: "Other"})
	m.Add(hit(0, stranger, boss, 11_020_100, 1))
	if r := m.Rows(boss)[0]; r.Name != "Other" {
		t.Fatalf("%+v", r)
	}
}

// The tutorial self record has no name, but it's still you.
func TestSelfWithoutANameIsStillYou(t *testing.T) {
	m := New(nil)
	m.Identify(wire.Identity{Entity: groot, Self: true})
	m.Add(hit(0, groot, boss, 12_020_100, 1))
	if r := m.Rows(boss)[0]; !r.Self {
		t.Fatalf("%+v", r)
	}
}

func TestLeavingClearsTheParty(t *testing.T) {
	m := partyMeter()
	m.LeaveParty()
	m.Add(hit(0, cleric, boss, 17_050_240, 1))
	if r := m.Rows(boss)[0]; r.InParty {
		t.Fatalf("still in party: %+v", r)
	}
}

// Once your own roster has given your party's key, another party's complete
// roster (without you) doesn't replace yours.
func TestAnotherPartysRosterIsIgnored(t *testing.T) {
	m := New(nil)
	m.Identify(wire.Identity{Entity: groot, Self: true, Name: "Groot"})
	m.Identify(wire.Identity{Entity: cleric, Name: "Pinky"})
	m.SetParty(wire.Roster{Key: 7, Complete: true, Members: []wire.Member{{Name: "Groot"}, {Name: "Pinky"}}})
	m.SetParty(wire.Roster{Key: 9, Complete: true, Members: []wire.Member{{Name: "Rando"}, {Name: "Other"}}})
	m.Add(hit(0, cleric, boss, 17_050_240, 1))
	if r := m.Rows(boss)[0]; !r.InParty {
		t.Fatal("lost our party to someone else's roster")
	}
	// A roster of our own party (same key) still updates it.
	m.SetParty(wire.Roster{Key: 7, Complete: true, Members: []wire.Member{{Name: "Groot"}}})
	if r := m.Rows(boss)[0]; r.InParty {
		t.Fatal("our own roster didn't update")
	}
}
