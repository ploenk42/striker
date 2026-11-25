# Striker in Go, bundled with the LCARS layout

Date: 2026-11-21. Status: approved by the user in chat ("plan ok").

## Why

Striker (the person's control desk: one Claude Code that talks to the per-repo leads) lives in
the old standalone repo as PowerShell scripts with a hardcoded Windows path (`D:/develop/striker`). The LCARS
board in this repo calls `scripts/status.ps1`. The person now also uses Herdr on a Mac, where there
is no Striker checkout and no Windows PowerShell. They want:

- one repo for the Herdr layout, the mods and Striker;
- Striker as a small program instead of PowerShell scripts, the same on macOS and Windows;
- the board on each computer to show the server repos (remote1) plus the leads running on that
  computer itself, depending on where the Herdr app runs.

## Goals

1. The old standalone repo moves into this repo as `striker/`, history kept (`git subtree add`).
2. One Go program, `striker`, with subcommands replacing the scripts:
   - `striker status` prints the fleet state as JSON (the shape `status.ps1` prints today).
   - `striker send <address> <text>` (or `--file`) sends a message to a lead, prefixed `[Striker]`.
   - `striker start-lead <repo>` starts a lead for a fleet repo, as `start-lead.ps1` does.
   - `striker serve` serves `GET /status` (same JSON) on `127.0.0.1:<port>`, default port 7522.
3. Builds for darwin/arm64, windows/amd64 and linux/amd64 from this repo.
4. The LCARS board (`lcars/striker.js`) reads `http://127.0.0.1:7522/status` with `fetch` on every
   platform. No PowerShell, no Striker checkout path in the mod.
5. Local leads: every terminal on this computer labeled `striker.lead=<repo>` shows on the board,
   with its status file `~/.striker/status/<repo>.json` read locally. Local repos need no
   `fleet.json` entry. On the Mac the board shows remote1 + Mac leads, on Windows remote1 +
   Windows leads.
6. Install scripts per platform: `install.sh` (macOS: symlinks, layout copy, build, launchd agent
   for `striker serve`) and `install.ps1` (Windows: junctions, layout copy, build, scheduled task at
   logon).
7. Everything keeps working on macOS and on Windows.

## Non-goals

- No daemon on remote1. The local `striker serve` reaches remote1 through the local `hc`
  (`hc ls`, `hc get`, `hc file read --on remote1`), as `status.ps1` does today. A server-side
  service is a later option if something without a Herdr client needs the state.
- Striker's own Claude stays on the computer in front of the person. Moving it is out of scope.
- No change to the lead brief's status-file format.
- Blueprint stays unchanged.

## Anti-goals

- A port reachable from outside the computer (bind only to 127.0.0.1).
- Windows argument mangling of quotes in `send` (the reason for `native.ps1`): Go's `os/exec`
  must pass text exactly.
- Breaking the Windows setup while building the Mac one.

## Data

`fleet.json` lists server repos only (`machine` is a Herdr machine name, e.g. `remote1`), plus
`status_dir` per machine. The `epsilon` entry with `machine: "local"` is removed; local repos
are discovered.

Status JSON (unchanged shape):

```json
{ "updated": "<ISO 8601 UTC>",
  "repos": [ { "name", "title"?, "machine": "remote1" | null, "path", "space",
               "lead": { "address", "state", "msg", "status_ms", "where" } | null,
               "status": <the lead's status file> | { "error": "..." } | null } ] }
```

`machine: null` means this computer. Lead addresses are as the local `hc` reports them (remote
ones already carry the `remote1/` prefix), so the board can call `agent.focus` with them.

Discovery of local leads: `hc ls --json` carries no labels and `--label` needs a value, so
`striker` runs `hc ls --json`, then `hc get <t> --json` (which has `labels`) for each local terminal
(address without machine prefix), and keeps those with a `striker.lead` label. A local repo's `name`
is the label value, `path` its terminal's `cwd`, `space` the terminal's `where` before " / ".

## Constraints and facts

- `hc` is on PATH on both computers; Go is not installed on the Mac yet (`brew install go`).
- The Mac has no Herdr machine `remote1` yet. Adding it (`hc machine add remote1`) needs the
  remote1 SSH host key verified by the person first. Until then the board shows local leads and
  an error line for remote1 repos.
- Herdr does not hot-reload mods linked via junction/symlink; restart the window after a change.
- Files with CRLF line endings keep them.
- The layout's `striker-prompt` block points at terminal `t12`, a Windows-local address.

## Left to the builder

- Go package layout, HTTP details, timeouts (a fleet poll may take up to ~60 s through `hc`;
  `serve` caches the last answer and refreshes in the background every 20 s).
- How `striker` finds `fleet.json` and `briefs/`: `STRIKER_HOME`, default `<user home>/herdr-mods/striker`
  (decided in the plan).
