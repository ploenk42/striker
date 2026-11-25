# Demo fleet

The screenshots on the documentation site show this demo fleet, never the person's own: two small git repos,
`alpha` and `beta`, a demo `STRIKER_HOME` and lead status files with invented, harmless questions.

```
demo/
  demo.ps1                  sets the demo up (-Target) and removes it (-Clean)
  repos/alpha/              README, AGENTS.md (OpenSpec section), change add-greeting with three tasks
  repos/beta/               README, one spec (charts), one archived change (chart-list)
  home/fleet.json           demo fleet.json; {repos} and {status} are filled in by demo.ps1
  status/alpha.json         lead status files, copied into the status dir
  status/beta.json
```

## Setting it up and removing it

```powershell
.\docs-site\demo\demo.ps1 -Target C:\Temp\striker-demo
.\docs-site\demo\demo.ps1 -Clean C:\Temp\striker-demo
```

`-Target` makes, in a folder that must not exist yet:

- `repos\alpha` and `repos\beta`: git repos on `main`, committed as `Demo <demo@example.invalid>`;
- `home\fleet.json` (both repos on this computer, spaces `demo-alpha` and `demo-beta`, no machine, no user)
  and `home\briefs\` (copied from `striker\briefs`); this folder is the demo `STRIKER_HOME`;
- `demo.manifest`: the status files the demo owns, so `-Clean` knows what to remove.

It copies `status\alpha.json` and `status\beta.json` into the status dir, by default `~\.striker\status`: Striker
reads the status files of leads on this computer there, whatever `STRIKER_HOME` says. It stops without changing
anything if `alpha.json`, `beta.json` or `alpha--add-greeting.json` is already there. `-StatusDir <folder>`
uses another folder (for a dry run; Striker then does not see the files).

`-Clean` removes those status files (also the coordinator's `alpha--add-greeting.json`) and the whole demo
folder, including the worktree `repos\alpha-add-greeting` the coordinator makes. It refuses a folder without
`demo.manifest`.

## Taking the shots

The shots stop the person's `striker serve` (and with it the relay for remote leads) for a while and open demo
spaces in the person's Herdr. Ask first and agree a time window. **Herdr is never restarted**: everything
below runs inside the running app.

1. **Set up:** `.\docs-site\demo\demo.ps1 -Target C:\Temp\striker-demo`.
2. **Stop Striker's task:** `Stop-ScheduledTask -TaskName Striker`. Check that no `striker serve` is left
   (`Get-Process striker`; a `striker` you started by hand elsewhere is not yours to stop without asking).
3. **Demo serve:** in a new PowerShell, point this shell (only this one) at the demo and start serve in the
   foreground:

   ```powershell
   $env:STRIKER_HOME = 'C:\Temp\striker-demo\home'
   striker serve
   ```

   Leave it running. Every `striker` command below runs in a second shell with the same `$env:STRIKER_HOME`.
4. **Demo leads:** `striker start-lead alpha` and `striker start-lead beta`. Each opens its space (`demo-alpha`,
   `demo-beta`), starts Claude in the repo and sends the lead brief; the lead reads its status file as the
   handover and keeps the `needs_you`. If a lead does not start (for example because `sh` is missing), open the
   space by hand, start `claude` in the repo, label it (`hc label <address> striker.lead=alpha`) and send the
   brief as `striker/CLAUDE.md` describes. The brief is German, so tell each demo lead (and later the
   coordinator) to answer and write its status file in English; otherwise German status lines end up in the shots.
5. **A live change:** `striker start-coordinator alpha add-greeting` (from the demo shell, so it finds the demo
   fleet). It creates the worktree `repos\alpha-add-greeting`, opens the space `alpha-add-greeting` with the
   tab `coord add-greeting` and sends the coordinator brief. Tasks 2 and 3 depend only on task 1, so after task
   1 the coordinator runs task agents in parallel: wait for that before shooting PLAN. Answer questions from the
   coordinator only with harmless demo answers; nothing it does leaves the demo folder (no remote, nothing to
   push).
6. **Shoot** with `herdr-app shot`: Fleet Operations, PLAN (with running task agents and the REPO tree), SPEC
   (change deltas in `alpha`, vision and spec list on `main` in `beta`), REPO, a terminal in the bottom field.
   striker, Striker's own repo, may appear on the board.
7. **Crop and check:** the sidebar still lists the person's other spaces. Crop every shot to the work view, or
   cover the sidebar list with a plain LCARS-coloured block. Scale to at most 1920 px wide, keep each PNG under
   ~400 KB, and check each one by eye for private names (the person's machines, repos, users, home paths)
   before it goes into `public/shots/`.
8. **Tear down:**
   - stop the demo serve (Ctrl+C in its shell) and close both demo shells;
   - close the demo spaces `demo-alpha`, `demo-beta` and `alpha-add-greeting` in Herdr (only these; the
     person's other spaces stay as they are);
   - `.\docs-site\demo\demo.ps1 -Clean C:\Temp\striker-demo`;
   - **start Striker's task again:** `Start-ScheduledTask -TaskName Striker`, then check that `striker serve` runs
     (`Get-Process striker`) and that Fleet Operations shows the person's fleet again.
