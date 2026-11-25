## ADDED Requirements

### Requirement: Chart list

The file `charts.txt` SHALL hold one chart name per line, with no blank lines and no duplicates.

#### Scenario: Adding a chart

- **WHEN** a chart that is not yet listed is added
- **THEN** it appears as a new last line of `charts.txt`

#### Scenario: Adding a chart twice

- **WHEN** a chart that is already listed is added
- **THEN** `charts.txt` stays unchanged
