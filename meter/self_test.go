// SPDX-License-Identifier: EUPL-1.2

package meter

import (
	"encoding/hex"
	"testing"

	"groot/dps/wire"
)

// Finding you without a zone load (no teleport needed). Golden records from
// A2Tools-DPS-Meter's tests (c55788c).

func entityOf(t *testing.T, s string) (uint16, uint32) {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	op, _ := wire.Opcode(b)
	id, ok := wire.EntityOf(b)
	if !ok {
		t.Fatalf("%s: no entity", s)
	}
	return op, id
}

// 4A 36 is about your own stats and no one else's: a potion, scroll, buff or
// gear change sends them. Three in a row naming one entity: that's you.
func TestOwnStatsRecordsFindYou(t *testing.T) {
	op, id := entityOf(t, "114A36B8300A0000020050165636")
	if op != wire.OpSelfStats || id != 6200 {
		t.Fatalf("op %#x id %d", op, id)
	}
	m := New(nil)
	m.Identify(wire.Identity{Entity: 5100, Self: true, Name: "Mine"})
	m.SelfStats(id)
	m.SelfStats(id)
	if m.Me() != 5100 {
		t.Fatal("two records are not enough")
	}
	m.SelfStats(id)
	if m.Me() != 6200 {
		t.Fatalf("me %d", m.Me())
	}
}

// 06 38 names you several times a second while you play.
func TestPartyScopeRecordsFindYou(t *testing.T) {
	b, _ := hex.DecodeString("240438efc9010600b115c859d100fc0300000001c711c75101000000904eca050100")
	d, err := wire.ParseDamage(b)
	if err != nil {
		t.Fatal(err)
	}
	op, id := entityOf(t, "0e0638b115171dd200f700")
	if op != wire.OpPartyScope || id != 2737 || d.Actor != 2737 {
		t.Fatalf("op %#x id %d actor %d", op, id, d.Actor)
	}
	m := New(nil)
	m.Add(FromDamage(0, d)) // they hit something with a class skill: a player
	for range 23 {
		m.PartyScope(id)
	}
	if m.Me() != 0 {
		t.Fatal("23 records decided")
	}
	m.PartyScope(id)
	if m.Me() != 2737 {
		t.Fatalf("me %d", m.Me())
	}
}

// Only a clear leader: 4x the next player's records.
func TestPartyScopeNeedsAClearLeader(t *testing.T) {
	m := New(nil)
	m.Add(hit(0, groot, boss, 12_020_100, 1))
	m.Add(hit(0, cleric, boss, 17_050_240, 1))
	for range 10 {
		m.PartyScope(cleric)
	}
	for range 32 {
		m.PartyScope(groot)
	}
	if m.Me() != 0 {
		t.Fatalf("32 vs 10 decided: me %d", m.Me())
	}
	for range 8 {
		m.PartyScope(groot)
	}
	if m.Me() != groot {
		t.Fatalf("40 vs 10: me %d", m.Me())
	}
}

// The self record has the last word over 06 38 guesses.
func TestTheSelfRecordBeatsPartyScope(t *testing.T) {
	m := New(nil)
	m.Identify(wire.Identity{Entity: cleric, Self: true, Name: "Pinky"})
	m.Add(hit(0, groot, boss, 12_020_100, 1))
	for range 64 {
		m.PartyScope(groot)
	}
	if m.Me() != cleric {
		t.Fatalf("me %d", m.Me())
	}
}
