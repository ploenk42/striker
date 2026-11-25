# Striker in Go, bundled with LCARS — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move Striker into `herdr-mods/striker/`, replace its PowerShell scripts with one Go program (`striker status|send|start-lead|serve`), and make the LCARS board read `striker serve` over `http://127.0.0.1:7522/status` on macOS and Windows.

**Architecture:** `striker` shells out to the local `hc` for everything about terminals and remote files, behind a small `Runner` interface so tests use a fake. `status` merges fleet (server) repos with locally discovered leads. `serve` caches the last report and refreshes it in the background. The board only fetches JSON.

**Tech Stack:** Go 1.23+ standard library only (no third-party modules); gpui-kit JavaScript for the mod; launchd (macOS) and Task Scheduler (Windows).

**Spec:** `docs/superpowers/specs/2026-11-21-striker-go-bundle-design.md`

## Global Constraints

- Module path `github.com/ploenk42/herdr-mods/striker`, `go.mod` in `striker/`. Standard library only.
- `serve` binds to `127.0.0.1` only; default port `7522`, flag `--addr`.
- Status JSON shape exactly as in the spec ("Data"); `machine: null` = this computer.
- Messages to leads always start with `[Striker]`; never `hc interrupt`, never `--force`.
- `fleet.json` and `briefs/` are found in `STRIKER_HOME`, default `<user home>/herdr-mods/striker` (Windows: `%USERPROFILE%\herdr-mods\striker`).
- Local status files: `<home>/.striker/status/<repo>.json`, UTF-8, may start with a BOM.
- Everything must keep working on macOS and Windows. CRLF files keep CRLF.
- Builds: `GOOS=darwin GOARCH=arm64`, `GOOS=windows GOARCH=amd64`, `GOOS=linux GOARCH=amd64`, output in `striker/bin/` (gitignored).

## Review Focus

1. A machine that is unreachable (`hc` fails for remote1): local repos still show; each remote1 repo gets `status: {"error": ...}`, the report does not fail as a whole. Test in Task 3.
2. A status file with a UTF-8 BOM (Windows PowerShell writes one) or invalid JSON: BOM parses, invalid gives `{"error":"Statusdatei ist kein gültiges JSON"}`. Test in Task 3.
3. A local lead whose label value also names a fleet repo: appears once, as the fleet entry. Test in Task 3.
4. `send` text with double quotes, backslashes and newlines arrives byte-exact at `hc` (one argv element). Test in Task 4.
5. `/status` requested before the first refresh finished: the request waits for that first build instead of returning an empty list. Test in Task 5.

---

### Task 1: Bundle Striker into herdr-mods

**Files:**
- Modify: `lcars/main.js`, `lcars/striker.js`, `README.md`, `install.sh` (already changed, uncommitted)
- Create: `striker/` (via subtree), `striker/fleet.json` (edit), `.gitignore` (append `striker/bin/`)

- [x] **Step 1:** Commit the pending macOS changes on their own: `git add lcars/main.js lcars/striker.js README.md install.sh && git commit -m "macOS: install.sh, platform-aware Striker path and URL opener"`.
- [x] **Step 2:** `git subtree add --prefix=striker <the old standalone repo> main` (no `--squash`, history kept). Expected: `striker/CLAUDE.md`, `striker/scripts/*.ps1` exist.
- [x] **Step 3:** Remove the `epsilon` entry (`machine: "local"`) and the `"local"` key of `status_dir` from `striker/fleet.json`. Append `striker/bin/` to `.gitignore`.
- [x] **Step 4:** `brew install go` if `go version` fails. Expected: `go version go1.2x darwin/arm64`.
- [x] **Step 5:** Commit: `git commit -am "striker: drop local fleet entry; local leads are discovered"`.

### Task 2: hc client and fleet loading

**Files:**
- Create: `striker/go.mod`, `striker/internal/hc/hc.go`, `striker/internal/hc/hc_test.go`, `striker/internal/fleet/fleet.go`, `striker/internal/fleet/fleet_test.go`

