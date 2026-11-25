# Design: crew names, model and tokens; one lane per change

Decisions of the person (2026-11-22): invented crew names with a rank by role or model;
the crew line is "repo · status · model · tokens"; the tree keeps a node for every open task.

## 1. Crew identity (Striker)

A new package `striker/internal/crew` owns who is who. `striker serve` refreshes it with the status report and
serves `GET /crew`:

```json
{ "agents": [ { "address": "remote1/t18", "name": "PATA", "rank": "CAPT.", "role": "lead",
    "repo": "alpha", "model": "Opus 5.5", "tokens": 1234567, "context_pct": 41, "seen": "2026-11-23T08:00:00Z" } ] }
```

- **Who is an agent:** every terminal of `hc ls --json` whose `agent` is `claude-code`; `hc get` gives its
  labels.
- **Role:** `striker.lead` → `lead`; `striker.coordinator` → `coordinator`; `striker.role=<r>` → `<r>` (implementer,
  spec-reviewer, ...); else none.
- **Repo:** `striker.lead`, `striker.repo`, else the fleet repo whose space is the terminal's space (`where` before
  ` / `), else the space name.
- **Rank:** lead `CAPT.`, coordinator `CMDR.`, a reviewer role (`*reviewer`) `LT.`, any other role `ENS.`.
  Without a role the model decides: Fable `CMDR.`, Opus `LT.`, Sonnet `LT. JG.`, Haiku `ENS.`, unknown `CREW`.
