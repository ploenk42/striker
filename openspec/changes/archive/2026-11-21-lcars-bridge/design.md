# LCARS Bridge: one view per worktree

Date: 2026-11-21. Status: approved by the person in chat ("freigegeben").
Depends on: striker-go-bundle

## Why

The person works with Herdr as: machine → git repo → worktree = space. Each worktree carries one
spec and one plan (superpowers format), and its tabs are Herdr terminals: a coordinator, one
implementer per task, a reviewer, a shell. Today nothing shows, for the space they are in, what is
planned, what runs, what waits on them and how this work fits into the rest of the repo. They want
one LCARS view per space that answers that, and a repo tree that shows past git history and the
planned future in one picture, including the order and the blockers between plans.

## Goals

1. A Bridge view for the active space, shown in the LCARS work area (like Striker's board), with two
   modes switched by pseudo-tabs at the left of the tab bar: PLAN (default) and SPEC.
2. PLAN mode: the worktree's plan as an LCARS elbow, tasks as accordion rows, and three status
   columns WORKING, NEEDS YOU, DONE whose cards sit on the same row as their task.
3. SPEC mode: the same elbow with the spec's headings, and a reading area that renders the spec's
   markdown, with changes against HEAD marked.
4. A flat bottom field (about 130 px) that shows the REPO tree by default, the coordinator's
   terminal after a click on the plan heading, or a task agent's terminal after a click on its card,
   with a ◂ REPO button back.
5. The REPO tree shows, for the whole repo: past git history (solid), every spec/plan of the repo
   as a present or future branch (dashed, fainter), task nodes on each branch, parallel tasks on
   their own strands, and dependencies between plans (a plan branches off only after the plans it
   depends on are merged; a plan whose dependency is not merged is shown as blocked by it).
6. Task numbers are GitHub issue numbers, created when a plan is approved, so commits link to
   their task on GitHub (see Conventions).
7. The data comes from Striker (`striker serve`, see striker-go-bundle); `lcars/bridge.js` only draws.
8. The coordinator starts every task agent (implementer, reviewer, …) as its own Herdr terminal
   with `hc run` and the labels below, instead of in-process subagents. This is part of this work:
   a coordinator wrapper or skill variant in the Striker part of this repo (`striker/`), plus the
   matching section of the striker brief.
9. Works on macOS and Windows, for local and remote (remote1) worktrees.

## Non-goals

- A PROJECT view across several repos. A project may span repos; that is a separate, later view.
- Editing specs or plans in the Bridge. It reads; editing happens in the terminals.
- Replacing Herdr's worktree dialog: new/open/remove worktree stay Herdr's (right-click menu,
  new-menu).

## Anti-goals

- A view that is wrong in silence: missing or unparsable data shows as such ("NO PLAN", "PLAN
  UNREADABLE: …"), never as an empty board that looks like "nothing to do".
- New colours with new meanings. All colours follow `docs/lcars-colors.md` and `lcars/palette.js`.
- Polling that makes the window redraw while nothing changed (same rule as `striker.js`: only a
  changed answer is written to shared state).

## Look (decided with the person, mockups v1–v8)

Colours by role (`lcars/palette.js`):

| Element | Role | Hue |
|---|---|---|
| Plan elbow, spec elbow, `main` in the tree | `frame` | orange |
| Open (expanded) task row, current step | `active` | orange text on black |
| WORKING heading, running task nodes, branch with running work | `working` | blue |
| NEEDS YOU heading, blocked task nodes | `alert` | red |
| DONE heading, done nodes, merged branches | `done` | gold |
| Not-started tasks, planned branches and nodes (dashed) | `neutral` | gray |
| Diff: added text | `working` | blue |
| Diff: removed text | `neutral` | gray, struck through |

Layout, PLAN mode:
- Tab bar: pseudo-tabs PLAN and SPEC first (active one orange, other peach), then the space's real
  tabs. No index numbers in tabs or headings.
- Left: an LCARS elbow (rounded outer top-left, concave inner corner) in `frame`. Its arm carries
  the plan title; clicking the title shows the coordinator in the bottom field. Plan text is black
  on orange, rendered from markdown. Each task is one row: a state oval, the task heading, a
  chevron. Clicking a row expands it (its steps with checkboxes, the current step marked) and
  collapses any other. Rows are separated by darker orange lines.
- Right: three columns, each only a square coloured heading (WORKING, NEEDS YOU, DONE) with black
  below; rows separated by dark lines. A task's card sits in the column of its state, on the same
  row, as tall as the row. Not-started tasks have no card. A WORKING card shows agent tab, step
  n/m and a step bar; clicking it shows that agent's terminal in the bottom field.
- Plan and status columns scroll together when the rows do not fit.

Layout, SPEC mode: the elbow is narrower and lists the spec's `##` headings; the one last selected is
inverted (Herdr's ScrollHandle cannot report what is in view — found in Task 9); a changed section shows `+a −r`. The reading area replaces the columns and renders the
spec. Clicking a heading scrolls to it.

Bottom field: a thin rule labelled REPO (or COORDINATOR · <plan>, or <task> · <agent tab> · LIVE),
the content below, and a round `◂ REPO` button at its right end whenever it does not show the
tree.

## Conventions the Bridge relies on

These are new and part of this work (documented in the striker brief and implemented by the
coordinator variant of Goal 8):

1. **Spec header lines.** Under the title, a spec may carry:
   - `Depends on: <topic>[, <topic>…]` — other plans of the same repo that must be merged first.
   - `Branch: <name>` — the worktree branch this plan is built on. Default: the topic.
   The topic is the spec file name without date and `-design.md`
   (`2026-11-21-striker-go-bundle-design.md` → `striker-go-bundle`).
2. **Spec ↔ plan pairing** by topic: `docs/superpowers/plans/<date>-<topic>.md`.
3. **Tasks** are the plan's `### Task #<issue>: <title>` sections; their steps are the `- [ ]` /
   `- [x]` lines inside. Task numbers are GitHub issue numbers of the repo, so they are unique across
   all plans of the repo and GitHub links commits to them. A task may say
   `Depends on: #41, #43` on a line of its own; without it a task depends on the one before. Tasks
   whose dependencies are all met may run in parallel.
   A plan that has not been approved yet still uses `### Task N:` (N counting from 1 in that plan).
4. **Issues are created when a plan is approved.** `striker issues <plan file>` creates one GitHub
   issue per `### Task N:` section (`gh issue create` in the repo): title = task title, body = a
   short description of the task (its first paragraph) and a link to the task's section in the plan
   on the default branch. It then rewrites each heading to `### Task #<issue>:` and the plan's
   `Depends on: Task N` lines to `#<issue>`, and commits the plan. Running it twice creates nothing
   new (headings that already carry `#` are skipped).
5. **Terminals** started for a plan carry Herdr/core labels:
   - coordinator: `striker.coordinator=<topic>`
   - task agents: `striker.task=<issue>` (implementer, reviewer, …; `striker.role=<role>` optional)
6. **Commits** made for a task reference its issue: `Refs #<issue>` in the message (GitHub links it;
   `Fixes #<issue>` on the commit that completes the task closes it). Commits without a reference
   still show in the tree, unlabelled.

## Data (Striker, `striker serve`)

New endpoints next to `GET /status` (same server, 127.0.0.1 only):

- `GET /repo?machine=<m>&path=<repo path>` → the REPO tree and every plan of the repo:
  - `trunk`: default branch name and its recent commits (bounded, e.g. last 30).
  - `plans[]`: one per spec/plan topic found on the default branch or on any worktree branch:
    `topic`, `title`, `spec`/`plan` file paths, `depends_on[]`, `branch`, `worktree` (path or null),
    `state` (`merged` | `active` | `planned` | `blocked`), `blocked_by[]`, `tasks[]`
    (`issue` (number, or null before approval), `n` (position in the plan), `title`, `steps_done`, `steps_total`, `depends_on[]`, `state`: `todo` | `working` |
    `needs_you` | `done`, `agents[]`: terminal address, role, status), `commits[]` (sha, subject,
    `issue` from `Refs #`/`Fixes #`).
  - `merged[]`: branches already merged, with their merge point, for the solid past part.
- `GET /doc?machine=<m>&path=<worktree path>&file=<repo-relative md>` → `text` (working tree),
  `base` (HEAD), and `hunks[]` so LCARS can mark added/removed lines.
- `GET /space?machine=<m>&path=<worktree path>` → which topic this worktree builds (via `Branch:`
  or topic = branch name), its coordinator terminal, and its tasks (same shape as in `/repo`).

Task state, in this order: `needs_you` if an agent labelled for it is `blocked`; `working` if one
is running; `done` if all steps are checked; else `todo`.
Plan state (decided in review, not by the person — check):
- `merged` (finished) if every task is done: for an approved plan, every task issue is closed; for
  a plan without issues, every step is checked on the default branch. This holds whether the work
  came in through a merge, a squash or rebase merge, or straight commits on the default branch, and
  after the branch is deleted.
- `active` if not finished and a worktree for its branch exists;
- `blocked` if not finished and a `depends_on` plan is not finished;
- else `planned`.
Where a finished plan meets the trunk in the tree: its branch's merge commit if there is one, else
the last commit on the default branch that references one of its issues, else the commit that
checked its last step.

Scope of "the repo" (decided in review — check): the clone at `path` on `machine`, its local
branches and worktrees, plus its `origin/*` branches (fetched at most every few minutes). A worktree
of the same repo on another machine shows through its pushed branch. Clones on several machines are
not merged otherwise.

Remote worktrees are read the way striker-go-bundle reaches remote1 (through the local `hc`, e.g.
`hc file read --on remote1`, and `git` run on that machine).

## LCARS side

- `lcars/bridge.js` (new, like `striker.js`): polls the three endpoints for the active space (only
  while the Bridge is shown, and `/repo` slower), keeps answers in shared state, draws PLAN, SPEC
  and the bottom field. Markdown via `TextView.markdown`. `TextView` does not take CSS, so
  diffs are drawn per block (decided in review — check): the document is split into markdown
  blocks (paragraph, list, heading, code block); unchanged blocks render as markdown; an added block
  renders as markdown inside a container with a `working` bar on its left and `working` text; a
  removed block renders as plain text in `neutral`, struck through if gpui offers it, else marked
  with a `neutral` bar and a "REMOVED" label. A changed block shows as removed + added.
  Found while building (Task 8): `TextView.markdown` ignores `text_color` and always draws tan.
  Therefore (decided by the controller): the plan's step lines in PLAN mode are plain text, black
  on orange (markdown markers stripped); in SPEC mode unchanged and added blocks use
  `TextView.markdown` on black (its tan matches `ROLE.text`), an added block is marked only by the
  4 px `working` bar on its left (its text stays tan); removed blocks are plain gpui text in
  `neutral` with `line_through()`.
- `layouts/lcars.json`: a new `work-view` child for the Bridge, plus the bottom-field block.
- `lcars/main.js`: the pseudo-tabs PLAN/SPEC in `tabs()`; leaving the Bridge works as leaving
  Striker's board does (any click on a real tab shows the terminals).
- Bottom field switching: the layout cannot swap one block's content directly. Two candidate ways,
  decided by a spike as the first task of the plan:
  1. `layout.set {layout}` with the bottom block rewritten to `{ "terminal": "<address>" }`
     (preferred: works for any terminal, agent-detected or not);
  2. a `{ "agent": "focused" }` block plus `agent.focus` (needs Herdr to detect the agents, and may
     also switch the main tab).
  The spike checks: flicker, whether layout clicks (sizes) survive, whether mod copies restart,
  focus side effects, and that the installed `lcars.json` does not drift from the repo copy in a
  way `install.sh` would undo.
  If both fail (decided by the person): a floating Herdr window (`openOverlay` with
  `kind: "window"` and a `{ "terminal": … }` layout), placed over the bottom field at its size and
  position, with the same ◂ REPO button.

## Approaches considered

- **A. Data in `lcars/bridge.js`** (exec git and read files from the mod): no new dependency,
  but parsing and graph logic would be hard to test.
- **B. Data from Striker Go (chosen by the person).** Parsing, git reading and the graph are tested
  Go code; LCARS only draws; same path to remote machines as the rest of Striker. Costs: depends on
  striker-go-bundle being done; Striker learns about worktrees and plans, not only repos.
- **C. Separate `bridge/` mod**: rejected; it would duplicate palette and shapes for an
  LCARS-only view.

## Constraints

- macOS and Windows (README); remote machine remote1.
- Herdr mod API as in `~/.config/herdr-app/herdr.d.ts` and `AGENTS.md`.
- Builds on striker-go-bundle (Go program, `striker serve` on 127.0.0.1:7522).

## Left to the builder

- Exact polling intervals and caching in Striker.
- Graph layout algorithm (lane assignment) as long as: trunk straight, one lane per plan branch,
  extra lanes for parallel tasks, dependencies respected, about 16 px per lane.
- How `/doc` computes hunks (`git diff` parsing or a Go diff library).
- Row heights and scrolling details within the look above.

## Spike result

Tested in the running Herdr app with a scratch `spike` child in `work-view` (a mod block above a 130 px `spike-bottom` block). Test terminal: local plain zsh `t2`.

**1. `layout.set` swaps `spike-bottom.content` to `{ "terminal": "t2" }`.**
- Terminal visible: yes, in the bottom field at the right place.
- Sidebar width, tabs and other blocks: unchanged (yes).
- File on disk changed: yes. `layout.set` rewrites `layouts/lcars.json`, so a live switch rewrites the person's file.
- `top` mod state and mod copy restarts, and flicker: not observable via screenshots; the other lcars blocks looked identical before and after. Not verified.
- Swap back to the mod block: not recorded as a separate step (the test went on to step 3 via a second `layout.set`, which also worked).

**2. `{ "agent": "focused" }` plus `agent.focus {terminal}`.**
- With no agent: the block shows "no agents yet. start claude, codex or pi in a terminal".
- `agent.focus {"terminal":"t1"}` showed t1 in the bottom field: yes, and it works for an unlabelled plain shell (yes).
- Main tab/space switched: YES. `agent.focus` also selected t1's space (epsilon), so the main view changed. This fails the "no switching" criterion.

**3. Window fallback (`openOverlay kind:"window"`, scratch mod, `at` = {x:222,y:902}, 1494x130).**
- Placement: accurate to the bottom field (top left and size matched within a pixel or two in the screenshot), but it draws a window frame and a close button.
- Follows a window resize: NO. After `window.maximize` the window stayed at fixed coordinates and overlapped the bottom bar. It does not follow the layout.
- Not closeable from the CLI without its id (`window.close` closes the app window; `overlay.close` without id does not close windows). It went away on layout switches.

**Chosen mechanism: `layout.set`.** It shows the terminal without disturbing sidebar or other blocks, and works for plain shells. `agent` is rejected because it switches the main space. Window is rejected because it does not track resizes. Caveats: it rewrites the layout file on disk, and mod restarts/flicker were not measured.
