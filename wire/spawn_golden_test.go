// SPDX-License-Identifier: EUPL-1.2

package wire

import "testing"

// Golden 41 36 spawns from A2Tools-DPS-Meter's tests (c55788c). The pet
// owner's name is scrubbed (same length).

const (
	bakarmaSpawn  = "4136fbaa030c220047412300000200d89f47001ca2c6004c38460000874300c001c0cf24c0cf24640000006400000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000604110181969800ffffffffffffffff8075d52abb030000fbaa03013000d89f47001ca2c6004c3846110284969800ffffffffffffffff8075d52abb030000fbaa030100d89f47001ca2c6004c38461103bc060000ffffffffffffffff8075d52abb030000fbaa030100d89f47001ca2c6004c38461106d6010000ffffffffffffffff8075d52abb030000fbaa030100d89f47001ca2c6004c384601002d00000003012800000028000000437253c900"
	minibossSpawn = "4136b3c0010c22000341230000028b6cccc64de0044600c8d7c50098b24300fe01e0a712e0a712640000006400000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000603110181969800ffffffffffffffff8075d52abb030000b3c00101288b6cccc64de0044600c8d7c5110284969800ffffffffffffffff8075d52abb030000b3c001018b6cccc64de0044600c8d7c51103bc060000ffffffffffffffff8075d52abb030000b3c001018b6cccc64de0044600c8d7c501002d000000030196000000960000000e7253c500"
	// Ritualist Garshim, as a bundle carried it (2026-10-27).
	garshimSpawn = "4136A3E3010420006D0B20000002C5ACD1C6D4E19B460064904649E7034273170180D0A54C80D0A54C640000006400000200025F"
	// A pet: owner 1480 as its parent_key, the owner's name inline.
	petSpawn = "4136b4c6015f000108536565646c696e67ac902c00000200c46ac600043f460030524524e90b437e6301f73ef73efc090000fc09000000000000000000000000000018f0010064000000f04902000100000000000000a08601000000000000e204000101110181969800ffffffffffffffff8075d52abb030000b4c601010200c46ac600043f4600305245070206c8050000ad01000000000f09054775696c6402000000000000000000000000000000000002cd0096000000d000340100002600000000"
)

func TestGoldenSpawns(t *testing.T) {
	cases := []struct {
		name       string
		rec        string
		entity     uint32
		kind       byte
		npc, hp    uint32
		staggerMax uint32
		owner      uint32
		ownerName  string
	}{
		{"boss", bakarmaSpawn, 54651, KindNPC, 2310471, 600_000, 40, 0, ""},
		{"miniboss", minibossSpawn, 0x33 | 0x40<<7 | 1<<14, KindNPC, 0x234103, 300_000, 150, 0, ""},
		{"garshim", garshimSpawn, 29091, 0x04, 2100077, 160_000_000, 0, 0, ""},
		{"pet", petSpawn, 25396, KindSummon, 0x2c90ac, 8055, 0, 1480, "Seedling"},
	}
	for _, c := range cases {
		s, err := ParseSpawn(frameOf(t, c.rec))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if s.Entity != c.entity || s.Kind != c.kind || s.NPC != c.npc || s.MaxHP != c.hp ||
			s.StaggerMax != c.staggerMax || s.Owner != c.owner || s.Name != c.ownerName {
			t.Errorf("%s: got %+v", c.name, s)
		}
	}
}

// Only bosses and minibosses spawn with a stagger gauge (3 of 129 spawns in
// A2Tools' 2026-10-22 capture, no false match).
func TestBossesHaveAStaggerGauge(t *testing.T) {
	for rec, boss := range map[string]bool{bakarmaSpawn: true, minibossSpawn: true, garshimSpawn: false, petSpawn: false} {
		s, err := ParseSpawn(frameOf(t, rec))
		if err != nil || s.Boss() != boss {
			t.Errorf("%s...: boss %v, want %v (%v)", rec[:20], s.Boss(), boss, err)
		}
	}
}

func TestSpawnRejects(t *testing.T) {
	for name, rec := range map[string]string{
		"not a spawn": "4536d74800000000070141",
		"no entity":   "4136ff",
		"entity 0":    "413600",
	} {
		if s, err := ParseSpawn(frameOf(t, rec)); err == nil {
			t.Errorf("%s: got %+v", name, s)
		}
	}
}

// A Wind Spirit (25676) unsummoned.
func TestGoldenDeath(t *testing.T) {
	d, err := ParseDeath(frameOf(t, "4236ccc8010007"))
	if err != nil || d != (Death{Entity: 25676, Flag: DeathGone}) {
		t.Fatalf("got %+v, %v", d, err)
	}
}

// Leaving a party: 13 92 00 00 (open world) or 1D 97 00 00 (dungeon, raid),
// nothing after (A2Tools' captures, 2026-10-13/14).
func TestPartyLeft(t *testing.T) {
	for rec, want := range map[string]bool{
		"13920000":   true,
		"1d970000":   true,
		"1392000001": false, // something after: not a leave
		"1f9203":     false,
		"13920100":   false,
	} {
		if got := PartyLeft(frameOf(t, rec)); got != want {
			t.Errorf("%s: %v, want %v", rec, got, want)
		}
	}
}

// A Sorcerer on Ventus (1305) killing a mob (A2Tools, 2026-10-01; name scrubbed).
func TestGoldenKill(t *testing.T) {
	k, err := ParseKill(frameOf(t, "048decde027228e900ae0b1905054f616b6c790656656e74757301000000"))
	want := Kill{Entity: 0x6c | 0x5e<<7 | 2<<14, Owner: 1454, Server: 1305, Name: "Oakly"}
	if err != nil || k != want {
		t.Fatalf("got %+v, %v", k, err)
	}
	// A name that runs into the next field is not one.
	if k, err := ParseKill(frameOf(t, "048decde027228e900ae0b1905074f616b6c790656656e74757301000000")); err == nil {
		t.Fatalf("garbled: %+v", k)
	}
}
