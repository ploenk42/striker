# Plan: striker-relay

Design: `design.md`. Go code under `striker/`, tests with `go -C striker test ./...` (Windows:
`C:\Program Files\Go\bin\go.exe`). Shell rules of AGENTS.md apply to every step (one simple command, absolute
paths). Commit per task; the task's line in `tasks.md` and its steps here are ticked in the commit with its last
work.

### Task 1: Check on remote1: hc parent across machines, sudo with worktree and prompt as arguments

No code; the findings go into `design.md` (sections 4 and 6) before task 3 starts.

- [x] **Step 1: parent.** Start a throwaway parent on remote1 (`hc run --on remote1 --bg --no-parent --name
  relay-check-parent sleep 600`), then a child from this computer with `hc run --on remote1 --bg --parent
  remote1/<parent> --no-notify --name relay-check-child sleep 600`. `hc get remote1/<child> --json`: is
  `parent` the parent? Does the child end when the parent is killed (`hc kill`)? Try both parent forms
  (`remote1/tX` and `tX`). Clean up with `hc kill` and `hc rm`.
- [x] **Step 2: sudo with arguments.** `hc exec --on remote1 sudo -n -u awayteam -i sh -c 'cd "$1" && shift &&
  printf "<%s>\n" "$@"' sh /srv/alpha 'a "b" $c' 'zwei
  Zeilen'`: each argument must come out unchanged (sudo -i escapes the command for the login shell). If not,
  pass the prompt through a file in the worktree instead and note it.
- [x] **Step 3: wait and read.** On a running remote1 Claude (a lead, with its person's go, or a throwaway
  `claude` as awayteam in a scratch folder): `hc wait <addr> --timeout 30` output when done, blocked, ended;
  `hc read <addr> --lines 60` shows the last `●` answer block as section 6 expects.
- [x] **Step 4:** write the findings into `design.md`; if hc refuses a parent on another machine, section 4
  switches to `herdr.space=<coordinator's space>`. Commit `openspec: striker-relay checks on remote1`, tick 1.

### Task 2: fleet.ClaudeCommand: Claude as the fleet user with arguments, no remote control

Files: `striker/internal/fleet/user.go`, `user_test.go`.

- [x] **Step 1: failing test** `TestClaudeCommand`: for users `""`, `root`, `awayteam` and dir `/srv/x-c`, args
  `["a \"b\" $c", "zwei\nZeilen"]`:
  - awayteam: `sudo --preserve-env=HC_TERMINAL_ID -u awayteam -H sh -lc 'cd "$1" && shift && exec
    "$HOME/.local/bin/claude" "$@"' sh /srv/x-c <args...>` (no `-i`: task 1 found that `sudo -i` expands `$`
    and drops newlines in the script and the arguments, design section 4)
  - root: `sudo --preserve-env=HC_TERMINAL_ID -H sh -lc '... exec /root/.local/bin/claude "$@"' sh /srv/x-c
    <args...>`; `""`: `sh -lc 'cd "$1" && shift && exec claude "$@"' sh /srv/x-c <args...>`.
  - no `--remote-control` in any.
- [x] **Step 2:** implement `ClaudeCommand(user, dir string, args ...string) []string` beside `ClaudeArgv`
  (shared `keepID`); doc comment says why arguments are not put into the `sh -c` text.
- [x] **Step 3:** tests green; commit `striker: fleet.ClaudeCommand for task agents`, tick 2.

### Task 3: dispatch: hc argv for a task agent on another machine, under its coordinator

Files: `striker/internal/dispatch/dispatch.go`, `dispatch_test.go`.

- [x] **Step 1: failing test** `TestRemoteCommandChange`: change `rb` with task `2.3`, repo `alpha`, machine
  `remote1`, user `awayteam`, worktree `/srv/alpha-rb`, parent `remote1/t2b` gives
  `run --on remote1 --bg --name implementer-2.3 --parent <parent per task 1> --label striker.task=2.3 --label
  striker.role=implementer --label striker.topic=rb --label striker.repo=alpha <fleet.ClaudeCommand(awayteam,
  /srv/alpha-rb, prompt)>`, prompt = `PromptChange` as today. (If task 1 found no cross-machine parent: `--label
  herdr.space=<space id>` instead of `--parent`, the space id an argument.)
