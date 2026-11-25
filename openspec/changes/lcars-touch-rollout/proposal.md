# Proposal

## Why

The Bridge now has touch-sized controls (44 px) and readable text (14 px minimum), but the rest of the LCARS
layout does not: the sidebar (spaces, FLEET, CREW), the top and the bottom frame in `lcars/main.js` still use
10-13 px text and 26-40 px rows. The person wants everything usable by finger. The repo overview also left
rollout steps on remote1 that only the person can do or approve.

## What Changes

- Sidebar, top and bottom frame: text at least 14 px, every clickable at least 44 px.
- Bridge cards: row height 56 → 60 px so a working card's step bar is not clipped.
- Rollout on remote1: passwordless sudo for `striker start-coordinator`, the standalone `openspec` CLI, a first
  live coordinator run, existing lead tabs renamed to `claude`.

## Impact

`lcars/main.js`, `lcars/bridge.js`, `layouts/lcars.json`, `docs/lcars-colors.md`; remote1 setup (no code).
Design and plan come from brainstorming before work starts.
