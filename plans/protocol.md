# Aion 2 wire protocol (notes)

Concepts learnt from github.com/Kuroukihime/aion2-dps-meter (C#/WPF, GPL-3, read at `57974f8`, 2026-10-15).
We borrow the protocol knowledge only and write our own Go. No code copied.

## Transport
- TCP, plaintext. Server → client stream is what matters.
- The server port isn't fixed. Kuroukihime counts heartbeat packets per adapter and port, then locks onto the busiest one. Capture filter is just `tcp`.
- Reassemble TCP by sequence number (drop duplicates, log gaps) before framing.

## Framing
- Each packet: `varint length` then the body. **Packet size = varint value + varint byte length − 4.**
- varint = LEB128 (7 bits per byte, little-endian, high bit means more bytes follow).
- Sync pattern `0E 00 36` = an 11-byte keepalive/heartbeat. Use it to find the first packet boundary mid-stream, then skip it. On a nonsense length (≤0 or >64 KB), drop 1 byte and re-sync.
- `00` bytes between frames are padding: skip them (they used to knock us out of sync until the next keepalive).
- TLS records (`14`-`17`, `03 01`-`03 04`, u16 BE length) turn up on the same port from a second connection. Skip them whole. Read as frames they invent packets out of ciphertext; A2Tools saw kills 20-35% short of boss HP until they did. None of 538,000 game frames starts like one.
- A length over 16 KB that's still arriving is garbage, not a fragment worth waiting for, unless the frame starts with a bundle header (`FF FF` + size ≤ 1 MB). Zone-load bundles run to 17 KB+ and carry your own player record.
- Opcode = 2 bytes after the length varint, read as u16 LE (bytes `04 38` → `0x3804`).

## Compression
- Opcode `FF FF` = a container: `FF FF` + u32 LE uncompressed size + **LZ4 block** payload.
- Can be preceded by a flag byte `0xF?` (not `0xFF`); skip it.
- The decompressed buffer is a series of frames (same varint framing), and frames can be nested containers. `00` bytes between frames are padding.
- Go: `github.com/pierrec/lz4/v4` `UncompressBlock`.

## Opcodes
| Opcode | Name | Use |
|---|---|---|
| `0x3804` | Damage | **DPS core** |
| `0x3805` | DoT damage | DPS |
| `0x3802` | Skill cast | casts, rotation |
| `0x3847` | Skill CD update | |
| `0x382A` / `0x382B` | Buff effect A/B | buff uptime |
| `0x3603` | Server time | timestamps |
| `0x3633` | Player info (self) | name, class |
| `0x3645` | Other players' info | names |
| `0x3620` | Global ↔ session id link | map ids to players |
| `0x3649` | Player stats | |
| `0x9702` | Party info | who's in the static |
| `0x3641` | Mob summon/spawn | target names (boss vs trash) |
| `0x8D00` | Remaining HP | boss HP % |
| `0x8D04` | Kill / summon ownership | names who got the kill → **drop DB** |
| `0x3642` | Death | flag 1 loaded dead, 3 killed, 7 gone |
| `0x364A` | Own stats | about you only: finds you without a zone load |
| `0x3806` | Party scope | about you (+ party in older regions), several a second |

## Damage packet (`0x3804`)
Source of truth: A2Tools (`src-tauri/src/capture/stream_processor.rs`, `parsing_damage_inner`), checked against the game's own Damage Analyzer. Golden records in `wire/damage_golden_test.go`. All varints are at most 5 bytes and never past int32.

After length + opcode:
1. `target` varint
2. `switch` varint. Low nibble = layout: 4-7 has a value, 0 or 2 is a miss/resist with none (stop after the hit type). Bit `0x02` = plotter present, `0x01` = 4 more unknown bytes (layouts 5/7, **no real record checked yet**). `0x20` = extra hits in the tail, `0x40` = absorbed by a shield.
3. unknown varint
4. `actor` varint (reject if equal to target)
5. `skill` u32 LE, valid 1..299,999,999. 7 digits = an NPC's skill (monster hitting a player = damage taken). 3,000,000-3,099,999 are theostone item ids (A2Tools maps them to `id*10+1`).
6. `uid` u8: the same for every record of one cast
7. `hitType` varint: 3 crit, 1 miss, 6 resist
8. plotter (layouts 6/7): `flags` u8, `restoredHP` varint, `angle` u8 (1 back, 2 front, 0 untagged). Flags: 0x01 shield block, 0x02 parry, 0x04 perfect, 0x08 double (HardHit), 0x10 iron wall, 0x20 regeneration, 0x40 perfect block.
9. 4 unknown bytes (layouts 5/7)
10. 8 unknown bytes (looks like u32 + u32 count)
11. `scalar` varint: the actor's damage multiplier in hundredths of a % (mobs 10000, geared players 16000-22000, moves with buffs). A summon carries its owner's, which is how A2Tools ties orphan summons to players. A `0` before it is a pad: skip it.
12. `amount` varint. **It includes the extra hits.**
13. tail, trusted only if it ends cleanly (end of packet, `0n 00` with n 1-7, or `04 38`):
    - layout 4: an unknown varint 1-25 (a spirit's layout-4 record leaves it out, so retry as layout 6)
    - `0x20`: count 1-25, then that many hit values (sum < amount)
    - `0x40`: count 1-8, then that many u32 shield effect ids

Not ported yet: chained records (`01 00` then another record), the `03 00` heal suffix, A2Tools' fallback tail heuristics, and their compact skill `99745942`.

## DoT / heal tick (`0x3805`)
Golden ticks in `wire/dot_golden_test.go`.
1. `target` varint
2. `kind` u8, exact values (not a mask): `02`/`0A` damage, `01`/`09` heal, `0B` HoT; `+04` = damage into a shield (`06`/`0E`). `00` status and `08` buff carry nothing we use.
3. `actor` varint (self-damage is noise, a self-heal is real)
4. unknown varint
5. `effect` u32 LE: skill = effect / 100
6. `amount` varint (1..99,999,999)
7. into a shield: count 1-8, then that many u32 shield effect ids. Other trailing bytes unknown.

**Double counting:** A2Tools only counts damage ticks whose skill is on its curated DoT allowlist (`src/data/dot_skill_ids.json`, 2,641 ids). Everything else in `05 38` duplicates damage already sent as `04 38`. That's the aggregator's job, not the decoder's.

## Identity: you (`33 36`) and other players (`45 36`, `44 36` before June 2026)
Golden records in `wire/names_golden_test.go` (names scrubbed).
1. `entity` varint (1..9,999,999; raid ids run past 99,999)
2. `mask` u32
3. `mask2` u8: bit 0 = a name follows (the other bits vary by patch; never match literal values)
4. `len` u8 (1-48 bytes), then the UTF-8 name: 1-12 letters (any script) or digits, at least one letter. The whole field must be the name.
5. Self only, straight after the name: `server` u16 (1000-2999, or it wasn't a real self record), `class` u32, unknown u8, `level` u32, `gearScore` u32. Combat power is near the end of the record (A2Tools `self_combat_power`, not ported).
- A new character in the tutorial is `$` + random letters until named: still you, no name.
- Entity ids are reissued on zone changes, so the latest name for an id wins.
- Class ids come in fours: 5-8 Gladiator, 9-12 Templar, 13-16 Ranger, 17-20 Assassin, 21-24 Elementalist, 25-28 Sorcerer, 29-32 Cleric, 33-36 Chanter.

## Party roster (`02 97`)
**Members carry account ids, not entity ids: join them to identities by name.**
1. `partyKey` u32, `titleLen` u8 (1-40) + UTF-8 title (the party name/LFG note), `size` u8 (1-12)
2. `dungeon` u32 (instance + tier, e.g. 600091/2/3 for one den's three tiers), 2 pad, leader account u64, 3 pad
3. `count` varint (1-12), then per member:
   - `mask` u8, `slot` u8, `account` u64 (home world in its top u16), `nameLen` u8 + name
   - `class` u32, `level` u32 (1-200), `gearScore` u32
   - a variable gap, then `world` u16 (the home world again, or for a visitor the world they're on), u16, tag u8 (1-4), `combatPower` u64
   - The width varies, so find the world u16 within 10 bytes. Failing that (a visitor), find any world 1000-2999 with a tag 1-4 and a power 1..100M after it.
   - The next member is re-found by header: the next slot, a world in the account's top u16, a valid name, within 32 bytes. A vacant slot is `00 <slot>` + zero account + empty name; past one, look up to 48 bytes per slot for a later member.
- **Leaving:** a frame that is exactly `13 92 00 00` (open-world party) or `1D 97 00 00` (dungeon/raid), nothing after. `1F 92 03` came when the last other member left; `1B 97 <key>` followed leaving a raid.
- **Other parties' rosters arrive too** (raids, nearby groups). Once a roster with you in it has given your party's key, a complete roster with another key and without you is someone else's: ignore it.

## Spawn (`41 36`, `40 36` before June 2026)
NPCs, summons/pets and skill effects, never real players. Golden spawns in `wire/spawn_golden_test.go`.
1. `entity` varint
2. `mask` u32 (u16 in older builds). Low byte = kind: `0C`/`0D` NPC, `5F` summon/pet, `1C` skill effect (parented to the skill's **target**; its name is the caster's), `1F`/`1D`/`5D` spirits and lingering ground spells.
3. The name-gate byte: at +4 (u32 mask) or +2 (u16 mask), try both. Bit 0 set = `len` u8 + a name: a summon's owner, an effect's caster.
4. The rest is three variable subtrees, so fields are found by landmarks:
   - **NPC code:** the 3 bytes before `00 (00|40) 02`, within 60 bytes of the mask.
   - **HP:** within 64 bytes after that, `01 <hp varint> <maxHP varint>` with max ≥ hp > 0.
   - **Stagger gauge:** the record ends `03 01 <max u32> <current u32> <4 bytes> 00`. **Only bosses and minibosses have one** (3 of 129 spawns in A2Tools' 2026-10-22 capture, no false match). That's our boss flag, no NPC table needed.
   - **Owner** (kind `5F`, mask bit `0x10`): after a `06` byte, `<owner u32> <legion u32> <u16 0> <world u16> <len u8> <legion name>`.
- Not ported: A2Tools' spirit-owner fallbacks (buff block anchor `80 75 D5 2A BB 03 00 00`, effect caster after `07 02 06`), `NpcLookup` boss/dummy tables, the old id remap for ids over 1M.

## Death (`42 36`)
`entity` varint, varint (always 0), `flag` varint: 1 = already dead when the zone loaded, 3 = killed, 7 = gone from the world (only trust 7 under `42 36`). The game doesn't count a spirit's ticks after it's gone; a direct hit means the id is back.

## Kill / ownership (`04 8D`)
`entity` varint, 4 unknown bytes, `owner` varint, `server` u16, `len` u8 + name, `len` u8 + server name, then more unknown. Golden record in `wire/spawn_golden_test.go`.
- After a kill: `owner` got the kill of `entity`. It names them, so your first kill names you (no zone load needed). Identity records still win over it.
- About a summon: `owner` owns `entity`.
- (Kuroukihime called `0x8D04` "death"; deaths are `42 36`.) Kills plus loot are what the drop DB needs: loot is still unknown.

## From A2Tools' plugin decoders
Plugins (github.com/taengu/A2Tools-Plugins, GPL-3, read at `24aaf21`, cloned in `ref/`) declare the frames they want in `manifest.json`: opcode bytes as on the wire, then fields in order (numbers only, `skip:N` = unknown bytes). `self=x` means the meter only passes it on when field `x` is your own character. **Unverified by us**: plugin authors' readings, not golden-tested. Promote a row to a decoder only with a real record.

| Wire bytes | Plugin / event | Fields |
|---|---|---|
| `00 90` | pet-souls/pet-list | skip:4, levels[varint](pet:u32, skip:4, level:u32), points[varint](pet:u32, points:u32) |
| `01 38` | cooldowns/skill-used (immediate=True) | skip:3, skill:u32 |
| `02 38` | boss-telegraph/boss-skill (immediate=True) | caster:varint, skip:1, skill:u32 |
| `03 38` | boss-telegraph/boss-wave (immediate=True) | caster:varint, skip:1, source:varint, skill:u32 |
| `04 38` | mantra-watch/my-hit (self=actor immediate=True) | target:varint, flags:varint, x:varint, actor:varint, skill:u32 |
| `09 38` | boss-telegraph/boss-cast (immediate=True) | caster:varint, skip:1, skill:u32 |
| `0D 90` | pet-souls/soul-gained (immediate=True) | souls[u8](pet:u32, amount:u32) |
| `14 90` | pet-souls/level-up (immediate=True) | skip:2, pet:u32, level:u32 |
| `1B 92` | heal-alert/party-hp (immediate=True) | member:varint, hp:varint, max:varint |
| `21 36` | mantra-watch/map-load (immediate=True) | (match only) |
| `21 36` | pet-souls/map-load | skip:4, map:u32, skip:8, x:u32, y:u32, z:u32 |
| `23 36` | mantra-watch/landed (immediate=True) | (match only) |
| `23 36` | pet-souls/teleport | flag:u8, skip:5, x:u32, y:u32, z:u32 |
| `2A 38` | boss-telegraph/boss-buff-added (immediate=True) | entity:varint, skip:1, flags:u8, instance:varint, abnormal:u32, length:u64 |
| `2A 38` | cooldowns/buff-added (self=entity immediate=True) | entity:varint, skip:1, flags:u8, instance:varint, abnormal:u32, length:u64, end:u64, caster:varint, level:u8, skill:u32 |
| `2A 38` | pet-souls/own-position (self=source) | source:varint, one:u8, kind:u8, seq:varint, skip:20, target:varint, p:u8, q:u8, x:u32, y:u32, z:u32 |
| `2B 38` | cooldowns/buff-changed (self=entity immediate=True) | entity:varint, flags:u8, instance:varint, abnormal:u32, length:u64, end:u64 |
| `2B 38` | mantra-watch/mantra-tick (self=caster immediate=True) | on:varint, kind:u8, seq:varint, effect:u32, skip:16, caster:varint, p:u8, skill:u32, q:u8 |
| `2B 38` | pet-souls/mantra-tick (self=source) | source:varint, kind:u8, seq:varint, skip:20, target:varint, p:u8, skip:4, q:u8, x:u32, y:u32, z:u32 |
| `2C 38` | boss-telegraph/boss-buff-removed (immediate=True), cooldowns/buff-removed (self=entity immediate=True) | entity:varint, count:u8, kind:u8, instance:varint |
| `41 36` | pet-souls/npc-spawn | entity:varint, skip:3, npc:u32, skip:2, x:u32, y:u32, z:u32 |
| `42 36` | pet-souls/npc-gone | entity:varint |
| `47 38` | cooldowns/cooldowns (immediate=True) | skills[u8](group:u32, ms:varint) |

Handy for us: `1B 92` party HP (a heal alert for the cleric), `41 36` / `42 36` spawn and despawn with position, `21 36` map load (zone changes reissue entity ids), `09 38` / `02 38` boss casts and skills (mechanic callouts).

## Not covered by the reference
- **Loot / item drops: no opcode known.** That one is ours to find. Capture with a kill and a loot pickup in it, then diff.
