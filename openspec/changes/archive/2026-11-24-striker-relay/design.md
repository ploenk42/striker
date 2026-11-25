# Design: Striker relays hc for agents without it

Decision of the person (2026-11-24): agents that run as another user (`awayteam`) do not get the person's hc
socket; Striker, which has it, does the hc work for them, and task agents run as the fleet user through `sudo`
like the leads.

## 1. Who is relayed

A fleet repo with a `user` (today: all four remote1 repos, `awayteam`) is **relayed**: its lead and its
coordinators cannot run `striker` or `hc`. A repo without `user` (this computer) is **direct**: its agents keep
calling `striker dispatch`, `striker start-coordinator`, `hc wait` and `hc read` as today. Nothing below changes
direct repos.

## 2. Requests in the status file

A relayed agent asks by adding entries to `requests` in its own status file (`<status_dir>/<repo>.json` for the
lead, `<status_dir>/<repo>--<change>.json` for a coordinator). The rest of the file stays as it is.

```json
"requests": [
  { "id": "c-omega", "do": "start-coordinator", "change": "omega" },
  { "id": "2.3-impl-1", "do": "dispatch", "role": "implementer", "task": "2.3" },
  { "id": "2.3-impl-fix-1", "do": "send", "to": "implementer-2.3", "text": "Spec review found: ..." }
]
```

- `id`: chosen by the agent, unique within its file (a new attempt gets a new id). Striker carries out each id
  once; requests it has done may stay in the file or be removed, Striker ignores them either way.
- `start-coordinator` (lead only): `change` of this repo. Striker runs `striker start-coordinator <repo> <change>`.
- `dispatch` (coordinator only): `role` (`implementer`, `spec-reviewer`, `quality-reviewer`, ... as
  `dispatch.ValidRole`) and `task` (task id or `#issue`) of the coordinator's own change.
- `send` (coordinator only): `text` to a task agent Striker started for this coordinator (`to` is its terminal
  name `<role>-<task>`); used for the fix rounds after a review. The answer comes back like a dispatch result.

Striker never writes into an agent's status file (two writers would race). What it has done lives in its own
ledger `~/.striker/relay.json`: per request key `<machine>|<status file>|<id>` the time, the outcome (`started
<address>`, `sent`, `error <text>`) and, for agents it watches, whether their result was delivered.

## 3. The relay loop (`striker serve`)

A new package `striker/internal/relay`, run by `striker serve` every 10 s (its own ticker, beside the 20 s status
report):

1. For each machine with relayed repos: list the status files of the machine's `status_dir` changed within a
   day (one `hc exec ... find`), read each with `hc file read`, parse `requests`.
2. Drop requests whose key is in the ledger, and files that belong to no relayed fleet repo (`<repo>.json`,
   `<repo>--<change>.json` with a fleet repo name and a valid change name).
3. Carry out the rest (section 4), write the ledger after each one, then answer the requester (section 5).
4. Watches (section 6) run as goroutines; their state is in the ledger, so a restart of `striker serve`
   resumes them.

Errors are answered, never retried silently: a failed request gets `error <text>` in the ledger and a
`[Striker]` message with the reason; the agent retries with a new id if it wants to.

## 4. Carrying out a request

- **start-coordinator:** `coord.Start` as `striker start-coordinator` does (it already works across machines and
  starts Claude as the fleet user). The brief it sends is the relayed variant (section 7).
- **dispatch:** the coordinator's worktree is the one `coord` derives from the change's `x-branch` (same
  function as `start-coordinator`). Striker reads the change from that worktree through hc
  (`openspec.Read(gitrepo.Remote, "")`), resolves the task (`dispatch.ResolveChange`), builds the prompt
  (`dispatch.PromptChange`) and the labels (`striker.task`, `striker.role`, `striker.topic`, `striker.repo`) as today,
  and starts:

  ```
  hc run --on <machine> --bg --name <role>-<task> --parent <coordinator> --label ... --
     sudo --preserve-env=HC_TERMINAL_ID -u <user> -H sh -lc 'cd "$1" && shift && exec "$HOME/.local/bin/claude" "$@"' sh <worktree> <prompt>
  ```

  The worktree and the prompt go in as arguments, not inside the `sh -c` text, so no quoting of the prompt is
  needed. **No `sudo -i` here** (task 1): `-i` hands the command to the target's login shell as one escaped
  string, which leaves `$` unescaped and turns newlines into line continuations; `cd "$1"` became `cd ""` (the
  agent started in `/home/awayteam`), `$c` in an argument vanished, `zwei\nZeilen` came out `zweiZeilen`.
  `sudo -H sh -lc` execs directly: every argument arrives unchanged, `HOME` is the fleet user's, the login
  profile puts `~/.local/bin` on `PATH`, and `--preserve-env=HC_TERMINAL_ID` still works with `-n`. Root
  (no fleet repo today, untested): `sudo --preserve-env=HC_TERMINAL_ID -H sh -lc '... exec /root/.local/bin/claude
  "$@"' sh <dir> <args>`. A new `fleet.ClaudeCommand(user, dir, args...)` builds this beside `fleet.ClaudeArgv`
  (which keeps its `-i` form and its `--remote-control` for leads and coordinators: it has no arguments after
  the script and single-quotes the dir; task agents get no Remote Control, they would crowd claude.ai).
  `--parent` makes the agent a sub-agent of the coordinator: the sidebar shows it under the coordinator, and it
  ends with it. Task 1 checked this from this computer for a parent and child on remote1: hc takes both
  `--parent remote1/tX` and `--parent tX` (the bare id resolves on the `--on` machine), `hc get` shows
  `"parent":"remote1/tX"` and `"die_with_parent":true`, and `hc kill` of the parent ends the children
  (`"reason":"killed"`). The relay passes the coordinator's full address (`remote1/tX`).
