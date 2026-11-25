# Repo overview on OpenSpec — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move herdr-mods onto OpenSpec (store) with superpowers (process), add a coordinator per change that reports to Striker, and give every space an OVERVIEW mode — same structure as PLAN, one level up — fed by a new Striker `/overview` endpoint; plus three small fixes.

**Architecture:** A new Go package reads an OpenSpec store from git (any ref) without Node. `repoview` picks the OpenSpec reader or the legacy reader per repo and maps changes onto the existing `PlanView`/`TaskView` shapes, so `/repo`, `/space`, the tree and PLAN keep working. Striker gains coordinator discovery, `start-coordinator` and `/overview`. `lcars/bridge.js` adds the OVERVIEW mode and maps spaces to repos through `/status`.

**Tech Stack:** Go 1.23+ stdlib only; `git`, `gh`, `hc`, `herdr-app` as programs; gpui-kit JavaScript; OpenSpec CLI (only on the person's machine, for init/validate/list/archive).

**Spec:** `openspec/changes/repo-overview/design.md` (layout: `.superpowers/brainstorm/58621-1795303178/content/overview-layout-v2.html`)
**Depends on:** lcars-bridge (done), striker-go-bundle (done except the ps1 removal).

## Global Constraints

- Store layout exactly as in the spec ("Store layout and conventions"): `openspec/specs/<capability>/spec.md`, `openspec/changes/<name>/{.openspec.yaml, proposal.md, design.md, specs/…, tasks.md, plan.md}`, `openspec/changes/archive/<YYYY-MM-DD>-<name>/`.
- Change metadata only as `x-depends_on: [<change>]` and `x-branch: <name>` in `.openspec.yaml` (default branch: the change name).
- Task line: top-level `- [ ] <id> <title>` in `tasks.md`, optional suffix `(#<issue>; after <id>, <id>)`; no `after` → depends on the previous task in the file. Detailed steps per task in `plan.md` under `### Task <id>: <title>`.
- Change state: archived → `done` (date from folder); active → `needs_you` (coordinator needs_you/blocked) > `in progress` (any tick or worktree) > `blocked` (an `x-depends_on` change not archived) > `planned`.
- Liveness: a change is live while its coordinator (`striker.coordinator=<change>`), one of its task agents (`striker.task=…` + `striker.topic=<change>`) runs, or the lead works (`working`) with its status `task` naming the change. Task `interrupted` = change has ticked and open tasks, task is the first open one, change not live.
- Coordinator status file: `<status_dir>/<repo>--<change>.json`, lead schema plus `"change"`; terminal labels `striker.coordinator=<change>`, `striker.repo=<repo>`; tab name `coord <change>`.
- `/overview` keys exactly: `briefing`, `context`, `inventory` (`capabilities`, `changes`, `archived`, `legacy`), `open_items`, `warnings`; arrays never null; same Host + worktree-top guards as `/doc`; cache repo parts 60 s, lead/coordinator status 10 s.
- REGISTER items: entries under headings containing "Open items", "Offene Punkte" or "Conflict" (case-insensitive); struck-through or resolved/decided/closed/erledigt/entschieden left out; anchor = explicit `{#id}` else GitHub slug.
- Colours only via `lcars/palette.js` roles; INTERRUPTED `ROLE.neutral`, `ROLE.alert` only when listed in needs_you.
- `lcars/main.js` CRLF, other JS LF; `herdr-app check /Users/you/.config/herdr-app/mods/lcars` must pass; run `/Users/you/herdr-mods/install.sh` before screenshots.
- Commands for agents: one plain Bash command per call with absolute literal paths — no `$(...)`, backticks, heredocs, `$VAR`, loops, subshells, `cd … &&`, globs, redirection (incl. `/dev/null`); read with Read; `grep -n "x" /abs/file` allowed. Go via `go -C /Users/you/herdr-mods/striker …`.
- Commits end with `-m "Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>" -m "Claude-Session: https://claude.ai/code/session_01G8gkp5AjppBvGe5M9XdbSz"`. Never push.

## Review Focus

1. A repo without `openspec/` (alpha, beta today) keeps working: `/repo`, `/space`, tree and PLAN use the legacy reader; `/overview` lists legacy specs/plans under `inventory.legacy` and says `NO OPENSPEC`. Test in Task 3 and Task 5.
2. A change whose branch copy of `tasks.md` differs from main's: the branch copy wins (ticks show on main's overview before merge). Test in Task 2.
3. Malformed task suffixes (`(#x)`, `(after 9.9)` naming a missing id, two suffixes): line still counts as a task, bad parts ignored with a warning, no panic. Test in Task 2.
4. A coordinator status file that is missing or invalid JSON while its terminal runs: coordinator shown with live state, `status: {"error": …}`, overview still renders. Test in Task 4.
5. A space whose name matches no `fleet.json` entry and has no local lead (a fresh local repo): Bridge falls back to the space folder, and if that is not a repo shows `NO REPO FOR THIS SPACE`, never a raw error. Test in Task 6.

