# Groot DPS 🌳⚔️

A DPS meter for Aion 2 that only reads a copy of the network traffic (Npcap). It never touches the game client: no memory reads, no hooks, no injection, nothing sent.

## Layout
- `wire/`: game stream → packets (varint framing, LZ4 containers) and decoders (damage so far)
- `tcp/`: puts TCP segments back in order
- `cmd/grootdump`: reads a Wireshark capture and shows what the game sent
- `plans/`: the plan, protocol notes and data points spotted

## Dump a capture
```
go run ./cmd/grootdump testdata/fight.pcapng          # opcode histogram per flow
go run ./cmd/grootdump -damage testdata/fight.pcapng  # decoded hits
go run ./cmd/grootdump -list testdata/fight.pcapng    # every packet in order (hunting loot)
go run ./cmd/grootdump -op 0x3804 -hex testdata/fight.pcapng
```
Captures go in `testdata/` and are git-ignored, because they contain character names.

Protocol knowledge comes from [A2Tools-DPS-Meter](https://github.com/taengu/A2Tools-DPS-Meter) (GPL-3), whose real-packet test records are our golden tests, and before that [Kuroukihime/aion2-dps-meter](https://github.com/Kuroukihime/aion2-dps-meter) (GPL-3). The facts are borrowed and the code is our own.

## Licence
[EUPL-1.2](LICENSE). It's copyleft: if you change it and share it, you share the source too. It's GPL-3 compatible.
