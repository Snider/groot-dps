# Groot DPS 🌳⚔️

A DPS meter for Aion 2, made for a five-player static by one of its players.

## Why another meter
Because a meter sees everything you and everyone around you do, and you should be able to trust it.

- **Nothing leaves your PC.** No uploads, no accounts, no telemetry, no "anonymous usage stats", no update pings. The meter makes no network connections of its own. It only listens.
- **Nothing is collected or aggregated**, not even anonymously. No leaderboards built from your fights, no class statistics and no logs kept for anyone. Your numbers are yours, on your screen.
- **Open source, and small enough to read.** EUPL-1.2. You can check every line and build it yourself instead of trusting a download. Releases will come with checksums so you can check what you run.
- **Passive only.** It reads a copy of the game's network traffic (Npcap) and never touches the game client: no memory reads, no hooks, no injection, nothing sent to the game.

## No teleport needed
Some meters only learn which character is yours when you load a zone, so you teleport around the open world before your row shows up. This one also listens for things the server only sends about you:

- **Your own stats** (`4A 36`): use a potion, scroll, buff or skill that changes your stats, or swap gear, and three of these records in a row point to you.
- **Your own updates** (`06 38`): sent about you several times a second while you play. Within a few seconds the meter picks the one player they name far more than anyone else.

You show up marked as you straight away. Your name fills in at the next zone load, when the game sends your full character record. (These rules come from A2Tools' captures. We'll verify them on our own once the live meter runs.)

## Status
The decoders and the maths are done and tested against real game records: damage, DoT/heal ticks, names, party, spawns (bosses spot themselves by their stagger gauge, pets find their owners) and deaths. Still to come: the live Windows window. Until then `grootdump` runs a capture through the meter.

## Layout
- `wire/`: game stream → packets (framing, LZ4 bundles) and decoders (damage, ticks, identities, party roster)
- `tcp/`: puts TCP segments back in order
- `meter/`: hits → fights → DPS rows, names, your party, which entity is you (pure, no IO)
- `cmd/grootdump`: reads a Wireshark capture and shows what the game sent
- `plans/`: the plan, protocol notes, data points spotted, capture mode

## Dump a capture
```
go run ./cmd/grootdump testdata/fight.pcapng          # opcode histogram per flow
go run ./cmd/grootdump -damage testdata/fight.pcapng  # decoded hits
go run ./cmd/grootdump -list testdata/fight.pcapng    # every packet in order (hunting loot)
go run ./cmd/grootdump -op 0x3804 -hex testdata/fight.pcapng
go run ./cmd/grootdump -meter -dots ref/A2Tools-DPS-Meter/src/data/dot_skill_ids.json testdata/fight.pcapng  # DPS per fight
```
Captures go in `testdata/` and are git-ignored, because they contain character names. Golden test records have their names swapped for made-up ones of the same length.

## Thanks
Protocol knowledge comes from [A2Tools-DPS-Meter](https://github.com/taengu/A2Tools-DPS-Meter) (GPL-3), whose real-packet test records are our golden tests, and before that [Kuroukihime/aion2-dps-meter](https://github.com/Kuroukihime/aion2-dps-meter) (GPL-3). The facts are borrowed and the code is our own.

## Licence
[EUPL-1.2](LICENSE). It's copyleft: if you change it and share it, you share the source too. It's GPL-3 compatible.
