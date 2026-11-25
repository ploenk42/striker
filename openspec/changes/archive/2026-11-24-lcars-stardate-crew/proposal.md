# Proposal

## Why

- The LCARS frame shows the time in three places and three ways: a made-up stardate in the top right (year
  minus 1920, then the fraction of the year, UTC, refreshed every 15 s), `SCAN 14:05` in the STRIKER FLEET
  OPERATIONS heading, and `CTRL SHIFT P · 14:05` in the bottom right. None reads as an exact clock that looks
  like a stardate, and the bottom one is redundant.
- On the STRIKER FLEET OPERATIONS board a column's header reads `REMOTE1 · NEEDS YOU` over the repo name: the
  machine, not who asks. When a repo is named like its machine that is even misleading. The crew
  names (CAPT. PATA, CMDR. WERF) the sidebar already shows appear nowhere on the board.
- Striker's prompt under the board cuts off a prompt that needs more than one line: the rest of the text is not
  visible.

## What Changes

- One stardate format for every time on the frame: local date and time as one number, `YYYYMMDD.hhmmss`
  (2026-11-24 15:30:42 → `20261124.153042`).
  - Top right: `STARDATE 20261124.153042` is a clock, refreshed every second.
  - STRIKER FLEET OPERATIONS heading: `SCAN 20261124.153042`.
  - Bottom right: the clock goes; the cell stays as the palette button `CTRL SHIFT P`.
- Board columns name who asks: the header's small line is the asker's crew name and what it works on
  (`CAPT. PATA · NEEDS YOU`, or `CMDR. WERF · OMEGA · NEEDS YOU` when a coordinator asks), over the repo
  name. Lead and coordinator lines and the NEXT MOVE list use the crew names too; coordinator lines name their
  worktree.
- Striker's prompt shows a prompt of several lines in full.

## Impact

`lcars/main.js`, `lcars/striker.js`, `lcars/board.js`, a new pure helper `lcars/stardate.js`,
`lcars/bridge.check.mjs`, `docs/lcars-colors.md`; for the prompt possibly `layouts/lcars.json` and
`lcars/prompt.js`.