**Interfaces:**
- Produces:
  - `hc.Runner interface { Run(args ...string) (stdout []byte, err error) }`; `hc.Exec{}` runs the real `hc` via `os/exec` (non-zero exit → error carrying stderr).
  - `hc.Client{R Runner}` with `List() ([]Terminal, error)` (`hc ls --json`), `ListLabel(k, v string) ([]Terminal, error)` (`hc ls --label k=v --json`), `Get(addr string) (Info, error)` (`hc get <addr> --json`), `ReadFile(machine, path string) ([]byte, error)` (`hc file read --on <machine> <path>`), `Send(addr, text string) error`, `Label(addr, kv string) error`, `Wait(addr string, timeout int) error`.
  - `hc.Terminal{Address, Status, Cwd, Where string}`; `hc.Info{Address, Cwd string; Labels map[string]string; Status *struct{State, Msg string}; StatusMs *int64}` (JSON tags as `hc` prints: `address`, `cwd`, `labels`, `status.state`, `status.msg`, `status_ms`, `where`).
  - `hc.IsLocal(addr string) bool`: true when the address has no `/`.
  - `fleet.Fleet{Repos []Repo; StatusDir map[string]string}`, `fleet.Repo{Name, Title, Machine, Path, User, Space string}`, `fleet.Load(path string) (Fleet, error)` (accepts a BOM).

