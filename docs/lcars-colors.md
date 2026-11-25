# LCARS colours and shapes

Every colour of the LCARS frame lives in `lcars/palette.js`. `main.js` and `striker.js` import from there and define no colours of their own. Shape and colour each carry one kind of information:

- **Shape** says what a thing is: heading, display, button or list entry.
- **Colour family** says what it means: frame, status, identity or nothing.

## Shapes

| Kind | Shape | Clickable | Examples |
|---|---|---|---|
| Section heading | Small orange square, peach text on black, peach rule to the right. 12 px black above | No | FLEET, CREW. No counts |
| Display | Square block of colour, value bottom-right | No | KIND tags, "no lead" |
| Button | Block of colour with a round end (sidebar: right end, board header and tabs: both ends) | Yes | + NEW SPACE, STRIKER, tabs, TALK TO STRIKER, TERMINALS |
| Machine section | Block of the sidebar in the machine's colour, as tall as its spaces, name top-right, a 2 px rule in the same colour along the top of its rows | No | THIS COMPUTER, REMOTE1 … |
| Space row | Text on black: number, name, an oval for the state on the right | Yes | Each space under its machine |
| List entry | Status strip on the left, two lines of text on black | Yes | Crew members (`LCDR PATA` / `ALPHA · WORKING · OPUS 5.5 · 1.2M`: name and rank from Striker's `/crew`, then repo, state, model and tokens; without Striker the program and tab, then space and state), Striker's lead and coordinator lines |
| Frame | Elbows, arms, curved corners | Only the MENU elbow | Top and bottom of the window, head and foot of each Striker column |
| Striker column head | Frame head: small line who asks (`LCDR PATA · NEEDS YOU`, or `LT WERF · OMEGA · NEEDS YOU` when a coordinator asks), else the lead and its state; repo name below. The one square shape that is clickable | Yes: to whoever asks, else the lead (a question row likewise goes to its asker) | One per fleet repo |

Times on the frame are stardates: the local clock as one number, `YYYYMMDD.hhmmss` (`lcars/stardate.js`). Top
right it is a clock that counts seconds; Striker's heading shows `SCAN <stardate>` of the last scan. On Striker's
board, column heads, lead and coordinator lines name the crew from Striker's `/crew` (`LCDR PATA`,
`LT WERF · OMEGA`); a coordinator line also names its worktree folder. Each column's body scrolls.

Bridge shapes (see "Bridge" below):

| Kind | Shape | Clickable | Examples |
|---|---|---|---|
| Elbow | Orange block, rounded outer top-left corner, concave inner corner, arm on top. Plan text black on orange | Title only (shows the coordinator) | PLAN elbow, narrower SPEC elbow |
| Square column heading | Square block of colour with black below, no round end | No | WORKING, NEEDS YOU, DONE |
| Accordion row | Task row on the elbow: state oval, heading, chevron. Separated by darker orange lines | Yes (expands, collapses the others) | Each plan task |
| Card | Block in a status column, on its task's row, as tall as the row. Every task has one (see "PLAN cards") | WORKING and NEEDS YOU with an agent (shows the agent in the bottom field); never in a read-only plan | WORKING card with step bar |
| Tree mark | Node on a lane of the repo tree: filled, ring or dashed ring | No | Task nodes in REPO |
| Lane label | The change's name and progress on its lane where the branch starts, right-aligned, in the lane's colour; MAIN left of the trunk | No | MAIN, REPO-OVERVIEW 3/5 · THIS PLAN |
| Pseudo-tab | Tab pill before the space's tabs, orange when shown, else peach. On a main checkout PLAN reads the vision file (`VISION · <title>`) and SPEC lists every spec on the elbow (living specs, the active changes' design.md, legacy specs), the shown one open with its headings | Yes | OVERVIEW, PLAN, SPEC |

PLAN cards (`overview.js` `planCard`), one per started or finished task on its row; an open task has no card:

