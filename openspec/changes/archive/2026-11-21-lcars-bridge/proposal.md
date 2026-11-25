# Proposal

## Why

The person works with Herdr as: machine, git repo, worktree = space. Each worktree carries one spec and one plan, and its tabs are Herdr terminals (a coordinator, one implementer per task, a reviewer, a shell). Nothing showed, for the space they are in, what is planned, what runs, what waits on them and how this work fits into the rest of the repo.

## What Changes

- A Bridge view per worktree in the LCARS layout: PLAN and SPEC modes, a REPO tree of past git history and planned work, coordinator and task terminals in a bottom field.
- New Striker endpoints `/repo`, `/space`, `/doc`, and `striker issues` (GitHub issues as task numbers).
- The coordinator runs task agents as labelled Herdr terminals (`striker dispatch`).

## Impact

`striker/`, `lcars/bridge.js`, `lcars/main.js`, striker briefs. No capability specs were back-filled (`skip_specs: true`).