---

### Task 1: herdr-mods on OpenSpec, workflow in AGENTS.md and briefs

**Files:**
- Create: `openspec/` (via `openspec init --tools none`), `openspec/changes/archive/2026-11-21-lcars-bridge/{.openspec.yaml,proposal.md,design.md,tasks.md,plan.md}`, `openspec/changes/striker-go-bundle/{…}`, `openspec/changes/repo-overview/{…}`, `AGENTS.md`, `CLAUDE.md`
- Modify: `striker/briefs/lead.md` (rule 5), `striker/briefs/coordinator.md`, `README.md` (one paragraph: where specs/changes live)
- Delete: `docs/superpowers/specs/*` and `docs/superpowers/plans/*` of the three topics after their content moved (git mv where possible); the `openspec-spike` worktree and branch; `.superpowers/openspec-tool` stays (CLI).

- [ ] **Step 1:** `/Users/you/herdr-mods/.superpowers/openspec-tool/node_modules/.bin/openspec init --tools none --scope project --no-animation /Users/you/herdr-mods` (telemetry already off). Expected: `openspec/config.yaml`, `openspec/specs/`, `openspec/changes/archive/`.
- [ ] **Step 2:** Move the three designs: `lcars-bridge` → archived change `2026-11-21-lcars-bridge` (`design.md` = the lcars-bridge spec, `plan.md` = its plan, `tasks.md` = its 11 tasks all ticked, `proposal.md` = Why + What changes + `skip_specs: true` in `.openspec.yaml`); `striker-go-bundle` → active change (all tasks ticked except one line `- [ ] 7.2 Remove striker/scripts/*.ps1 after Windows is confirmed`); `repo-overview` → active change (`design.md` = the overview spec, `plan.md` = this plan, `tasks.md` = one line per task of this plan, Task 1 ticked in its own commit, `x-depends_on: [lcars-bridge]`). Front matter is dropped from moved files.
- [ ] **Step 3:** `AGENTS.md` with a section "Arbeitsweise: OpenSpec + Superpowers" (German) stating: store = OpenSpec layout; brainstorming writes `proposal.md` + `design.md` (+ spec deltas), planning writes `tasks.md` + `plan.md`, ticking in the same commit, finishing = `openspec validate` + `openspec archive <name> --yes` on main after merge; task suffix syntax; `x-` metadata; coordinators per change via `striker start-coordinator`; the plain-command rule. `CLAUDE.md` = one line including `AGENTS.md` (`@AGENTS.md`). Same mapping into `striker/briefs/lead.md` rule 5 and `striker/briefs/coordinator.md` (coordinator writes `<status_dir>/<repo>--<change>.json`, escalates via its `needs_you`, ticks `tasks.md`).
- [ ] **Step 4:** `…/openspec validate --all --strict` from `/Users/you/herdr-mods` → all valid; `…/openspec list --json` shows `striker-go-bundle` and `repo-overview`. `git -C /Users/you/herdr-mods worktree remove --force /Users/you/herdr-mods/.superpowers/wt-openspec`, `git -C /Users/you/herdr-mods branch -D openspec-spike`.
- [ ] **Step 5:** Commit `openspec: herdr-mods store, workflow in AGENTS.md and briefs`; then tick Task 1 in `openspec/changes/repo-overview/tasks.md` and commit `openspec: repo-overview task 1 done`.

### Task 2: OpenSpec reader (Go)

**Files:**
- Create: `striker/internal/openspec/openspec.go`, `striker/internal/openspec/openspec_test.go`

