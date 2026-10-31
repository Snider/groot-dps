// SPDX-License-Identifier: EUPL-1.2

package wire

import (
	"encoding/hex"
	"slices"
	"testing"
)

// Golden damage records: real 04 38 records (the bytes after the opcode) from
// A2Tools-DPS-Meter's tests (github.com/taengu/A2Tools-DPS-Meter, GPL-3, read at
// c55788c), where the values were checked against the game's own Damage Analyzer.
// The facts are theirs; the decoder is ours.

func record(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return pkt(OpDamage, b...)
}

func mustDamage(t *testing.T, s string) Damage {
	t.Helper()
	d, err := ParseDamage(record(t, s))
	if err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return d
}

func TestGoldenWhoHitWhom(t *testing.T) {
	cases := []struct {
		rec                  string
		target, actor, skill uint32
		amount               uint32
	}{
		{"e9de020600c80b9147ff003f02020002aff4b76301000000e4506a0100", 44905, 1480, 16730001, 106},
		{"fcc2010600bf6bc759d1003203020002c711c75101000000904e660100", 24956, 13759, 13720007, 102},
		{"cac20406009e389147ff00e102040000aff4b76301000000a4587f0100", 74058, 7198, 16730001, 127},
		{"bfe6020600d23310ffd6006f0208000055a2fb5302000000aa56d20e0200", 45887, 6610, 14090000, 1874},
		// Gatekeeper Pinopi (a 7-digit NPC skill) hitting a player for 1,258.
		{"D14B0600D5F3027E871800020202000243ED940901000000904EEA090100", 9681, 47573, 1607550, 1258},
	}
	for _, c := range cases {
		d := mustDamage(t, c.rec)
		if d.Target != c.target || d.Actor != c.actor || d.Skill != c.skill || d.Amount != c.amount {
			t.Errorf("%s:\n got  target %d actor %d skill %d amount %d\n want target %d actor %d skill %d amount %d",
				c.rec, d.Target, d.Actor, d.Skill, d.Amount, c.target, c.actor, c.skill, c.amount)
		}
	}
}

func TestGoldenHitFlags(t *testing.T) {
	cases := []struct {
		rec                                 string
		amount                              uint32
		crit, parry, perfect, double, front bool
	}{
		{"e9de020600c80b9147ff003f02020002aff4b76301000000e4506a0100", 106, false, true, false, false, true},
		{"fcc2010600bf6bc759d1003203020002c711c75101000000904e660100", 102, true, true, false, false, true},
		{"cac20406009e389147ff00e102040000aff4b76301000000a4587f0100", 127, false, false, true, false, false},
		{"bfe6020600d23310ffd6006f0208000055a2fb5302000000aa56d20e0200", 1874, false, false, false, true, false},
	}
	for _, c := range cases {
		d := mustDamage(t, c.rec)
		got := []bool{d.Crit(), d.Parry(), d.Perfect(), d.Double(), d.Front()}
		want := []bool{c.crit, c.parry, c.perfect, c.double, c.front}
		if d.Amount != c.amount || !slices.Equal(got, want) {
			t.Errorf("%s: amount %d flags %v, want %d %v", c.rec, d.Amount, got, c.amount, want)
		}
	}
}

// Monsters' hits on players carry the defensive flags.
func TestGoldenReceivedHits(t *testing.T) {
	cases := []struct {
		rec    string
		flags  byte
		hp     uint32
		angle  byte
		amount uint32
	}{
		{"ca6f0600ca9804b8c112000102010002ebab530701000000904e4a0100", FlagShieldBlock, 0, AngleFront, 74},
		{"fe2a0600c68d03b8c112000102100002ebab530701000000904e740100", FlagIronWall, 0, AngleFront, 116},
		{"df7e0600a3b103b0b512000302200d02cbf84e0701000000904e440100", FlagRegeneration, 13, AngleFront, 68},
		{"e9230600ee9e03e4cc120001024100021b09580701000000904e010100", FlagShieldBlock | FlagPerfectBlock, 0, AngleFront, 1},
		{"e06d0600aeb1024cba12000202420002bbc5500701000000904e010100", FlagParry | FlagPerfectBlock, 0, AngleFront, 1},
		// Restored HP is a varint: 177 takes two bytes (b1 01), 20% of 889.
		{"ed080600f0ef04bab51200010220b10102b3fc4e0701000000904ef9060100", FlagRegeneration, 177, AngleFront, 889},
		{"ee0d0600849d03bab51200010220cc0101b3fc4e0701000000904efc070100", FlagRegeneration, 204, AngleBack, 1020},
	}
	for _, c := range cases {
		d := mustDamage(t, c.rec)
		if d.Flags != c.flags || d.RestoredHP != c.hp || d.Angle != c.angle || d.Amount != c.amount {
			t.Errorf("%s: flags %#x hp %d angle %d amount %d, want %#x %d %d %d",
				c.rec, d.Flags, d.RestoredHP, d.Angle, d.Amount, c.flags, c.hp, c.angle, c.amount)
		}
		if d.Scalar != 10000 {
			t.Errorf("%s: scalar %d, want 10000 (monsters hit at 100.00%%)", c.rec, d.Scalar)
		}
	}
}