- [x] **Step 1: Failing tests** — `TestListParsesTerminals` (fake runner returns `{"terminals":[{"address":"t1","cwd":"/x","where":"epsilon / 1"}]}`, expects 1 terminal, `Where == "epsilon / 1"`, runner got args `["ls","--json"]`); `TestGetParsesLabels` (input = the `hc get t1 --json` sample in the spec's facts, expects `Labels["herdr.space"]` set, `Status == nil`); `TestIsLocal` (`"t3"` true, `"remote1/t3"` false); `TestLoadFleetWithBOM` (file starting `\uFEFF{"repos":[{"name":"alpha","machine":"remote1"}],"status_dir":{"remote1":"/s"}}` loads 1 repo).
- [x] **Step 2:** `cd striker && go test ./...` → FAIL (undefined).
- [x] **Step 3:** Implement the interfaces above.
- [x] **Step 4:** `go test ./...` → PASS.
- [x] **Step 5:** Commit `striker: hc client and fleet loading in Go`.

### Task 3: Status report (fleet + local discovery)

**Files:**
- Create: `striker/internal/status/status.go`, `striker/internal/status/status_test.go`

**Interfaces:**
- Consumes: `hc.Client`, `fleet.Fleet`.
- Produces: `status.Report{Updated string; Repos []Repo}`, `status.Repo{Name string; Title string \`json:",omitempty"\`; Machine *string; Path, Space string; Lead *Lead; Status json.RawMessage}`, `status.Lead{Address, State, Msg string; StatusMs *int64; Where string}`, `status.Build(c hc.Client, f fleet.Fleet, home string, now time.Time) Report`.

Rules: fleet repos first, in file order; lead = first terminal of `ListLabel("striker.lead", name)`, state/msg from `Get`; remote status file via `ReadFile(machine, StatusDir[machine]+"/"+name+".json")`. Then local leads: for each `List()` terminal with `IsLocal`, `Get` it; with label `striker.lead` and no fleet repo of that name, add a repo with `Machine nil`, `Path = Info.Cwd`, `Space = Where` before `" / "`, status file read from `home/.striker/status/<name>.json` with `os.ReadFile`. A failing `hc` call for one repo yields that repo with `Lead nil` and/or `Status {"error": "<first stderr line>"}`; `Build` never returns an error.

- [x] **Step 1: Failing tests** with a fake runner keyed by joined args:
  - `TestBuildFleetRepoWithLead`: alpha lead `remote1/t3` state `working`, status file `{"phase":"plan"}` → `Repos[0].Lead.Address == "remote1/t3"`, `*Machine == "remote1"`, status contains `"plan"`.
  - `TestBuildDiscoversLocalLead`: `ls --json` lists `t5`; `get t5` has label `striker.lead=epsilon`, cwd `/Users/you/epsilon`, where `epsilon / 1`; temp home has `.striker/status/epsilon.json` → a repo `epsilon`, `Machine == nil`, `Space == "epsilon"`.
  - `TestBuildUnreachableMachine` (Review Focus 1): every `--on remote1` call and `ls --label` fails → report still has the local repo; remote1 repos have `Lead == nil`.
  - `TestBuildStatusFileBOMAndInvalid` (Review Focus 2): BOM file parses; `{nope` → `{"error":"Statusdatei ist kein gültiges JSON"}`.
  - `TestBuildNoDuplicateLocalLead` (Review Focus 3): local label `alpha` while `alpha` is in the fleet → exactly one `alpha`.
- [x] **Step 2:** `go test ./internal/status/` → FAIL.
- [x] **Step 3:** Implement `Build`.
- [x] **Step 4:** `go test ./...` → PASS.
- [x] **Step 5:** Commit `striker: status report merges fleet repos and local leads`.

### Task 4: CLI — status, send, start-lead

**Files:**
- Create: `striker/cmd/striker/main.go`, `striker/internal/lead/lead.go`, `striker/internal/lead/lead_test.go`, `striker/internal/home/home.go`

**Interfaces:**
- Consumes: `hc.Client`, `fleet.Load`, `status.Build`.
- Produces: `home.Dir() string` (`STRIKER_HOME` or `<user home>/herdr-mods/striker`); `lead.Message(text string) string` (prefixes `[Striker] ` unless present, trims trailing whitespace); `lead.Start(c hc.Client, run func(args ...string) ([]byte, error), f fleet.Fleet, repo, briefPath string) (address string, err error)` — ports `start-lead.ps1` (refuse when `ListLabel` finds a lead; argv `sudo -i sh -c "cd '<path>' && exec ~/.local/bin/claude"` for root, else `sh -lc "cd '<path>' && exec claude"`; `herdr-app do space.new --name <space> --machine <machine> --argv <json>`; label; `Wait(addr, 90)`; return exit code 2 path when state is `blocked`/`working`; brief placeholders `{repo} {path} {machine} {status_file}`).
- CLI: `striker status` (prints indented JSON, exit 0), `striker send <addr> <text>` / `striker send <addr> --file <f>` (prints `Zugestellt an <addr>.`), `striker start-lead <repo>`, `striker serve [--addr 127.0.0.1:7522]` (Task 5). Unknown command → usage on stderr, exit 2.

- [x] **Step 1: Failing tests** — `TestMessagePrefix` (`"hi"` → `"[Striker] hi"`, `"[Striker] x\n"` → `"[Striker] x"`); `TestSendPassesTextAsOneArg` (Review Focus 4: text `say "hi" C:\path\` + `"\n"` + `line2` reaches the fake runner as args `["send","t3","[Striker] say \"hi\" C:\\path\\\nline2"]`); `TestStartLeadRefusesExisting`; `TestStartLeadRootArgv` (captured `space.new` argv JSON equals `["sudo","-i","sh","-c","cd '/srv/beta' && exec ~/.local/bin/claude"]`).
- [x] **Step 2:** `go test ./...` → FAIL.
- [x] **Step 3:** Implement `lead`, `home`, `main.go`.
- [x] **Step 4:** `go test ./... && go build -o bin/striker ./cmd/striker && ./bin/striker status` → PASS; output is JSON with `"repos"`.
- [x] **Step 5:** Commit `striker: Go CLI for status, send and start-lead`.

### Task 5: striker serve

**Files:**
- Create: `striker/internal/serve/serve.go`, `striker/internal/serve/serve_test.go`; Modify: `striker/cmd/striker/main.go`

**Interfaces:**
- Consumes: a `func() status.Report` builder.
- Produces: `serve.New(build func() status.Report, every time.Duration) *Server`; `(*Server).Handler() http.Handler` (`GET /status` → JSON, `Content-Type: application/json`; other paths 404); `(*Server).Run(ctx context.Context)` refreshes every `every` (20 s in main). The first request blocks until the first build is done.

- [x] **Step 1: Failing tests** — `TestStatusWaitsForFirstBuild` (Review Focus 5: builder sleeps 50 ms then returns 1 repo; a request made immediately gets 200 with 1 repo); `TestServesCachedReport` (builder counts calls; two requests → 1 call); `TestUnknownPath404`.
- [x] **Step 2:** `go test ./internal/serve/` → FAIL.
- [x] **Step 3:** Implement; main binds `--addr` default `127.0.0.1:7522` and refuses a non-loopback host.
- [x] **Step 4:** `go test ./...` → PASS; `./bin/striker serve & curl -s 127.0.0.1:7522/status` returns JSON.
- [x] **Step 5:** Commit `striker: serve /status on 127.0.0.1:7522`.

### Task 6: Board reads striker serve

**Files:**
- Modify: `lcars/striker.js` (LF), `lcars/main.js` (CRLF, only if imports change)

- [x] **Step 1:** In `pollFleet()`, replace the `exec(... status.ps1 ...)` call with `fetch("http://127.0.0.1:7522/status")`; `ok` → `data = await r.json()`; not ok or thrown → keep previous repos, `ok:false`, `error: "STRIKER SERVE NOT RUNNING"` when the fetch throws. Remove `strikerDir()`, `WINDOWS()`, and the `home`/`window` imports added for them. The empty-board text becomes `NO REPOS FROM STRIKER SERVE`.
- [x] **Step 2:** `herdr-app check ~/.config/herdr-app/mods/lcars` → `check passed`.
- [x] **Step 3:** With `striker serve` running, `herdr-app do layout.select '{"stack":"work-view","child":"striker-board"}'`, `herdr-app shot`, confirm a column per repo (or the error line for remote1 while the machine is not added).
- [x] **Step 4:** Commit `lcars: board reads striker serve instead of status.ps1`.

### Task 7: Install, autostart, docs; retire the scripts

**Files:**
- Modify: `install.sh`, `README.md`, `striker/CLAUDE.md`, `striker/README.md`, `striker/briefs/lead.md` (drop "auf seinem Windows-Rechner")
- Create: `install.ps1`, `striker/launchd/io.github.ploenk42.striker.plist` (template)
- Delete later: `striker/scripts/*.ps1`, only after the person confirms `install.ps1` works on Windows

- [x] **Step 1:** `install.sh`: after linking mods, `go build -o striker/bin/striker ./striker/cmd/striker` (from `striker/`), write `~/Library/LaunchAgents/io.github.ploenk42.striker.plist` (ProgramArguments `<repo>/striker/bin/striker serve`, `RunAtLoad` and `KeepAlive` true, `EnvironmentVariables` PATH incl. `/opt/homebrew/bin:$HOME/.local/bin` so `hc` is found), `launchctl bootout` then `bootstrap gui/$(id -u)`.
- [x] **Step 2:** `install.ps1`: junctions for `lcars`, `blueprint`; copy layout; `go build -o striker\bin\striker.exe .\striker\cmd\striker`; `Register-ScheduledTask` named `Striker` at logon, hidden, running `striker.exe serve`; start it.
- [x] **Step 3:** Docs: `striker/CLAUDE.md` uses `striker status`, `striker send <addr> "<text>"`, `striker start-lead <repo>`; README (root) describes both installers and that local leads are discovered.
- [x] **Step 4:** Verify on the Mac: `./install.sh`, `curl -s 127.0.0.1:7522/status` → JSON, `launchctl print gui/$(id -u)/io.github.ploenk42.striker` shows `state = running`. Windows: build check only here: `GOOS=windows GOARCH=amd64 go build ./...` → exit 0; `install.ps1` is run by the person on Windows.
- [x] **Step 5:** Commit `striker: installers with autostart, docs`. Once the person confirms Windows works: `git rm striker/scripts/*.ps1`, commit `striker: retire PowerShell scripts`. (ps1 removal waits for Windows confirmation)
