// SPDX-License-Identifier: EUPL-1.2

package wire

import (
	"encoding/hex"
	"slices"
	"testing"
)

// Golden identity and party records from A2Tools-DPS-Meter's tests (c55788c).
// Names scrubbed: each real name swapped for one of the same byte length, so
// every offset is as captured.

// frameOf takes a record as A2Tools' tests hold it (opcode first) and frames it.
func frameOf(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return frame(b)
}

func TestGoldenSelfRecords(t *testing.T) {
	cases := []struct {
		rec       string
		entity    uint32
		name      string
		server    uint16
		class     Class
		level, gs uint32
	}{
		{"3336ed745e91c128370647726f6f747918051e000000011c0000007f0100007f0100001c000000d002040000000000", 14957, "Grooty", 1304, Cleric, 28, 383},
		{"3336ef295fa1c128370b5472656562656172647373080924000000022d000000f8070000f80700002d000000", 0x6f | 0x29<<7, "Treebeardss", 2312, Chanter, 45, 2040},
		{"3336a91d5f81c12837054c656166790809200000000223000000aa010000aa01000023000000", 0x29 | 0x1d<<7, "Leafy", 2312, Cleric, 35, 426},
	}
	for _, c := range cases {
		id, err := ParseIdentity(frameOf(t, c.rec))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		want := Identity{Entity: c.entity, Self: true, Name: c.name, Server: c.server, Class: c.class, Level: c.level, GearScore: c.gs}
		if id != want {
			t.Errorf("got  %+v\nwant %+v", id, want)
		}
	}
}

func TestGoldenPlayerSpawn(t *testing.T) {
	id, err := ParseIdentity(frameOf(t, "4536d74800000000070141000000000000000000"))
	if err != nil || id != (Identity{Entity: 9303, Name: "A"}) {
		t.Fatalf("got %+v, %v", id, err)
	}
}

func TestIdentityRejectsWhatIsntAName(t *testing.T) {
	cases := map[string]string{
		"no name bit":        "4536d74800000000060141000000000000000000",
		"self without world": "3336ed745e91c128370647726f6f74790000",
		"not letters":        "4536d7480000000007032d2d2d",
		"digits only":        "4536d74800000000070331323300",
		"13 characters":      "4536d74800000000070d41414141414141414141414141",
	}
	for name, rec := range cases {
		if id, err := ParseIdentity(frameOf(t, rec)); err == nil {
			t.Errorf("%s: got %+v", name, id)
		}
	}
}

// A new character in the tutorial is "$" + random letters: still you, no name yet.
func TestSelfInTheTutorialHasNoName(t *testing.T) {
	rec := "3336ed745e91c12837" + "0d" + hex.EncodeToString([]byte("$Kc03nyeQHr4x"))
	id, err := ParseIdentity(frameOf(t, rec))
	if err != nil || id.Name != "" || !id.Self || id.Entity != 14957 {
		t.Fatalf("got %+v, %v", id, err)
	}
}

func members(r Roster) []Member {
	return r.Members
}

// A party of five, one of them visiting from another world (2302, home 2308).
func TestGoldenRoster(t *testing.T) {
	const rec = "029771931a001d416c6c20626f7373657320616c742072756e204c46206368616e74657205cc2709000003f0b3030000000009ff0202051e01f0b303000000000906526f6f747379200000002d000000db0500000009cd1004be1201000000000000300000000000000001051e02e9b7020000000209054261726b790c0000002d000000f30600000209cd1004f85201000000000000790000000000000001051e03e212040000000009065477696767791c0000002d000000cc0500000009cd1004210001000000000000250000000000000001051e04efd1020000000409064d6f737369651c0000002d0000001d08000007fe08cd1004e85201000000000000750000000000000001051e05b2860200000024050841636f726e626f69210000002d000000990600002405cd1003234301000000000000b20000000000000001050009"
	r, err := ParseRoster(frameOf(t, rec))
	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "All bosses alt run LF chanter" || !r.Complete || r.Dungeon != 0x0927cc {
		t.Errorf("roster %+v", r)
	}
	type m struct {
		slot   uint8
		name   string
		server uint16
		class  Class
		level  uint32
		gs     uint32
		cp     uint64
	}
	var got []m
	for _, x := range members(r) {
		got = append(got, m{x.Slot, x.Name, x.Server, x.Class, x.Level, x.GearScore, x.CombatPower})
	}
	want := []m{
		{1, "Rootsy", 2304, Cleric, 45, 1499, 70334},
		{2, "Barky", 2306, Templar, 45, 1779, 86776},
		{3, "Twiggy", 2304, Sorcerer, 45, 1484, 65569},
		{4, "Mossie", 2308, Sorcerer, 45, 2077, 86760},
		{5, "Acornboi", 1316, Chanter, 45, 1689, 82723},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

// The same visitor with her party on 2313 (09 09): a small low byte once made
// the search land a byte early and read her 86,760 as 22,210,564.
func TestGoldenRosterVisitorOnAnyWorld(t *testing.T) {
	const rec = "029771931a001d416c6c20626f7373657320616c742072756e204c46206368616e74657205cc2709000003f0b3030000000009ff0202051e01f0b303000000000906526f6f747379200000002d000000db0500000009cd1004be1201000000000000300000000000000001051e02e9b7020000000209054261726b790c0000002d000000f30600000209cd1004f85201000000000000790000000000000001051e03e212040000000009065477696767791c0000002d000000cc0500000009cd1004210001000000000000250000000000000001051e04efd1020000000409064d6f737369651c0000002d0000001d080000070909cd1004e85201000000000000750000000000000001051e05b2860200000024050841636f726e626f69210000002d000000990600002405cd1003234301000000000000b20000000000000001050009"
	r, err := ParseRoster(frameOf(t, rec))
	if err != nil || len(r.Members) != 5 {
		t.Fatalf("roster %+v, %v", r, err)
	}
	if r.Members[3].CombatPower != 86760 || r.Members[4].CombatPower != 82723 {
		t.Errorf("got %+v", r.Members[3:])
	}
}

// Slots 1 and 5 filled, 2-4 vacant: the walk used to stop at slot 2.
func TestGoldenRosterPastVacantSlots(t *testing.T) {
	const rec = "0297a5f908000b3835304b20e8bf9ee588b705632809000003854b01000000f6031f0200050c01854b01000000f6030fe69ca8e69ca8e69ca8e69ca8e69ca8060000003200000007170000f603f6030438390d00000000000001010002000000000000000000000000000000000000000000040000000000000000000000000003000000000000000000000000000000000000000000040000000000000000000000000004000000000000000000000000000000000000000000040000000000000000000000000e05626401000000f6030251370e000000320000005e160000f603f603046a030d0000000000000101020e0036"
	r, err := ParseRoster(frameOf(t, rec))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range r.Members {
		got = append(got, x.Name)
	}
	if !slices.Equal(got, []string{"木木木木木", "Q7"}) || r.Members[1].Slot != 5 || !r.Complete {
		t.Fatalf("got %v %+v", got, r)
	}
}