// Switch bit 0x20: additional hits after the value. The value already includes them.
func TestGoldenAdditionalHits(t *testing.T) {
	cases := []struct {
		rec    string
		amount uint32
		extra  []uint32
	}{
		{"b1ea013600f30a40c0f4007a038000010b199b5f01000000ac52a40d021818", 1700, []uint32{24, 24}},
		{"b1ea013400f30ae0b7f800cd028bd3276101000000ac52d330010159", 6227, []uint32{89}},
		// A spirit's layout-4 records: no layout-4 field, the hits right after the value.
		{"fe9e0224009e9b01c3f8f5000302792b156002000000ac52e10301060200", 481, []uint32{6}},
		{"fe9e0224009e9b01c18601001702a7a2980001000000ac52e401030303030100", 228, []uint32{3, 3, 3}},
		{"fe9e0204009e9b01c3f8f5000302792b156003000000ac52b7030300", 439, nil},
		{"b1ea011600f30a40c0f40063028000010b199b5f01000000ac52d007", 976, nil},
	}
	for _, c := range cases {
		d := mustDamage(t, c.rec)
		if d.Amount != c.amount || !slices.Equal(d.Extra, c.extra) {
			t.Errorf("%s: amount %d extra %v, want %d %v", c.rec, d.Amount, d.Extra, c.amount, c.extra)
		}
		if d.Scalar != 10540 {
			t.Errorf("%s: scalar %d, want 10540", c.rec, d.Scalar)
		}
	}
}

// Switch bit 0x40: a hit into a shield. The ids of the absorbing effects follow
// the value; read as extra hits they'd add 8,387 to each.
func TestGoldenShield(t *testing.T) {
	for _, c := range []struct {
		rec    string
		amount uint32
	}{
		{"F3B7024600A17B680BC8001502000002D545244E02000000F462C50401C3414A070200", 581},
		{"F3B7024400A17B181DD2001A026B5D135201000000F462BD0201C3414A070100", 317},
	} {
		d := mustDamage(t, c.rec)
		if d.Amount != c.amount || len(d.Extra) != 0 || !slices.Equal(d.Absorbed, []uint32{122307011}) {
			t.Errorf("%s: amount %d extra %v absorbed %v, want %d [] [122307011]", c.rec, d.Amount, d.Extra, d.Absorbed, c.amount)
		}
	}
}

// Switch low nibble 0 or 2: no value. The hit type says why.
func TestGoldenMissAndResist(t *testing.T) {
	hit := mustDamage(t, "a8a3041400be77802a04015d02109aa06501000000dc51b80f010100")
	if hit.Amount != 1976 || hit.NoValue {
		t.Errorf("hit: %+v", hit)
	}
	resist := mustDamage(t, "a8a3040001be77802a04015d06139aa06501000000dc5101c9e1f5050100")
	if !resist.NoValue || !resist.Resist() || resist.Amount != 0 ||
		resist.Target != 70056 || resist.Actor != 15294 || resist.Skill != 17050240 || resist.UID != hit.UID {
		t.Errorf("resist: %+v", resist)
	}
	miss := mustDamage(t, "e981020000899803172df9000101079d556101000000904e0100")
	if !miss.NoValue || !miss.Miss() || miss.Amount != 0 {
		t.Errorf("miss: %+v", miss)
	}
}
