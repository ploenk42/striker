# Design: the stardate is a clock; the board names its crew; Striker's prompt shows in full

Wishes of the person (2026-11-24):

- A stardate top right that is a clock with year, month, day, hour, minute and second as one number. The scan
  time in the Striker heading uses the same format. The clock in the bottom right goes.
- The board columns say which lead asks, by crew name, together with the repo or the plan or worktree it
  works on, instead of the machine.
- A prompt longer than one line under the board must stay readable.

## 1. Stardate format

`stardate(ms)` lives in a new `lcars/stardate.js` (no gpui imports, so `bridge.check.mjs` can test it):

- It uses local time (the person's clock, not UTC) in the form `YYYYMMDD.hhmmss`, every field zero-padded:
  - `new Date(2026, 10, 24, 15, 30, 42)` → `"20261124.153042"`
  - `new Date(2026, 0, 2, 3, 4, 5)` → `"20260102.030405"`
- The dot splits date and time. It keeps the number readable and looks like a stardate (`47634.4`).
- The LCARS font's digits are not monospaced, so the label may shift a little as the seconds change. It is
  right-aligned left of the space name, so the space name does not move. If the shift is visible, the stardate
  gets a fixed `min_w` measured on `20288888.888888`.

## 2. Top right: a clock

- `main.js` drops its old `stardate` (year − 1920 plus the fraction of the year, UTC) and imports the new one.
- Only the `top` part ticks every second (`cx.timer.every(1000, …)`). Every other part keeps its 15 s tick, so
  the side, the board and the Bridge are not redrawn by it. The tick may land up to a second late (timers are
  not aligned to the full second), which is fine for a display clock.

## 3. Striker heading and bottom right

- `scanText` in `striker.js` becomes `SCAN ${stardate(t)}`, plus ` · STALE` as today. `t` stays the time of the
  last scan, not now.
- The `palette` cell drops ` · ${clock}` and reads `CTRL SHIFT P` (`⌘ SHIFT P` on macOS). It is still a
  TOUCH-high button that opens the command palette.

## 4. Crew names on the board

The crew comes from `shared(CREW_KEY)` (Striker's `/crew`, already polled by the sidebar), matched with
`crewEntry` from `space.js` by address. `crewLabel(entry)` gives `CAPT. PATA`. Without an entry (Striker has no
`/crew`, or a new agent not yet named) the board keeps today's words (`LEAD`, `COORD`).

- **Column header, small line: who asks.**
  - If the lead needs the person (needs_you entries, or its terminal is blocked): `CAPT. PATA · NEEDS YOU`.
  - Else if a coordinator needs them: `CMDR. WERF · OMEGA · NEEDS YOU` (crew name, change). With several
    asking coordinators: the first, then `+1`.
  - Not needy: the lead's name and state (`CAPT. PATA · WORKING`, `· COMPLETE`, `NO LEAD`).
  - The machine leaves the header. It stays in the lead line's address (`REMOTE1/T1D`).
- **Column header, big line:** the repo name, as today. The repo stays the column's identity.
- **Lead line:** `CAPT. PATA · WORKING` instead of `LEAD REMOTE1/T1D · WORKING`. The second line begins with
  the address and keeps the message and `UPDATED …`.
- **Coordinator line:** `CMDR. WERF · OMEGA · NEEDS YOU`. The second line gives the address, the worktree
  folder (last path part, e.g. `ALPHA-OMEGA`, from `/status`'s coordinator entry or from the terminal's
  cwd), the task and `UPDATED …`.
- **NEXT MOVE list** (`repoMoves`): `PATA · ALPHA · FRAGE · …` for a lead, and
  `WERF · ALPHA · OMEGA · FRAGE · …` for a coordinator. The rank is left out there to save width.
- `boardCoords` and `repoMoves` stay pure. The names are passed in, so `striker.js` looks them up and
  `bridge.check.mjs` tests them with and without names.

## 5. Striker's prompt under the board

Observed (screenshot 2026-11-24): the striker-prompt block shows Claude's status and the first line of the
prompt input. The rest of a long prompt and Claude's footer below it are missing, so the bottom of the
terminal is cut off.

- **Likely cause:** Striker's terminal is shown twice, in its STRIKER space pane (tall) and in the board's block
  (about 170 px). The terminal has one grid size, taken from the tall pane. The short block shows that grid's
  top rows and cuts off the bottom, where Claude draws its input and footer.
- **Task 1 checks this first.** It reads the size of t16 and compares a shot of the block while the STRIKER space
  pane is closed or hidden with one while it is open.
- **Fix, in this order of preference:**
  1. Size the grid by the block while the board shows (Herdr resizes a terminal to the view that has focus or
     was last shown), if Herdr offers it.
  2. Bottom-anchor the block, so a too-tall grid shows its last rows. The input and footer are what matter
     under the board. This would be a block or terminal option in the layout, if Herdr has one.
  3. Make the block grow while Striker's prompt has more than one line. Herdr gives no reliable signal for
     that, so this is a fallback only.
  4. If none works in a mod, the finding goes to the person, who can file it with Herdr.
- **Found (task 3, 2026-11-24):** the guess was wrong. `hc get t16` reports 312 × **7** rows, so the grid already
  follows the block (min 160 px, at font 14 about 7 rows). Claude Code needs more than that: status line, a blank
  line, the input box with its border, the footer. With 7 rows it keeps the top of its frame and the input gets
  one line. Herdr has no bottom anchor for a terminal block, and a second view does not set the size.
- **Fix:** the block gets a taller minimum, `min: 300` (about 12 rows at font 14). That is room for a three-line
  prompt plus Claude's status and footer, and costs the board about 140 px. Growing only while the prompt has
  focus would need a focus signal that mods do not get reliably, so it is not done.
- The chosen fix lands in `layouts/lcars.json` and `lcars/prompt.js` (`promptBlock`), so a dropped block that
  is put back keeps it.

## 6. Board: clicks go to who asks; NEXT MOVE goes

Decisions of the person (2026-11-24):

- The NEXT MOVE list above the columns goes. It only repeated the columns' questions, and its click opened
  Striker's prompt, which was unclear. `repoMoves` and its checks go with it.
- A click on a column's blinking head goes to the terminal of whoever asks (`headLine`'s asker: the lead, else
  the first asking coordinator), through `focusLead` (board off, `agent.focus`). Without an asker it goes to the
  lead.
- A click on a question row goes to its asker. The lead's questions go to the lead, a coordinator's questions
  to that coordinator.
- Head and question rows get the pointer cursor and hover like the lead line. The head keeps its square frame
  shape, so the "round end means click" rule has one exception: the column head. `docs/lcars-colors.md` says so.

## 7. herdr-mods on the board

`striker/fleet.json` gets herdr-mods as a repo of this computer: `name: "herdr-mods"`, no `machine`,
`path: "C:\\Users\\you\\herdr-mods"`, no `user`, `space: "herdr-mods"`. Striker's status report already
handles local repos (a local lead by `striker.lead`, the status file under `~/.striker`), so the board shows a
fifth column (`NO LEAD` while none runs). STRIKER gets no column: the board is Striker's own view. Task 7 checks
that the report and `/crew` treat the local repo right, then live, and fixes what does not.

## 8. PLAN and SPEC on a main checkout

On a main checkout without a pick, PLAN shows `NO PLAN FOR BRANCH MAIN`, though the repo has changes and
specs. Decision of the person: on main, PLAN shows the vision and SPEC lists all specs.

- **PLAN on main:** the vision file (`/overview`'s `context.file`, the same file as OVERVIEW's VISION row),
  rendered in the SPEC reader with its headings on the elbow, read-only. Without a vision file:
  `NO VISION FOUND (README OR CONTEXT FILE)`.
- **SPEC on main:** a list of all specs on the elbow, one accordion-style row per spec:
  - the living specs `openspec/specs/**/spec.md`, labelled by their path below `specs/` (`BRIDGE / REPO-OVERVIEW`)
  - the active changes' `design.md` (`CHANGE · STRIKER-RELAY`)
  - legacy specs under `docs/superpowers/specs/`

  A click reads the file in the reader beside the list, like a picked file today (`PICK_KEY` with `file`), and
  the picked row is marked. The list comes from Striker's `/overview` (task 8 adds `specs: [path]` there if it
  is not already given, from `ls-tree` of the main checkout).
- A worktree space (a change's branch) keeps today's PLAN and SPEC of its change. "Main checkout" is
  `isMainCheckout` as the default mode already uses it.

## 9. Board columns scroll

Found by the person (2026-11-24): beta's questions and notes are taller than its column, and the board does
not scroll, so the rest is cut off. Each column's body (questions, phase, task, progress, status) scrolls on
its own; the head, the lead and coordinator lines and the foot stay in place.

## 10. Docs and checks

- `docs/lcars-colors.md` gets a line saying the stardate is the local clock `YYYYMMDD.hhmmss`, the same format
  as scan times. The board section describes the crew names.
- `bridge.check.mjs` checks:
  - `stardate` for the two dates above and for 23:59:59 on 31 December.
  - `boardCoords` and `repoMoves` with and without names.
- Live, with `herdr-app shot`:
  - the top right counts seconds
  - the heading shows `SCAN 2026…`
  - the bottom right shows only `CTRL SHIFT P`
  - the board headers show crew names
  - a three-line prompt under the board is fully visible