- **Name:** from two pools. Leads and coordinators take the next free name of the bridge pool (PATA, WERF,
  TREY, LE FERGE, CRUSHA, E'BRYEN, YEAR, GUYNAN, BARCLAY, RE, PULESKY, EGAWA); everyone else the next free name
  of the lower-decks pool (BEYMLA, MARYNA, TENDY, RUTHAFERD, FREEMAN, RANSEM, SHAKES, DANNA, BYLLUPS, MYGLEEME,
  JENNYFA, BARNES, STEVENS, KAYSHEN). A pool that runs out falls back to the other, then to `CREWMAN <n>`.
  STRIKER is never handed out: it is the orchestrator's name.
- **Kept for life:** the map address → name lives in `~/.striker/crew.json` (Striker's machine), so a restart of
  `striker serve` keeps the names. A name is freed when its terminal is gone from `hc ls` (a new terminal at the
  same address is a new agent: the entry also stores `created_ms` from `hc get` and is dropped when it
  differs).

## 2. Model and tokens (status line)

Claude Code runs a status line command after each assistant message and passes JSON on stdin with
`model.display_name`, `context_window.total_input_tokens`, `total_output_tokens`, `used_percentage` and
`session_id`. A status line writer stores that per terminal:

- File: `<agents_dir>/<HC_TERMINAL_ID>.json` = `{"terminal": "<id>", "at": <unix ms>, "claude": <stdin as is>}`.
  Without `HC_TERMINAL_ID` (Claude outside herdr) the writer stores nothing. It prints one short line for the
  status bar itself: `<model> · <tokens>`.
- `agents_dir`: on remote1 `/var/lib/striker/agents` (made by `setup/awayteam.sh` like the status dir,
  `you:awayteam` 2775); on Striker's machine `~/.striker/agents`. `fleet.json` gets `"agents_dir":
  {"remote1": "/var/lib/striker/agents"}`; a machine without an entry uses `~/.striker/agents` of the hc user.
- Writers: `striker/setup/statusline.sh` (POSIX sh, no jq: it wraps stdin unparsed; the short line uses
  `sed`), installed for awayteam as `~/.claude/statusline.sh` and set in `awayteam-settings.json`
  (`"statusLine": {"type": "command", "command": "sh ~/.claude/statusline.sh"}`); and `striker statusline` (Go)
  for the agents on Striker's machine, set in the person's `~/.claude/settings.json` by the installer only if no
  status line is set there yet.
- **Terminal id across sudo:** `sudo -i` drops the environment, so `fleet.ClaudeArgv` for root or another user
  adds `--preserve-env=HC_TERMINAL_ID` (a sudo rule of the form `<owner> ALL=(awayteam) NOPASSWD: ALL` implies
  SETENV; checked on remote1:
  `env HC_TERMINAL_ID=t99 sudo -n --preserve-env=HC_TERMINAL_ID -u awayteam -i printenv HC_TERMINAL_ID` prints
  `t99`). Running leads get it on their next restart.
- **Striker reads** each machine's agents dir once per refresh with one call, `hc exec --on <m> find <dir>
  -name '*.json' -mmin -1440 -exec cat {} +` (each file is a single line, so the output is one JSON object per
  line), and joins on `<machine>/<terminal>`. Tokens = input + output. Files older than a day are skipped;
  there is one file per terminal id, rewritten in place, so nothing piles up.

## 3. Crew row (LCARS)

`lcars/main.js` polls `/crew` with `/status` (same 5 s loop in `striker.js`) and draws each member:

- line 1: `<RANK> <NAME>` (e.g. `CAPT. PATA`), in the focus colour when focused;
- line 2: `<REPO> · <STATUS> · <MODEL> · <TOKENS>`, tokens short (`812K`, `1.2M`), model short (`OPUS 5.5`);
  parts without data are left out.
- Striker not answering, or an agent missing from `/crew`: the row stays as today (program · tab, space ·
  status).

## 4. REPO tree: one lane per change

**Striker (`graph.go`):** a change with branch commits no longer places its done tasks: the commits are its past.
Todo tasks whose dependencies were done tasks start from the branch head (they already take the entry edges
when no placed dependency is left). Done tasks of a change without branch commits stay on lane 0 of the group
(no extra sub-lane: `doneSub` goes). The "make room for done tasks" Now shift counts only those changes.
Parallel todo tasks keep their sub-lanes (real parallel work), unlabelled.

**LCARS (`tree.js`):**

- Only a group's first lane (`label` not empty) is named; sub-lanes have no label. The label text is
  `<CHANGE> <done>/<total>` from the plan's tasks (`view.plans`), then ` · THIS PLAN` / ` · AFTER …`.
- The label column goes. A lane label is right-aligned, vertically centred on the lane, and ends `LABEL_GAP`
  left of the lane's first node or of the fork (the source of an edge coming into the lane from another lane),
  whichever is further left: the curve from the fork runs nearly flat along the lane's row for its last ~40 px,
  so a label just before the first node would lie on it. It may use the whole empty stretch from the left edge,
  and is cut as before (suffix first, then the progress, then "…"). Only `MAIN` keeps a fixed spot at the trunk's
  start, so the trunk starts after `labelWidth(["MAIN"])`.
- Room for fewer than 4 characters: no label (the PLAN list still names the change). A fallback above the lane
  was dropped while building: the row above is the trunk or another lane, so the label would lie on a line.
- **Future spacing:** wide gaps after the one following NOW (that one keeps room for the NOW tag) are at most
  `FUTURE_STEP` = 32 px (past plan columns keep `MAX_STEP` 80); every open task keeps its node. If the future
  still does not fit, past and future share alike, and below 8 px all gaps shrink alike as before.

## 5. Tests and checks

- Go: `crew` (role, rank, repo, name pools, kept names, freed names, STRIKER never), status line file parsing and
  the join, `ClaudeArgv` with the terminal id, `graph` (no done nodes with branch commits, done nodes on lane 0
  without, no duplicate plan lanes), `serve` `/crew`.
- `striker statusline`: stdin JSON → file and printed line; no `HC_TERMINAL_ID` → no file.
- `bridge.check.mjs`: crew line formatting (tokens, model, fallbacks); tree labels at the fork, cut, above,
  dropped; future step.
- Live: the crew rows show names and tokens for the remote1 leads after their restart; the tree of
  `lcars-touch-rollout` shows one lane with `5/7`.

## Not in this change

Cost in dollars, per-agent history, names for non-Claude agents (they keep today's row).
