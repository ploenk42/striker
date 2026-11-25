# Plan: add-greeting

Goal: `greet.ps1` prints a greeting, optionally with a name (design.md). Tasks 2 and 3 both build on task 1
and can run in parallel.

### Task 1: Greeting script with tests

Files: `greet.ps1`, `tests/greet.Tests.ps1`.

- [ ] **Step 1:** failing Pester test: `greet.ps1` prints `Hello, crew!`.
- [ ] **Step 2:** `greet.ps1` with `Write-Output 'Hello, crew!'`; test green.
- [ ] **Step 3:** Commit `Task 1: greeting script`, tick 1.

### Task 2: Optional name parameter

Files: `greet.ps1`, `tests/greet.Tests.ps1`.

- [ ] **Step 1:** failing tests for `-Name World` and `-Name " World "` (both `Hello, World!`) and `-Name ""`
  (`Hello, crew!`).
- [ ] **Step 2:** `param([string]$Name)`, trimmed, empty means none; tests green.
- [ ] **Step 3:** Commit `Task 2: name parameter`, tick 2.

### Task 3: README usage section

Files: `README.md`.

- [ ] **Step 1:** section "Usage" with `.\greet.ps1` and `.\greet.ps1 -Name World` and their output.
- [ ] **Step 2:** Commit `Task 3: usage`, tick 3.
