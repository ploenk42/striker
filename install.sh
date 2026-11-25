#!/bin/sh
# macOS: link each mod into Herdr's mods/, copy the LCARS layout, build Striker and start it at login.
set -e
HOME_DIR="${HERDR_APP_HOME:-$HOME/.config/herdr-app}"
REPO="$(cd "$(dirname "$0")" && pwd)"
for mod in lcars blueprint; do
  target="$HOME_DIR/mods/$mod"
  if [ -e "$target" ] && [ ! -L "$target" ]; then
    echo "skip $mod: $target exists and is not a link" >&2
    continue
  fi
  ln -sfn "$REPO/$mod" "$target"
  echo "linked $target -> $REPO/$mod"
done
cp "$REPO/layouts/lcars.json" "$HOME_DIR/layouts/lcars.json"
echo "copied layouts/lcars.json"

# Striker: build the Go program and run `striker serve` as a LaunchAgent.
if ! command -v go >/dev/null 2>&1; then
  echo "go not found in PATH; install it from https://go.dev/dl/ and rerun" >&2
  exit 1
fi
mkdir -p "$REPO/striker/bin"
(cd "$REPO/striker" && go build -o bin/striker ./cmd/striker)
echo "built $REPO/striker/bin/striker"
# fleet.json is the person's own and untracked: start from the demo fleet when there is none
if [ ! -e "$REPO/striker/fleet.json" ]; then
  cp "$REPO/striker/fleet.example.json" "$REPO/striker/fleet.json"
  echo "copied striker/fleet.example.json to $REPO/striker/fleet.json; enter your repos there"
fi
mkdir -p "$HOME/.local/bin"
ln -sfn "$REPO/striker/bin/striker" "$HOME/.local/bin/striker"
echo "linked $HOME/.local/bin/striker -> $REPO/striker/bin/striker"
# Claude's status line stores model and tokens per terminal for Striker's crew (only if none is set yet)
"$REPO/striker/bin/striker" install-statusline || echo "striker install-statusline failed: no model and tokens for local agents" >&2

LABEL=io.github.ploenk42.striker
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs"
sed -e "s|@REPO@|$REPO|g" -e "s|@HOME@|$HOME|g" "$REPO/striker/launchd/$LABEL.plist" > "$PLIST"
launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
# bootout is asynchronous; bootstrap can fail until the old job is gone
n=0
until err=$(launchctl bootstrap "gui/$(id -u)" "$PLIST" 2>&1); do
  n=$((n + 1))
  if [ "$n" -ge 5 ]; then
    echo "launchctl bootstrap failed after $n attempts: $err" >&2
    exit 1
  fi
  sleep 1
done
echo "started $LABEL (log: ~/Library/Logs/striker.log)"
