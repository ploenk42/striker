package fleet

import "regexp"

// A fleet entry's user is who runs Claude and git for the repo: "" is the user hc logs in as (no sudo),
// "root" is root through sudo -i, any other name (awayteam) is that user through sudo -u, never root.

// keepID keeps the herdr terminal id across sudo, so Claude's status line can name its terminal
// (sudo -i resets the environment; the sudoers rule's command ALL implies SETENV).
const keepID = "--preserve-env=HC_TERMINAL_ID"

// plainName is a Remote Control name safe inside single quotes in a shell command.
var plainName = regexp.MustCompile(`^[A-Za-z0-9 ._-]+$`)

// ClaudeArgv is the argv that starts Claude in dir as user, with Remote Control on under name (so the
// session shows up in claude.ai/code and the app and reconnects after a restart); a name that is empty
// or not plain text leaves Remote Control off.
func ClaudeArgv(user, dir, name string) []string {
	rc := ""
	if plainName.MatchString(name) {
		rc = " --remote-control '" + name + "'"
	}
	switch user {
	case "":
		return []string{"sh", "-lc", "cd '" + dir + "' && exec claude" + rc}
	case "root":
		return []string{"sudo", keepID, "-i", "sh", "-c", "cd '" + dir + "' && exec /root/.local/bin/claude" + rc}
	default:
		return []string{"sudo", keepID, "-u", user, "-i", "sh", "-c", "cd '" + dir + `' && exec "$HOME/.local/bin/claude"` + rc}
	}
}

// ClaudeCommand is the argv that starts Claude in dir as user with args, for task agents (no Remote
// Control: they would crowd claude.ai). dir and args go in as positional parameters after the script
// rather than into the sh -c text, so a prompt with quotes, $ or newlines needs no quoting and arrives
// unchanged. No sudo -i: it hands the command to the login shell as one escaped string, which expands $
// and drops newlines; sudo -H sh -lc execs directly with the user's HOME and login PATH.
func ClaudeCommand(user, dir string, args ...string) []string {
	var argv []string
	claude := "claude"
	switch user {
	case "":
	case "root":
		argv = []string{"sudo", keepID, "-H"}
		claude = "/root/.local/bin/claude"
	default:
		argv = []string{"sudo", keepID, "-u", user, "-H"}
		claude = `"$HOME/.local/bin/claude"`
	}
	argv = append(argv, "sh", "-lc", `cd "$1" && shift && exec `+claude+` "$@"`, "sh", dir)
	return append(argv, args...)
}

// Sudo is the prefix that runs a command as user without asking for a password (it fails instead); nil for "".
func Sudo(user string) []string {
	switch user {
	case "":
		return nil
	case "root":
		return []string{"sudo", "-n", "-i"}
	default:
		return []string{"sudo", "-n", "-u", user, "-i"}
	}
}
