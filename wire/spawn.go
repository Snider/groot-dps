// SPDX-License-Identifier: EUPL-1.2

package wire

import (
	"encoding/binary"
	"errors"
	"unicode/utf8"
)

// OpMobSpawnOld is 0x3641 before the June 2026 opcode shift.
const OpMobSpawnOld uint16 = 0x3640

// Spawn kinds: the low byte of a spawn's mask.
const (
	KindNPC    = 0x0C // also 0x0D
	KindEffect = 0x1C // a short-lived skill effect, parented to the skill's target
	KindSummon = 0x5F // a summon or pet
)

var ErrNotSpawn = errors.New("spawn: not a spawn record")

// Spawn is a 41 36 record: an NPC, a summon/pet or a skill effect. Never a
// real player (they get 45 36).
type Spawn struct {
	Entity uint32
	Mask   uint32
	Kind   byte
	Name   string // inline name: a summon's owner, an effect's caster
	NPC    uint32 // NPC / model code, 0 if not found
	HP     uint32
	MaxHP  uint32
	// The stagger gauge a boss or miniboss spawns with (0 = none).
	StaggerMax, Stagger uint32
	Owner               uint32 // a summon's owner, from its parent_key
}

// Boss: only bosses and minibosses spawn with a stagger gauge.
func (s Spawn) Boss() bool { return s.StaggerMax > 0 }

// ParseSpawn reads a spawn. Past the mask the record is three variable
// subtrees, so like A2Tools we find each field by its landmarks rather than
// walking them.
func ParseSpawn(p []byte) (Spawn, error) {
	op, ok := Opcode(p)
	if !ok || op != OpMobSpawn && op != OpMobSpawnOld {
		return Spawn{}, ErrNotSpawn
	}
	_, n := uvarint(p)
	d := p[n+2:]
	r := reader{b: d}
	var s Spawn
	s.Entity = r.varint32()
	if r.err != nil || s.Entity == 0 || r.remaining() < 1 {
		return Spawn{}, ErrNotSpawn
	}
	off := len(d) - r.remaining()
	var m [4]byte
	copy(m[:], d[off:])
	s.Mask = binary.LittleEndian.Uint32(m[:])
	s.Kind = d[off]

	// The mask grew from u16 to u32, moving the byte that gates the name: try
	// both. A wrong guess almost never decodes as a whole name.
	cursor := off + 5
	for _, at := range []int{off + 4, off + 2} {
		if name, end, ok := gatedName(d, at); ok {
			s.Name, cursor = name, end
			break
		}
	}

	if code, at, ok := npcCode(d, off); ok {
		s.NPC = code
		s.HP, s.MaxHP = spawnHP(d, at+3)
	}
	if op == OpMobSpawn {
		s.StaggerMax, s.Stagger = staggerGauge(d)
	}
	if s.Kind == KindSummon && s.Mask&0x10 != 0 {
		s.Owner = parentKey(d, cursor, s.Entity)
	}
	return s, nil
}

func gatedName(d []byte, at int) (string, int, bool) {
	if at+1 >= len(d) || d[at]&0x01 == 0 {
		return "", 0, false
	}
	k := int(d[at+1])
	start := at + 2
	if k < 1 || k > 48 || start+k > len(d) || !exactName(string(d[start:start+k])) {
		return "", 0, false
	}
	return string(d[start : start+k]), start + k, true
}

// npcCode: the 3-byte code sits right before `00 (00|40) 02`, within 60 bytes.
func npcCode(d []byte, from int) (uint32, int, bool) {
	for i := from; i < min(len(d)-2, from+60); i++ {
		if d[i] == 0x00 && (d[i+1] == 0x40 || d[i+1] == 0x00) && d[i+2] == 0x02 {
			if i < from+3 {
				return 0, 0, false
			}
			return uint32(d[i-3]) | uint32(d[i-2])<<8 | uint32(d[i-1])<<16, i, true
		}
	}
	return 0, 0, false
}

// spawnHP: within 64 bytes after the code, `01 <hp varint> <max varint>` with max ≥ hp > 0.
func spawnHP(d []byte, from int) (hp, maxHP uint32) {
	for i := from; i < min(len(d)-2, from+64); i++ {
		if d[i] != 0x01 {
			continue
		}
		r := reader{b: d[i+1:]}
		cur, top := r.varint32(), r.varint32()
		if r.err == nil && cur > 0 && top >= cur {
			return cur, top
		}
	}
	return 0, 0
}

