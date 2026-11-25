#!/usr/bin/env bash
# One-time setup on a remote machine, run as root: the user awayteam runs every agent (leads, coordinators,
# task agents) instead of root. The owner (the account Striker reaches the machine as) may become awayteam
# without a password, awayteam may not become root.
# Idempotent: running it again changes nothing that is already in place.
#   sudo bash awayteam.sh <owner> <repo path>...
#   e.g. sudo bash awayteam.sh you /srv/alpha /srv/beta
set -euo pipefail

AGENT=awayteam
usage="usage: sudo bash $0 <owner> <repo path>..."
[ "$#" -ge 2 ] || { echo "$usage" >&2; exit 1; }
OWNER=$1
shift
REPOS=("$@")
[ "$OWNER" != root ] && [ "$OWNER" != "$AGENT" ] || { echo "the owner must be a normal user: $usage" >&2; exit 1; }
id "$OWNER" >/dev/null 2>&1 || { echo "owner $OWNER does not exist" >&2; exit 1; }
STATUS=/var/lib/striker/status
AGENTS=/var/lib/striker/agents
OLD_STATUS=/home/$OWNER/.striker/status
SUDOERS=/etc/sudoers.d/striker

[ "$(id -u)" -eq 0 ] || { echo "run as root: sudo bash $0" >&2; exit 1; }
command -v setfacl >/dev/null || { echo "setfacl missing: apt install acl" >&2; exit 1; }

# user and group; docker access if the machine has docker (a lead may run containers), no sudo group
if ! id "$AGENT" >/dev/null 2>&1; then
  useradd --create-home --shell /bin/bash --user-group "$AGENT"
  echo "created user $AGENT"
fi
if getent group docker >/dev/null; then usermod -aG docker "$AGENT"; fi
if id -nG "$AGENT" | tr ' ' '\n' | grep -qx sudo; then gpasswd -d "$AGENT" sudo; fi

# the repos and their worktrees (<repo>-<change>): rwX for awayteam, now and for new files; owners stay
for repo in "${REPOS[@]}"; do
  for dir in "$repo" "$repo"-*; do
    [ -d "$dir" ] || continue
    setfacl -R -m "u:$AGENT:rwX" "$dir"
    find "$dir" -type d -exec setfacl -m "d:u:$AGENT:rwX" {} +
    echo "acl $dir"
  done
done
# new worktrees are made next to the repos, in their parent folders
for repo in "${REPOS[@]}"; do
  setfacl -m "u:$AGENT:rwx" "$(dirname "$repo")"
done

# git refuses repos owned by someone else unless they are safe
sudo -u "$AGENT" -H git config --global --get-all safe.directory 2>/dev/null | grep -qx '\*' \
  || sudo -u "$AGENT" -H git config --global --add safe.directory '*'

# commits by the agents carry the owner's git identity (awayteam has none of its own, so git refuses to commit)
for key in user.name user.email; do
  value=$(sudo -u "$OWNER" -H git config --global --get "$key" || true)
  [ -n "$value" ] && sudo -u "$AGENT" -H git config --global "$key" "$value"
done

# status files: one directory both the owner (Striker reads through hc) and awayteam (the agents write) can use
install -d -o "$OWNER" -g "$AGENT" -m 2775 "$STATUS"
setfacl -m "d:g:$AGENT:rwX" -m "d:u:$OWNER:rwX" "$STATUS"
if [ -d "$OLD_STATUS" ]; then
  for f in "$OLD_STATUS"/*.json; do
    [ -e "$f" ] || continue
    [ -e "$STATUS/$(basename "$f")" ] || cp -p "$f" "$STATUS/"
  done
  chown "$OWNER:$AGENT" "$STATUS"/*.json 2>/dev/null || true
  chmod g+rw "$STATUS"/*.json 2>/dev/null || true
  echo "status files copied to $STATUS (the old ones stay in $OLD_STATUS)"
fi

# status line files (model and tokens per terminal, for Striker's crew): shared the same way
install -d -o "$OWNER" -g "$AGENT" -m 2775 "$AGENTS"
setfacl -m "d:g:$AGENT:rwX" -m "d:u:$OWNER:rwX" "$AGENTS"

# sudoers, built from the owner: "<owner> ALL=(awayteam) NOPASSWD: ALL", the owner becomes awayteam without a
# password, nothing more
tmp=$(mktemp)
echo "$OWNER ALL=($AGENT) NOPASSWD: ALL" > "$tmp"
visudo -cf "$tmp" >/dev/null
install -o root -g root -m 0440 "$tmp" "$SUDOERS"
rm -f "$tmp"
echo "sudoers $SUDOERS"

# Claude Code for awayteam, and root's project memory so the leads keep it
if [ ! -x "/home/$AGENT/.local/bin/claude" ]; then
  sudo -u "$AGENT" -i bash -c 'curl -fsSL https://claude.ai/install.sh | bash'
fi
if [ -d /root/.claude/projects ]; then
  install -d -o "$AGENT" -g "$AGENT" "/home/$AGENT/.claude/projects"
  cp -rn /root/.claude/projects/. "/home/$AGENT/.claude/projects/"
  chown -R "$AGENT:$AGENT" "/home/$AGENT/.claude"
  echo "copied /root/.claude/projects (existing files kept)"
fi

# Claude settings for awayteam (language, plugins, the status dir as a working directory, the status line),
# from awayteam-settings.json beside this script; a settings file that already has a statusLine stays. The
# status line script beside this one is always installed, so a rerun brings its latest version.
here=$(dirname "$0")
settings=$here/awayteam-settings.json
target=/home/$AGENT/.claude/settings.json
if [ -f "$settings" ] && ! grep -qs '"statusLine"' "$target"; then
  install -D -o "$AGENT" -g "$AGENT" -m 0644 "$settings" "$target"
  echo "settings $target"
fi
if [ -f "$here/statusline.sh" ]; then
  install -D -o "$AGENT" -g "$AGENT" -m 0755 "$here/statusline.sh" "/home/$AGENT/.claude/statusline.sh"
  echo "status line /home/$AGENT/.claude/statusline.sh"
fi

echo
echo "left to do: log awayteam in to Claude once:  sudo -u $AGENT -i claude"
