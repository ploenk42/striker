# Repo overview on OpenSpec: briefing for a space

## Why

A Herdr space is first of all a repo (its main checkout), not a worktree. When the person opens a
space they want, at a glance: where the work stopped and what comes next, how that sits in the
whole repo (alpha and beta start from a vision, `docs/VISION.md` / `docs/SPEC.md`, from which the
designs follow), which specs, changes and tasks exist and how far each is, and what waits on them.
The LCARS Bridge shows one worktree's plan and says `NO PLAN FOR BRANCH main` on a main checkout.

In alpha and beta plan checkboxes were never ticked ("plans are protocols"), so nothing in the repo
records how far a plan got, and a session that dies mid-task loses that state. The person decided
that progress is written back into the repo, and — after an OpenSpec spike (branch
`openspec-spike`) — that repos keep their specs, changes and tasks in **OpenSpec**, while the
**superpowers** skills stay the way of working (brainstorming, planning, TDD, subagents, reviews).

## Goals

1. **OpenSpec as the store** in every repo (herdr-mods now, alpha and beta as rollout): the
   living description per capability in `openspec/specs/`, each piece of work as a change in
   `openspec/changes/<name>/`, finished changes archived with their date.
2. **Superpowers write into it** (mapping below): brainstorming produces the change's
   `proposal.md` and `design.md`; planning produces `tasks.md` and `plan.md`; execution ticks
   `tasks.md` in the same commit as the work; finishing archives the change.
3. **An OVERVIEW mode in the Bridge**, the first pseudo-tab before PLAN and SPEC, default on a
   repo's main checkout, with four parts in this order:
   1. **Briefing** — the lead's status (task, progress, note, updated) and the last commits.
   2. **Context** — title and first paragraph of `docs/VISION.md`, else `docs/SPEC.md`, else
      `README.md`; a link opens it in SPEC mode.
   3. **Inventory** — capabilities (`openspec/specs/`), active changes with `done/total`, worktree,
      agent and INTERRUPTED, and recently archived changes with their date. Clicking a change
      opens PLAN for it.
   4. **Open items** — the lead's `needs_you` and the unresolved REGISTER entries.
4. **INTERRUPTED** made visible: an active change with ticked and open tasks and nobody working
   on it (state rules below).
