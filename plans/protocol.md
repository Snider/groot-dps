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
| `0x8D04` | Entity death | fight end, **kill events → drop DB** |

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

## Not covered by the reference
- **Loot / item drops: no opcode known.** That one is ours to find. Capture with a kill and a loot pickup in it, then diff.
