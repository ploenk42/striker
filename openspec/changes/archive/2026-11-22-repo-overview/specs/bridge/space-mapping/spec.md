# Spec Delta

## Purpose

Defines how a Herdr space finds the repo it belongs to, including remote spaces whose agents run behind sudo.

## ADDED Requirements

### Requirement: Space maps through fleet.json
The Bridge SHALL take a space's repo path, in this order, from the `fleet.json` entry whose `space` equals the Herdr space name, then, when the space holds a coordinator terminal (labels `striker.repo` and `striker.coordinator`), from the space folder (else that terminal's cwd) on the coordinator's machine, then from a local lead terminal's cwd, then (local spaces only) from the first terminal of the space that reports a cwd, then from the space folder.

#### Scenario: Remote space behind sudo
- **WHEN** the space `alpha` on remote1 has only terminals whose cwd is `/home/you`
- **THEN** the Bridge uses `/srv/alpha` from `fleet.json`

#### Scenario: Coordinator space
- **WHEN** a space whose name has no `fleet.json` entry holds a terminal labelled `striker.repo=alpha` and `striker.coordinator=omega`, on remote1 behind sudo with cwd `/root`
- **THEN** the Bridge uses the space folder `/srv/alpha-omega` on remote1, even when a local lead terminal shares the space

#### Scenario: Local space made in another folder
- **WHEN** a local space with no `fleet.json` entry, no coordinator and no lead was made in `/home/you/gamma` and its first terminal with a cwd sits in `~/herdr-mods`
- **THEN** the Bridge uses `~/herdr-mods`

#### Scenario: Remote terminal cwd is not used
- **WHEN** a remote space with no `fleet.json` entry, no coordinator and no lead has a terminal whose cwd is `/root`
- **THEN** the Bridge uses the space folder, not `/root`

#### Scenario: Nothing matches
- **WHEN** no rung gives a path and the space folder is not a git worktree
- **THEN** the Bridge says `NO REPO FOR THIS SPACE`