5. **One coordinator per change, reporting up to Striker** (the person needs parallel changes per
   repo). Chain: person ⇄ Striker ⇄ lead per repo ⇄ coordinator per change ⇄ task agents.
   - `striker start-coordinator <change>` (run by the lead, or by Striker on the person's word) opens
     a tab named `coord <change>` in the change's worktree space on the repo's machine (creating
     the worktree from `x-branch` and the space if missing), starts Claude there with the
     coordinator brief, and labels the terminal `striker.coordinator=<change>` and
     `striker.repo=<repo>`.
   - Each coordinator keeps a status file `<status_dir>/<repo>--<change>.json`, same schema as the
     lead's plus `"change": "<name>"`; everything it cannot decide goes into its `needs_you`.
   - `striker status` and `/status` collect coordinators next to leads; the Striker board shows them
     under their repo; Striker presents their `needs_you` together with the leads' and sends the
     person's answer with `striker send` to the right coordinator.
   - Task agents that block are seen by their coordinator (`hc wait`), which decides or escalates
     through its `needs_you`; they never escalate past it.
6. **Fixes** bundled here: the Bridge maps a space to its repo through `fleet.json` (not the first
   terminal's cwd, which is `/home/<user>` behind `sudo -i`); crew entries read
   `<SPACE> · <STATUS>`; a lead's Claude tab is named `claude`.

## Non-goals

- A view across several repos ("project" spanning repos). Later, separate.
- OpenSpec's own skills (propose/apply/archive) as the way of working: `openspec init` is run with
  `--tools none`, the superpowers skills drive the work; the `openspec` CLI is used for
  `validate`, `list` and `archive` only.
- GitHub Projects as source of state. GitHub issues stay optional per task.
- Editing from the overview; summarising with a model at view time.

## Anti-goals

- Wrong in silence: a repo without OpenSpec, vision or register shows what it has and says what is
  missing (`NO OPENSPEC`, `NO VISION FILE`), never an empty or misleading board.
- Two sources of truth: no roadmap file, no status lines duplicating what OpenSpec already says.

## Store layout and conventions

```
openspec/
  config.yaml
  specs/<capability>/spec.md                 living requirements (SHALL + scenarios)
  changes/<name>/
    .openspec.yaml                           OpenSpec metadata (+ depends_on, branch — see 3)
    proposal.md                              why, what changes, capabilities, impact
    design.md                                the design (what superpowers brainstorming writes)
    specs/<capability>/spec.md               spec deltas (ADDED / MODIFIED / REMOVED)
    tasks.md                                 checklist — the state of the change
    plan.md                                  the superpowers implementation plan (steps, files, tests)
  changes/archive/<YYYY-MM-DD>-<name>/       finished changes
```

1. **State comes from OpenSpec, nothing else:** an archived change is done (date from the folder
   name); an active change is `in progress` when any task is ticked or a worktree exists for its
   branch, `blocked` when a `depends_on` change is not archived, else `planned`. The YAML front
   matter convention of the earlier draft is dropped.
2. **Tasks** are the top-level checkbox lines of `tasks.md` (`- [ ] 2.3 Title`). A task line may
   end with a parenthesis carrying an issue number and dependencies:
   `- [ ] 2.3 OVERVIEW mode (#42; after 2.1, 2.2)`. Without `after`, a task depends on the task
   before it in the file. `striker issues` writes the `#n`; `striker dispatch` takes the task id or the
   issue. Only top-level lines count; detailed steps live in `plan.md` per task (heading
   `### Task 2.3: …`).
3. **Change metadata** goes into the change's `.openspec.yaml` with the `x-` prefix OpenSpec
   reserves for tool metadata: `x-depends_on: [<change>]` and `x-branch: <name>` (default: the
   change name). Verified in the spike: `openspec validate --strict` accepts `x-` keys and warns
   on unprefixed unknown keys.
4. **Ticking:** a task is ticked in `tasks.md` in the same commit as its last piece of work.
   Finishing a change runs `openspec validate` and `openspec archive <name> --yes` on the default
   branch after merge.
5. **Decisions** that need the person go into the lead status file's `needs_you` (short) and, when
   they shape the design, into `docs/REGISTER.md`, as alpha and beta do.
6. **Superpowers mapping**, written into every repo's agent instructions so every agent keeps it
   (the person's request): a section "Arbeitsweise: OpenSpec + Superpowers" in the repo's
   `AGENTS.md` (created in herdr-mods together with a `CLAUDE.md` that includes it; added to the
   existing `AGENTS.md` / `CLAUDE.md` in alpha and beta during rollout), plus the lead and
   coordinator briefs. It notes where the skills' defaults differ:
   - brainstorming → `openspec new change <name>`, then the design doc as `design.md` and a short
     `proposal.md`; spec deltas for capabilities whose behaviour changes;
   - writing-plans → `tasks.md` (one line per task) and `plan.md` (the detailed plan);
   - subagent-driven / executing-plans → tick `tasks.md` per task; coordinator and
     `striker dispatch` read tasks from `tasks.md`;
   - finishing → validate and archive.
7. **Legacy:** repos whose work still lives in `docs/superpowers/specs|plans` are read by the
   existing lcars-bridge parser and shown in the inventory marked `LEGACY`, read-only, until
   migrated. Migration of herdr-mods' own three designs (striker-go-bundle, lcars-bridge, this one)
   is in scope: the first two become archived changes, this one becomes the active change
   `repo-overview`. alpha and beta are migrated by their leads (rollout, not code).

## State rules (tasks)

A change is **live** while any of these runs: its coordinator (`striker.coordinator=<change>`), a
task agent of it (`striker.task=…` with `striker.topic=<change>`), or the repo's lead working (live
state `working`) while its status file names the change in `task`.

Task state, in order: `needs_you` (its task agent is blocked, or the coordinator's `needs_you`
names the task), `working` (its task agent runs), `done` (ticked, or its issue is closed),
`interrupted` (the change has ticked and open tasks, this task is the first open one, and the
change is not live), else `todo`. Inside an archived change every task is done.

Change state: `needs_you` when its coordinator's status has entries in `needs_you` or the
coordinator terminal is blocked; otherwise as in Conventions 1.

## Data (Striker)

Striker reads the files itself (no Node needed on remote machines); the `openspec` CLI is only a
convenience on the person's machine.

`GET /overview?machine=<m>&path=<repo path>`, same server and guards as `/doc` (Host check,
worktree top) →
- `briefing`: lead status (`task`, `progress` as `{done,total}` or text, `note`, `updated`,
  `phase`), lead address and live state, last 5 commits on the default branch;
- `context`: `{file, title, lead}` or null;
- `inventory`: `capabilities` (`{path, requirements}`), `changes` (`{name, state, done, total,
  worktree, agent, interrupted_at, depends_on, branch}` — depends_on/branch from `x-depends_on`/`x-branch`), `archived` (last 10 `{name, date}`),
  `legacy` (legacy specs/plans, if any);
- `open_items`: lead `needs_you` (`source: "lead"`, no id), each coordinator's `needs_you`
  (`source: "coordinator"`, `id: <change>`) and REGISTER entries under headings
  containing "Open items", "Offene Punkte" or "Conflict" (case-insensitive), struck-through or
  resolved/decided/closed/erledigt/entschieden entries left out, each `{source, id, title,
  anchor}` (anchor = explicit `{#id}`, else GitHub slug);
- `warnings`.
Caching: repo parts 60 s, lead status 10 s, merged per response.

`/repo`, `/space` and the tree (graph) switch from the legacy plans to OpenSpec changes: a change
replaces a "plan", tasks come from `tasks.md`, `plan.md` gives PLAN mode its expandable steps.
`/status` gains the repo path per space (fix 6) and a `coordinators` list per repo
(`{change, address, state, status}` from labels and `<repo>--<change>.json` status files, local
and remote, found the way local leads are found today).

## LCARS side

- `OVERVIEW · PLAN · SPEC`; OVERVIEW default on a main checkout. PLAN/SPEC for a change without a
  worktree read the main checkout, read-only (no coordinator, no agent cards).
- Layout (agreed with the person, mockup `overview-layout-v2.html`): the same structure as PLAN,
  one level up.
  - Arm of the elbow: repo name and the briefing line (phase, next step, lead state and age).
  - Elbow rows, accordion like PLAN tasks: first row **Vision** (title; open = first paragraph,
    link to SPEC); then one row per **change** (state oval, name, `done/total`, "after X" when
    blocked; open = its table of contents: `proposal.md`/`design.md` headings, then the
    `tasks.md` lines with ticks and `◂ NOW`); then **Register** (count; open = the open items);
    then **Done** (archived changes, newest first); last a line listing the capabilities.
  - Columns WORKING / NEEDS YOU / DONE as in PLAN; the cards are the **coordinators** (WORKING:
    coordinator, current task, progress bar; NEEDS YOU: the coordinator's question, or the
    register items on the Register row; DONE: archived summary on the Done row). Clicking a
    coordinator card shows its terminal in the bottom field (◂ REPO back), like task agents in
    PLAN.
  - Bottom field: the REPO tree exactly as in the worktree view (it already shows all changes).
- INTERRUPTED shows in `ROLE.neutral` with the last ticked task; `ROLE.alert` only when listed in
  `needs_you`.
- Crew entry `<HERDR SPACE NAME> · <AGENT LIVE STATE WORD>`. `striker start-lead` names the lead
  tab `claude`; existing lead tabs are renamed once during rollout.

## Approaches considered

- **Store:** OpenSpec + superpowers process (chosen by the person after the spike) vs superpowers
  only with YAML front matter (status had to be bolted on; no living spec) vs OpenSpec only with
  its own skills (loses brainstorming/TDD/subagent discipline).
- **Overview:** a mode of the Bridge fed by Striker (my call; reuses data path, guards, look) vs a
  separate page (leaves Herdr) vs a lead-written `STATUS.md` (drifts).

## Constraints

- Builds on lcars-bridge and striker-go-bundle. Go stdlib only; macOS and Windows; `lcars/main.js`
  CRLF, other JS LF; colours via palette roles.
- `openspec` CLI: Node (or the standalone binary from `openspec install`) on the person's machine;
  telemetry off (`openspec config set telemetry.enabled false`).
- Remote repos are root-owned with an ACL for the person's user; Striker reads them via `hc exec` /
  `hc file read`.

## Decisions from the second builder check (the controller's calls — check)

1. **Which copy counts.** A change is created and its `proposal.md`, `design.md`, `tasks.md` and
   `plan.md` are committed on the default branch when approved, so main always knows it (state
   `planned`). Work and ticks happen on `x-branch`. The reader takes `tasks.md` from the change's
   branch when one exists (local, worktree or `origin/`, the same ref scan as the legacy Gather),
   else from the default branch; the branch copy wins. Archive happens on the default branch after
   merge.
2. **Archive on remote repos.** The standalone `openspec` binary (`openspec install`, no Node) is
   installed for the user that runs the leads (root on remote1) during rollout; leads archive
   themselves. Striker never re-implements archive.
3. **Migration.** The spike tree is discarded; herdr-mods gets a fresh `openspec init --tools
   none`. `lcars-bridge` becomes an archived change with `skip_specs: true` (no back-filled
   requirements); `striker-go-bundle` stays an active change with its one open task (remove the
   `.ps1` scripts after Windows is confirmed); this design becomes the active change
   `repo-overview`. Capability specs grow from new changes onward.
4. **Legacy path stays.** For repos not yet on OpenSpec (alpha, beta), `/repo`, `/space`, the
   tree and PLAN keep using the lcars-bridge parser; Striker picks the reader per repo (`openspec/`
   present or not). Both paths stay tested until the rollout is done.
5. **Opening a change.** Clicking a change whose worktree has its own Herdr space selects that
   space and opens PLAN there (full PLAN, cards live). A change without a worktree opens PLAN
   read-only in the current space.

## Left to the builder

- How many commits, archived changes and open items to show before "+N MORE".
- REGISTER parsing beyond the heading rule.
- Whether Striker shells out to `openspec list --json` locally as a cross-check in tests.
