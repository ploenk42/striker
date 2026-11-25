# Design: Striker documentation site

## 1. Audience and language

The site is written in English, for people who want to rebuild Striker on their own computers: they know Git, a
terminal and Claude Code, but not Herdr, hc or this repo. Concept and usage come first, setup is its own
section, and the reference closes. The person's own fleet (machine names, paths, repos, lead questions) never
appears; examples use the demo fleet (section 6).

## 2. Site

- `docs-site/`: a Next.js app with Fumadocs (`fumadocs-core`, `fumadocs-ui`, `fumadocs-mdx`), versions pinned
  exactly in `package.json` and a lockfile committed. Node from the person's install, npm.
- Static export (`output: 'export'`, `basePath: '/herdr-mods'`, `images.unoptimized`), so the build is plain
  files for GitHub Pages; `trailingSlash: true` so Pages serves every page as `…/index.html`.
- Content as MDX in `docs-site/content/docs/`, navigation through `meta.json` per folder. Fumadocs' built-in
  search runs on the static index (`staticGET` search route exported at build).
- Images in `docs-site/public/shots/`, referenced with the base path. Each screenshot is a PNG cut to the part
  it explains (section 6), at most ~400 KB (scaled to 1920 px wide).
- Mermaid: a client component `<Mermaid chart="…" />` that renders with the `mermaid` package in the browser and
  follows the light/dark theme of the site. Diagrams live in the MDX next to the text that explains them.
- `docs-site/` is outside the Go module and the mods; `install.ps1`/`install.sh` ignore it.

## 3. Pages

```
content/docs/
  index.mdx                  Introduction: what Striker is, one picture (Fleet Operations), where to go next
  concepts/
    roles.mdx                Human, Striker, Lead, Coordinator, Task agent: who runs where, as whom, labels
    command-chain.mdx        diagram A; status files and needs_you as the way back up
    decisions.mdx            decisions stay with the human; quoting answers verbatim; no --force, no interrupt
    openspec-workflow.mdx    change -> tasks/plan -> worktree -> coordinator -> task agents -> merge -> archive
  security.mdx               diagram B; the rules that hold the boundaries
  relay.mdx                  diagram C; request kinds, ledger, answers, watching task agents
  bridge/
    fleet-operations.mdx     columns, crew names, stardate, scan time, click targets, scrolling
    plan.mdx                 PLAN: task accordion, WORKING / NEEDS YOU / DONE cards
    spec.mdx                 SPEC: headings, deltas; vision and spec list on main
    repo.mdx                 REPO tree: trunk, NOW, lanes, future plans; terminal swap
  setup/
    prerequisites.mdx        Herdr app + herdr-core (hc), Claude Code, Go, Node, OpenSpec CLI; macOS and Windows
    install.mdx              install.ps1 / install.sh: what they link, build and register; striker serve
    fleet.mdx                fleet.json fields, status_dir, agents_dir; adding a repo
    remote-machine.mdx       a second machine: hc, setup/awayteam.sh, sudo rules, statusline.sh
    first-lead.mdx           striker start-lead, the brief, the first change and its coordinator
  reference/
    cli.mdx                  every striker subcommand with its arguments
    status-file.mdx          lead and coordinator status file, requests, needs_you kinds
    labels.mdx               striker.self, striker.lead, striker.coordinator, striker.repo, striker.task, striker.role, striker.topic
    files.mdx                ~/.striker (status, agents, relay.json), STRIKER_HOME, the briefs
```

Each page says what something is and how to use it, then why it is that way where the reason is not obvious.
Facts come from the code and `striker/CLAUDE.md` at the time of writing; the reference pages are checked against
`striker`'s usage text and the briefs.

## 4. Diagrams

**A. Command chain** (flowchart, top-down): Human -> Striker (this computer) -> Lead (per repo, its machine) ->
Coordinator (per change, its worktree) -> Task agents (implementer, spec reviewer, quality reviewer). Dashed
edges back up: task agent result -> coordinator; coordinator and lead status files (`needs_you`) -> Striker ->
human; the human's answer goes down again verbatim. Edge labels name the mechanism (`striker send`,
`striker start-coordinator`, `striker dispatch`, status file, relay request).

**B. Trust boundaries** (flowchart with subgraphs): "This computer, as the person" (Striker, `striker serve`, hc,
Herdr) and "Remote machine" split into "as the person" (hc server, `sudo -u awayteam` only) and "as awayteam"
(lead, coordinator, task agents, repo worktree, status files). Arrows show the only ways across: Striker -> hc ->
`sudo -u awayteam claude …` (start, send), awayteam -> its own status file -> `striker serve` reads it. Notes: no
hc socket for awayteam, no root, sudo only to awayteam, request kinds fixed per role, agents never push,
decisions only from the human.

