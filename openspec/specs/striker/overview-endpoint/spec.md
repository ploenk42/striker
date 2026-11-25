# striker/overview-endpoint Specification

## Purpose

Defines the data contract Striker serves for the repo overview, so the Bridge only draws and never reads repos itself.

## Requirements

### Requirement: Overview endpoint
Striker SHALL serve `GET /overview?machine=<m>&path=<repo>` returning `briefing`, `context`, `inventory`, `open_items` and `warnings`, behind the same Host and worktree-top guards as `/doc`.

#### Scenario: Good request
- **WHEN** the Bridge asks for a main checkout on 127.0.0.1
- **THEN** it gets 200 with all five keys, arrays never null

#### Scenario: Unreadable part
- **WHEN** `docs/REGISTER.md` exists but cannot be read
- **THEN** `open_items` holds no register entries (lead and coordinator entries stay) and `warnings` names the REGISTER

#### Scenario: No REGISTER
- **WHEN** the repo has no `docs/REGISTER.md`
- **THEN** `open_items` holds no register entries and `warnings` does not mention the REGISTER

### Requirement: Separate freshness
Striker SHALL refresh the lead status at least every 10 seconds and the repo parts at least every 60 seconds.

#### Scenario: Lead updates status
- **WHEN** the lead rewrites its status file
- **THEN** the next overview within 10 seconds shows the new note