- [x] **Step 2:** implement `RemoteCommandChange(role, repo string, c openspec.Change, t openspec.Task,
  machine, user, worktree, parent string) []string`, reusing the label code of `argv`.
- [x] **Step 3:** tests green; commit `striker: dispatch argv for relayed task agents`, tick 3.

### Task 4: coord: find an existing change worktree and its git runner for the relay

Files: `striker/internal/coord/coord.go`, `coord_test.go`.

- [x] **Step 1: failing test** `TestFindWorktree`: with the fake hc of the existing tests, a fleet repo with
  user `awayteam` on remote1 and change `rb` (`x-branch: rb`): `FindWorktree(c, f, "alpha", "rb")` returns the
  worktree path of branch `rb` from `git worktree list` and a `gitrepo.Repo` whose runner goes through
  `sudo -n -u awayteam` (calls seen in the fake). A missing worktree is an error naming the change; nothing is
  created (no `worktree add`).
- [x] **Step 2:** implement `FindWorktree(c hc.Client, f fleet.Fleet, repo, change string) (Place, error)` with
  `Place{Machine, User, StatusDir, Worktree string; Repo gitrepo.Repo}`, sharing `find`, the meta read and the
  branch checks with `Start` (factor them out, `Start` behaves as before: its tests stay green).
- [x] **Step 3:** tests green; commit `striker: coord.FindWorktree for the relay`, tick 4.

### Task 5: relay: requests from status files, filter, ledger

Files: new `striker/internal/relay/requests.go`, `ledger.go`, tests.

- [x] **Step 1: failing tests.**
  - `TestParseFiles`: the file names come from one `hc exec --on <m> find <dir> -maxdepth 1 -name *.json -mmin
    -1440 -printf %f\n`, each file is then read with `c.ReadFile` (as `status` does); the name decides repo and
    change (`<repo>.json` lead, `<repo>--<change>.json` coordinator), the content gives the requests. A file
    without `requests` or with unparsable JSON yields none (the status report already shows its error).
  - `TestFilter`: requests of a file that is no relayed fleet repo, a lead's `dispatch`, a coordinator's
    `start-coordinator`, a bad role or change name, a missing id are dropped with a reason; ids in the ledger
    are dropped silently.
  - `TestLedger`: load/save round trip of `~/.striker/relay.json` (tmp file, rename), keys
    `<machine>|<file>|<id>`, entries `{at, outcome, address, watch, delivered}`.
- [x] **Step 2:** implement `Request{ID, Do, Role, Task, Change, To, Text}`, `File{Machine, Path, Repo, Change,
  Requests}`, `Read(c hc.Client, f fleet.Fleet) []File`, `Filter`, `Ledger`.
- [x] **Step 3:** tests green; commit `striker: relay requests and ledger`, tick 5.

### Task 6: relay: carry out start-coordinator, dispatch, send and answer

Files: `striker/internal/relay/act.go`, tests.

- [x] **Step 1: failing tests** against a fake hc and a fake herdr runner:
  - start-coordinator from `alpha.json` calls `coord.Start(..., "alpha", "rb", <relayed brief>)` and sends the lead
    (found by `striker.lead=alpha` on remote1) `[Striker] Anfrage <id>: Koordinator für rb läuft als <addr>.`
  - dispatch from `alpha--rb.json`: `FindWorktree`, `openspec.Read` of the worktree, `ResolveChange`,
    `RemoteCommandChange` with the coordinator (`striker.coordinator=rb`, `striker.repo=alpha`) as parent; the hc
    argv run is exactly that; the answer names the new address; the ledger gets `started <addr>` and
    `watch: true`.
  - send to `implementer-2.3`: only if the ledger has that terminal started for this coordinator; text through
    `lead.Message`-style quoting as `striker send`; ledger `sent`, `watch: true`.
  - an unknown task, a missing worktree, a send to a foreign terminal: one `[Striker] Anfrage <id> abgelehnt:
    <reason>` and ledger `error <reason>`; nothing started.
  - a requester that is gone: no send, ledger still written.
