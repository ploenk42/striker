# Crew names, model and tokens; one lane per change — Implementation Plan

**Goal:** Crew rows read `CAPT. PATA` / `ALPHA · WORKING · OPUS 5.5 · 1.2M`; the REPO tree has one labelled
lane per change, its label at the branch start, and a compact future.

**Design:** `openspec/changes/lcars-crew-tree/design.md`.

**Branch:** `lcars-crew-tree`, made from `lcars-touch-rollout` (it needs that branch's `main.js`, `fleet.json`
and awayteam setup). Work in a worktree, never by switching the main checkout: `striker serve` reads `fleet.json`
live from `C:/Users/you/herdr-mods`.

**Checks:** `go -C <worktree>/striker test ./...` → PASS; `node <worktree>/lcars/bridge.check.mjs` → ok;
`herdr-app check <worktree>/lcars` → no output. Live checks need Striker rebuilt from the main checkout and a
Herdr restart (Windows mods do not hot-reload); the person restarts Herdr.

### Task 1: Striker graph: one lane per change

Files: `striker/internal/graph/graph.go`, `striker/internal/graph/graph_test.go`.

- [ ] **Step 1: failing tests.** (a) an active change with 2 branch commits and tasks done, done, todo, todo:
  no `t:` node for the done tasks, exactly one lane with that plan, the first todo task has an edge from the
  newest branch commit; (b) a change without branch commits and 2 done tasks: their nodes are on the group's
  first lane, no second lane; (c) Now is not pushed right by done tasks of a change with branch commits.
- [ ] **Step 2:** in the task loop skip `tv.State == "done"` when `len(branchCols[t]) > 0`; drop `doneSub`
  (done tasks use sub 0); the Now shift loop counts done tasks only of plans without branch commits (compute
  after pass 1, where `branchCols` is known).
- [ ] **Step 3:** `go test ./...` → PASS; commit `striker graph: one lane per change`, tick 1.

### Task 2: LCARS tree: label at the branch start, closer future

Files: `lcars/tree.js`, `lcars/bridge.js` (label drawing, if it assumes the column), `lcars/bridge.check.mjs`,
`docs/lcars-colors.md`.

- [ ] **Step 1: failing checks** in `bridge.check.mjs`: (a) sub-lanes (label `""`) get no label; (b) the label of
  a lane whose first node is at x = 400 ends at `400 - LABEL_GAP` and is right-aligned; (c) text
  `LCARS-TOUCH-ROLLOUT 5/7` from a plan with 7 tasks, 5 done; with `THIS PLAN` when it fits, else cut as
  `fitLabel` does; (d) a first node at x < x0 + 4 chars → label above when the lane above is free there, else
  none; (e) `columnXs` with 10 future plan columns: each future gap ≤ `FUTURE_STEP` (32), past plan gaps keep
  up to `MAX_STEP`.
- [ ] **Step 2:** `tree.js`: `named` from lanes with `l.label`; progress from `plans` (`tasks` done/total);
  labels placed per lane (x, w, align right); `x0 = labelWidth(["MAIN"]) + LABEL_GAP`; `columnXs` takes `now`
  and caps wide columns after it at `FUTURE_STEP`. Return `align` with each label; `bridge.js` paints
  `text_right()` for `align: "right"`.
- [ ] **Step 3:** `docs/lcars-colors.md` REPO tree section: labels at the branch start, progress, future step.
- [ ] **Step 4:** checks pass; commit `lcars tree: label at the branch start, compact future`, tick 2.

### Task 3: ClaudeArgv passes HC_TERMINAL_ID through sudo

Files: `striker/internal/fleet/user.go`, `user_test.go`, lead and coord tests that pin the argv.

