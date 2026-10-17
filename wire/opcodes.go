// Package wire decodes Aion 2's server→client TCP stream into game packets.
// Protocol notes: plans/protocol.md.
package wire

// Opcodes are the 2 bytes after the length varint, read little-endian
// (wire bytes 04 38 → 0x3804).
const (
	OpServerTime   uint16 = 0x3603
	OpDamage       uint16 = 0x3804
	OpDotDamage    uint16 = 0x3805
	OpSkillCast    uint16 = 0x3802
	OpSkillCD      uint16 = 0x3847
	OpBuffA        uint16 = 0x382A
	OpBuffB        uint16 = 0x382B
	OpPlayerInfo   uint16 = 0x3633
	OpOtherPlayers uint16 = 0x3645
	OpIDLink       uint16 = 0x3620
	OpPlayerStats  uint16 = 0x3649
	OpPartyInfo    uint16 = 0x9702
	OpMobSpawn     uint16 = 0x3641
	OpRemainHP     uint16 = 0x8D00
	OpDeath        uint16 = 0x8D04
	OpContainer    uint16 = 0xFFFF // LZ4 block of more frames
)

// OpName is for dumps and logs; unknown opcodes come back as "".
func OpName(op uint16) string {
	return opNames[op]
}

var opNames = map[uint16]string{
	OpServerTime: "ServerTime", OpDamage: "Damage", OpDotDamage: "DotDamage",
	OpSkillCast: "SkillCast", OpSkillCD: "SkillCD", OpBuffA: "BuffA", OpBuffB: "BuffB",
	OpPlayerInfo: "PlayerInfo", OpOtherPlayers: "OtherPlayers", OpIDLink: "IDLink",
	OpPlayerStats: "PlayerStats", OpPartyInfo: "PartyInfo", OpMobSpawn: "MobSpawn",
	OpRemainHP: "RemainHP", OpDeath: "Death", OpContainer: "Container",
}
