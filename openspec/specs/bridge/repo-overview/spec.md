# bridge/repo-overview Specification

## Purpose

Gives the person a briefing for a repo when they open its space: where work stopped, the vision it serves, what exists and what waits on them.

## Requirements

### Requirement: Overview is the entry of a main checkout
The Bridge SHALL show an OVERVIEW pseudo-tab before PLAN and SPEC, and SHALL open it by default when the space is a repo's main checkout.

#### Scenario: Opening a main checkout
- **WHEN** the person opens the Bridge in a space whose repo path is a main checkout
- **THEN** OVERVIEW is shown, not `NO PLAN FOR BRANCH main`

### Requirement: Briefing first
The overview SHALL start with a briefing from the lead's status (task, progress, note, last update) and the last commits on the default branch.

#### Scenario: Lead wrote a status
- **WHEN** the lead status file has task and note
- **THEN** both appear at the top with the time of the last update

#### Scenario: No lead status
- **WHEN** no status file exists
- **THEN** the briefing says `NO LEAD STATUS` and still shows the last commits

### Requirement: Context from the vision
The overview SHALL show the title and first paragraph of `docs/VISION.md`, else `docs/SPEC.md`, else `README.md`, and say `NO VISION FILE` when none exists.

#### Scenario: Vision present
- **WHEN** `docs/VISION.md` exists
- **THEN** its title and first paragraph are shown with a link that opens it in SPEC mode

### Requirement: Inventory from OpenSpec
The overview SHALL list every OpenSpec capability spec and every change, active changes with `done/total` from `tasks.md` and archived changes as done with their archive date.

#### Scenario: Active change with ticked tasks
- **WHEN** a change has 3 of 8 tasks ticked
- **THEN** it shows as in progress with `3/8`

#### Scenario: Archived change
- **WHEN** a change lives in `changes/archive/2026-11-21-x/`
- **THEN** it shows as done on 2026-11-21

### Requirement: Interrupted work is visible
A change SHALL show INTERRUPTED when some tasks are ticked, some are open, and no labelled agent runs for it.

#### Scenario: Agent session died
- **WHEN** a change has ticked and open tasks and no terminal labelled for it is live
- **THEN** it shows INTERRUPTED with the last ticked task

### Requirement: Open items
The overview SHALL list the lead's `needs_you` entries and the unresolved entries under REGISTER headings for open items or conflicts.

#### Scenario: Resolved register entry
- **WHEN** a REGISTER entry is struck through or marked decided
- **THEN** it is not listed
