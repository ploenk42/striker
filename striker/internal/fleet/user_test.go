package fleet

import (
	"reflect"
	"strings"
	"testing"
)

func TestClaudeArgv(t *testing.T) {
	cases := map[string][]string{
		"root":     {"sudo", "--preserve-env=HC_TERMINAL_ID", "-i", "sh", "-c", "cd '/x' && exec /root/.local/bin/claude --remote-control 'alpha'"},
		"awayteam": {"sudo", "--preserve-env=HC_TERMINAL_ID", "-u", "awayteam", "-i", "sh", "-c", `cd '/x' && exec "$HOME/.local/bin/claude" --remote-control 'alpha'`},
		"":         {"sh", "-lc", "cd '/x' && exec claude --remote-control 'alpha'"},
	}
	for user, want := range cases {
		if got := ClaudeArgv(user, "/x", "alpha"); !reflect.DeepEqual(got, want) {
			t.Errorf("ClaudeArgv(%q) = %q, want %q", user, got, want)
		}
	}
}

// A name that is not plain text (quotes, newlines, shell characters) starts Claude without Remote Control
// rather than putting it into the shell command.
func TestClaudeArgvOddName(t *testing.T) {
	for _, name := range []string{"", "a'b", "a\nb", "$(x)", "a;b"} {
		got := ClaudeArgv("awayteam", "/x", name)
		want := []string{"sudo", "--preserve-env=HC_TERMINAL_ID", "-u", "awayteam", "-i", "sh", "-c", `cd '/x' && exec "$HOME/.local/bin/claude"`}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClaudeArgv(name %q) = %q", name, got)
		}
	}
	if got := ClaudeArgv("", "/x", "alpha omega-1.2_a"); got[2] != "cd '/x' && exec claude --remote-control 'alpha omega-1.2_a'" {
		t.Errorf("plain name: %q", got)
	}
}

// Task agents: the dir and the arguments go in after the script, so quotes, $ and newlines arrive unchanged;
// no sudo -i (it hands the command to the login shell as one string) and no Remote Control.
func TestClaudeCommand(t *testing.T) {
	args := []string{"a \"b\" $c", "zwei\nZeilen"}
	tail := []string{"sh", "/srv/x-c", "a \"b\" $c", "zwei\nZeilen"}
	cases := map[string][]string{
		"awayteam": append([]string{"sudo", "--preserve-env=HC_TERMINAL_ID", "-u", "awayteam", "-H", "sh", "-lc",
			`cd "$1" && shift && exec "$HOME/.local/bin/claude" "$@"`}, tail...),
		"root": append([]string{"sudo", "--preserve-env=HC_TERMINAL_ID", "-H", "sh", "-lc",
			`cd "$1" && shift && exec /root/.local/bin/claude "$@"`}, tail...),
		"": append([]string{"sh", "-lc", `cd "$1" && shift && exec claude "$@"`}, tail...),
	}
	for user, want := range cases {
		got := ClaudeCommand(user, "/srv/x-c", args...)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClaudeCommand(%q) = %q, want %q", user, got, want)
		}
		for _, a := range got {
			if strings.Contains(a, "--remote-control") {
				t.Errorf("ClaudeCommand(%q) turns on Remote Control: %q", user, got)
			}
		}
	}
}

func TestSudo(t *testing.T) {
	cases := map[string][]string{
		"root":     {"sudo", "-n", "-i"},
		"awayteam": {"sudo", "-n", "-u", "awayteam", "-i"},
		"":         nil,
	}
	for user, want := range cases {
		if got := Sudo(user); !reflect.DeepEqual(got, want) {
			t.Errorf("Sudo(%q) = %q, want %q", user, got, want)
		}
	}
}