| Task state | Column | Card |
|---|---|---|
| working | WORKING | `working` strip, agent tab · STEP n/m, step bar. Clickable |
| needs_you | NEEDS YOU | `alert` strip, blinking, agent tab · NEEDS YOU. Clickable |
| interrupted (the first open task after ticks, nobody on it) | WORKING | `neutral` strip, `INTERRUPTED · NOBODY ON IT` |
| todo | — | No card. Its row's oval is orange with a 2 px ink outline (like a planned change) |
| done | DONE | `done` strip, `DONE` |

A read-only PLAN (a change without a worktree, read from the main checkout) shows the same cards, none clickable.

OVERVIEW shapes (PLAN one level up, mockup `overview-layout-v2.html`):

| Kind | Shape | Clickable | Examples |
|---|---|---|---|
| Briefing arm | The elbow's arm, taller (84 px): repo name in 22 px, the briefing line under it (16 px: phase, next step, progress, lead and age, else the last commit), then the last commits (14 px). The column headings are just as tall, so the concave corner lines up with their bottom edge | No | HERDR-MODS · no lead · last commit … |
| Vision row | Accordion row with a black oval: `VISION · <title>`; open: the first paragraph and a round black `<FILE> IN SPEC ▸` button | Row: expands. Button: SPEC on that file | README fallback |
| Change row | Accordion row: state oval (planned: outline only), name, `done/total`, `AFTER X` while blocked, `READY TO ARCHIVE` when every task is ticked and nobody is on it, a round `PLAN ▸` button (black with orange text on the orange row; orange with black text on the open, black row). Open: the table of contents (`proposal:` / `design:` headings, small), then the `tasks.md` lines with ☑/☐ and `◂ NOW` | Row: expands. PLAN ▸: the worktree's space in PLAN, else PLAN read-only here | repo-overview · 7/7 · READY TO ARCHIVE |
| Coordinator card | Card on its change's row: WORKING (coordinator, its task, tick bar; or a task agent or the lead on it: `NO COORDINATOR`, no click), NEEDS YOU (the coordinator's question), READY TO ARCHIVE (`done`, all ticked, nobody on it) | Yes when a coordinator runs (its terminal in the bottom field, ◂ REPO back) | REPO-OVERVIEW · COORD · TASK 7 · WORKING |
| Interrupted card | Card in WORKING with a `neutral` strip: `INTERRUPTED · LAST DONE <id> <title>`, tick bar in `neutral` (no blue, no orange next segment) | No | A change with ticks and open tasks, nobody on it |
| Register row | Accordion row, oval `alert` with open items, else `neutral`; open: one line per item. Its NEEDS YOU card lists the item ids and the lead's count | Row: expands | REGISTER · 2 OPEN ITEMS |
| Done row | Accordion row, `done` oval, archived names newest first; open: names and dates. DONE card `N DONE · <date>` | Row: expands | DONE · LCARS-BRIDGE |
| Capabilities line | 14 px black text on the elbow under the rows | No | CAPABILITIES: RETRO · REGISTER |

Rule: a round end means "click me". Nothing that cannot be clicked gets a round end, except the frame's own curves.

## Sizes

