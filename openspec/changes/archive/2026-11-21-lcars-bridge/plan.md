# LCARS Bridge — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Bridge view per worktree in the LCARS layout (PLAN and SPEC modes, a REPO tree of past and planned work, coordinator and task terminals in a bottom field), fed by new Striker endpoints, with GitHub issues as task numbers and a coordinator that runs task agents as labelled Herdr terminals.

**Architecture:** Striker (Go, from striker-go-bundle) reads specs, plans, git and terminal labels, computes plan/task states and the tree layout, and serves `/repo`, `/space`, `/doc` next to `/status`. `lcars/bridge.js` polls those endpoints for the active space and only draws. The bottom field swaps between the REPO tree and a live terminal by the mechanism the first task's spike picks.

**Tech Stack:** Go 1.23+ standard library only; `git`, `gh` and `hc` as external programs; gpui-kit JavaScript (Herdr mod API).

**Spec:** `docs/superpowers/specs/2026-11-21-lcars-bridge-design.md`
**Depends on:** `docs/superpowers/plans/2026-11-21-striker-go-bundle.md` must be done (it creates `striker/`, `hc.Client`, `serve`, `cmd/striker`).

## Global Constraints

- Go: module `github.com/ploenk42/herdr-mods/striker`, standard library only; tests use fake runners, never real `git`/`gh`/`hc`.
- `striker serve` binds `127.0.0.1` only (port 7522); new endpoints live on the same server.
- Remote machines: git via `hc exec --on <machine> --cwd <dir> git …`, files via `hc file read --on <machine> <path>`. `machine` empty or `local` = this computer.
- Topic = spec/plan file name without `YYYY-MM-DD-` and without `-design.md` / `.md`.
- Spec header lines: `Depends on: <topic>[, <topic>…]`, `Branch: <name>` (default: topic).
- Task headings: `### Task #<issue>: <title>` after approval, `### Task <n>: <title>` before. Task dependency line: `Depends on: #41, #43` (or `Task 2, Task 3` before approval). No line: depends on the previous task.
- Terminal labels: `striker.coordinator=<topic>`, `striker.task=<issue>`, optional `striker.role=<role>` and `striker.topic=<topic>`.
- Commit references: `Refs #<issue>`, `Fixes #<issue>` on the completing commit.
- States: task `todo | working | needs_you | done`; plan `merged | active | blocked | planned`; rules exactly as in the spec ("Data").
- Colours only via `lcars/palette.js` roles, mapping as in the spec ("Look"). No new hues.
- macOS and Windows; `lcars/main.js` is CRLF, `lcars/striker.js`, `lcars/palette.js` and new JS files are LF.
- Mod checks: `herdr-app check ~/.config/herdr-app/mods/lcars` must print `check passed` after every LCARS task.

## Review Focus

1. A worktree whose branch has no spec/plan (the main checkout, a quick-fix branch): the Bridge says `NO PLAN FOR BRANCH <name>`, never an empty board. Test in Task 6 (`/space` returns `plan: null`), text in Task 8.
2. The space's machine is unreachable: `/repo` and `/space` answer `{"error": "<first stderr line>"}` with status 200; LCARS keeps the last good answer and shows `REPO UNREADABLE: <error>`. Test in Task 6.
3. `striker issues` fails half-way (gh not logged in, network): issues already created are written into the plan before the error returns, so a rerun creates only the missing ones. Test in Task 3.
4. A plan mixing `### Task #42:` and `### Task 3:` headings (after such a failure): both parse, positions stay in file order, `issue` is null for the unnumbered ones. Test in Task 2.
5. `Depends on:` naming an unknown topic, or two plans depending on each other: both show `blocked`, `blocked_by` names the topic (unknown ones as `<topic>?`), and the tree layout terminates. Tests in Task 4 and Task 5.

---

### Task 1: Spike — switching the bottom field

**Files:**
- Modify: `docs/superpowers/specs/2026-11-21-lcars-bridge-design.md` (append "Spike result")
- Scratch only (not committed): a copy of `layouts/lcars.json` with a test stack child

