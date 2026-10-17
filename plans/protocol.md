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
- Sync pattern `0E 00 36` = an 11-byte keepalive/heartbeat. Use it to find the first packet boundary mid-stream, then skip it. On a nonsense length (≤0 or >40 KB), drop 1 byte and re-sync.
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
After length + opcode:
1. `targetId` varint
2. `switch` varint: low nibble must be 4–7, else invalid
3. unknown varint
4. `actorId` varint (reject if equal to target)
5. `skillCode` u32 LE
6. unknown u8
7. `damageType` varint (3 = crit)
8. special block:
   - switch == 4: no flag bytes, needs ≥8 remaining
   - else: `flags` u8 (1 back, 2 parry, 4 perfect, 8 double), unknown u8, `direction` u8 (1 back, 2 front); needs ≥12 remaining
   - then u32 unknown + 4 unknown bytes
9. unknown varint
10. `damage` varint (use 64-bit)

The class comes from the skill code via a skill table. Healing skills and theostones (procs) are handled separately. We'll need that skill table data too.

## Not covered by the reference
- **Loot / item drops: no opcode known.** That one is ours to find. Capture with a kill and a loot pickup in it, then diff.
