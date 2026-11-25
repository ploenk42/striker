# Proposal

## Why

Striker is spread over `striker/CLAUDE.md`, two READMEs, the briefs and the code. Nobody but the person and the
agents can follow how a request travels from the human to a task agent on another machine, what an agent there
may and may not do, or how to set the whole thing up from nothing. The root README still describes GitHub
issues, which are gone. The person wants a documentation site others can rebuild Striker from, with screenshots
of every view and diagrams of the security concept and the command chain.

## What Changes

- A Fumadocs site in `docs-site/` (Next.js, static export), in English, for people who want to rebuild Striker:
  introduction, concepts (roles, command chain, decisions, OpenSpec workflow), security, the relay, the LCARS
  bridge, setup, reference.
- Mermaid diagrams for the command chain, the trust boundaries and the relay sequence.
- Screenshots of every view taken from a demo fleet (harmless repos and status files), never from the person's
  real fleet; `shots/` stays private and ignored.
- A GitHub Actions workflow that builds the site and deploys it to GitHub Pages
  (`ploenk42.github.io/herdr-mods`).
- The repository becomes public, after an audit of the tree and its history for private data that the person
  reviews; the switch happens only on the person's word.
- The root README drops the GitHub-issue sections and links to the site.

## Impact

New `docs-site/`, `.github/workflows/docs.yml`, a demo fleet script, `README.md`, `striker/README.md` (link).
No change to `striker` behaviour or the LCARS mods, so no spec deltas. Outward-facing: the repository's
visibility and a public website.
