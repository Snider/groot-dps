// SPDX-License-Identifier: EUPL-1.2

package wire

import (
	"encoding/hex"
	"slices"
	"testing"
)

// Golden 05 38 ticks, whole frames from A2Tools-DPS-Meter's tests (c55788c).

func TestGoldenTicks(t *testing.T) {
	cases := []struct {
		name, frame          string
		target, actor, skill uint32
		amount               uint32
		heal, hot            bool
		absorbed             []uint32
	}{
		// Restore HP, a full heal on yourself (5041's max HP 6683). 1900001 is no skill but has a name.
		{"self heal", "130538b12701b127b601032c530bb739", 5041, 5041, 1900001, 7351, true, false, nil},
		// A Burn many monster skills share (effect 120001211) on player 5377.
		{"burn", "170538812a0abdd1019002bb1227073332931200", 5377, 26813, 1200012, 51, false, false, nil},
		// Into a shield: effect type 0x0E, the absorbing effects after the amount.
		{"shielded", "1B0538F3B7020EA17B020854D6514D01C3414A07D780D100", 0xF3&0x7F | 0x37<<7 | 2<<14, 0x21 | 0x7B<<7, 13730007, 77, false, false, []uint32{122307011}},
		// A Wind Spirit's Malicious Whirlwind.
		{"spirit", "19053888fd020accc801881241c15f5ff7025828f400", 48776, 25676, 16001109, 375, false, false, nil},
		// A Punishment tick on Bakarma.
		{"punishment", "130538fbaa0302d8290040e00f48bf1b", 0x7B | 0x2A<<7 | 3<<14, 0x58 | 0x29<<7, 12090000, 3519, false, false, nil},
	}
	for _, c := range cases {
		b, err := hex.DecodeString(c.frame)
		if err != nil {
			t.Fatal(err)
		}
		d, err := ParseTick(b)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if d.Target != c.target || d.Actor != c.actor || d.Skill() != c.skill || d.Amount != c.amount ||
			d.Heal() != c.heal || d.HoT() != c.hot || !slices.Equal(d.Absorbed, c.absorbed) {
			t.Errorf("%s: got %+v (skill %d heal %v)", c.name, d, d.Skill(), d.Heal())
		}
	}
}

func TestParseTickRejects(t *testing.T) {
	tick := func(kind byte, target, actor uint64, effect uint32, amount uint64) []byte {
		return pkt(OpDotDamage, cat(uv(target), []byte{kind}, uv(actor), uv(2), []byte{byte(effect), byte(effect >> 8), byte(effect >> 16), byte(effect >> 24)}, uv(amount))...)
	}
	if _, err := ParseTick(tick(TickDamage, 7, 8, 1200012_11, 51)); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	cases := map[string][]byte{
		"wrong opcode": pkt(OpDamage, 1, 2, 3),
		"status tick":  tick(0x00, 7, 8, 1200012_11, 51),
		"buff tick":    tick(0x08, 7, 8, 1200012_11, 51),
		"self damage":  tick(TickDamage, 7, 7, 1200012_11, 51),
		"skill 0":      tick(TickDamage, 7, 8, 99, 51),
		"amount 0":     tick(TickDamage, 7, 8, 1200012_11, 0),
		"amount huge":  tick(TickDamage, 7, 8, 1200012_11, 100_000_000),
		"truncated":    tick(TickDamage, 7, 8, 1200012_11, 51)[:6],
	}
	for name, p := range cases {
		if _, err := ParseTick(p); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := ParseTick(tick(TickHeal, 7, 7, 1900001_31, 51)); err != nil {
		t.Errorf("self heal: %v", err)
	}
}