- [x] **Step 1:** Add to the installed `~/.config/herdr-app/layouts/lcars.json` (keep a backup) a `work-view` child `spike` = column of a mod block (`{"mod":"lcars","props":{"part":"top"}}`) and a block `spike-bottom` (`px: 130`) with that mod; `herdr-app do layout.select '{"stack":"work-view","child":"spike"}'`.
- [x] **Step 2:** `layout.set` test: read the layout JSON, replace `spike-bottom.content` with `{"terminal":"<address of any live terminal from hc ls>"}`, `herdr-app do layout.set '{"layout": <tree>}'`, `herdr-app shot`. Record: terminal visible (y/n), flicker, sidebar width/other clicks unchanged (y/n), lcars mod copies restarted (does `top` lose its state?), file on disk changed (y/n). Swap back to the mod block, record the same.
- [x] **Step 3:** `agent` test: set `spike-bottom.content` to `{"agent":"focused"}`, run `herdr-app do agent.focus '{"terminal":"<agent address>"}'`, shot. Record: shown (y/n), main tab switched (y/n), works for an unlabelled plain shell (y/n).
- [x] **Step 4:** Window fallback test: from a scratch mod or `herdr-app do`, `openOverlay`-equivalent with `kind:"window"`, `layout: {"type":"block","content":{"terminal":"<addr>"}}`, `at` = bottom field's position, width/height = its size. Record placement accuracy and whether it follows a window resize.
- [x] **Step 5:** Restore the backup layout. Append "## Spike result" to the spec: the three recordings and the chosen mechanism (`layout.set` if Step 2 shows the terminal without losing clicks or restarting mods; else `agent` if Step 3 passes without switching the main tab; else window). Commit `spec: bridge bottom-field spike result`.

### Task 2: Parse specs and plans

**Files:**
- Create: `striker/internal/plans/plans.go`, `striker/internal/plans/plans_test.go`

