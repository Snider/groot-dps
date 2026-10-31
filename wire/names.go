// SPDX-License-Identifier: EUPL-1.2

package wire

import (
	"encoding/binary"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Class is a character class, from the class ids in self and party records.
type Class uint8

const (
	NoClass Class = iota
	Gladiator
	Templar
	Ranger
	Assassin
	Elementalist
	Sorcerer
	Cleric
	Chanter
)

var classNames = [...]string{"", "Gladiator", "Templar", "Ranger", "Assassin", "Elementalist", "Sorcerer", "Cleric", "Chanter"}

func (c Class) String() string {
	if int(c) < len(classNames) {
		return classNames[c]
	}
	return ""
}

// ClassOf: ids come in fours per class (5-8 Gladiator ... 33-36 Chanter).
func ClassOf(id uint32) Class {
	if id < 5 || id > 36 {
		return NoClass
	}
	return Class((id-5)/4 + 1)
}

// OpPlayerSpawnOld is 0x3645 before the June 2026 opcode shift.
const OpPlayerSpawnOld uint16 = 0x3644

var ErrNotName = errors.New("identity: no name here")

// Identity binds an entity id to a character name: 33 36 is you, 45 36 anyone else.
type Identity struct {
	Entity uint32
	Self   bool
	Name   string // "" for a new character still in the tutorial (named "$..." until then)
	// Self only:
	Server           uint16
	Class            Class
	Level, GearScore uint32
}

// ParseIdentity reads <entity varint> <mask u32> <mask2 u8> [mask2 & 1: <len u8> <utf8 name>].
func ParseIdentity(p []byte) (Identity, error) {
	op, ok := Opcode(p)
	if !ok || op != OpPlayerInfo && op != OpOtherPlayers && op != OpPlayerSpawnOld {
		return Identity{}, ErrNotName
	}
	_, n := uvarint(p)
	r := reader{b: p[n+2:]}
	id := Identity{Self: op == OpPlayerInfo}
	id.Entity = r.varint32()
	r.skip(4)
	if r.u8()&0x01 == 0 || r.err != nil || id.Entity < 1 || id.Entity > 9_999_999 {
		return Identity{}, ErrNotName
	}
	k := int(r.u8())
	if k < 1 || k > 48 || r.remaining() < k {
		return Identity{}, ErrNotName
	}
	raw := string(r.b[:k])
	r.skip(k)
	if id.Self && placeholder(raw) {
		return id, nil
	}
	if !exactName(raw) {
		return Identity{}, ErrNotName
	}
	id.Name = raw
	if id.Self {
		// A self record goes on with your world and class; without them it
		// was a stray 33 36 inside another record.
		id.Server = r.u16()
		id.Class = ClassOf(r.u32())
		r.skip(1) // unknown
		id.Level = r.u32()
		id.GearScore = r.u32()
		if r.err != nil || id.Server < 1000 || id.Server >= 3000 {
			return Identity{}, ErrNotName
		}
	}
	return id, nil
}

// exactName: 1-12 letters (any script) and digits, at least one letter.
func exactName(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	n, letter := 0, false
	for _, c := range s {
		n++
		switch {
		case unicode.IsLetter(c):
			letter = true
		case unicode.IsNumber(c):
		default:
			return false
		}
	}
	return n >= 1 && n <= 12 && letter
}

func placeholder(s string) bool {
	rest, ok := strings.CutPrefix(s, "$")
	if !ok || len(rest) < 4 {
		return false
	}
	for _, c := range rest {
		if c > unicode.MaxASCII || !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return false
		}
	}
	return true
}

func (r *reader) u16() uint16 {
	if len(r.b) < 2 {
		r.fail()
		return 0
	}
	v := binary.LittleEndian.Uint16(r.b)
	r.b = r.b[2:]
	return v
}
