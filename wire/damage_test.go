package wire

import (
	"encoding/binary"
	"testing"
)

func damageBody(target, sw, actor uint64, skill uint32, dmgType uint64, special []byte, damage uint64) []byte {
	return cat(
		uv(target), uv(sw), uv(0), uv(actor),
		binary.LittleEndian.AppendUint32(nil, skill), []byte{0},
		uv(dmgType), special, uv(0), uv(damage),
	)
}

func TestParseDamageWithFlags(t *testing.T) {
	// flag byte 0x01 (back) is overridden by the direction byte, as in the reference meter.
	special := cat([]byte{0x01 | 0x04 | 0x08, 0x00, 0x01}, make([]byte, 8)) // perfect+double, direction back
	p := pkt(OpDamage, damageBody(500, 0x05, 42, 11020100, 3, special, 123456789012)...)

	d, err := ParseDamage(p)
	if err != nil {
		t.Fatal(err)
	}
	want := Damage{Target: 500, Actor: 42, Skill: 11020100, Type: 3, Amount: 123456789012,
		Crit: true, Back: true, Perfect: true, Double: true}
	if d != want {
		t.Fatalf("got  %+v\nwant %+v", d, want)
	}
}

func TestParseDamageSwitchFourHasNoFlagBytes(t *testing.T) {
	p := pkt(OpDamage, damageBody(7, 0x04, 8, 1000, 1, make([]byte, 8), 999)...)
	d, err := ParseDamage(p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Amount != 999 || d.Crit || d.Back {
		t.Fatalf("got %+v", d)
	}
}

func TestParseDamageRejects(t *testing.T) {
	cases := map[string][]byte{
		"wrong opcode": pkt(0x3805, damageBody(7, 0x04, 8, 1000, 1, make([]byte, 8), 9)...),
		"bad switch":   pkt(OpDamage, damageBody(7, 0x03, 8, 1000, 1, make([]byte, 8), 9)...),
		"self hit":     pkt(OpDamage, damageBody(8, 0x04, 8, 1000, 1, make([]byte, 8), 9)...),
		"truncated":    pkt(OpDamage, damageBody(7, 0x05, 8, 1000, 1, make([]byte, 4), 9)...),
		"empty":        pkt(OpDamage),
		"switch > 255": pkt(OpDamage, damageBody(7, 0x104, 8, 1000, 1, make([]byte, 8), 9)...),
	}
	for name, p := range cases {
		if _, err := ParseDamage(p); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