For touch and legibility the whole layout (frame, sidebar, tab bar, Bridge, Striker's board) uses three constants from `lcars/palette.js`:

| Constant | Value | Means |
|---|---|---|
| `TEXT_MIN` | 14 px | The smallest text anywhere: card status lines, tree labels, the NOW tag, TOC line, capabilities, rule label, error lines, board labels |
| `TEXT_BODY` | 16 px | Row headings, task steps, open-row lines, card titles, the briefing line, tab words, space names, the STRIKER word |
| `TOUCH` | 44 px | The least height (and width, for small buttons) of anything clickable: pseudo-tabs and tabs, PLAN ▸ / IN SPEC ▸ (a 32 px pill in a 44 px hit box), SPEC heading rows, ◂ REPO, Striker's lead, coordinator and NEXT MOVE lines, space and crew rows, + NEW SPACE, the STRIKER slab, the machines popup's rows, the status row's STRIKER, SHEPHERDR and palette cells, the gear |

Titles stay larger (arm title 22 px, column headings 18 px). PLAN rows are 60 px and OVERVIEW rows 64 px, so a card's title, status line and bar fit. The arm is 52 px (84 px in OVERVIEW), the column headings as tall as the arm. In `layouts/lcars.json` the tabs block and the bridge rule are 44 px high, the bridge field 190 px, the bottom block 60 px (a 44 px status row over the 16 px arm). The sidebar scrolls as one area when its spaces and crew outgrow the window; a machine's name stands one word per line in its block.

## Colour families

| Family | Hues | Means | Never used for |
|---|---|---|---|
| Frame (warm) | orange, peach, tan | Structure: elbows, arms, headings, labels. Tan is also content text | Status |
| Status | red, gold, blue | Waits on you, finished, working | Anything else |
| Identity (purple) | lilac, violet, purple | Tells machines apart | Anything else |
| Neutral | gray, dark, black | Nothing to report: inactive, idle, zero, empty track, background | Status that needs attention |

## Roles

| Role | Hue | Hex | Used for |
|---|---|---|---|
| `bg` | black | `#000000` | Background, the 4 px gaps between blocks |
| `ink` | black | `#000000` | Text set on a block of colour |
| `frame` | orange | `#ff9900` | Top elbow, title bar, space name, Striker's title, heading squares |
| `active` | orange | `#ff9900` | Active space, active tab, focused agent, current phase, Striker button while the board shows |
| `alert` | red | `#ff5544` | blocked, NEEDS YOU, ASKS YOU, Striker notes. Blinks |
| `done` | gold | `#ffcc66` | DONE, READY TO MERGE, Striker columns whose phase is done |
| `working` | blue | `#99ccff` | WORKING, progress made |
| `idle` | gray | `#666688` | STANDING BY |
| `hover` | tan | `#ffcc99` | Grey buttons under the pointer. Coloured buttons dim to 85 % instead |
| `text` | tan | `#ffcc99` | Names, tasks, rows, notes, stardate |
| `section` | peach | `#ff9966` | Section heading text and rule |
| `label` | peach | `#ff9966` | PHASE, TASK, PROGRESS, STATUS on Striker's board |
| `neutral` | gray | `#666688` | Inactive, offline, zero counts, plain buttons, filler |
| `track` | dark | `#333344` | Empty part of a phase row or progress bar |

`SPACE_COLORS` is lilac `#cc99cc`, violet `#9999ff`, purple `#aa77cc`, one per machine section. Rose was dropped: next to a blinking red button it read as alert. A space row shows its state in the oval and the name: orange when active, red and blinking when blocked, otherwise the status colour.

## Bridge

The Bridge (`lcars/bridge.js`) adds no hue and no role. It reuses the roles above.

| Element | Role | Notes |
|---|---|---|
| PLAN / SPEC elbow, trunk frame of the tree, tick marks for commits, NOW rule | `frame` | Orange |
| Open task row, active pseudo-tab | `active` | Orange text on black (the row is `bg`, not a frame fill) |
| Task steps on the elbow | `ink` | Plain text, black on orange |
| Column heading WORKING, working lanes, working node ring | `working` | Blue |
| Column heading NEEDS YOU, blocked marker, needs-you node ring | `alert` | Red, blinks |
| Column heading DONE, merged lanes, filled done nodes | `done` | Gold |
| Blocked and planned lanes, todo nodes (dashed ring), removed spec text | `neutral` | Gray. Removed text is struck through |
| Added spec block | `working` | A 4 px bar on its left only. The text stays `text` (tan), because `TextView.markdown` ignores a text colour |
| Spec reader | `text` | Tan content |
| Lane labels | the lane's colour | Same role as the lane's line (`done`, `working` or `neutral`); the MAIN label is `frame` |
| Rule label and line, REPO | `section` | |
| Rule label and line, COORDINATOR · plan | `frame` | Orange |
| Rule label and line, task terminal (#issue · tab · LIVE) | `working` | `alert` for a NEEDS YOU task |

OVERVIEW colours: a change's colour says what is happening (`overview.js` `changeStatus`, the same rule for its oval, its card and its tree lane):

| Status | When | Colour |
|---|---|---|
| needs_you | its coordinator asks the person | `alert`, blinking |
| working | someone is on it now: its coordinator, a task agent of its topic, or the lead working on it (Striker's `live`) | `working` |
| ready | every task ticked, not archived, nobody on it: `READY TO ARCHIVE` | `done` |
| interrupted | ticked and open tasks, nobody on it | `neutral` (oval, card strip and text) |
| blocked | waits for unarchived dependencies | `neutral` |
| planned | nothing ticked, nobody on it | an ink outline only |

Blue never means "in progress" alone: a change nobody works on is gold, gray or an outline. The Vision oval is `ink`. Open rows and their `◂ NOW` line are `active` on black, like PLAN.

Spec mode: the elbow lists the spec's `##` headings; the last one selected is inverted (Herdr's `ScrollHandle` cannot report what is in view). A changed section shows `+a −r`.

Bottom field: a rule block (REPO, or COORDINATOR · plan, or #issue · tab · LIVE; colours in the table above) with a round ◂ REPO button when it is not showing the tree. Under it sits the `bridge-bottom` block, swapped with `layout.set` to a terminal and back.

REPO tree:

- Trunk: straight `frame` line with a tick for every commit, a NOW rule at the head, older history compressed.
- Lanes of OpenSpec changes by the OVERVIEW's rule above: working `working` (only while live), ready and done `done`, needs_you `alert`, interrupted, blocked and planned `neutral`. Legacy plans keep their own states: merged `done`, active `working`, blocked or planned `neutral`. Future lanes are dashed at 45 % opacity.
- Node marks by task state: done filled, working ring, needs_you ring (`alert`), interrupted solid ring (`neutral`, never `alert`), todo dashed ring.
- One lane per change: a change with branch commits shows those as its past and no separate done-task nodes; parallel open tasks get extra lanes below it, without a label.
- Edges between lanes are drawn as git graphs do: over one column a single curve; over more, a merge runs on its own lane and bends into the trunk in the last gap, a fork bends in the first gap and runs on in its new lane, so no edge cuts across other lanes or their nodes.
- Lane labels, 14 px, centred on their lane: MAIN left of the trunk; a change's label on its first lane where the branch starts, right-aligned and ending 8 px before the fork (the source of the edge coming into the lane) or the first node, whichever is further left, so the curve from the fork never runs through it. Text `<TOPIC> <done>/<total>`, then `· THIS PLAN` for the worktree's own plan or `· AFTER <deps>` for a blocked plan; with too little room the suffix goes first, then the progress, then the name is cut with `…`; room for fewer than 4 characters drops the label. `+N LANES` when lanes are folded.
- Spacing: past columns with plan work up to 80 px apart, plain trunk commits 8 px (older ones 3 px); after NOW every open task keeps its node, at most 32 px apart (the gap right after NOW stays wide for the tag).

`layouts/lcars.json` is stored in Herdr's own key order with LF line ends (`.gitattributes`). That keeps a `layout.set` swap back byte-identical to the file. A hand edit must keep that form.

## Rules

1. Status colours (red, gold, blue) appear only through `STATUS`, `ROLE.alert`, `ROLE.done` and `ROLE.working`.
2. Orange is frame or selection, never status.
3. Purple is identity only.
4. A count of zero goes gray, whatever its colour would be.
5. New UI picks a shape first, then a role. Add a role to `ROLE` only when no existing one fits, and add it here.

## Known overlaps

- Idle and inactive look the same (gray). The word STANDING BY tells them apart.
- Completed phases on Striker's board are tan (`C.tan`), so that gold stays reserved for a finished task.
- `layouts/lcars.json` sets Herdr's own `idle` colour to the same gray, so Herdr's widgets match.
