package wire

import (
	"encoding/binary"
	"testing"
)

// damageBody builds a record in the layout the golden records show: plotter
// (layouts 6/7, given in special), 8 unknown bytes, scalar, value.
func damageBody(target, sw, actor uint64, skill uint32, hitType uint64, special []byte, damage uint64) []byte {
	return cat(
		uv(target), uv(sw), uv(0), uv(actor),
		binary.LittleEndian.AppendUint32(nil, skill), []byte{0x5d},
		uv(hitType), special, make([]byte, 8), uv(10000), uv(damage),
	)
}

func TestParseDamageSynthetic(t *testing.T) {
	d, err := ParseDamage(pkt(OpDamage, damageBody(500, 0x06, 42, 11020100, 3, []byte{FlagPerfect | FlagDouble, 0, AngleBack}, 123456)...))
	if err != nil {
		t.Fatal(err)
	}
	if d.Target != 500 || d.Actor != 42 || d.Skill != 11020100 || d.Amount != 123456 || d.Scalar != 10000 ||
		!d.Crit() || !d.Perfect() || !d.Double() || !d.Back() || d.Parry() {
		t.Fatalf("got %+v", d)
	}
}

// A tail that doesn't end cleanly is left alone: the amount still stands.
func TestParseDamageMessyTailKeepsAmount(t *testing.T) {
	d, err := ParseDamage(pkt(OpDamage, cat(damageBody(7, 0x24, 8, 1000, 2, nil, 999), []byte{0x63, 0x63})...))
	if err != nil || d.Amount != 999 || d.Extra != nil {
		t.Fatalf("got %+v, %v", d, err)
	}
}

func TestParseDamageRejects(t *testing.T) {
	cases := map[string][]byte{
		"wrong opcode": pkt(0x3805, damageBody(7, 0x04, 8, 1000, 1, nil, 9)...),
		"bad switch":   pkt(OpDamage, damageBody(7, 0x03, 8, 1000, 1, nil, 9)...),
		"self hit":     pkt(OpDamage, damageBody(8, 0x04, 8, 1000, 1, nil, 9)...),
		"skill 0":      pkt(OpDamage, damageBody(7, 0x04, 8, 0, 1, nil, 9)...),
		"skill huge":   pkt(OpDamage, damageBody(7, 0x04, 8, 300_000_000, 1, nil, 9)...),
		"truncated":    pkt(OpDamage, damageBody(7, 0x06, 8, 1000, 1, nil, 9)[:14]...),
		"empty":        pkt(OpDamage),
		"varint > i32": pkt(OpDamage, damageBody(1<<31, 0x04, 8, 1000, 1, nil, 9)...),
	}
	for name, p := range cases {
		if _, err := ParseDamage(p); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
