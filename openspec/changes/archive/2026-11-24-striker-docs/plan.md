# Plan: striker-docs

Design: `design.md`. The site lives in `docs-site/`; build with
`& "C:\Program Files\nodejs\npm.cmd" --prefix C:\Users\you\herdr-mods-striker-docs\docs-site run build`
(path of the worktree; adjust to where the coordinator runs). Shell rules of AGENTS.md apply to every step (one
simple command, absolute paths, no pipes or redirects; files with Read, Edit and Write). Never run an interactive
scaffolder (`npm create …`): write the files by hand. Pages are English. No private data in any page or image:
not the person's machine names, repos, users, home paths or lead questions (list in Task 2's findings); use the
demo fleet (`alpha`, `beta`, machine `demo-remote`, user `awayteam`). Commit per task (`Task <id>: …`); the
task's line in `tasks.md` and its steps here are ticked in the commit with its last work. Nobody pushes, and
nobody changes the repository's visibility or GitHub settings.

### Task 1: docs-site skeleton: Next.js + Fumadocs, static export, search, Mermaid component

Files: `docs-site/package.json`, `package-lock.json`, `next.config.mjs`, `source.config.ts`, `tsconfig.json`,
`app/layout.tsx`, `app/layout.config.tsx`, `app/docs/layout.tsx`, `app/docs/[[...slug]]/page.tsx`,
`app/api/search/route.ts`, `app/page.tsx` (redirect to `/docs`), `lib/source.ts`, `components/mermaid.tsx`,
`mdx-components.tsx`, `content/docs/index.mdx`, `content/docs/meta.json`, `.gitignore` (`node_modules`, `.next`,
`out`, `.source`).

- [x] **Step 1: versions.** `npm view <pkg> version` for `next`, `react`, `react-dom`, `fumadocs-core`,
  `fumadocs-ui`, `fumadocs-mdx`, `mermaid`, `typescript`, `@types/react`, `@types/node`, `tailwindcss`,
  `@tailwindcss/postcss`; check the Fumadocs docs (fumadocs.dev, "Manual installation" and "Static export") for
  the versions that belong together, and pin them exactly (no `^`). Note the chosen versions in the commit text.
- [x] **Step 2: app.** Write the files above following Fumadocs' manual installation for the pinned version.
  `next.config.mjs`: `output: 'export'`, `basePath: '/herdr-mods'`, `trailingSlash: true`,
  `images: { unoptimized: true }`, wrapped with `createMDX()`. Search: the static search route
  (`createFromSource` with `staticGET`, `export const revalidate = false`) and the UI's static search client.
  Title "Striker", nav link to the GitHub repo.
- [x] **Step 3: Mermaid.** `components/mermaid.tsx` (`'use client'`): takes `chart: string`, renders it with
  `mermaid.render` in `useEffect` into an SVG, re-renders when `useTheme().resolvedTheme` changes (`theme:
  'dark'` / `'default'`), shows the source in a `<pre>` if rendering fails. Register it in `mdx-components.tsx`.
  `index.mdx` holds a placeholder text and one tiny flowchart to prove it works.
- [x] **Step 4:** `npm install` (writes the lockfile), `npm run build`: green, `docs-site/out/docs/index.html`
  exists and its asset URLs start with `/herdr-mods/`. `npm run dev`, open the page in a browser: the diagram
  shows in light and dark. Commit `Task 1: docs-site skeleton`, tick 1.

### Task 2: Audit the repository and its history for private data before it goes public

No change to tracked files except a findings file; nothing is rewritten.

