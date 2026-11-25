## ADDED Requirements

### Requirement: Greeting

The script `greet.ps1` SHALL print a greeting, naming the person given with `-Name` or the crew when no name
is given.

#### Scenario: No name

- **WHEN** `greet.ps1` runs without arguments
- **THEN** it prints `Hello, crew!`

#### Scenario: A name

- **WHEN** `greet.ps1 -Name " World "` runs
- **THEN** it prints `Hello, World!`