**Interfaces:**
- Produces:
  - `plans.Topic(file string) string`
  - `plans.Spec{Topic, Title, Branch string; DependsOn []string; Headings []string}`; `plans.ParseSpec(file string, text []byte) Spec` (`Title` = first `# ` line; `Headings` = `## ` lines in order; `Branch` defaults to `Topic`).
  - `plans.Plan{Topic, Title string; Tasks []Task}`; `plans.Task{N int; Issue int; Title string; StepsDone, StepsTotal int; DependsOn []int; Body string}`; `plans.ParsePlan(file string, text []byte) Plan`. `N` = 1-based position; `Issue` = 0 when unnumbered; `DependsOn` = positions (issue refs resolved through the plan's own tasks, unknown refs dropped); default `[N-1]` for N>1; `Body` = the section text up to the next `### ` or `## `; steps = lines matching `^\s*- \[( |x|X)\]`.

- [x] **Step 1: Failing tests:**
  - `TestTopic`: `2026-11-21-striker-go-bundle-design.md` → `striker-go-bundle`; `2026-11-21-striker-go-bundle.md` → `striker-go-bundle`; `docs/superpowers/plans/2026-11-21-lcars-bridge.md` → `lcars-bridge`.
  - `TestParseSpecHeader`: text with `Depends on: striker-go-bundle, fonts` and `Branch: rg` → `DependsOn == ["striker-go-bundle","fonts"]`, `Branch == "rg"`; without `Branch:` → `Branch == Topic`; `Headings` lists `## Why`, `## Goals` as `["Why","Goals"]`.
  - `TestParsePlanTasks`: three tasks, steps `- [x]`,`- [x]`,`- [ ]` in task 1 → `StepsDone 2, StepsTotal 3`; task 3 with `Depends on: Task 1` → `DependsOn == [1]`; task 2 → `[1]`.
  - `TestParsePlanMixedNumbering` (Review Focus 4): `### Task #42: A`, `### Task 2: B`, `### Task #44: C` with `Depends on: #42` in C → `Issue` `42,0,44`; `N` `1,2,3`; C `DependsOn == [1]`.
  - `TestParsePlanCRLF`: same plan with `\r\n` parses identically.
- [x] **Step 2:** `cd striker && go test ./internal/plans/` → FAIL (undefined).
- [x] **Step 3:** Implement the interfaces above.
- [x] **Step 4:** `go test ./...` → PASS.
- [x] **Step 5:** Commit `striker: parse specs and plans`.

### Task 3: Git access and `striker issues`

**Files:**
- Create: `striker/internal/gitrepo/gitrepo.go`, `striker/internal/gitrepo/gitrepo_test.go`, `striker/internal/issues/issues.go`, `striker/internal/issues/issues_test.go`
- Modify: `striker/cmd/striker/main.go` (subcommand `issues`)

**Interfaces:**
- Consumes: `hc.Runner` (striker-go-bundle Task 2), `plans.ParsePlan`.
- Produces:
  - `gitrepo.Runner interface { Git(dir string, args ...string) ([]byte, error); Read(path string) ([]byte, error) }`; `gitrepo.Local{}` (`git -C dir …`, `os.ReadFile`); `gitrepo.Remote{HC hc.Runner; Machine string}` (`hc exec --on M --cwd dir git …`, `hc file read --on M path`); `gitrepo.For(h hc.Runner, machine string) Runner`.
  - `gitrepo.Repo{R Runner; Dir string}` with: `DefaultBranch() string` (`symbolic-ref --short refs/remotes/origin/HEAD` minus `origin/`, else `main`); `Log(ref string, n int) ([]Commit, error)`; `Worktrees() ([]Worktree, error)` (`worktree list --porcelain`); `Branches() ([]string, error)` (`for-each-ref --format=%(refname:short) refs/heads refs/remotes/origin`, without `origin/HEAD`); `MergeCommit(branch, into string) string` (oldest of `rev-list --merges --ancestry-path branch..into`, "" if none); `ListFiles(ref, dir string) ([]string, error)` (`ls-tree --name-only ref dir/`); `Show(ref, path string) ([]byte, error)`; `ReadWorking(rel string) ([]byte, error)`; `Hunks(rel string) ([]Hunk, error)` (`diff -U0 HEAD -- rel`).
  - `gitrepo.Commit{SHA string; Parents []string; Subject string; Refs []int; Fixes []int}` (log format `%H%x1f%P%x1f%s%x1f%b%x1e`; `Refs #N`/`Fixes #N` from subject and body); `gitrepo.Worktree{Path, Branch string}`; `gitrepo.Hunk{OldStart, OldLines, NewStart, NewLines int; Removed, Added []string}`.
  - `issues.GH interface { Run(dir string, args ...string) ([]byte, error) }`; `issues.Create(gh GH, repo gitrepo.Repo, planRel string, write func(rel string, text []byte) error) (map[int]int, error)` returns position → issue; `issues.Closed(gh GH, dir string) (map[int]bool, error)` (`gh issue list --state closed --json number --limit 1000`).
  - CLI: `striker issues <plan file>` in the current repo; prints `#<issue> <title>` per created task.

`Create`: for each task with `Issue == 0`, `gh issue create --title <title> --body <first paragraph of Body>\n\nPlan: <plan path> (Task <n>)` and parse the number from the printed URL; after each success, rewrite that heading to `### Task #<issue>:` and the `Depends on: Task n` lines that point at it to `#<issue>`, and call `write` right away. When all are created: `gh issue edit <issue> --body` adding the link `https://github.com/<owner>/<repo>/blob/<default>/<plan path>#task-<issue>-<slug>` (owner/repo from `gh repo view --json nameWithOwner`), then `git add <plan> && git commit -m "plan: issues for <topic>"`.

- [x] **Step 1: Failing tests:**
  - `TestLogParsesRefs`: fake output with body `Refs #42\nFixes #43` → `Refs == [42]`, `Fixes == [43]`, `Parents` split on space.
  - `TestWorktreesPorcelain`: two entries, the second `branch refs/heads/striker-go` → `Branch == "striker-go"`.
  - `TestRemoteRunsThroughHC`: `Remote{Machine:"remote1"}.Git("/srv/x","status")` → hc args `["exec","--on","remote1","--cwd","/srv/x","git","status"]`.
  - `TestHunksU0`: `@@ -3,2 +3,1 @@` with `-a`,`-b`,`+c` → one hunk, `Removed ["a","b"]`, `Added ["c"]`.
  - `TestCreateNumbersHeadings`: plan with tasks 1,2 (2 depends on Task 1); fake gh returns `.../issues/42` then `.../issues/43` → written text has `### Task #42:`, `### Task #43:`, `Depends on: #42`; result `{1:42, 2:43}`.
  - `TestCreateSkipsNumbered`: plan already containing `### Task #42:` → no `issue create` call for it.
  - `TestCreatePartialFailure` (Review Focus 3): second `issue create` fails → error returned, and the last `write` call contains `### Task #42:` and `### Task 2:`.
- [x] **Step 2:** `go test ./internal/gitrepo/ ./internal/issues/` → FAIL.
- [x] **Step 3:** Implement; wire `issues` into `main.go` with `gitrepo.Local{}` and a real `gh` runner.
- [x] **Step 4:** `go test ./...` → PASS.
- [x] **Step 5:** Commit `striker: git access and striker issues`.

### Task 4: Repo view (plans, states, tasks)

**Files:**
- Create: `striker/internal/repoview/repoview.go`, `striker/internal/repoview/gather.go`, `striker/internal/repoview/repoview_test.go`

**Interfaces:**
- Consumes: `plans.*`, `gitrepo.*`, `issues.Closed`, `hc.Client.List`/`Get` (labels, status).
- Produces:
  - `repoview.Inputs{Default string; Trunk []gitrepo.Commit; Branches []Branch; Specs []plans.Spec; Plans []plans.Plan; SpecFiles, PlanFiles map[string]string; Closed map[int]bool; Agents []Agent; Now time.Time}`; `repoview.Branch{Name, Worktree, Merge string; Commits []gitrepo.Commit}`; `repoview.Agent{Address, Status, Role, Coordinator string; Task int}`.
  - `repoview.Build(in Inputs) View` (pure).
  - `repoview.Gather(r gitrepo.Repo, h hc.Client, gh issues.GH, now time.Time) (Inputs, error)`: specs/plans from `docs/superpowers/specs|plans` on the default branch and on every branch that has a worktree or an `origin/` counterpart (a branch's copy wins over the default branch's for its own topic); `Trunk` = `Log(default, 30)`; agents = local and remote terminals with `striker.task` or `striker.coordinator` labels.
  - `repoview.View{Default string; Trunk []gitrepo.Commit; Plans []PlanView}` with JSON tags `default`, `trunk`, `plans`; `PlanView{Topic, Title, Spec, Plan, Branch string; DependsOn []string; Worktree *string; State string; BlockedBy []string; MergePoint string; Coordinator string; Tasks []TaskView; Commits []gitrepo.Commit}` (`topic,title,spec,plan,branch,depends_on,worktree,state,blocked_by,merge_point,coordinator,tasks,commits`); `TaskView{Issue *int; N int; Title string; StepsDone, StepsTotal int; DependsOn []int; State string; Agents []AgentView; Body string}`; `AgentView{Address, Role, Status string}`.

Rules (spec "Data"): task `needs_you` if a labelled agent's status is `blocked`, else `working` if one runs (`working` or `idle` with a live terminal), else `done` if its issue is closed or all steps are checked, else `todo`. Plan `merged` if every task is done; `active` if not and a worktree exists for its branch; `blocked` if not and a `depends_on` topic is not merged (unknown topic → `blocked`, `blocked_by` entry `<topic>?`); else `planned`. `MergePoint`: branch merge commit, else the newest trunk commit whose `Refs`/`Fixes` hit one of its issues, else "".

- [x] **Step 1: Failing tests** (all on `Build` with hand-made `Inputs`):
  - `TestTaskStates`: agent `striker.task=42` status `blocked` → task 42 `needs_you`; agent for 43 `working` → `working`; issue 41 closed → `done`; task without issue, 3/3 steps → `done`; else `todo`.
  - `TestPlanMergedWithoutBranch`: all tasks done, no branch (built on main) → `merged`, `MergePoint` = newest trunk commit with `Fixes #41`.
  - `TestPlanBlockedByDependency`: B `Depends on: A`, A has a todo task → B `blocked`, `BlockedBy == ["A"]`.
  - `TestUnknownAndCyclicDependency` (Review Focus 5): C depends on `nope` → `blocked`, `["nope?"]`; D↔E depend on each other → both `blocked`.
  - `TestActiveWithWorktree`: branch `striker-go` with worktree `/w` → plan with `Branch "striker-go"` is `active`, `Worktree == "/w"`.
- [x] **Step 2:** `go test ./internal/repoview/` → FAIL.
- [x] **Step 3:** Implement `Build` and `Gather`.
- [x] **Step 4:** `go test ./...` → PASS.
- [x] **Step 5:** Commit `striker: repo view with plan and task states`.

### Task 5: Tree layout

**Files:**
- Create: `striker/internal/graph/graph.go`, `striker/internal/graph/graph_test.go`

**Interfaces:**
- Consumes: `repoview.View`.
- Produces: `graph.Layout(v repoview.View) Graph`; `graph.Graph{Cols, Now int; Lanes []Lane; Nodes []Node; Edges []Edge}` (`cols,now,lanes,nodes,edges`); `Lane{Index int; Plan, Label string}` (lane 0 = trunk, `Plan ""`); `Node{ID string; Col, Lane int; Kind string /* commit|task|merge */; Plan string; Issue *int; State string; Future bool; Label string}`; `Edge{From, To string; Future bool}`.

Rules: trunk commits on lane 0 in time order up to `Now`; future trunk columns after `Now`. One lane per non-merged plan, ordered: active, then blocked/planned in dependency order; merged plans get a lane for their solid past part only if their branch commits are in the trunk window. Task nodes in plan order; tasks whose `DependsOn` are all earlier and that are not on the plan's main strand (they share predecessors with another task) get an extra lane directly below. A plan branches off at its first dependency's merge column (future) or at `Now` if it has none and is not active. Done/working/needs_you nodes are not `Future`; todo nodes and future merges are. Cycles: break by ignoring edges into an already-placed plan (no infinite loop).

- [x] **Step 1: Failing tests:**
  - `TestTrunkOnLaneZero`: 3 trunk commits → nodes on lane 0, cols 0..2, `Now == 2`.
  - `TestDependentPlanStartsAfterMerge`: A active (merge planned at col X), B depends on A → B's first node col > A's merge node col.
  - `TestParallelTasksOwnLane`: tasks 3 and 4 both depend on 2 → 3 and 4 on different lanes, same col range.
  - `TestFutureFlags`: todo task node `Future true`; done node `Future false`.
  - `TestCycleTerminates` (Review Focus 5): D↔E → `Layout` returns, both plans have a lane.
- [x] **Step 2:** `go test ./internal/graph/` → FAIL.
- [x] **Step 3:** Implement.
- [x] **Step 4:** `go test ./...` → PASS.
- [x] **Step 5:** Commit `striker: tree layout for the repo view`.

### Task 6: Endpoints `/repo`, `/space`, `/doc`

**Files:**
- Create: `striker/internal/bridge/bridge.go`, `striker/internal/bridge/bridge_test.go`
- Modify: `striker/cmd/striker/main.go` (mount next to `/status`)

**Interfaces:**
- Consumes: `repoview.Gather/Build`, `graph.Layout`, `gitrepo.For`, `plans.*`.
- Produces: `bridge.Source interface { Repo(machine, path string) (repoview.View, error); Doc(machine, path, rel string) (Doc, error) }`; `bridge.Handler(src Source) http.Handler`:
  - `GET /repo?machine=&path=` → `{"view": View, "graph": Graph}`, cached 60 s per (machine,path).
  - `GET /space?machine=&path=` → `{"topic": string|null, "branch": string, "plan": PlanView|null, "spec_headings": [], "coordinator": string|null}`, cached 10 s; branch from `Worktrees()` entry whose path = `path`; plan = the `PlanView` whose `Branch` equals it.
  - `GET /doc?machine=&path=&file=` → `Doc{Text, Base string; Hunks []gitrepo.Hunk}` (`text,base,hunks`), not cached.
  - Any source error → 200 with `{"error": "<first line>"}`; missing parameter → 400.

- [x] **Step 1: Failing tests** (httptest, fake `Source`):
  - `TestSpaceNoPlan` (Review Focus 1): worktree on branch `hotfix` without a matching plan → `plan == null`, `branch == "hotfix"`.
  - `TestRepoErrorIs200` (Review Focus 2): `Repo` returns error `hc: machine remote1 unreachable\nmore` → 200, body `{"error":"hc: machine remote1 unreachable"}`.
  - `TestRepoCached`: two requests within 60 s → one `Repo` call.
  - `TestDocHunks`: fake doc → JSON has `hunks[0].added`.
  - `TestMissingParam400`.
- [x] **Step 2:** `go test ./internal/bridge/` → FAIL.
- [x] **Step 3:** Implement; real `Source` uses `gitrepo.For(hc.Exec{}, machine)`.
- [x] **Step 4:** `go test ./... && go build -o bin/striker ./cmd/striker`; run `./bin/striker serve &` and `curl -s '127.0.0.1:7522/space?machine=&path='"$PWD"` → JSON with `"branch"`.
- [x] **Step 5:** Commit `striker: bridge endpoints repo, space, doc`.

### Task 7: Coordinator runs task agents as labelled terminals

**Files:**
- Create: `striker/internal/dispatch/dispatch.go`, `striker/internal/dispatch/dispatch_test.go`, `striker/briefs/coordinator.md`
- Modify: `striker/cmd/striker/main.go` (subcommands `coordinator`, `dispatch`), `striker/CLAUDE.md` (section "Coordinator")

**Interfaces:**
- Consumes: `hc.Client`, `plans.ParsePlan`.
- Produces:
  - `dispatch.Argv(role string, issue int, topic, cwd, prompt string) []string` → `["run","--bg","--name","<role>-<issue>","--cwd",cwd,"--label","striker.task=<issue>","--label","striker.role=<role>","--label","striker.topic=<topic>","claude",prompt]`.
  - `dispatch.Prompt(role string, p plans.Plan, t plans.Task, planRel, specRel string) string` — role brief + the task section + "Commit with `Refs #<issue>`; the commit that completes the task uses `Fixes #<issue>`."
  - CLI `striker dispatch <role> <issue> [--plan <file>]` (in a worktree; prints the new terminal address); `striker coordinator <plan file>` labels the current terminal `striker.coordinator=<topic>` (`hc label`) and prints the brief path.
  - `briefs/coordinator.md`: run the plan like superpowers:subagent-driven-development, but start every implementer/reviewer with `striker dispatch` and wait with `hc wait <address>`; run tasks in parallel only when their `Depends on` are done; never start in-process subagents for plan tasks; run `striker issues <plan>` first if headings are unnumbered.

- [x] **Step 1: Failing tests:** `TestArgvLabels` (role `implementer`, issue 42, topic `striker-go`, cwd `/w` → exact slice above); `TestPromptCarriesIssueRefs` (contains `Refs #42` and `Fixes #42` and the task's first step line); `TestCoordinatorLabel` (fake hc: `label <self> striker.coordinator=lcars-bridge`).
- [x] **Step 2:** `go test ./internal/dispatch/` → FAIL.
- [x] **Step 3:** Implement; write the brief; add the CLAUDE.md section.
- [x] **Step 4:** `go test ./...` → PASS; in a scratch worktree `./bin/striker dispatch implementer 1 --plan <plan>` → `hc ls` shows `implementer-1` with label `striker.task=1`; `hc kill` it.
- [x] **Step 5:** Commit `striker: coordinator dispatches task agents as labelled terminals`.

### Task 8: Bridge view — data, PLAN mode, tabs, layout

**Files:**
- Create: `lcars/bridge.js` (LF)
- Modify: `lcars/main.js` (CRLF: pseudo-tabs, `part === "bridge"`, leaving the Bridge), `layouts/lcars.json` (new `work-view` child `bridge`), `lcars/palette.js` (only if a role is missing — none expected)

**Interfaces:**
- Consumes: `/space`, `/repo` JSON (Task 6); `ROLE`, `C` from `palette.js`; `showBoard` pattern from `striker.js`.
- Produces (exports of `bridge.js`): `BRIDGE_KEY = "bridge.view"` (`"plan" | "spec" | null`), `showBridge(mode: "plan"|"spec"|null)`, `pollBridge()`, `bridge(now): Element` (the work-area view), `bridgeBottom(now): Element` (REPO tree or header, Task 10), shared keys `bridge.space`, `bridge.repo`, `bridge.doc`, `bridge.open` (expanded task position), `bridge.bottom` (`{kind:"repo"} | {kind:"terminal", address, label}`).

Layout: `work-view` child `bridge` = column: block `bridge-view` (weight 1, `{"mod":"lcars","props":{"part":"bridge"}}`) and block `bridge-bottom` (`px: 130`, `{"mod":"lcars","props":{"part":"bridge-bottom"}}`). `showBridge(mode)` sets `BRIDGE_KEY` and `layout.select {stack:"work-view", child: mode ? "bridge" : "main"}`; `toTerminals()` in `main.js` also leaves the Bridge. The space's machine/path: `space.machine` and the first terminal's cwd (as Herdr's worktree mod's `place()`).

PLAN mode drawing exactly as the spec's "Look": elbow (`ROLE.frame`, outer radius 40, concave inner corner radius 24), arm with plan title (click → `bridge.bottom = {kind:"terminal", address: coordinator, label: "COORDINATOR · <title>"}`), task rows (oval + heading + chevron; click toggles `bridge.open`, one open at a time; open row shows steps via `TextView.markdown` of `Body`), columns WORKING/NEEDS YOU/DONE as square headings only, cards on their task's row and as tall (WORKING card click → bottom terminal of its first agent). Rows separated by 2 px lines: inside the elbow `ROLE.ink` at 20 % opacity over the frame, outside `ROLE.track`. `plan: null` → `NO PLAN FOR BRANCH <branch>`; `error` → `REPO UNREADABLE: <error>` over the last good data.

- [x] **Step 1:** Implement `bridge.js` data part: `pollBridge()` fetches `/space` every 10 s and `/repo` every 60 s, only while `BRIDGE_KEY` is set; writes shared state only when the JSON changed (as `striker.js`).
- [x] **Step 2:** Implement PLAN mode and the pseudo-tabs PLAN/SPEC (`main.js` `tabs()`, first, active `ROLE.frame`, other `C.peach`, no numbers); add the layout child; copy the layout with `./install.sh`.
- [x] **Step 3:** `herdr-app check ~/.config/herdr-app/mods/lcars` → `check passed`.
- [x] **Step 4:** With `striker serve` running in this repo's worktree: click PLAN, `herdr-app shot`; confirm elbow, rows for this plan's tasks, cards in the right columns; in a branch without a plan confirm `NO PLAN FOR BRANCH`.
- [x] **Step 5:** Commit `lcars: bridge view with plan mode`.

### Task 9: SPEC mode with diff

**Files:**
- Create: `lcars/blocks.js` (LF, no gpui imports), `lcars/bridge.check.mjs`
- Modify: `lcars/bridge.js`

**Interfaces:**
- Consumes: `/doc` (Task 6), `spec_headings` from `/space`.
- Produces: `splitBlocks(text: string): string[]` (markdown blocks: heading, paragraph, list, fenced code) and `diffBlocks(base: string, text: string, hunks): {kind:"same"|"added"|"removed", md: string}[]` exported for the check script.

SPEC mode: narrower elbow (vertical part 240 px) listing `spec_headings`; the heading in view inverted (`ROLE.frame` text on black); changed sections show `+a −r`. Reading area: `same` blocks via `TextView.markdown`; `added` inside a div with `border_l` 4 px `ROLE.working` and `text_color(ROLE.working)`; `removed` as plain text `ROLE.neutral` with `line_through()`. Heading click scrolls to its block. `/doc` fetched when SPEC opens and every 10 s while open.

- [x] **Step 1:** Write `lcars/bridge.check.mjs` (node, imports `splitBlocks`/`diffBlocks` from `lcars/blocks.js`): asserts a hunk replacing one paragraph yields `removed` then `added`; an untouched heading stays `same`; CRLF text splits like LF.
- [x] **Step 2:** `node lcars/bridge.check.mjs` → FAIL (missing module).
- [x] **Step 3:** Implement `lcars/blocks.js` and SPEC mode.
- [x] **Step 4:** `node lcars/bridge.check.mjs` → `ok`; `herdr-app check …` → `check passed`; edit the spec in the worktree without committing, open SPEC, shot: blue added block, grey struck removed block.
- [x] **Step 5:** Commit `lcars: bridge spec mode with block diff`.

### Task 10: Bottom field — REPO tree and terminal switching

**Files:**
- Modify: `lcars/bridge.js`, `lcars/main.js` (`part === "bridge-bottom"`), `layouts/lcars.json` (only if the spike chose `agent`)

**Interfaces:**
- Consumes: `/repo` `graph` (Task 5), the spike result (Task 1), `bridge.bottom`.
- Produces: `drawTree(graph, width): Element`; `setBottom(next)` that applies the chosen mechanism.

Tree drawing: 16 px per lane, trunk `ROLE.frame` 3 px; plan lanes by state: merged `ROLE.done`, active `ROLE.working`, blocked/planned `ROLE.neutral`; future parts dashed (gaps of 6 px) at 45 % opacity; nodes: done filled `ROLE.done`, working ring `ROLE.working`, needs_you ring `ROLE.alert`, todo dashed ring `ROLE.neutral`; a NOW rule `ROLE.frame` dotted; plan labels at lane start (`<TOPIC> · THIS PLAN` for the space's plan; `· AFTER <dep>` for blocked). Header rule labelled `REPO`, or `COORDINATOR · <plan>` / `#<issue> · <agent> · LIVE` with a round `◂ REPO` button (`ROLE.frame`) at its right when showing a terminal.

`setBottom`: `layout.set` → rewrite `bridge-bottom.content` in the current layout tree to `{"terminal": address}` or back to the mod; `agent` → `agent.focus {terminal}`; window → `openOverlay({kind:"window", key:"bridge-bottom", layout:{type:"block", content:{terminal}}, at, width, height})` placed over `bridge-bottom` and closed by `◂ REPO`.

- [x] **Step 1:** Implement `drawTree` and the header.
- [x] **Step 2:** Implement `setBottom` for the mechanism recorded in the spec's "Spike result".
- [x] **Step 3:** `herdr-app check …` → `check passed`.
- [x] **Step 4:** Shot with the Bridge open: tree with NOW, this plan's lane blue, a dependent plan dashed after it. Click the plan title → coordinator terminal in the bottom field; `◂ REPO` → tree; click a WORKING card → that agent's terminal.
- [x] **Step 5:** Commit `lcars: bridge bottom field with repo tree and terminals`.

### Task 11: Docs

**Files:**
- Modify: `docs/lcars-colors.md` (shapes: elbow, square column heading, accordion row, tree marks), `README.md` (Bridge, `striker issues`, `striker dispatch`, conventions), `striker/briefs/lead.md` (spec header lines, issue numbering)

- [x] **Step 1:** Write the sections; every colour named by role.
- [x] **Step 2:** `grep -n "Bridge" README.md docs/lcars-colors.md` shows the new sections; `grep -nE "#[0-9a-fA-F]{6}" docs/lcars-colors.md` lists only hues already in `palette.js`.
- [x] **Step 3:** Commit `docs: bridge, issues and coordinator conventions`.
