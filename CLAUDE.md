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
