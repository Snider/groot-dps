// SPDX-License-Identifier: EUPL-1.2

package wire

import (
	"encoding/binary"
	"errors"
	"math"
)

// Damage is one hit from a 0x3804 record (layout: plans/protocol.md).
type Damage struct {
	Target, Actor uint32
	Switch        uint32 // low nibble: layout (4-7 has a value, 0/2 none); 0x20 extra hits, 0x40 absorbed
	Skill         uint32 // 7 digits = an NPC's skill (a monster hitting a player)
	UID           byte   // same for every record of one cast
	HitType       uint32 // see Crit, Miss, Resist
	NoValue       bool   // a miss or a resist: no Amount
	Flags         byte   // layouts 6/7: Flag* bits
	RestoredHP    uint32 // with FlagRegeneration
	Angle         byte   // AngleBack, AngleFront, 0 = untagged
	Scalar        uint32 // the actor's damage multiplier in hundredths of a % (10000 = mobs); summons inherit their owner's
	Amount        uint32 // includes Extra
	Extra         []uint32
	Absorbed      []uint32 // ids of the shield effects that soaked part of it; Amount is what got through
	Unknown4      uint32   // layout 4's field before the extra hits (1-25), meaning unknown
}

// Plotter flags (layouts 6/7), checked against the game's Damage Analyzer by A2Tools.
const (
	FlagShieldBlock  = 0x01
	FlagParry        = 0x02
	FlagPerfect      = 0x04
	FlagDouble       = 0x08 // HardHit, 강타
	FlagIronWall     = 0x10
	FlagRegeneration = 0x20
	FlagPerfectBlock = 0x40
)

const (
	AngleBack  = 1
	AngleFront = 2
)

func (d Damage) Crit() bool    { return d.HitType == 3 }
func (d Damage) Miss() bool    { return d.NoValue && d.HitType == 1 }
func (d Damage) Resist() bool  { return d.NoValue && d.HitType == 6 }
func (d Damage) Parry() bool   { return d.Flags&FlagParry != 0 }
func (d Damage) Perfect() bool { return d.Flags&FlagPerfect != 0 }
func (d Damage) Double() bool  { return d.Flags&FlagDouble != 0 }
func (d Damage) Back() bool    { return d.Angle == AngleBack }
func (d Damage) Front() bool   { return d.Angle == AngleFront }

var (
	ErrNotDamage = errors.New("not a damage packet")
	ErrSwitch    = errors.New("damage: bad switch value")
	ErrSelfHit   = errors.New("damage: actor == target")
	ErrSkill     = errors.New("damage: skill out of range")
	ErrShort     = errors.New("damage: packet too short")
)

// ParseDamage decodes the first record of a 0x3804 packet.
func ParseDamage(p []byte) (Damage, error) {
	if op, ok := Opcode(p); !ok || op != OpDamage {
		return Damage{}, ErrNotDamage
	}
	_, n := uvarint(p)
	r := reader{b: p[n+2:]}

	var d Damage
	d.Target = r.varint32()
	d.Switch = r.varint32()
	layout := d.Switch & 0x0F
	d.NoValue = layout == 0 || layout == 2
	if r.err == nil && !d.NoValue && (layout < 4 || layout > 7) {
		return Damage{}, ErrSwitch
	}
	r.varint32() // unknown
	d.Actor = r.varint32()
	if r.err == nil && d.Actor == d.Target {
		return Damage{}, ErrSelfHit
	}
	d.Skill = r.u32()
	if r.err == nil && (d.Skill < 1 || d.Skill > 299_999_999) {
		return Damage{}, ErrSkill
	}
	d.UID = r.u8()
	d.HitType = r.varint32()
	if d.NoValue || r.err != nil {
		return d, r.err
	}

	if layout&0x02 != 0 {
		d.Flags = r.u8()
		d.RestoredHP = r.varint32()
		d.Angle = r.u8()
	}
	if layout&0x01 != 0 {
		r.skip(4) // unknown; no real record of layouts 5/7 checked yet
	}
	r.skip(8) // u32 + u32, unknown
	d.Scalar = r.varint32()
	if d.Scalar == 0 { // a zero pad seen before the scalar since 2026-06
		d.Scalar = r.varint32()
	}
	d.Amount = r.varint32()
	if r.err != nil {
		return Damage{}, r.err
	}

	// The tail only counts when it ends cleanly. Layout 4 has an extra field,
	// except a spirit's, which reads like layout 6.
	if !d.tail(r.b, layout) && layout == 4 {
		d.tail(r.b, 6)
	}
	return d, nil
}

func (d *Damage) tail(b []byte, layout uint32) bool {
	r := reader{b: b}
	var field uint32
	if layout == 4 {
		if field = r.varint32(); field < 1 || field > 25 {
			return false
		}
	}
	var extra, absorbed []uint32
	if d.Switch&0x20 != 0 {
		n := r.varint32()
		if n < 1 || n > 25 {
			return false
		}
		var sum uint64
		for range n {
			h := r.varint32()
			extra = append(extra, h)
			sum += uint64(h)
		}
		if sum >= uint64(d.Amount) {
			return false
		}
	}
	if d.Switch&0x40 != 0 {
		n := r.varint32()
		if n < 1 || n > 8 {
			return false
		}
		for range n {
			absorbed = append(absorbed, r.u32())
		}
	}
	rest := r.b
	clean := r.err == nil && (len(rest) == 0 ||
		len(rest) >= 2 && rest[1] == 0 && rest[0] >= 1 && rest[0] <= 7 ||
		len(rest) >= 2 && rest[0] == 0x04 && rest[1] == 0x38)
	if clean {
		d.Unknown4, d.Extra, d.Absorbed = field, extra, absorbed
	}
	return clean
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

// varint32 is the game's varint: at most 5 bytes, never past int32.
func (r *reader) varint32() uint32 {
	v, n := uvarint(r.b)
	if n <= 0 || n > 5 || v > math.MaxInt32 {
		r.fail()
		return 0
	}
	r.b = r.b[n:]
	return uint32(v)
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
