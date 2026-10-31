// SPDX-License-Identifier: EUPL-1.2

package wire

import (
	"encoding/binary"
	"errors"
	"unicode/utf8"
)

// Roster is a 02 97 party roster. Members carry account ids, not the entity
// ids combat uses: join them to Identity records by name.
type Roster struct {
	Key      uint32
	Title    string // the party's name / LFG note
	Dungeon  uint32 // the instance it's queued for or in, tier included
	Members  []Member
	Complete bool // the walk reached the last member
}

type Member struct {
	Slot             uint8
	Name             string
	Account          uint64 // top u16 is the home world
	Server           uint16
	Class            Class
	Level, GearScore uint32
	CombatPower      uint64
}

var ErrNotRoster = errors.New("roster: not a party roster")

// ParseRoster reads a party roster. Member records vary in width, so after
// each one the walk re-finds the next by its header (A2Tools does the same).
func ParseRoster(p []byte) (Roster, error) {
	if op, ok := Opcode(p); !ok || op != OpPartyInfo {
		return Roster{}, ErrNotRoster
	}
	_, n := uvarint(p)
	d := p[n+2:]

	var r Roster
	h := reader{b: d}
	r.Key = h.u32()
	k := int(h.u8())
	if h.err != nil || k < 1 || k > 40 || h.remaining() < k || !utf8.Valid(h.b[:k]) {
		return Roster{}, ErrNotRoster
	}
	r.Title = string(h.b[:k])
	h.skip(k)
	if size := h.u8(); size < 1 || size > 12 {
		return Roster{}, ErrNotRoster
	}
	r.Dungeon = h.u32()
	h.skip(2 + 8 + 3) // pad, leader account, pad
	count := h.varint32()
	if h.err != nil || count < 1 || count > 12 {
		return Roster{}, ErrNotRoster
	}
	o := len(d) - h.remaining()

	for range count {
		if o+20 > len(d) {
			break
		}
		m := Member{Slot: d[o+1]}
		o += 2 // presence mask, slot
		m.Account = binary.LittleEndian.Uint64(d[o:])
		m.Server = uint16(m.Account >> 48)
		o += 8
		k := int(d[o])
		o++
		if k == 0 { // a vacant slot: maybe a member further on, maybe the end
			next, ok := laterMember(d, o, m.Slot, uint8(count))
			if !ok {
				r.Complete = true
				break
			}
			o = next
			continue
		}
		if k > 40 || o+k > len(d) || !exactName(string(d[o:o+k])) {
			break
		}
		m.Name = string(d[o : o+k])
		o += k
		if o+12 > len(d) {
			break
		}
		m.Class = ClassOf(binary.LittleEndian.Uint32(d[o:]))
		m.Level = binary.LittleEndian.Uint32(d[o+4:])
		m.GearScore = binary.LittleEndian.Uint32(d[o+8:])
		o += 12
		if m.Level < 1 || m.Level > 200 || m.GearScore > 1_000_000 {
			break
		}
		// The gap before the combat power varies. Anchor on the member's world
		// repeated as a u16; a visitor has the world they're on instead, so
		// failing that, any world with a tag byte 1-4 and a sane power after.
		at, ok := findU16(d, o, o+10, m.Server)
		if !ok {
			at, ok = visitorWorld(d, o)
		}
		if !ok {
			break
		}
		o = at + 2 + 2 + 1
		if o+8 > len(d) {
			break
		}
		m.CombatPower = binary.LittleEndian.Uint64(d[o:])
		o += 8
		if m.CombatPower > 100_000_000 {
			break
		}
		r.Members = append(r.Members, m)
		if uint32(m.Slot) >= count {
			r.Complete = true
			break
		}
		next, ok := memberWithin(d, o, 32, m.Slot+1)
		if !ok {
			next, ok = vacantSlot(d, o, m.Slot+1)
		}
		if !ok {
			break
		}
		o = next
	}
	if len(r.Members) == 0 {
		return Roster{}, ErrNotRoster
	}
	return r, nil
}

func findU16(d []byte, from, to int, want uint16) (int, bool) {
	for i := from; i <= min(to, len(d)-2); i++ {
		if binary.LittleEndian.Uint16(d[i:]) == want {
			return i, true
		}
	}
	return 0, false
}

func visitorWorld(d []byte, from int) (int, bool) {
	for i := from; i < from+10 && i+13 <= len(d); i++ {
		w := binary.LittleEndian.Uint16(d[i:])
		tag := d[i+4]
		power := binary.LittleEndian.Uint64(d[i+5:])
		if w >= 1000 && w <= 2999 && tag >= 1 && tag <= 4 && power >= 1 && power <= 100_000_000 {
			return i, true
		}
	}
	return 0, false
}

// memberWithin finds a member header near from: <mask> <slot> <account u64,
// world in its top u16> <name len> <utf8 name>.
func memberWithin(d []byte, from, span int, slot uint8) (int, bool) {
	for i := from; i <= min(from+span, len(d)-12); i++ {
		if d[i+1] != slot {
			continue
		}
		if w := binary.LittleEndian.Uint16(d[i+8:]); w == 0 || w > 9_999 {
			continue
		}
		k := int(d[i+10])
		if k == 0 || k > 40 || i+11+k > len(d) || !utf8.Valid(d[i+11:i+11+k]) {
			continue
		}
		return i, true
	}
	return 0, false
}

// laterMember: a named member in a slot after this vacant one (a vacant record is ~36 bytes).
func laterMember(d []byte, from int, slot, last uint8) (int, bool) {
	for next := int(slot) + 1; next <= int(last); next++ {
		if i, ok := memberWithin(d, from, 48*(next-int(slot)), uint8(next)); ok {
			return i, true
		}
	}
	return 0, false
}

// vacantSlot: a zero mask, the slot, a zero account and an empty name.
func vacantSlot(d []byte, from int, slot uint8) (int, bool) {
	for i := from; i <= min(from+32, len(d)-11); i++ {
		if d[i] != 0 || d[i+1] != slot {
			continue
		}
		zero := true
		for _, b := range d[i+2 : i+11] {
			zero = zero && b == 0
		}
		if zero {
			return i, true
		}
	}
	return 0, false
}

const (
	OpPartyLeft         uint16 = 0x9213 // you left an open-world party
	OpPartyLeftInstance uint16 = 0x971D // you left a dungeon or raid party
)

// PartyLeft: 13 92 00 00 or 1D 97 00 00, and nothing after.
func PartyLeft(p []byte) bool {
	op, ok := Opcode(p)
	if !ok || op != OpPartyLeft && op != OpPartyLeftInstance {
		return false
	}
	_, n := uvarint(p)
	body := p[n+2:]
	return len(body) == 2 && body[0] == 0 && body[1] == 0
}