- [x] **Step 2:** implement `Act(c hc.Client, run func(...string) ([]byte, error), f fleet.Fleet, briefs string,
  l *Ledger, file File, r Request)`.
- [x] **Step 3:** tests green; commit `striker: relay carries out requests`, tick 6.

### Task 7: relay: watch task agents and send their last answer

Files: `striker/internal/relay/watch.go`, tests.

- [x] **Step 1: failing tests.**
  - `TestLastAnswer`: screen texts (from task 1's real samples) give the text after the last `●` up to the
    prompt line, at most 40 lines; blocked and no-marker screens give the tail.
  - `TestWatch`: a fake `hc wait` returning done, then the message `[Striker] implementer-2.3 (remote1/t2a) ist
    fertig. Seine letzte Antwort: ...` to the coordinator, ledger `delivered: true`; blocked gives the
    "wartet auf eine Eingabe" message and keeps watching after the next send; ended gives the exit message;
    `hc wait` timing out just waits again.
  - `TestResume`: a ledger with `watch: true, delivered: false` starts a watch on `Start`.
- [x] **Step 2:** implement `Watcher` (one goroutine per address, at most one at a time, stops when delivered or
  the terminal is gone), `LastAnswer`.
- [x] **Step 3:** tests green; commit `striker: relay watches task agents`, tick 7.

### Task 8: striker serve runs the relay every 10 s

Files: `striker/cmd/striker/main.go` (`runServe`), `striker/internal/relay/relay.go`, tests.

- [x] **Step 1: failing test** `TestRelayTick`: one `Tick` reads the files, filters, acts on new requests in file
  order, saves the ledger; a second tick with the same files does nothing.
- [x] **Step 2:** `relay.Run(ctx, every)` with `Tick`; `runServe` starts it beside `s.Run` with the fleet loaded
  per tick (like the status report), `~/.striker/relay.json`, the briefs from `home.Dir()`, the herdr-app runner.
  Watches resume on start.
- [x] **Step 3:** tests green; commit `striker: serve runs the relay`, tick 8.

### Task 9: Briefs, striker/CLAUDE.md and README: relayed and direct variants

Files: `striker/briefs/coordinator.md`, `striker/briefs/lead.md`, `striker/internal/coord/coord.go` (`FillBrief`),
`striker/internal/lead/lead.go`, tests, `striker/CLAUDE.md`, `striker/README.md`.

- [x] **Step 1: failing tests:** `coord.FillBrief` and the lead brief fill `{agents}` / `{coordinators}` with the
  direct text for a repo without user and the relayed text for one with a user; the relayed text names the
  status file, the request format and "end your turn, Striker's message brings the result".
- [x] **Step 2:** move steps 2 and 3 of the coordinator's "Ablauf" into the two variants (direct: today's
  `striker dispatch`, `hc wait`, `hc read`; relayed: `dispatch` and `send` requests, never `sleep`, never poll the
  file); rule 5's coordinator line of the lead brief likewise; describe `requests` in both status formats.
- [x] **Step 3:** `striker/CLAUDE.md` (Striker relays through `striker serve`, ledger `~/.striker/relay.json`, how to
  look up what was done); `striker/README.md` (the relay in a paragraph).
- [x] **Step 4:** tests green; commit `striker: briefs for relayed agents`, tick 9.

### Task 10: Live check on remote1 with a trial change

- [x] **Step 1:** merge to main, `install.ps1` (Striker rebuilt, serve restarted).
- [x] **Step 2:** with the person's go and in a repo the person picks: a tiny trial change (one task, e.g. a
  line in a README) on its default branch; ask the lead (`striker send`) to request its coordinator.
- [x] **Step 3:** watch it through: coordinator started by the relay, `dispatch` of implementer and reviewers,
  answers in the coordinator's terminal, agents under the coordinator in the sidebar, Bridge card and board
  line, ledger entries. Screenshots; note what was not seen.
- [x] **Step 4:** tick 10 (commit `openspec: striker-relay live check`); `lcars-touch-rollout` task 6 can then be
  done with this run (tell the person).
