# DPS meter

## Decided
- **Source: passive packet capture via Npcap.** We read copies of the game's traffic and never touch the client: no memory reads, no hooks, no injection, no packets sent. That's the safety line. If it needs more than sniffing, we don't do it.
- **DPS first.** Loot, drops and the rest come later, but we note every data point we spot along the way (see below).
- Built on CoreGO (`~/Code/core`).

## Open questions
- [x] Traffic is plaintext TCP (some frames LZ4-compressed). See `protocol.md`.
- [ ] Capture from Go on Windows: gopacket/pcap (cgo + Npcap SDK) vs a pure-Go Npcap binding. The game runs on Windows, so we need a Windows box or a `.pcap` someone captures for us to develop against.
- [ ] What we show: per player, per fight, boss vs trash, skill breakdown.
- [ ] Maybe: post a fight summary to the static's Discord via the Groot bot (a `g!` command, not a second bot).

## Plan
1. Get a `.pcap` of a short fight from a static member (Npcap + Wireshark, filtered to the game's ports).
2. TDD the decoder against that capture: packet → damage event.
3. Aggregate: events → per-player DPS per fight (pure and fully tested).
4. Thin live layer: Npcap capture → decoder → aggregator → CoreGO window.

## Data points spotted (for later)
Running list of what the packets give us, even if we don't use it yet. The aim is a **drop database**, so a session can answer "where's my best luck for Expert Refinement Stones?". That needs loot events with item, source mob or boss, zone and time.

| Data point | Packet / opcode | Fields | Seen in | Notes |
|---|---|---|---|---|
| Kill | `0x8D04` entity death | entity id | Kuroukihime | anchor for drops (what died, where, when) |
| Mob spawn | `0x3641` | mob id → name | Kuroukihime | boss vs trash, drop source |
| Party | `0x9702` | members | Kuroukihime | tag drops per static run |
| Loot | **unknown** | item, qty | — | ours to find: capture a kill + pickup |
