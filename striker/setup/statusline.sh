#!/bin/sh
# Claude Code status line for the agents on a Linux host (installed for awayteam by setup/awayteam.sh).
# Stores what Claude sends (model, tokens) as <dir>/<HC_TERMINAL_ID>.json, one line, for Striker's crew, and
# prints "<model> · <tokens>". Same file format as `striker statusline` (striker/internal/statusline):
#   {"terminal":"t7","at":<unix ms>,"claude":<the JSON Claude sent>}
# No jq: the JSON is stored as it came (newlines removed), model and tokens are picked out with sed.
dir=${STRIKER_AGENTS_DIR:-/var/lib/striker/agents}
if [ -n "$1" ]; then json=$(tr -d '\n\r' < "$1"); else json=$(tr -d '\n\r'); fi # a file instead of stdin: for tests
model=$(printf '%s' "$json" | sed -n 's/.*"model"[[:space:]]*:[[:space:]]*{[^}]*"display_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
num() { printf '%s' "$json" | sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p"; }
tin=$(num total_input_tokens)
tout=$(num total_output_tokens)
tokens=$(( ${tin:-0} + ${tout:-0} ))

case "$HC_TERMINAL_ID" in
  ''|*[!A-Za-z0-9_-]*) ;;
  *)
    if [ -n "$json" ] && [ -d "$dir" ] && [ -w "$dir" ]; then
      at=$(( $(date +%s) * 1000 ))
      tmp="$dir/.$HC_TERMINAL_ID.tmp"
      printf '{"terminal":"%s","at":%s,"claude":%s}\n' "$HC_TERMINAL_ID" "$at" "$json" > "$tmp" \
        && mv -f "$tmp" "$dir/$HC_TERMINAL_ID.json"
    fi
    ;;
esac

awk -v m="$model" -v n="$tokens" '
function short(n) {
  if (n < 1000) return n
  if (n < 1000000) return int(n / 1000) "K"
  if (n < 10000000) return sprintf("%.1fM", int(n / 100000) / 10)
  return int(n / 1000000) "M"
}
BEGIN {
  t = (n > 0) ? short(n) : ""
  if (m != "" && t != "") print m " · " t
  else if (m != "") print m
  else print t
}'
