# Proposal

## Why

On remote1 the leads and coordinators run as `awayteam`, which may not use the person's hc server: whoever can
reach that socket acts as the person's own user (reads and types into every hc terminal there, starts programs as
that user, uses that user's sudo rights). That boundary is wanted. But the workflow needs hc in four places:
the lead starts a coordinator (`striker start-coordinator`), the coordinator starts task agents
(`striker dispatch`), waits for them (`hc wait`) and reads their result (`hc read`). None of it works today, and
there is no `striker` binary on remote1 either. `lcars-touch-rollout` task 6 (the first live coordinator run) is
blocked on this.

## What Changes

- Striker relays for agents without hc: a lead or coordinator writes a request into its own status file
  (`requests`); `striker serve` (as the person, with hc) carries it out on the agent's machine and reports back
  into the agent's terminal with a `[Striker]` message.
- Requests: start a coordinator (lead), start a task agent (coordinator). For every agent Striker started, Striker
  waits for it and, when it is done or blocked, sends the coordinator its final answer.
- `striker dispatch` can target a fleet repo on another machine and starts the task agent as the fleet user
  (`sudo -u awayteam`, like the leads), under the coordinator as its parent.
- The briefs tell agents of a fleet entry with a `user` to use requests instead of `striker`/`hc`; agents that
  have hc (this computer, no `user`) keep calling them directly.

## Impact

`striker/` (a new relay package run by `striker serve`, `dispatch`, `fleet.ClaudeArgv`, `coord`, the briefs),
`striker/CLAUDE.md`. No LCARS change: task agents keep their labels, so the Bridge shows them as today.
Unblocks `lcars-touch-rollout` task 6.
