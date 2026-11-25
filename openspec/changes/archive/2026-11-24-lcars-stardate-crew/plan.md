# Plan: lcars-stardate-crew

Design: `design.md`. LCARS checks: `node lcars/bridge.check.mjs` (prints `ok`) and `herdr-app check lcars`. Shell
rules of AGENTS.md apply (one simple command, absolute paths). Commit per task; its line in `tasks.md` and its
steps here are ticked in the commit with its last work.

### Task 1: stardate.js: the local clock as YYYYMMDD.hhmmss; top right ticks every second, SCAN in the same format, bottom clock gone

Files: new `lcars/stardate.js`, `lcars/main.js`, `lcars/striker.js`, `lcars/bridge.check.mjs`.

- [x] **Step 1: failing check** in `bridge.check.mjs`: `stardate(new Date(2026, 10, 24, 15, 30, 42).getTime())`
  is `"20261124.153042"`, `(2026, 0, 2, 3, 4, 5)` is `"20260102.030405"`, `(2026, 11, 31, 23, 59, 59)` is
  `"20261231.235959"`.
- [x] **Step 2:** `stardate.js` exports `stardate(ms)` (local time, zero-padded).
- [x] **Step 3:** `main.js`: import it, drop the old `stardate`; in `init`, the `top` part ticks every 1000 ms,
  every other part keeps 15000 ms; `bottom()` drops `clock`, the palette cell reads `CTRL SHIFT P` (`⌘` on macOS).
- [x] **Step 4:** `striker.js` `scanText`: `SCAN ${stardate(t)}` plus ` · STALE`; the fleet write check (line ~50)
  still compares scan texts (it now changes with each scan's second, which is a real change).
- [x] **Step 5:** checks green; commit `lcars: the stardate is a clock`, tick 1.

### Task 2: Board: crew names in column headers, lead and coordinator lines and NEXT MOVE

Files: `lcars/board.js`, `lcars/striker.js`, `lcars/bridge.check.mjs`.

- [x] **Step 1: failing checks:**
  - `boardCoords(repo, live, nameOf)`: with `nameOf(address)` returning `{label: "CMDR. WERF", short: "WERF"}`,
    the label is `CMDR. WERF · RB · NEEDS YOU`; without a name `COORD RB · NEEDS YOU` as today; `worktree` is the
    last path part of the coordinator's `worktree` or `cwd` (upper case), else null.
  - `repoMoves({..., lead: "PATA", coords: [{..., who: "WERF"}]})`: `PATA · ALPHA · FRAGE · …` and
    `WERF · ALPHA · RB · FRAGE · …`; without names as today.
  - `headLine(v)` (new, pure, in `board.js`): lead needy → `CAPT. PATA · NEEDS YOU`; a needy coordinator →
    `CMDR. WERF · RB · NEEDS YOU`, two → `… · +1`; not needy → `CAPT. PATA · WORKING`; no lead → `NO LEAD`;
    without crew names `LEAD · NEEDS YOU` / `COORD RB · NEEDS YOU`.
- [x] **Step 2:** implement in `board.js`; `striker.js` builds `nameOf` from `shared(CREW_KEY)` with `crewEntry`
  (`space.js`; rank and name of the entry), uses `headLine` for the header's small line, `CAPT. PATA · WORKING` on the lead
  line (address first on its second line), the coordinator's worktree on its second line, names in
  `repoMoves`. Find where `/status` gives a coordinator's worktree or cwd; if neither, take the terminal's cwd
  from herdr's live agents. (Found: `/status` has neither; the worktree is the folder of the live agent's `cwd`.)
- [x] **Step 3:** checks green; commit `lcars: the board names its crew`, tick 2.

### Task 3: Striker's prompt: find why the block cuts off its bottom rows

No code; findings go into `design.md` section 5.

- [x] **Step 1:** `hc get t<striker> --json` and herdr's view of the terminal (`herdr-app status`): rows and columns.
  Compare with the block's height in px and with the STRIKER space pane. (312 × 7: the grid follows the block.)
- [x] **Step 2:** not needed: the grid already follows the block, the block is just too short.
- [x] **Step 3:** the layout has `size.px`; no bottom anchor. Fix: a fixed 300 px block.
- [x] **Step 4:** written into `design.md`; committed together with task 4.

### Task 4: Striker's prompt: fix it

Files: per task 3 (`layouts/lcars.json`, `lcars/prompt.js`, maybe `lcars/striker.js`).

- [x] **Step 1:** the fix task 3 found possible, with a check in `bridge.check.mjs` for `promptBlock` if it changes.
- [x] **Step 2:** the live config's block edited the same way (not `install.ps1`, which would put back the
  repo's terminal t14); Herdr took it at once, `hc get t16` reports 13 rows.
- [x] **Step 3:** commit `lcars: Striker's prompt shows in full`, tick 4.

### Task 5: Docs, checks and a live look

