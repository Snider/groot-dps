# Groot DPS 🌳⚔️

**Groot mode.** Sibling of `../groot` (the static's Discord bot). This is a hobby: Snider is gaming and recharging, not working.

- None of the Lethean rules apply: no house style, no core.E, no gates, no fleets, no 200-year anything.
- Keep it light, fun and useful. If it works and the static likes it, it's good.
- Keep quota use low (Pro plan): small steps, no agents, no big explorations.
- Git is here to help, not as a ritual. No remotes needed. Commit as we go.

## How we work
- Snider is an architect/dev on holiday: Claude is the programmer and drives. Be direct, skip the basics, no hand-holding, no essays.
- British, dry, sarcastic. If something sounds like "I know nothing", it's a joke, not a request for a tutorial.
- Casual and fun tone, gamer chat welcome (they'll tell you about PvP fights mid-session; enjoy it).
- TDD for anything pure (parsing, maths, aggregation). Real-game/IO bits get a thin layer on top.
- Report straight: what broke, why, what changed, what they need to do. Short.
- How we work here is whatever we work out. Write it down in this file as we go.

## The game and the crew
- Aion 2 (NCSoft), EU region, server Siel (id 1301, streamer server).
- Groot = Snider's Templar. 5-man static plus a small Discord ("Aion Static").
- The cleric: lovely, forgetful. Groot the bot exists partly for them.
- Things the static cares about: hourly Shugo runs (3-min entry, 3 keys a day, reset 08:00 Berlin), world boss timers, gear checks (`g!whoneeds vakron`), patch notes (patch day Wednesday).

## What already exists next door (`../groot`)
- `bot/`: Node 26 ESM + discord.js bot, runs as LaunchAgent `uk.host.groot` on the Mac Studio. Data in `bot/groot.duckdb` (characters, gear, static roster with Discord links, music catalog).
- `aion/`: `groot-aion`, Go CLI over github.com/nuriland/aion2-api (character search, profile/combat power, equipment, patch notes). Flags `-region eu -server 1301` before the subcommand.
- DuckDB: one process per file. Don't open `groot.duckdb` while the bot runs; copy it or ask the bot. Open each DB instance once (repeated `fromCache` hung the bot).
- Bot token is `GROOT_TOKEN`, never `DISCORD_TOKEN` (that's a Lethean bot in the shell). Keep Groot and Lethean apart.
- If the DPS meter should post to Discord, it plugs into the bot (new `g!` command / `/aion` sub) rather than being a second bot.

## This project
- Goal: a DPS meter for Aion 2.
- Built on Snider's own Go native app framework (CoreGO at `~/Code/core`, already attached to the session; no `/add-dir` needed). No DPS code exists yet.
- Why: the static wants a meter, and Snider building it means friends can trust what they run. So: open source, readable, no telemetry, nothing sneaky. Trust is the feature.
- Ideas and next steps go in `plans/`.

## How we build it (learnt as we go)
- **Safety line:** passive capture only. Read a copy of the traffic and never touch the client or send anything. If a feature needs more, we don't build it.
- **Trust is the feature:** no telemetry, no network calls of our own (Discord goes through the Groot bot), few dependencies, pure Go where we can. Friends should be able to read it and build it themselves.
- **Every packet is suspect.** Bounds-check everything, cap sizes and nesting, never panic. Each decoder gets a fuzz test (`wire/fuzz_test.go`). The 2026-10-17 decompression-bomb fix is why.
- **Synthetic tests prove our reading, not the game.** Tests built from our own packets only show we match our notes. Every decoder also needs a **golden test**: a real packet from a capture, saved as hex, with the expected decode.
- **Captures are private.** They contain names. `.pcap`/`.pcapng` stay out of git. Golden packets get trimmed to the one packet and names scrubbed before they're committed.
- **Unknown fields stay named `unknown`.** They're documented in `plans/protocol.md`. When we work one out, update the notes and the tests together. Every new data point goes in the `plans/dps-meter.md` table.
- **Source of truth: A2Tools** (github.com/taengu/A2Tools-DPS-Meter, Rust/Tauri, GPL-3), cloned into `ref/` (git-ignored; `git -C ref/A2Tools-DPS-Meter pull` to update). It's mature and its tests use real records checked against the game's Damage Analyzer. TDD loop: lift a real record and its expected values from their tests into our golden tests, then write our own Go to pass them. Port facts and test vectors, not code. Note the commit we read at. Credit in the README. Kuroukihime's meter was the first reference; where they disagree, A2Tools wins.
- **Before committing:** `./check.sh` (gofmt, vet, tests, Windows build). `./check.sh fuzz` after touching a decoder.
- **Licence: EUPL-1.2** (`LICENSE`), Lethean style: every Go file starts with `// SPDX-License-Identifier: EUPL-1.2` (check.sh enforces it). Public repo: github.com/Snider/groot-dps.
- **Open:** releases need checksums so the static can verify what they run.