**Interfaces:**
- Consumes: `gitrepo.Repo` (`ListFiles(ref, dir)`, `Show(ref, path)`, `ReadWorking(rel)`).
- Produces:
  - `openspec.Task{ID, Title string; Done bool; Issue int; After []string; Body string}` (Body = the `### Task <id>:` section of plan.md, "" if absent)
  - `openspec.Change{Name string; Archived string /* date or "" */; DependsOn []string; Branch string; Tasks []Task; Toc []string /* "## " headings of proposal.md then design.md */; Design, Proposal, TasksPath, PlanPath string; Warnings []string}`
  - `openspec.Capability{Path string; Requirements int}`
  - `openspec.Store{Changes []Change; Capabilities []Capability; Warnings []string}`
  - `openspec.ParseTasks(text []byte) ([]Task, []string)` (pure), `openspec.ParseMeta(text []byte) (dependsOn []string, branch string)` (pure, reads `x-depends_on`/`x-branch`)
  - `openspec.Read(r gitrepo.Repo, ref string) (Store, bool /* has openspec/ */, error)` — `ref` "" = working tree.
  - `openspec.Merge(main Store, branches map[string]Store) Store` — for each active change, the store of the branch named by its `Branch` wins for `tasks.md`/`plan.md`.

- [ ] **Step 1: Failing tests:** `TestParseTasksSuffix` (`- [x] 2.1 A`, `- [ ] 2.3 B (#42; after 2.1, 2.2)` → B Issue 42, After ["2.1","2.2"]; C with no suffix → After [previous id]); `TestParseTasksMalformed` (Review Focus 3: `(#x)`, `(after 9.9)`, two suffixes → still tasks, warnings non-empty); `TestParseTasksNestedIgnored` (indented `  - [ ]` not counted); `TestParseMeta` (`x-depends_on: [a, b]`, `x-branch: br`; unprefixed keys ignored); `TestReadArchivedDate` (fake runner: `changes/archive/2026-11-21-lcars-bridge/` → Archived "2026-11-21"); `TestMergeBranchWins` (Review Focus 2: main 0/3 ticked, branch 2/3 → merged 2/3); `TestCapabilitiesCount` (spec with 2 `### Requirement:` → 2).
- [ ] **Step 2:** `go -C /Users/you/herdr-mods/striker test ./internal/openspec/` → FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `go -C /Users/you/herdr-mods/striker test ./...` → PASS; `gofmt -l /Users/you/herdr-mods/striker` empty.
- [ ] **Step 5:** Commit `striker: OpenSpec reader`.

### Task 3: repoview on OpenSpec (reader choice, states, liveness)

**Files:**
- Modify: `striker/internal/repoview/repoview.go`, `striker/internal/repoview/gather.go`, tests

**Interfaces:**
- Consumes: `openspec.Read/Merge/Store/Change/Task`; existing `Inputs`, `Agent`, `Build`.
- Produces: `Inputs.OpenSpec *openspec.Store` (nil → legacy path unchanged), `Inputs.Coordinators []Coordinator` (`Coordinator{Change, Address, State string; NeedsYou int; Status json.RawMessage}`), `Inputs.LeadTask, LeadState string`; `PlanView` gains `Archived string` (`archived`), `Toc []string` (`toc`), `InterruptedAt string` (`interrupted_at`, task id), `Legacy bool` (`legacy`); `TaskView` gains `ID string` (`id`). For OpenSpec changes: `Topic`=change name, `Spec`=design.md path, `Plan`=tasks.md path, `Tasks` from `openspec.Task` (`N` = index+1, `DependsOn` = indexes of `After`, `StepsDone/Total` = 1/1 or 0/1, `Body` = plan.md section).
- Rules exactly as Global Constraints (change state, liveness, interrupted).

