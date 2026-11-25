# Proposal

## Why

A Herdr space is first of all a repo, not a worktree. Opening a space should give a short briefing: where the work stopped, how it fits the repo's vision, which specs, changes and tasks exist, and what waits on the person. Today the Bridge only shows one worktree's plan and says `NO PLAN FOR BRANCH main` on a main checkout; progress of finished work is recorded nowhere durable.

## What Changes

- New OVERVIEW mode in the LCARS Bridge, first pseudo-tab, default on a repo's main checkout: Briefing, Context (vision), Inventory (specs and changes with task progress, INTERRUPTED work), Open items (lead `needs_you`, REGISTER).
- New Striker endpoint `GET /overview` that assembles this from the repo.
- Progress is read from OpenSpec itself: active vs archived changes, `tasks.md` checkboxes.
- One coordinator per change, reporting to Striker (`striker start-coordinator <repo> <change>`).
- Fixes: Bridge maps a space to its repo via `fleet.json`; crew entries read `SPACE · STATUS`; a lead's Claude tab is named `claude`.

## Capabilities

### New Capabilities
- `bridge/repo-overview`: the OVERVIEW mode — what a space shows for its repo and where each part comes from.
- `striker/overview-endpoint`: the `/overview` data contract.
- `bridge/space-mapping`: how a space finds its repo (via `fleet.json`, not the first terminal's cwd).

### Modified Capabilities
- none (no OpenSpec specs exist yet in this repo)

## Impact

`striker/internal/bridge`, new `striker/internal/openspec` and `striker/internal/overview`, `lcars/bridge.js`, `lcars/main.js` (crew labels), `striker start-lead` (tab name), lead and coordinator briefs.
