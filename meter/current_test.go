// SPDX-License-Identifier: EUPL-1.2

package meter

import (
	"testing"

	"groot/dps/wire"
)

func TestCurrentIsYourLatestFight(t *testing.T) {
	m := partyMeter()
	m.Add(hit(0, groot, monster, 12_020_100, 1))
	m.Add(hit(5000, stranger, 99999, 11_020_100, 1)) // later, but not ours
	m.Add(hit(3000, groot, 70001, 12_020_100, 1))
	if f := m.Current(); f == nil || f.Target != 70001 {
		t.Fatalf("current %+v", f)
	}
}

// Trash around a boss doesn't steal the window while the boss fight is live.
func TestCurrentPrefersALiveBoss(t *testing.T) {
	m := partyMeter()
	m.Spawn(wire.Spawn{Entity: boss, Kind: wire.KindNPC, StaggerMax: 40})
	m.Add(hit(0, groot, boss, 12_020_100, 1))
	m.Add(hit(20_000, groot, boss, 12_020_100, 1))
	m.Add(hit(25_000, groot, monster, 12_020_100, 1)) // an add
	if f := m.Current(); f.Target != boss {
		t.Fatalf("current %d", f.Target)
	}
	m.Add(hit(60_000, groot, monster, 12_020_100, 1)) // boss fight long over
	if f := m.Current(); f.Target != monster {
		t.Fatalf("current %d", f.Target)
	}
}

func TestCurrentShowsAnyoneUntilWeKnowYou(t *testing.T) {
	m := New(nil)
	m.Add(hit(0, groot, monster, 12_020_100, 1))
	if f := m.Current(); f == nil || f.Target != monster {
		t.Fatalf("current %+v", f)
	}
	if New(nil).Current() != nil {
		t.Fatal("empty meter has a fight")
	}
}

// Reset clears the numbers, not who's who.
func TestResetKeepsNamesAndParty(t *testing.T) {
	m := partyMeter()
	m.Link(spirit, cleric)
	m.Add(hit(0, groot, boss, 12_020_100, 1))
	m.Reset()
	if m.Current() != nil || len(m.Fights()) != 0 {
		t.Fatal("fights survived")
	}
	m.Add(hit(0, spirit, boss, 16_001_109, 1))
	if r := m.Rows(boss)[0]; r.Name != "Pinky" || !r.InParty || m.Me() != groot {
		t.Fatalf("lost who's who: %+v", r)
	}
}