// staggerGauge: a boss spawn ends `03 01 <max u32> <current u32> <4 bytes> 00`.
func staggerGauge(d []byte) (maxG, cur uint32) {
	if len(d) < 15 {
		return 0, 0
	}
	t := d[len(d)-15:]
	if t[0] != 0x03 || t[1] != 0x01 || t[14] != 0x00 {
		return 0, 0
	}
	maxG, cur = binary.LittleEndian.Uint32(t[2:]), binary.LittleEndian.Uint32(t[6:])
	if cur == 0 || cur > maxG || maxG > 100_000 {
		return 0, 0
	}
	return maxG, cur
}

// parentKey finds the owner block after a 06 byte: <owner u32> <legion u32>
// <u16 0> <world u16> <legion name len> <utf8>.
func parentKey(d []byte, from int, self uint32) uint32 {
	for i := max(from, 1); i+13 <= len(d); i++ {
		if d[i-1] != 0x06 {
			continue
		}
		owner := binary.LittleEndian.Uint32(d[i:])
		if owner == 0 || owner > 9_999_999 || owner == self || binary.LittleEndian.Uint16(d[i+8:]) != 0 {
			continue
		}
		if w := binary.LittleEndian.Uint16(d[i+10:]); w == 0 || w > 9_999 {
			continue
		}
		k := int(d[i+12])
		if k > 40 || i+13+k > len(d) || !utf8.Valid(d[i+13:i+13+k]) {
			continue
		}
		return owner
	}
	return 0
}

// Death flags.
const (
	DeathLoaded = 1 // the entity was already dead when the zone loaded
	DeathKilled = 3
	DeathGone   = 7 // left the world (a spirit unsummoned, a mob despawned)
)

type Death struct {
	Entity uint32
	Flag   uint32
}

var ErrNotDeath = errors.New("death: not a death record")

// ParseDeath reads <entity varint> <varint, always 0> <flag varint>.
func ParseDeath(p []byte) (Death, error) {
	if op, ok := Opcode(p); !ok || op != OpDeath {
		return Death{}, ErrNotDeath
	}
	_, n := uvarint(p)
	r := reader{b: p[n+2:]}
	var d Death
	d.Entity = r.varint32()
	r.varint32()
	d.Flag = r.varint32()
	if r.err != nil || d.Entity == 0 {
		return Death{}, ErrNotDeath
	}
	return d, nil
}

// Kill is a 04 8D record: who got a kill (or owns a summon). It names them,
// so your first kill names you without a zone load.
type Kill struct {
	Entity uint32 // what died (or the summon)
	Owner  uint32 // who killed it (or owns it)
	Server uint16
	Name   string
}

var ErrNotKill = errors.New("kill: not a kill record")

// ParseKill reads <entity varint> <4 bytes> <owner varint> <server u16>
// <len u8> <name> [<len u8> <server name> ...].
func ParseKill(p []byte) (Kill, error) {
	if op, ok := Opcode(p); !ok || op != OpKill {
		return Kill{}, ErrNotKill
	}
	_, n := uvarint(p)
	r := reader{b: p[n+2:]}
	var k Kill
	k.Entity = r.varint32()
	r.skip(4)
	k.Owner = r.varint32()
	k.Server = r.u16()
	if r.err != nil || k.Entity < 100 || k.Owner < 100 || k.Entity == k.Owner {
		return Kill{}, ErrNotKill
	}
	if name, _, ok := lenName(r.b); ok {
		k.Name = name
	}
	if k.Name == "" || k.Server < 1000 || k.Server > 9_999 {
		return Kill{}, ErrNotKill
	}
	return k, nil
}

// lenName: <len u8> <utf8 name>, the whole field a name.
func lenName(b []byte) (string, int, bool) {
	if len(b) < 1 {
		return "", 0, false
	}
	k := int(b[0])
	if k < 1 || k > 48 || 1+k > len(b) || !exactName(string(b[1:1+k])) {
		return "", 0, false
	}
	return string(b[1 : 1+k]), 1 + k, true
}
