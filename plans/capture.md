# Capture mode (pencilled in)

**Idea:** the meter can record a fight to a file that's safe to hand to a friend (or to us for golden tests). It keeps only what the meter needs about you and your party, with names scrubbed. Same passive-only line as the meter: we just write down what we already read.

## Why
- **Golden tests from our own runs.** Today our real records all come from A2Tools' tests. With this, every static run can give us new ones: boss mechanics, loot and new patches.
- **Friends can share captures** without leaking anyone's name or unrelated traffic. Raw `.pcapng` holds everything; this holds only the game frames we chose.
- **Drop DB:** loot + kill records per run, tagged by zone and time.

## Shape (v1)
- Record decoded game frames, not raw packets: after TCP reassembly and framing, server → client only. No IPs, no other connections, no TLS.
- **Opcode allowlist**, like A2Tools' Evidence Slice (`src-tauri/src/capture/evidence_slice.rs`): damage `04 38`, ticks `05 38`, spawns `41 36`/`42 36`, death, HP `8D 00`, buffs `2A`/`2B`/`2C 38`, party `02 97`, map load `21 36`, and loot once we find it. Everything else is dropped.
- **Name blinding:** every name string is swapped for a stable token (`P1`, `P2`, ...), plus a second pass that refuses to write anything still name-shaped. A2Tools' server checks the same way, so read their blinder before writing ours.
- **Format:** A2Tools' text format, `TIMESTAMP|STREAMKEY|HEX`, one segment per line. Then `a2t-derive` (their tool, run locally) can check our meter against theirs on the same file. That's a free conformance test.
- Lives in `capture/` (pure: frames in, lines out, TDD) plus a `-record` flag on the live meter.

## v2: party only
- Once identity and party decode (`45 36`, `33 36`, `02 97`, `06 38` party scope), keep a record only if one of its entity ids is you, a party member, their summons, or something one of you hit or got hit by. Strangers in the open world vanish from the file.
- Until then v1 keeps all combat records in the allowlist, still with names blinded.

## Open
- [ ] Opt-in only, off by default, with a visible "recording" indicator.
- [ ] Where files go, and auto-delete after N days.
- [ ] Check A2Tools' slice allowlist and blinder for anything we'd miss.