- [x] **Step 1:** `docs/lcars-colors.md`: stardate format; board crew names.
- [x] **Step 2:** `bridge.check.mjs` ok, `herdr-app check lcars` ok, `openspec validate lcars-stardate-crew --strict`.
- [x] **Step 3:** live (after Herdr reloads the mod): top right counts seconds, `SCAN 2026…`, bottom right only
  `CTRL SHIFT P`, board headers with crew names. Note what was not seen.
  (Seen 2026-11-24 after the restart: the clock counts (120240 → 120314), `SCAN 20261124.120223`, only `CTRL
  SHIFT P`, heads `CMDR. STRIKER · NEEDS YOU`, `CAPT. PATA · NEEDS YOU` …, no NEXT MOVE, herdr-mods column with
  Striker as lead, coordinator line `CMDR. CRUSHA · STRIKER-RELAY · WORKING`, the prompt with 13 rows. The worktree
  folder on the coordinator line was missing (agents() has no cwd): fixed from the pane's cwd. Not seen by
  Striker: clicks on heads and questions, column scrolling, PLAN and SPEC on main; the person checks them.)
- [x] **Step 4:** commit `lcars: docs for stardate and crew names`, tick 5; merge to main after the person's go.

### Task 6: Board: clicks on heads and questions go to who asks; NEXT MOVE goes

Files: `lcars/board.js`, `lcars/striker.js`, `lcars/bridge.check.mjs`, `docs/lcars-colors.md`.

- [x] **Step 1: failing check:** `asker({ lead, leadNeedy, coords })` (pure, `board.js`) gives the lead's
  address when the lead asks, else the first needy coordinator's, else the lead's, else null.
- [x] **Step 2:** `striker.js`: the head presses to `focusLead(asker(...))`; lead question rows to the lead, a
  coordinator's to it; the banner and `repoMoves` (and its checks and import) go.
- [x] **Step 3:** docs (column head clickable, no NEXT MOVE); checks green; commit `lcars: board clicks go to who
  asks`, tick 6.

### Task 7: herdr-mods on the board (fleet.json, local repo checked)

Files: `striker/fleet.json`, maybe `striker/internal/status`, `striker/internal/crew`.

- [x] **Step 1:** add herdr-mods as a local repo; `striker status` (the CLI) shows it with `lead: null` and no error.
- [x] **Step 2:** if the report or `/crew` mistreats a local repo (status file path, space match), a failing Go test
  first, then the fix. (Not needed: it shows with `lead: null`, `status: null`.)
- [x] **Step 3:** `install.ps1` (serve restarted); `/status` has five repos; commit `striker: herdr-mods in the
  fleet`, tick 7. (No restart needed: `striker serve` loads `fleet.json` on every report.)

### Task 8: PLAN on main shows the vision, SPEC on main lists all specs

Files: `lcars/bridge.js`, `lcars/overview.js`, `lcars/bridge.check.mjs`, maybe `striker/internal/repoview`
(`/overview`'s `specs`), `docs/lcars-colors.md`.

- [x] **Step 1:** find what `/overview` gives for the vision (`context.file`) and for specs (capabilities, files);
  if no spec list, add `specs: [path]` (living specs, active changes' design.md, legacy specs) with a Go test.
  (Found: `inventory.capabilities[].path`, `changes[].name` and `legacy[].spec` are enough; no Go change.)
- [x] **Step 2: failing checks:** `specList(ov)` (pure, `overview.js`) gives rows `{ label, file }` in the design's
  order and labels.
- [x] **Step 3:** `bridge.js`: on a main checkout without a pick, PLAN renders the vision file read-only in the
  reader (`specView` with the vision as file), SPEC shows the list on the elbow and the picked spec in the
  reader.
- [x] **Step 4:** docs; checks green; commit `lcars: PLAN and SPEC on main`, tick 8.

### Task 9: Board columns scroll when their questions and notes are taller than the column

Files: `lcars/striker.js`.

- [x] **Step 1:** the column body gets an id, `min_h(0)` and `overflow_y_scrollbar()` (as the Bridge's lists);
  the columns row `min_h(0)`; the body's trailing spacer goes. Head, lead and coordinator lines and foot stay
  fixed.
- [x] **Step 2:** `herdr-app check` ok; commit `lcars: board columns scroll`, tick 9.

### Task 10: Striker known by the label striker.self, so he can live in herdr-mods as its lead

Decision of the person (2026-11-24): herdr-mods is Striker's own project; the STRIKER space closes and Striker runs in
the herdr-mods space as its lead. Striker was known by the space name STRIKER (crew, TALK TO STRIKER, the prompt
block); now by the label `striker.self`, the space STRIKER only as the fallback.

Files: `striker/internal/crew/crew.go` + test, `lcars/space.js` (`strikerSelf`), `lcars/striker.js` (`strikerAgent`),
`lcars/bridge.check.mjs`, `striker/CLAUDE.md`.

- [x] **Step 1:** failing tests `TestStrikerByLabel` and the `strikerSelf` checks; then the code; Striker's crew repo is
  his `striker.lead` when set.
- [x] **Step 2:** `striker/CLAUDE.md`: herdr-mods is the exception to "never work in a repo".
- [x] **Step 3:** commit `striker: known by striker.self, lead of herdr-mods`, tick 10; then relabel and move t16,
  rebuild with `install.ps1`.
