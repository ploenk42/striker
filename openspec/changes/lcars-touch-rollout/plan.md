# LCARS touch sizes everywhere, rollout on remote1 — Implementation Plan

**Goal:** Every part of the LCARS layout follows TEXT_MIN 14 / TEXT_BODY 16 / TOUCH 44; the remote1 rollout
steps of the repo overview are done.

**Design:** `openspec/changes/lcars-touch-rollout/design.md`.

**Checks after every LCARS task:** `node C:/Users/you/herdr-mods/lcars/bridge.check.mjs` → all pass;
`herdr-app check C:/Users/you/herdr-mods/lcars` → no output (no errors); `herdr-app status` →
`"errors": []`. Screenshots go to `.superpowers/shots/` (ignored).

### Task 1: Sidebar and machines popup

Files: `lcars/main.js`.

- [x] **Step 1:** Import `TEXT_MIN` from `./palette.js`.
- [x] **Step 2:** `section()`: height 20, text `TEXT_MIN`.
- [x] **Step 3:** `side()` space row: `.h(TOUCH)`, index `text_size(TEXT_MIN)`, name `text_size(TEXT_BODY)`,
  state oval `w(22).h(12).rounded(6)`.
- [x] **Step 4:** machine block label `text_size(TEXT_MIN)`, `min_h(TOUCH)`. Wrapping did not happen (the
  text ran out on the left), so the name stands one word per line.
- [x] **Step 5:** `+ new space` slab: `slab(..., TOUCH, (el) => pill(el, TOUCH)...)`.
- [x] **Step 6:** crew `member()`: `.h(TOUCH)`, both lines `text_size(TEXT_MIN)`, colour bar `rounded_l(TOUCH / 2)`.
- [x] **Step 7:** `strikerSlab()`: `pill(h_flex(), TOUCH)`, `.h(TOUCH)`, count `TEXT_MIN`, word `TEXT_BODY`.
- [x] **Step 8:** the sidebar root `v_flex().id("side")` gets `.overflow_y_scrollbar()`; the filler keeps
  `flex_1().min_h(10)`. Not yet seen scrolling: only 5 spaces and 4 agents, which fit.
- [x] **Step 9:** delete the unused `shepherdr()` method and `SHEP_ROWS` (the status arm's `this.shep` stays);
  the snapshot's unused `rows` went too.
- [x] **Step 10:** `machines()`: rows `TOUCH` (`slab(..., TOUCH, ...)`, `pill(el, TOUCH)`); `newSpace()` popup
  height `12 + 30 + (GAP + TOUCH) * (away.length + 1)`. Popup not screenshotted (opens only by click).
- [x] **Step 11:** checks; screenshot `all1.png` (sidebar after a Herdr restart: on Windows the junctioned mod
  reloads only on restart).
- [x] **Step 12:** commit, together with Tasks 2 and 3 (they share `lcars/main.js`).

### Task 2: Top and bottom frame

Files: `lcars/main.js`, `layouts/lcars.json`.

- [x] **Step 1:** `top()`: elbow label and stardate `text_size(TEXT_MIN)`.
- [x] **Step 2:** `layouts/lcars.json`: `bottom` block `"px": 60`.
- [x] **Step 3:** `bottom()`: elbow text `TEXT_MIN`; the gear wrapped in a `TOUCH`×`TOUCH` box
  (`size(TOUCH).flex().items_center().justify_center()`, icon `size(18)`), its stop-propagation handlers on
  the box; status row text `TEXT_MIN`; `striker-status`, `shepherdr-status` and `palette` become
  `h_flex().h_full().min_w(TOUCH).px(10).items_center()` cells; the status row itself is `flex_1()` above the
  16 px arm, so it is 44 px in a 60 px block.
- [x] **Step 4:** run `install.ps1`'s layout copy by hand: copy `layouts/lcars.json` to
  `C:/Users/you/.config/herdr-app/layouts/lcars.json` (Copy-Item); Herdr restarted.
- [x] **Step 5:** checks; screenshot `all1.png` (bottom frame).
- [x] **Step 6:** commit, together with Tasks 1 and 3.

### Task 3: Bridge rows and docs

Files: `lcars/bridge.js`, `docs/lcars-colors.md`.

- [x] **Step 1:** `bridge.js`: `const ROW = 60`.
- [x] **Step 2:** `docs/lcars-colors.md` "Sizes": the constants apply to the whole layout; remove "The sidebar
  and the frame's top and bottom keep their older, smaller text."; PLAN rows 60 px; the bottom block 60 px
  with a 44 px status row; the sidebar scrolls.
- [x] **Step 3:** checks; screenshot `plan2.png` of this change's PLAN (cards fit 60 px rows; no working card
  with a step bar existed to look at).
- [x] **Step 4:** commit, together with Tasks 1 and 2.

### Task 4: remote1 — user awayteam for the agents

