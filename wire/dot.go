// SPDX-License-Identifier: EUPL-1.2

package wire

import "errors"

// Tick is one damage-over-time or heal tick from a 0x3805 packet.
type Tick struct {
	Target   uint32
	Kind     byte // TickDamage, TickHeal, TickHoT, ...; +0x04 = into a shield
	Actor    uint32
	Unknown  uint32
	Effect   uint32 // skill*100 + something (an effect level?)
	Amount   uint32
	Absorbed []uint32 // shield effects that soaked part of a damage tick
}

// Tick kinds: exact values, not a bitmask (0x0B, a HoT, has bit 0x02 set too).
// 0x00 is a status tick, 0x08 a buff: neither carries an amount we use.
const (
	TickHeal       = 0x01
	TickDamage     = 0x02
	TickHealB      = 0x09
	TickDamageB    = 0x0A
	TickHoT        = 0x0B
	tickIntoShield = 0x04
)

var ErrNotTick = errors.New("not a tick packet")

func (t Tick) Skill() uint32 { return t.Effect / 100 }
func (t Tick) Heal() bool    { return t.Kind == TickHeal || t.Kind == TickHealB || t.Kind == TickHoT }
func (t Tick) HoT() bool     { return t.Kind == TickHoT }
func (t Tick) Damage() bool {
	k := t.Kind &^ tickIntoShield
	return k == TickDamage || k == TickDamageB
}

// ParseTick decodes a 0x3805 packet: damage and heal ticks only.
func ParseTick(p []byte) (Tick, error) {
	if op, ok := Opcode(p); !ok || op != OpDotDamage {
		return Tick{}, ErrNotTick
	}
	_, n := uvarint(p)
	r := reader{b: p[n+2:]}

	var t Tick
	t.Target = r.varint32()
	t.Kind = r.u8()
	if r.err == nil && !t.Damage() && !t.Heal() {
		return Tick{}, ErrNotTick
	}
	t.Actor = r.varint32()
	if r.err == nil && t.Damage() && t.Actor == t.Target { // a self-heal is fine, self-damage is noise
		return Tick{}, ErrSelfHit
	}
	t.Unknown = r.varint32()
	t.Effect = r.u32()
	if r.err == nil && (t.Skill() < 1 || t.Skill() > 299_999_999) {
		return Tick{}, ErrSkill
	}
	t.Amount = r.varint32()
	if r.err != nil {
		return Tick{}, r.err
	}
	if t.Amount == 0 || t.Amount > 99_999_999 {
		return Tick{}, ErrAmount
	}
	if t.Kind&tickIntoShield != 0 && t.Damage() {
		if k := r.varint32(); r.err == nil && k >= 1 && k <= 8 {
			for range k {
				if id := r.u32(); r.err == nil {
					t.Absorbed = append(t.Absorbed, id)
				}
			}
		}
	}
	return t, nil
}
