# Proposal

## Why

Striker lived in the old standalone repo as PowerShell scripts with a hardcoded Windows path. The person now also uses Herdr on a Mac, where there is no Striker checkout and no Windows PowerShell. They want one repo for the Herdr layout, the mods and Striker, and Striker as a small program that is the same on macOS and Windows.

## What Changes

- The old standalone repo moves into this repo as `striker/`, history kept.
- One Go program `striker` (`status`, `send`, `start-lead`, `serve`) replaces the scripts.
- The LCARS board reads `striker serve` over `http://127.0.0.1:7522/status`.
- Installers with autostart for macOS and Windows.

## Impact

`striker/`, `lcars/striker.js`, `install.sh`, `install.ps1`. Still open: remove the `striker/scripts` `.ps1` files once Windows is confirmed.