- [x] **Step 1: failing test:** `ClaudeArgv("awayteam", "/x")` → `["sudo","--preserve-env=HC_TERMINAL_ID","-u",
  "awayteam","-i","sh","-c","cd '/x' && exec \"$HOME/.local/bin/claude\""]`; root alike; `""` unchanged. (The
  planned `sh -c '… env HC_TERMINAL_ID=…'` wrapper was replaced by sudo's own `--preserve-env`, checked first.)
- [x] **Step 2:** implement; update the lead and coordinator argv tests.
- [x] **Step 3:** live: `hc exec --on remote1 env HC_TERMINAL_ID=t99 sudo -n --preserve-env=HC_TERMINAL_ID -u
  awayteam -i printenv HC_TERMINAL_ID` → `t99`. Commit `striker: agents keep their terminal id across sudo`, tick 3.

### Task 4: Status line writers and their setup

Files: `striker/setup/statusline.sh`, `striker/setup/awayteam.sh`, `striker/setup/awayteam-settings.json`,
`striker/cmd/striker` (new `statusline` command), `striker/internal/statusline/` (+ tests), `install.ps1`,
`install.sh`, `striker/fleet.json` (`agents_dir`), `striker/internal/fleet/fleet.go`, `striker/README.md`.

- [x] **Step 1: tests** for `statusline.Write(stdin, terminal, dir, now)`: writes `<dir>/<id>.json` as one line
  `{"terminal":"t7","at":<ms>,"claude":{…}}`; returns the line `Opus 5.5 · 1.2M`; no or odd terminal id → no
  file, line still returned; bad JSON → no file, empty line; `Short`, `Parse`, `ParseLines`.
- [x] **Step 2:** implement; `striker statusline` reads stdin (`HC_TERMINAL_ID`, dir `~/.striker/agents`).
- [x] **Step 3:** `statusline.sh` (stdin, or a file as `$1` for tests); run on remote1 with a sample JSON:
  printed `Opus 5.5 · 1.2M`, wrote the expected line, which `TestParseShellLine` now pins.
- [x] **Step 4:** `awayteam.sh` makes `/var/lib/striker/agents` like the status dir and installs
  `statusline.sh` as `/home/awayteam/.claude/statusline.sh`; `awayteam-settings.json` gets `statusLine`; the
  settings install condition is now "no `statusLine` yet" (so the current file is replaced once).
  `fleet.json`: `"agents_dir": {"remote1": "/var/lib/striker/agents"}`; `fleet.Fleet.AgentsDir`.
- [x] **Step 5:** `striker install-statusline` (Go, `statusline.Install`: inserts the key as text after the opening
  brace when missing, so the person's file keeps its order; PowerShell 5.1's `ConvertTo-Json` would not), called
  by `install.ps1` and `install.sh`.
- [x] **Step 6:** the person ran the new `awayteam.sh` (copied with `hc put`; an earlier copy's name made
  `hc put` write `-2` files, so the old ones were removed first) on 2026-11-23: `/var/lib/striker/agents` is
  `you:awayteam` 2775, `~awayteam/.claude/statusline.sh` installed, its `settings.json` has `statusLine`.

### Task 5: Striker crew and GET /crew

Files: `striker/internal/crew/` (new, + tests), `striker/internal/serve/serve.go` (+ test), `striker/cmd/striker`.

- [x] **Step 1: tests:** role from labels; rank table (role, then model); repo from labels, fleet space,
  space name; names: bridge pool for lead/coordinator, lower decks for others, fallback across pools, then
  `CREWMAN <n>`, never STRIKER; a name survives a rebuild from `crew.json`; a vanished address or a changed
  `created_ms` frees the name; status line join by `<machine>/<terminal>`, tokens = in + out, files older than a
  day ignored. (`hc.Terminal` got `agent`, `hc.Info` got `created_ms`.)
- [x] **Step 2:** `crew.Build(hc, fleet, storePath, localAgents, now) Crew`, reading agents dirs with one `hc exec
  find … -exec cat {} +` per machine (local: read the dir directly).
- [x] **Step 3:** `serve`: `Server.Add(path, build)` for further cached reports, `/crew` added in `runServe` and
  routed; `TestAddedReport`.
- [x] **Step 4:** checked with the branch's build on port 7599 (the installed Striker stays on main until the
  merge): `/crew` lists the four remote1 leads as CAPT. PATA, WERF, TREY, LE FERGE; no model and tokens yet
  (their Claude has no status line and no terminal id until restarted).

### Task 6: LCARS crew rows from /crew

Files: `lcars/striker.js` (poll `/crew`), `lcars/space.js` or a new `lcars/crew.js` (pure formatting),
`lcars/main.js` (`member()`), `lcars/bridge.check.mjs`, `docs/lcars-colors.md`.

- [x] **Step 1: failing checks** for `crewLines(agent, entry)` (in `space.js`): `["CAPT. PATA",
  "ALPHA · WORKING · OPUS 5.5 · 1.2M"]`; tokens 812345 → `812K`, 999 → `999`; missing model/tokens left out; no
  entry → today's two lines; `crewEntry(crew, agent)` across address forms; `shortTokens`.
- [x] **Step 2:** implement; `member()` uses it; `pollCrew` (`striker.js`, shared `striker.crew`) polled with
  `/status` by the sidebar.
- [x] **Step 3:** checks; commit `lcars: crew rows with names, model and tokens`, tick 6. Not yet seen in the
  window: needs the merged Striker (with `/crew`) and a Herdr restart (Task 7).

### Task 7: Live check

- [x] **Step 1:** merged into main (fast-forward), Striker rebuilt with `install.ps1`; the person restarted Herdr.
- [x] **Step 2:** the four remote1 leads restarted on 2026-11-23 (handover through the status file; one lead
  overwrote its file before reading it and restored it from Striker's copy). `/crew` then had model and tokens
  for all four (Opus 5.5, 50K–99K). Striker now also starts leads with `--remote-control` (they lost it on restart).
- [x] **Step 3:** seen on 2026-11-24: the CREW rows read `CAPT. PATA` / `ALPHA · STANDING BY · OPUS 5.5 · 53K`
  (Striker's own row too: `LT. BEYMLA`, the orchestrator has no crew name yet); the herdr-mods REPO tree labels both lanes
  at their fork with progress (`LCARS-CREW-TREE 6/7`). Not seen: a lane whose branch has commits beyond main
  (both branches are merged), so done nodes hidden beside branch commits was checked only by tests. Opening the
  OVERVIEW first took 14–20 s (one git show per file); fixed in be57b61.