- [ ] **Step 1: Failing tests:** `TestOpenSpecArchivedDone`; `TestOpenSpecInProgressByTick`; `TestOpenSpecBlockedByDependency`; `TestOpenSpecNeedsYouFromCoordinator` (coordinator NeedsYou 1 → state needs_you); `TestInterruptedWhenNotLive` (ticks 2/4, no coordinator/agent/lead → InterruptedAt "3", task 3 state interrupted); `TestNotInterruptedWhenLeadWorks` (LeadState working, LeadTask contains change name → no interrupted); `TestLegacyRepoUnchanged` (Review Focus 1: OpenSpec nil → existing tests' outputs identical).
- [ ] **Step 2:** `go -C /Users/you/herdr-mods/striker test ./internal/repoview/` → FAIL.
- [ ] **Step 3:** Implement; `Gather` reads `openspec.Read` on the default branch and on every branch with a worktree or `origin/` counterpart, merges with `openspec.Merge`, sets `Inputs.OpenSpec` when `openspec/` exists.
- [ ] **Step 4:** `go -C /Users/you/herdr-mods/striker test ./...` → PASS (graph and bridge tests unchanged).
- [ ] **Step 5:** Commit `striker: repo view reads OpenSpec changes`.

### Task 4: Coordinators — discovery, status, start-coordinator

**Files:**
- Modify: `striker/internal/status/status.go` (+tests), `striker/cmd/striker/main.go`
- Create: `striker/internal/coord/coord.go`, `striker/internal/coord/coord_test.go`

**Interfaces:**
- Consumes: `hc.Client` (List, Get, Label, ReadFile), `fleet.Fleet`, `openspec.ParseMeta`.
- Produces:
  - `status.Repo` gains `Coordinators []Coord` (`coordinators`) with `status.Coord{Change, Address, State string; Status json.RawMessage}` and `SpacePath string` (`space_path`, = fleet path); local repos found via lead labels get their lead cwd.
  - `coord.Argv(repo, change, worktree, machine string) ([]string /* herdr-app do args */, error)`; `coord.Start(c hc.Client, run func(args ...string) ([]byte, error), f fleet.Fleet, repo, change, briefPath string) (address string, err error)` — ensures worktree (`git worktree add <repo>-<change> <x-branch>` via gitrepo on the repo machine) and a space for it, opens tab `coord <change>` running Claude (root via `sudo -i` like start-lead), labels `striker.coordinator=<change>`, `striker.repo=<repo>`, sends the coordinator brief with `{repo} {change} {status_file}` filled.
  - CLI `striker start-coordinator <repo> <change>`.
- [ ] **Step 1: Failing tests:** `TestBuildFindsCoordinators` (fake hc: terminal with labels `striker.coordinator=omega`, `striker.repo=alpha` → alpha.Coordinators[0].Change "omega"); `TestCoordinatorStatusInvalid` (Review Focus 4: invalid JSON → `{"error":…}`, coordinator still listed); `TestSpacePathFromFleet`; `TestStartCoordinatorArgv` (tab name `coord omega`, labels set, worktree created when missing); `TestStartCoordinatorRefusesDuplicate` (existing coordinator for change → error).
- [ ] **Step 2:** `go -C /Users/you/herdr-mods/striker test ./internal/status/ ./internal/coord/` → FAIL.
- [ ] **Step 3:** Implement; wire the CLI.
- [ ] **Step 4:** `go -C /Users/you/herdr-mods/striker test ./...` → PASS; `/Users/you/herdr-mods/striker/bin/striker start-coordinator --help` prints usage (no live start).
- [ ] **Step 5:** Commit `striker: coordinators per change report to Striker`.

### Task 5: `/overview` endpoint

**Files:**
- Create: `striker/internal/overview/overview.go`, `striker/internal/overview/overview_test.go`
- Modify: `striker/internal/bridge/bridge.go` (mount + cache), `striker/cmd/striker/main.go` if needed

**Interfaces:**
- Consumes: `repoview.View`, `openspec.Store`, `status.Repo` (lead status, coordinators), `gitrepo.Repo`.
- Produces: `overview.Build(v repoview.View, st openspec.Store, has bool, lead json.RawMessage, coords []status.Coord, commits []gitrepo.Commit, ctx *Context, register []byte) Overview` (pure) with JSON exactly per spec; `overview.ParseRegister(text []byte) []Item`; `overview.ContextFrom(text []byte, file string) *Context`; `bridge` serves `GET /overview` (guards, 60 s / 10 s split cache).
- [ ] **Step 1: Failing tests:** `TestRegisterOpenItems` (beta-like `## 13 Open items {#13}` with `### OI-29 …`, `### ~~OI-28~~` → only OI-29, anchor `13`/explicit id); `TestRegisterGermanAndConflicts`; `TestContextVisionFirst` (VISION → title + first paragraph; missing → SPEC → README → nil with warning `NO VISION FILE`); `TestOverviewLegacyRepo` (Review Focus 1: has=false → `inventory.legacy` filled, warning `NO OPENSPEC`); `TestOverviewProgressText` (lead progress as string kept as text); `TestOverviewEndpointGuards` (wrong Host 403; not worktree top → error JSON); `TestOverviewArraysNotNull`.
- [ ] **Step 2:** `go -C /Users/you/herdr-mods/striker test ./internal/overview/ ./internal/bridge/` → FAIL.
- [ ] **Step 3:** Implement and mount.
- [ ] **Step 4:** `go -C /Users/you/herdr-mods/striker test ./...` → PASS; `/Users/you/herdr-mods/install.sh`; `curl -s "http://127.0.0.1:7522/overview?machine=&path=/Users/you/herdr-mods"` → JSON with `inventory.changes` containing `repo-overview` and `striker-go-bundle`, `inventory.archived` containing `lcars-bridge`.
- [ ] **Step 5:** Commit `striker: overview endpoint`.

### Task 6: Fixes — space mapping, crew labels, lead tab name

**Files:**
- Create: `lcars/space.js` (LF, no gpui imports)
- Modify: `lcars/bridge.js`, `lcars/main.js` (CRLF), `lcars/bridge.check.mjs`, `striker/internal/lead/lead.go` (+test)

**Interfaces:**
- Produces: `space.js`: `repoPathFor(space, statusRepos, localLeads): {path, machine} | null` — fleet repo whose `space` equals the Herdr space name (`space_path`), else a local lead terminal's cwd in that space, else `space.folder`; `crewLabel(agent, spaceName): string` → `"<SPACE> · <STATE WORD>"`.
- [ ] **Step 1: Failing checks** in `bridge.check.mjs`: remote `alpha` space with cwd `/home/you` + fleet `space: "alpha"`, `space_path: "/srv/alpha"` → `/srv/alpha`; no match + local lead → its cwd; neither → folder; Review Focus 5: folder not a repo → Bridge shows `NO REPO FOR THIS SPACE` (assert via the text function used by bridge.js). Go: `TestStartLeadTabNamedClaude` (space.new argv carries tab name `claude`).
- [ ] **Step 2:** `node /Users/you/herdr-mods/lcars/bridge.check.mjs` and `go -C /Users/you/herdr-mods/striker test ./internal/lead/` → FAIL.
- [ ] **Step 3:** Implement; bridge.js uses `repoPathFor` instead of the first terminal's cwd; main.js crew entry uses `crewLabel`; `lead.Start` names the tab `claude`.
- [ ] **Step 4:** checks + tests PASS; `herdr-app check …` → `check passed`; install.sh; screenshot of the `alpha` space with PLAN open shows no worktree error.
- [ ] **Step 5:** Commit `lcars: spaces map to repos via fleet; crew labels; lead tab claude`.

### Task 7: OVERVIEW mode in the Bridge

**Files:**
- Modify: `lcars/bridge.js`, `lcars/main.js` (CRLF, pseudo-tab), `lcars/tree.js` only if needed, `lcars/bridge.check.mjs`, `docs/lcars-colors.md` (OVERVIEW shapes)

**Interfaces:**
- Consumes: `/overview` JSON (Task 5), `repoPathFor` (Task 6), existing PLAN drawing helpers, `setBottom`.
- Produces: pseudo-tabs `OVERVIEW · PLAN · SPEC`; `BRIDGE_KEY` value `"overview"`; mod command `lcars.overview`; OVERVIEW default when `git rev-parse --git-common-dir` of the space path is `<path>/.git`.

Layout exactly as the spec's LCARS section (mockup v2): arm = repo name + briefing line; rows Vision / one per change (oval, name, `done/total`, "after X"; open = TOC then tasks with ticks and `◂ NOW`) / Register / Done / capabilities line; columns WORKING/NEEDS YOU/DONE with coordinator cards (click → `setBottom` coordinator terminal); click a change with a worktree space → `space.select` + PLAN; change without worktree → PLAN read-only (no cards, no coordinator click); bottom field = REPO tree unchanged; INTERRUPTED neutral with last ticked task.

- [ ] **Step 1: Failing checks** in `bridge.check.mjs` for the pure parts: `overviewRows(overview)` order (Vision, active changes, Register, Done), `coordCard(change)` column choice (needs_you > working > done), `interruptedText(change)`.
- [ ] **Step 2:** `node /Users/you/herdr-mods/lcars/bridge.check.mjs` → FAIL.
- [ ] **Step 3:** Implement the mode.
- [ ] **Step 4:** checks PASS; `herdr-app check …` → `check passed`; install.sh; screenshots (`/Users/you/herdr-mods/.superpowers/shots/overview7.png`) of the herdr-mods space on OVERVIEW: rows Vision (README fallback), striker-go-bundle (1 open), repo-overview (in progress), Done lcars-bridge; tree below.
- [ ] **Step 5:** Commit `lcars: overview mode`.