Files: `striker/internal/fleet/fleet.go`, `striker/internal/lead/lead.go`, `striker/internal/coord/coord.go`, their
tests, `striker/fleet.json`, `striker/setup/awayteam.sh`, `striker/CLAUDE.md`, `striker/README.md`.

- [x] **Step 1: failing tests.** `fleet`: `TestClaudeArgv` — root → `["sudo","-i","sh","-c","cd '/x' && exec
  /root/.local/bin/claude"]`; `awayteam` → `["sudo","-u","awayteam","-i","sh","-c","cd '/x' && exec
  \"$HOME/.local/bin/claude\""]`; `""` → `["sh","-lc","cd '/x' && exec claude"]`. `TestSudo` — root →
  `["sudo","-n","-i"]`, awayteam → `["sudo","-n","-u","awayteam","-i"]`, `""` → nil. `lead`:
  `TestStartLeadAwayteamArgv`. `coord`: `TestStartCoordinatorAwayteam` (git through
  `exec --on remote1 --cwd / sudo -n -u awayteam -i git -C /srv/alpha …`, argv as above).
- [x] **Step 2:** `fleet.ClaudeArgv(user, dir)` and `fleet.Sudo(user)`; `lead.Start` and `coord.Argv` use
  `ClaudeArgv`; `coord.rootGit` becomes `userGit{user}` with `fleet.Sudo`, used for every user but `""`.
  `go -C C:/Users/you/herdr-mods/striker test ./...` → PASS.
- [x] **Step 3:** `striker/setup/awayteam.sh` (bash, `set -euo pipefail`, must run as root, idempotent): user and
  group, extra groups, ACLs on the four repos and their `-*` worktrees, `safe.directory`, status dir and the
  move of old status files, the sudo rule through `visudo -cf` then install 0440, Claude Code install for awayteam,
  copy of root's `~/.claude/projects` (without overwriting). Prints what is left: `sudo -u awayteam -i claude`
  once to log in.
- [x] **Step 4:** `striker/CLAUDE.md`, `striker/README.md`: leads run as the fleet entry's user (awayteam on
  remote1). Commit `striker: agents can run as a user of their own (awayteam)`. `fleet.json` stays as it is:
  Striker reads it live from the working tree, so it changes only after the setup (Step 5).
- [x] **Step 5 (person):** copy the script to remote1 (`hc file write`), run it as root, log awayteam in to
  Claude. Check from this computer: `hc exec --on remote1 sudo -n -u awayteam -i git -C /srv/alpha status`
  exits 0. The copy arrived with a BOM and CRLF (PowerShell pipe); stripped with `sed` on remote1.
  awayteam's git works; the root leads still ran through an older, broader rule, which the person removes
  when no root lead needs it any more.
- [x] **Step 6:** done 2026-11-22. The root leads wrote their handover into `/var/lib/striker/status`, were ended
  (`hc keep --off`, `hc kill`, `hc rm`; their spaces closed) and started again with `striker start-lead` as
  awayteam. Each new lead stopped at Claude's folder-trust dialog (confirmed by hand, so `start-lead` returned
  "nicht bereit" and the brief went out with `striker send --file`). awayteam got `awayteam-settings.json`
  (language, plugins, the status dir as additional directory, else every status read asks), installed by the
  setup script. Then that older rule was removed: `sudo -n -i true` now fails, awayteam's git works. `fleet.json`: `"user": "awayteam"` for all four repos, `status_dir.remote1` =
  `/var/lib/striker/status`; rebuild and restart Striker (`install.ps1`); `striker status` reads the status files
  from the new dir. The running root leads are told the new status file path with `striker send` (or restarted
  as awayteam when the person says so). Tick (commit `openspec: remote1 awayteam done`).

### Task 5: remote1 — openspec CLI

Done 2026-11-23, for awayteam and without root (the person's choice): the system Node is 18 without npm, so
the official `node-v24.22.3-linux-x64.tar.xz` (checksum checked against `SHASUMS256.txt`) went to
`~awayteam/.local/node`, with `node`, `npm` and `npx` linked into `~awayteam/.local/bin` (first in its login
PATH) and `npm config set prefix ~/.local`. Then `npm install -g @fission-ai/openspec` (2.1.1) and
`openspec config set telemetry.enabled false`. No repo on remote1 has `openspec/` yet, so `validate` was not
run against real changes; `openspec list` in a directory without one answers "No OpenSpec root found".

### Task 6: First live coordinator run

The person picks a small real change of a fleet repo. `striker start-coordinator <repo> <change>`; check the
Bridge card of the change, the coordinator line on Striker's board, and a `needs_you` entry reaching the
person through Striker. Fix what breaks in its own commit; tick (commit `openspec: first live coordinator run`).

### Task 7: Lead tabs named claude

`herdr-app ls` → the lead tabs of beta, gamma and delta; `herdr-app do tab.rename --id <tab> --name claude`
for each not yet named `claude`. Check `striker status` → `where` ends in `/ claude`. Tick
(commit `openspec: lead tabs renamed`).