- [x] **Step 1: tree.** Grep the tree (excluding `docs-site/node_modules`) for: the person's machine names
  and hostnames, home paths (`/home/`, `C:\Users\`, `/Users/`), `/mnt/`, `/opt/`, e-mail addresses, IPs,
  tokens or keys (`ghp_`, `sk-`, `token`, `secret`, `password`), the person's repo and family names from
  `striker/fleet.json` and the briefs. Read `striker/fleet.json`, `striker/setup/*`, `layouts/lcars.json`,
  `striker/launchd/*`, every test fixture with paths.
- [x] **Step 2: history.** `git -C <repo> log --all --format=%an%x09%ae` (authors and e-mails),
  `git -C <repo> log --all -p -G <pattern>` per pattern from step 1 for files since deleted, and the names of
  all files ever committed (`git -C <repo> log --all --name-only --format=`).
- [x] **Step 3: findings.** Write `openspec/changes/striker-docs/audit.md` (kept in the private history, not
  in the public tree): one row per finding (where, what,
  risk, proposal: keep / generalise in a new commit / rewrite history), and the list of private names the shots
  and pages must never show (used by tasks 8 and 11). Put one `needs_you` entry (`kind: abnahme`, task 2)
  asking the person to decide the findings. Commit `Task 2: audit for going public`, tick 2. Generalising
  commits come only after the person's answer, as their own commits referencing task 2.

### Task 3: Concepts pages with the command-chain diagram

Files: `content/docs/concepts/{meta.json,roles.mdx,command-chain.mdx,decisions.mdx,openspec-workflow.mdx}`.
Sources: `striker/CLAUDE.md`, `striker/briefs/*.md`, `striker/internal/lead/*.md`, `striker/internal/coord/*.md`,
`AGENTS.md`, design sections 1, 3, 4A.

- [x] **Step 1:** `roles.mdx`: one section per role (human, Striker, lead, coordinator, task agent): what it does,
  where it runs, as which user, which hc labels it carries, how it is started.
- [x] **Step 2:** `command-chain.mdx`: diagram A as `<Mermaid chart={…} />` (flowchart TD, solid edges down
  with the mechanism as label, dashed edges up), then the text: the way down (`striker send`,
  `striker start-coordinator`, `striker dispatch` or relay requests) and the way up (status files, `needs_you`,
  `striker status`).
- [x] **Step 3:** `decisions.mdx` (decisions stay with the human, verbatim quotes, no `--force`, no
  interrupt) and `openspec-workflow.mdx` (change layout, tasks and plan, `x-branch` worktree, coordinator,
  review roles, merge, validate, archive).
- [x] **Step 4:** build green, pages look right in `npm run dev`. Commit `Task 3: concepts pages`, tick 3.

### Task 4: Security and relay pages with the trust-boundary and relay diagrams

Files: `content/docs/security.mdx`, `content/docs/relay.mdx`, `meta.json` entries. Sources:
`striker/internal/relay/`, `striker/internal/fleet/user.go`, `striker/setup/awayteam.sh`, `striker/README.md`,
the archived design `openspec/changes/archive/2026-11-24-striker-relay/design.md`, design sections 4B, 4C, 5.

- [x] **Step 1:** `security.mdx`: diagram B (flowchart with subgraphs as in design 4B), then the rules of design
  section 5, each with its reason, and the closing "what this does not protect against". Check every rule
  against the code (which request kinds each role may send, the sudo command line, the ledger key).
- [x] **Step 2:** `relay.mdx`: diagram C (sequenceDiagram: lead, status file, striker serve, hc, coordinator,
  task agent), then request kinds with their JSON, ids and the ledger, the answer messages (accepted and
  rejected, quoted from the code), what serve sends when a task agent is done, blocked or gone.
- [x] **Step 3:** build green, diagrams render. Commit `Task 4: security and relay pages`, tick 4.

### Task 5: Setup pages

Files: `content/docs/setup/{meta.json,prerequisites.mdx,install.mdx,fleet.mdx,remote-machine.mdx,first-lead.mdx}`.
Sources: `install.ps1`, `install.sh`, `README.md`, `striker/README.md`, `striker/setup/*`, `striker/internal/fleet/`.

- [x] **Step 1:** `prerequisites.mdx` (Herdr app with herdr-core and `hc`, Claude Code, Go, Node, OpenSpec CLI
  `npm install --global @fission-ai/openspec`; macOS and Windows) and `install.mdx` (what each installer links,
  copies, builds and registers; `striker serve` on `127.0.0.1:7522`; `STRIKER_HOME`; how to check it runs).
- [x] **Step 2:** `fleet.mdx` (every field of `fleet.json` with the demo values, `status_dir`, `agents_dir`,
  local leads found by label) and `remote-machine.mdx` (hc on the second machine, running `setup/awayteam.sh`
  as root, the sudo rule, settings and status line, copying files with `hc put`).
- [x] **Step 3:** `first-lead.mdx`: `striker start-lead alpha`, what the brief asks, the first status file, a
  first change and its coordinator (direct and relayed). Build green. Commit `Task 5: setup pages`, tick 5.

### Task 6: Reference pages

Files: `content/docs/reference/{meta.json,cli.mdx,status-file.mdx,labels.mdx,files.mdx}`.

- [x] **Step 1:** `cli.mdx`: every subcommand from `striker`'s usage text (`striker/cmd/striker`), with arguments,
  what it does and an example; legacy plan-file forms marked as legacy, `striker issues` marked as legacy.
- [x] **Step 2:** `status-file.mdx` (lead and coordinator fields, `needs_you` kinds, `task`, `requests`; from
  `briefs/lead.md`, `briefs/coordinator.md`), `labels.mdx` (every `striker.*` label, who sets it, who reads it),
  `files.mdx` (`~/.striker/status`, `~/.striker/agents`, `~/.striker/relay.json`, `STRIKER_HOME`, the briefs).
- [x] **Step 3:** build green. Commit `Task 6: reference pages`, tick 6.

### Task 7: Demo fleet script

Files: `docs-site/demo/README.md`, `docs-site/demo/demo.ps1`, `docs-site/demo/home/` (demo `STRIKER_HOME`:
`fleet.json`, `briefs/` copied from `striker/briefs` by the script), `docs-site/demo/status/*.json`,
`docs-site/demo/repos/` templates.

- [x] **Step 1:** templates: repo `alpha` (README, `AGENTS.md` with the OpenSpec section, change `add-greeting`
  with proposal, design, three tasks with `after`, plan) and repo `beta` (README, one archived change, one spec);
  status files for both leads with invented, harmless `needs_you` (e.g. "Approve the greeting wording?").
- [x] **Step 2:** `demo.ps1 -Target <folder>` creates the two git repos from the templates (commits as
  "Demo <demo@example.invalid>"), writes the demo `fleet.json` with their paths, copies the status files to
  the demo status dir. `demo.ps1 -Clean <folder>` removes it all. Inside a script the AGENTS.md shell rules do
  not bind, but keep it plain PowerShell 5.1.
- [x] **Step 3:** `demo/README.md`: how to take shots (design section 6), including stopping and restarting the
  scheduled task `Striker` and never restarting Herdr. Run the script into a scratch folder and `-Clean` it once.
  Commit `Task 7: demo fleet`, tick 7.

### Task 8: Screenshots from the demo fleet, bridge pages and the introduction

Files: `docs-site/public/shots/*.png`, `content/docs/bridge/{meta.json,fleet-operations.mdx,plan.mdx,spec.mdx,repo.mdx}`,
`content/docs/index.mdx`.

- [x] **Step 1: ask first.** The shots stop `striker serve` (and its relay) for a while and open demo spaces in
  the person's Herdr. Put a `needs_you` (`kind: freigabe`, task 8) asking for a time window; wait for the
  quoted answer.
- [x] **Step 2: shoot.** Follow `demo/README.md`: demo up, demo leads, a coordinator on `alpha/add-greeting`
  with parallel task agents, then `herdr-app shot` of Fleet Operations, PLAN (with running agents and the REPO
  tree), SPEC (change deltas, vision and spec list on main), REPO, a terminal in the bottom field. Demo down,
  task `Striker` started again; check `striker status` shows the real fleet.
- [x] **Step 3: cut.** Crop each shot to the work view (or cover the sidebar list with an LCARS-coloured
  block), scale to 1920 px wide, keep under ~400 KB. Read every image and check it against the private-name
  list from Task 2; drop any that shows one.
- [x] **Step 4: pages.** The four bridge pages explain what each part of the view means and what a click does
  (from `README.md` "Bridge", `docs/lcars-colors.md`, `lcars/striker.js`, `lcars/bridge.js`); the introduction
  gets the Fleet Operations shot. Build green. Commit `Task 8: screenshots and bridge pages`, tick 8.

### Task 9: GitHub Pages workflow

Files: `.github/workflows/docs.yml`.

- [x] **Step 1:** on `push` to `main` with `paths: ['docs-site/**', '.github/workflows/docs.yml']` and
  `workflow_dispatch`; `permissions: contents: read, pages: write, id-token: write`; `concurrency: pages`.
  Job build: checkout, setup-node (Node LTS, npm cache on `docs-site/package-lock.json`), `npm ci` and
  `npm run build` in `docs-site`, `upload-pages-artifact` with `docs-site/out`. Job deploy: `deploy-pages`,
  environment `github-pages`. Every action pinned by full commit SHA with the tag as a comment (look the SHAs
  up with `gh api repos/<owner>/<action>/git/ref/tags/<tag>`).
- [x] **Step 2:** check the YAML by reading it once more against the GitHub docs for these actions; it is not
  run before the repo is public. Commit `Task 9: pages workflow`, tick 9.

### Task 10: README: drop the GitHub-issue sections, link the site

- [x] **Step 1:** root `README.md`: remove "Striker commands for plans" and "Conventions", keep "Bridge" short,
  add a "Documentation" line with `https://ploenk42.github.io/herdr-mods/` near the top; `striker/README.md`:
  same link under the title. Commit `Task 10: README links the docs`, tick 10.

### Task 11: Final check and release to the person: build, links, diagrams, shots, validate

- [x] **Step 1:** `npm run build` green with link validation; open every page in `npm run dev` in light and
  dark: diagrams render, images load with the base path.
- [x] **Step 2:** grep `docs-site/content` and `docs-site/demo` for every name on the private list from
  Task 2; Read every shot once more.
- [x] **Step 3:** `openspec validate striker-docs --strict`. Tick 11 in the commit `Task 11: final check`.
- [x] **Step 4:** `needs_you` for the person: (a) review the site (`npm run dev` link), (b) decide the audit
  findings if still open, (c) make the repository public, (d) enable Pages with source "GitHub Actions" and
  merge to `main` for the first deploy. Nothing of (c) and (d) is done by an agent.