- **First start in a worktree:** a folder Claude has not seen as that user shows the trust dialog; the agent is
  then `blocked` right away and the watch (section 6) reports it. Change worktrees of a repo the fleet user
  already trusts may or may not inherit that trust; task 10 shows it on the live run.
- **send:** only to a terminal Striker started for this coordinator (ledger), with `striker send` semantics
  (quotes intact, waits for a draft). Then the agent is watched again.

## 5. Answers to the requester

Striker writes into the requester's terminal with `striker send`, always beginning `[Striker]`:

- `[Striker] Anfrage 2.3-impl-1: implementer-2.3 läuft als remote1/t2a.`
- `[Striker] Anfrage c-omega: Koordinator für omega läuft als remote1/t2b.`
- `[Striker] Anfrage 2.3-impl-1 abgelehnt: kein Task 2.3 im Change omega.`

The requester is found by its labels (`striker.lead=<repo>`, or `striker.coordinator=<change>` with
`striker.repo=<repo>`) on its machine. If it is gone, the answer is only in the ledger.

## 6. Watching task agents

For every agent Striker started or sent to: `hc wait <address> --timeout 600` in a loop until its status is
`done` or `blocked`, or the terminal ended. Then Striker reads the screen with 200 lines of history above it
(`hc read <address> --lines 200`, so a long answer's `●` is usually still in it) and sends the coordinator:

- done: `[Striker] implementer-2.3 (remote1/t2a) ist fertig. Seine letzte Antwort: <the last answer block>`
- blocked: `[Striker] implementer-2.3 (remote1/t2a) wartet auf eine Eingabe: <screen tail>`; the coordinator
  decides, or puts it into its `needs_you` (the person answers in the agent's terminal).
- ended: `[Striker] implementer-2.3 (remote1/t2a) ist beendet (exit <code>).`

"Last answer block" is the text after the agent's last `●` answer marker up to the prompt line, at most 40
lines; the briefs already ask task agents to end with a one-paragraph summary. The coordinator reads details
itself in the worktree (git, files), which it may as the same user.

What hc gives (task 1, Claude Code 2.1.331 as awayteam on remote1). `hc wait <addr> --timeout <s>` prints one
JSON object and exits 0 in every case; `ended` only says the wait ended, not the terminal:

| case | `reason` | `status` | `ended` | `exit_code` |
|---|---|---|---|---|
| done | `status` | `done` | true | null |
| blocked (dialog) | `status` | `blocked` | true | null |
| terminal ended | `exit` | null | true | e.g. 129 after `hc kill` |
| timeout | `timeout` | null | false | null |

So the loop goes on while `reason` is `timeout`, and decides by `reason`/`status` otherwise (`turn` counts the
agent's turns). `hc read <addr> --lines 60` of a done agent shows the whole screen, with blank padding:

```
❯ <prompt>

● 2+3 ergibt 5, fertig $x

✻ Cogitated for 2s · done 11:02 AM
<blank lines>
──────── (rule)
❯
──────── (rule)
  Opus 5.5 · 43K
```

The block is the lines from the last line starting with `●` up to the first line starting with `✻` or `────`,
or a `❯` prompt line, trailing blanks dropped, the last 40 lines kept. If the `●` scrolled out even of 200
lines, the block is the text before the last `✻` status line, back to the previous `❯` prompt or the top
(no status line, rules or footer). A blocked screen has no new `●` (the trust dialog: "Accessing workspace:
... ❯ No, exit / Yes, I trust this folder"); for it Striker sends the last non-blank lines (at most 20), once per
blockade: the next message about a blockade comes only after `hc get` showed the agent in another state.

In a wide terminal Claude draws its diff panel as a right column beside the chat (seen on this computer, 200
columns): every line has a `│` in the same column (~110), most with a space before it, and text to its right
("No changes this session", file names). Striker cuts every line at that column: a `│` after a space in the same
column of at least half the non-blank lines, at least two, from column 40 on. A `│` of the answer itself (a
table) stays.

Turns and errors (checked on this computer): `hc wait` answers a done agent at once with its current `turn`,
so after a `send` the first wait still shows the turn Striker delivered; Striker waits again (every 10 s) until the
turn is higher, and tells the coordinator once if none began within 30 min (`hat seit der letzten Nachricht
keinen neuen Turn begonnen`), watching on. A terminal that does not exist makes `hc wait` and `hc get` exit 3
with `{"error":{"kind":"not_found",...}}` on stderr; only that ends a watch as `ist beendet.`. Any other
failure (no connection to the machine, `machine_unknown`, a timeout of the call) is waited out with a pause.

## 7. Briefs

- `briefs/coordinator.md` gets the steps for starting, waiting and reading as a placeholder `{agents}`, filled
  by `coord` with the direct text (today's: `striker dispatch`, `hc wait`, `hc read`) or the relayed text:
  write a `dispatch` request, end the turn, Striker's message brings the result; fix rounds with `send`; never
  `sleep`, never poll the file.
- `briefs/lead.md` rule 5's line on starting coordinators likewise: direct `striker start-coordinator`, relayed a
  `start-coordinator` request.
- Both describe `requests` in the status file format.
- `striker/CLAUDE.md`: Striker (the Claude) knows that `striker serve` relays, and that the ledger tells what was
  done (`~/.striker/relay.json`).

## 8. Safety

What an agent can make Striker do is fixed by the request kinds: start Claude as the fleet user in that repo's
own worktree, with a prompt Striker builds from that change's `tasks.md`/`plan.md`, or send text to an agent of
the same coordinator. No request runs a command of the agent's choosing, names another repo, machine or user,
or reaches a terminal Striker did not start for it. Roles and change names are checked against the existing
patterns (`dispatch.ValidRole`, the change name rules of `coord`).

## 9. Not in this change

- `striker issues` (GitHub issues for the task lines) for relayed repos: it needs `gh` and a token on the agent's
  side. Relayed coordinators dispatch by task id; tasks then carry no `#issue`.
- Terminal names `<role>-<task>` are unique per hc server only; two changes of one machine dispatching the same
  role and task id at once collide (also true today). Not addressed here.

## 10. Tests

- `relay`: requests parsed and filtered (unknown files, done ids, wrong kinds per file); each kind against a
  fake hc (argv of `hc run`, the answer text); ledger round trip; a restart resumes an open watch; errors
  answered once.
- `fleet.ClaudeCommand`: argv per user; worktree and prompt as arguments (a prompt with quotes, `$`, newlines).
- Answer block extraction from screen texts (done, blocked, no marker).
- `coord`: brief placeholders filled per repo kind.
- Live (last task): `lcars-touch-rollout` task 6 run through the relay on remote1.

## 11. Live check (task 10, 2026-11-24)

Trial change `relay-probe` in `gamma` on remote1 (lead `remote1/t1g`, awayteam), after the merge to main
(d3b7a29) and `install.ps1`. Seen:

- The lead wrote `{ "id": "c-relay-probe-1", "do": "start-coordinator", "change": "relay-probe" }`; within a tick
  the relay started the coordinator `remote1/t1t` (worktree `/srv/gamma-relay-probe`, relayed brief) and answered
  the lead `[Striker] Anfrage c-relay-probe-1: Koordinator für relay-probe läuft als remote1/t1t.`
- The coordinator requested `dispatch` for implementer, spec-reviewer and quality-reviewer one after another; each
  got its `läuft als …` answer and, when done, `[Striker] <name> (<addr>) ist fertig. Seine letzte Antwort: …` with
  the clean answer block (no diff column, no footer).
- `hc get remote1/t1v`: `parent: remote1/t1t`, `die_with_parent: true`, labels `striker.task/role/topic/repo`, argv
  `sudo --preserve-env=HC_TERMINAL_ID -u awayteam -H sh -lc '…' sh /srv/gamma-relay-probe <prompt>`, placed in the
  coordinator's tab (`gamma-relay-probe / coord relay-probe`). No trust dialog in the change worktree.
- Ledger `~/.striker/relay.json`: four entries, `started <addr>`, the three task agents `delivered: true`, `turn: 1`.

Not seen: the Bridge card and board line (no screenshots taken by the coordinator), a `send` request, a blocked
task agent, a lost connection. Side effects in gamma: the lead switched `/srv/gamma` from a feature branch to the
repo's default branch to commit the change there; the coordinator added `skip_specs: true` to the probe's
`.openspec.yaml` so `openspec validate --strict` passes for a docs-only change.
