# Proposal

## Why

The CREW list in the LCARS sidebar reads "CLAUDE CODE · CLAUDE" on every row: the person cannot tell the agents
apart and sees nothing of what they cost. The REPO tree on the Bridge shows a change twice (its branch commits
and its done tasks on two lanes with the same label), puts the labels in a column far left of their branches,
and spreads the planned future over as much width as the past.

## What Changes

- Crew: every agent gets an invented crew name with a rank from its role (lead, coordinator,
  task agent) or, without a role, from its model; Striker keeps the name for the agent's life. The second line
  shows repo · status · model · tokens. Model and tokens come from a Claude Code status line that writes one
  small file per terminal; Striker collects them and serves `/crew`.
- Agents started as another user (awayteam) keep their terminal id across `sudo`.
- Tree: one lane per change (done tasks of a change with branch commits are no separate nodes; the label
  carries the progress, e.g. `4/7`), the label sits at the start of its branch, every open task keeps its node
  but the columns after NOW are closer together.

## Impact

`striker/` (graph, a new crew package, `serve`, `fleet.ClaudeArgv`, a `statusline` command, the awayteam setup),
`lcars/main.js`, `lcars/tree.js`, `lcars/bridge.check.mjs`, `docs/lcars-colors.md`. Builds on
`lcars-touch-rollout` (its branch is the base of `lcars-crew-tree`).
