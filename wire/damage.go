package wire

import (
	"encoding/binary"
	"errors"
)

// Damage is one hit from a 0x3804 packet.
type Damage struct {
	Target, Actor uint64
	Skill         uint32
	Type          uint64 // 3 = crit
	Amount        uint64
	Crit          bool
	Back, Front   bool // from the direction byte
	Parry         bool
	Perfect       bool
	Double        bool
}

var (
	ErrNotDamage = errors.New("not a damage packet")
	ErrSwitch    = errors.New("damage: bad switch value")
	ErrSelfHit   = errors.New("damage: actor == target")
	ErrShort     = errors.New("damage: packet too short")
)

// ParseDamage decodes a 0x3804 packet (layout: plans/protocol.md).
func ParseDamage(p []byte) (Damage, error) {
	if op, ok := Opcode(p); !ok || op != OpDamage {
		return Damage{}, ErrNotDamage
	}
	_, n := uvarint(p)
	r := reader{b: p[n+2:]}

	var d Damage
	d.Target = r.varint()
	sw := r.varint()
	if sw > 255 || sw&0x0F < 4 || sw&0x0F > 7 {
		return Damage{}, ErrSwitch
	}
	sw &= 0x0F
	r.varint() // unknown
	d.Actor = r.varint()
	if r.err == nil && d.Actor == d.Target {
		return Damage{}, ErrSelfHit
	}
	d.Skill = r.u32()
	r.skip(1)
	d.Type = r.varint()
	d.Crit = d.Type == 3

	if sw == 4 {
		if r.remaining() < 8 {
			return Damage{}, ErrShort
		}
	} else {
		if r.remaining() < 12 {
			return Damage{}, ErrShort
		}
		flags := r.u8()
		r.skip(1)
		dir := r.u8()
		d.Parry = flags&0x02 != 0
		d.Perfect = flags&0x04 != 0
		d.Double = flags&0x08 != 0
		d.Back = dir&0x01 != 0
		d.Front = dir&0x02 != 0
	}
	r.skip(8) // u32 + 4 bytes, unknown
	r.varint() // unknown
	d.Amount = r.varint()

	if r.err != nil {
		return Damage{}, r.err
	}
	return d, nil
}

// reader is a cursor over a packet body; the first short read sticks in err.
type reader struct {
	b   []byte
	err error
}

func (r *reader) remaining() int { return len(r.b) }

func (r *reader) fail() { r.err, r.b = ErrShort, nil }

func (r *reader) varint() uint64 {
	v, n := uvarint(r.b)
	if n <= 0 {
		r.fail()
		return 0
	}
	r.b = r.b[n:]
	return v
}

func (r *reader) u8() byte {
	if len(r.b) < 1 {
		r.fail()
		return 0
	}
	v := r.b[0]
	r.b = r.b[1:]
	return v
}

func (r *reader) u32() uint32 {
	if len(r.b) < 4 {
		r.fail()
		return 0
	}
	v := binary.LittleEndian.Uint32(r.b)
	r.b = r.b[4:]
	return v
}

func (r *reader) skip(k int) {
	if len(r.b) < k {
		r.fail()
		return
	}
	r.b = r.b[k:]
}