**C. Relay** (sequence): Lead writes `requests: start-coordinator` -> `striker serve` (every 10 s) reads the
status file over hc -> checks kind and role, ledger key `<machine>|<file>|<id>` -> starts the coordinator as
awayteam in the worktree -> `[Striker] Anfrage <id>: …` to the lead. Coordinator writes `dispatch` -> task agent
started under it -> serve watches it -> when it is done, blocked or gone, sends the coordinator its last
answer. A rejected request returns `abgelehnt: <reason>`.

## 5. Security page

Stated as rules with their reason, each tied to diagram B:

- Only the person's account talks to hc; whoever reaches hc acts as the person.
- Agents on a remote machine run as a separate user (`awayteam`) via `sudo -u`, never root; the person's sudo
  rule allows only that target user.
- Agents without hc ask through their own status file; Striker carries out only the kinds their role may ask for
  (lead: `start-coordinator`; coordinator: `dispatch`, and `send` only to agents Striker started for it), each id
  once, logged in `~/.striker/relay.json`; Striker never writes into status files.
- Decisions (requests, approvals, releases, rights) belong to the human; a `[Striker]` message counts as approval
  only when it quotes the human's answer.
- Agents never push; Striker never interrupts a working agent unless the human says so.
- Shell rules (one simple command, absolute paths, no substitution or pipes) keep commands reviewable under the
  person's Claude permission settings.
- What this does not protect against (stated plainly): a compromised account of the person, or a repo whose
  content tricks an agent within its own rights.

## 6. Demo fleet and screenshots

`docs-site/demo/` holds a script (`demo.ps1`, one simple command per step inside it) that creates:

- two local git repos `alpha` and `beta` under a demo folder outside the real repos, each with a small README,
  an OpenSpec layout and one change with three tasks (one in `alpha` to be carried out live);
- a demo `STRIKER_HOME` with a `fleet.json` listing them (no machine, no user) and copies of the briefs;
- lead status files with invented, harmless `needs_you` entries.

Taking the shots: stop the scheduled task `Striker`, run `striker serve` with `STRIKER_HOME` pointing at the demo,
start the demo leads with `striker start-lead`, start a coordinator for the `alpha` change so PLAN shows real
task agents in parallel, take every view with `herdr-app shot`, then stop the demo serve, close the demo
spaces and start the task `Striker` again. Herdr itself is never restarted. herdr-mods (Striker's own repo, public
after section 7) may appear on the board.

The sidebar still lists the person's other spaces; every published shot is cropped to the work view, or the
sidebar list is covered with a plain LCARS-coloured block. Each shot is checked by eye before it goes in.

## 7. Making the repository public

A separate task before the first deploy:

1. Audit the current tree and the full history for private data: `striker/fleet.json` (machine names, paths),
   `striker/setup/*`, briefs and examples, commit author e-mails, anything under `layouts/`, hard-coded user
   paths, tokens or hostnames.
2. Write the findings with a proposal per finding (keep, generalise in a commit, or rewrite history) into the
   change's `needs_you`; nothing is rewritten without the person's word.
3. The person switches the repository to public (or tells Striker to with `gh repo edit --visibility public`).

`fleet.json` stays the person's real file; the docs explain it with the demo values.

## 8. Deployment

`.github/workflows/docs.yml`: on push to `main` touching `docs-site/**` (and by hand), check out, set up Node
with the npm cache, `npm ci` and `npm run build` in `docs-site/`, upload `docs-site/out` with
`actions/upload-pages-artifact`, deploy with `actions/deploy-pages`. Actions pinned by full commit SHA.
Pages is enabled with "GitHub Actions" as the source once the repo is public. The first deploy happens only
after the person approves the published content.

## 9. README

The root README drops "Striker commands for plans" and "Conventions" (GitHub issues, old plan headings) and links
to the site; `striker/README.md` gets the same link. The relay paragraph in `striker/README.md` stays, as the
in-repo summary.

## 10. Checks

- `npm run build` in `docs-site/` succeeds with no broken internal links (Fumadocs link validation in the build).
- Every page renders its Mermaid diagrams without errors in both themes (looked at in a browser).
- Every screenshot shows only demo data (checked by eye against a list of private names: the person's machines,
  repos, users, home paths).
- `openspec validate striker-docs --strict`.
