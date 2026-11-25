# Design: add-greeting

## 1. Script

`greet.ps1` takes one optional parameter `-Name`. Without it, it prints `Hello, crew!`; with it, it prints
`Hello, <Name>!`. Leading and trailing spaces of the name are trimmed; an empty name counts as none.

## 2. Tests

`tests/greet.Tests.ps1` (Pester) runs the script and checks the output for no name, a name, and a name with
spaces around it.

## 3. README

A section "Usage" with both calls and their output.
