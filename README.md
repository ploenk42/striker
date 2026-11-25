# herdr-mods

My Herdr app mods, kept outside `~/.config/herdr-app`.

**Documentation:** https://ploenk42.github.io/striker/ (Striker, the fleet, the bridge, setup and reference).

Each folder is linked into Herdr's
`mods/` with a junction:

```powershell
cmd /c mklink /J "$env:USERPROFILE\.config\herdr-app\mods\<name>" "$env:USERPROFILE\herdr-mods\<name>"
```

Herdr doesn't hot-reload a mod linked this way: restart the window after a change.

Two installers do the whole setup; both need [Go](https://go.dev/dl/) in `PATH`:

- macOS: `./install.sh` symlinks each mod into `~/.config/herdr-app/mods/`, copies `layouts/lcars.json`
  there, builds `striker/bin/striker` and installs the LaunchAgent `io.github.ploenk42.striker` (log:
  `~/Library/Logs/striker.log`).
- Windows: `.\install.ps1` does the same with junctions and registers a hidden scheduled task `Striker`
  that runs at logon.

Both start `striker serve`, which answers on `127.0.0.1:7522/status`. Every mod has to keep working on
both macOS and Windows.

| Folder | What it is | Origin |
|---|---|---|
| `lcars/` | The LCARS frame, extended with Striker's fleet board (`striker.js`, the STRIKER slab in the sidebar, the count in the status arm); Shepherdr removed from the sidebar | [jkuepker/herdr-layouts](https://github.com/jkuepker/herdr-layouts) `lcars/`, MIT (`lcars/LICENSE`) |
| `blueprint/` | Diagrams of what an agent builds, beside it | [chappse6/herdr-blueprint-app](https://github.com/chappse6/herdr-blueprint-app), MIT, unchanged |
| `layouts/lcars.json` | A copy of the LCARS layout, with the `work-view` stack (terminals or Striker's board) and Victor Mono as the terminal font | from jkuepker's `lcars/layouts/lcars.json` |
| `lcars/fonts/VictorMono-*.ttf` | Terminal font of the LCARS layout, loaded when the window starts | [Victor Mono](https://rubjo.github.io/victor-mono/) v1.5.6, SIL OFL 1.1 (`lcars/fonts/VictorMono-LICENSE.txt`) |

`layouts/` is a copy: Herdr reads `~/.config/herdr-app/layouts/lcars.json`. Copy it back there to restore it.

The board reads Striker's state from `http://127.0.0.1:7522/status`, served by `striker serve` (`striker/`).
Besides the repos in `striker/fleet.json`, it discovers local leads (hc label `striker.lead`) by itself.

## Specs and changes

Specs and work live in `openspec/`: living capability specs in `openspec/specs/`, each piece of work as a change in `openspec/changes/<name>/` (`proposal.md`, `design.md`, `tasks.md`, `plan.md`), finished changes in `openspec/changes/archive/<date>-<name>/`. The superpowers skills write into it; the workflow is in `AGENTS.md`.

## Bridge

One LCARS view per worktree (`lcars/bridge.js`, data from `striker serve`), with three modes: **PLAN** (the change's
tasks with WORKING, NEEDS YOU and DONE cards), **SPEC** (the spec with its deltas marked) and **OVERVIEW** (all
changes of the repo with their headings and tasks); the bottom field shows
the repo tree or an agent's terminal. The full description is on the documentation site.

The terminal swap uses `layout.set`, so `layouts/lcars.json` is stored in Herdr's own key order with LF line ends
(`.gitattributes`); keep that form when you edit it. Colours and shapes: `docs/lcars-colors.md`.

### After a change

Herdr does not hot-reload the symlinked mod reliably. Run `./install.sh` (macOS) or `.\install.ps1` (Windows) after changes.
