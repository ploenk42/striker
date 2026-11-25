# alpha

Demo repository for the Striker documentation: a small greeting script.

## Arbeitsweise: OpenSpec + Superpowers

OpenSpec is the store, the Superpowers skills are the process (brainstorming, planning, TDD, subagents,
reviews). OpenSpec's own skills (propose/apply/archive) are not used; the `openspec` CLI is only used for
`validate`, `list` and `archive`.

Layout:

```
openspec/
  specs/<capability>/spec.md            living requirements (SHALL + scenarios)
  changes/<name>/
    .openspec.yaml                      metadata (x-depends_on, x-branch)
    proposal.md                         why, what changes, capabilities, impact
    design.md                           the design
    specs/<capability>/spec.md          spec deltas (ADDED / MODIFIED / REMOVED)
    tasks.md                            checklist = state of the change
    plan.md                             the Superpowers plan (steps, files, tests)
  changes/archive/<YYYY-MM-DD>-<name>/  finished changes
```

1. **Brainstorming** writes `proposal.md` and `design.md`, plus spec deltas for capabilities whose behaviour
   changes.
2. **Planning** writes `tasks.md` (one line per task) and `plan.md` (one `### Task <id>: <title>` per task).
3. **Implementing**: a task is ticked in `tasks.md` in the same commit as its last piece of work. The
   coordinator and `striker dispatch` read the tasks from `tasks.md`.
4. **Finishing:** `openspec validate <name> --strict`; after the merge, on the default branch,
   `openspec archive <name> --yes`.

Task lines: only top-level lines in `tasks.md` count, `- [ ] <id> <title>`, optionally with the suffix
`(after <id>, <id>)`. Without `after`, a task depends on the one before it.

Coordinator: one per change, started with `striker start-coordinator <repo> <change>`. It reports through
`<status_dir>/<repo>--<change>.json` and lists whatever it cannot decide in its `needs_you`.

Shell commands: exactly one simple command per call, absolute paths, no substitution, pipes or chaining.
