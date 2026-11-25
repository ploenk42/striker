// Package dispatch starts task agents as labelled hc terminals and labels the
// coordinator terminal.
package dispatch

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/openspec"
	"github.com/ploenk42/striker/striker/internal/plans"
)

// Argv is the hc command line that starts a task agent in the background.
// An empty topic omits the striker.topic label.
func Argv(role string, issue int, topic, cwd, prompt string) []string {
	n := strconv.Itoa(issue)
	return argv(role, n, n, topic, "", cwd, prompt)
}

// argv: an empty topic or repo omits its label.
func argv(role, name, taskLabel, topic, repo, cwd, prompt string) []string {
	a := []string{"run", "--bg", "--name", role + "-" + name, "--cwd", cwd}
	a = append(a, labels(role, taskLabel, topic, repo)...)
	return append(a, "claude", prompt)
}

// labels are the --label flags of a task agent; an empty topic or repo omits its label.
func labels(role, taskLabel, topic, repo string) []string {
	a := []string{"--label", "striker.task=" + taskLabel, "--label", "striker.role=" + role}
	if topic != "" {
		a = append(a, "--label", "striker.topic="+topic)
	}
	if repo != "" {
		a = append(a, "--label", "striker.repo="+repo)
	}
	return a
}

var roleRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidRole reports whether role is usable in a terminal name and label.
func ValidRole(role string) bool { return roleRe.MatchString(role) }

// Resolve finds the plan task for a GitHub issue number. The first match wins.
// A plan without any issue numbers is refused, except in dry-run, where the
// issue is taken as the task position and the returned task has Issue 0 (the
// prompt and label then carry "<unassigned>" placeholders).
func Resolve(p plans.Plan, issue int, dry bool) (plans.Task, error) {
	if issue <= 0 {
		return plans.Task{}, fmt.Errorf("invalid issue number %d", issue)
	}
	numbered := false
	for _, t := range p.Tasks {
		if t.Issue == issue {
			return t, nil
		}
		numbered = numbered || t.Issue != 0
	}
	if dry && !numbered && issue >= 1 && issue <= len(p.Tasks) {
		t := p.Tasks[issue-1]
		t.Issue = 0
		return t, nil
	}
	return plans.Task{}, fmt.Errorf("no task with issue #%d in the plan (run `striker issues <plan>` first if the headings are unnumbered)", issue)
}

// Command builds the hc argv for a resolved task.
func Command(role string, issue int, p plans.Plan, t plans.Task, cwd, planRel, specRel string) []string {
	label := strconv.Itoa(t.Issue)
	if t.Issue == 0 {
		label = "<unassigned>"
	}
	return argv(role, strconv.Itoa(issue), label, p.Topic, "", cwd, Prompt(role, p, t, planRel, specRel))
}

func issueRef(t plans.Task) string {
	if t.Issue == 0 {
		return "<unassigned>"
	}
	return strconv.Itoa(t.Issue)
}

// Prompt is the first message of a task agent: its role, the task section and
// the working rules. specRel may be empty.
func Prompt(role string, p plans.Plan, t plans.Task, planRel, specRel string) string {
	n := issueRef(t)
	var b strings.Builder
	fmt.Fprintf(&b, "Du bist der %s für Task %d (Issue #%s) des Plans %s (Thema %s). Arbeite nur an diesem Task, in diesem Worktree.\n\n",
		role, t.N, n, planRel, p.Topic)
	fmt.Fprintf(&b, "Plan: %s\n", planRel)
	if specRel != "" {
		fmt.Fprintf(&b, "Spec: %s\n", specRel)
	}
	b.WriteString("Lies zuerst die Spec und den Plan (Abschnitt \"Conventions\" der Spec) und CLAUDE.md.\n\n")
	fmt.Fprintf(&b, "### Task #%s: %s\n\n%s\n\n", n, t.Title, t.Body)
	fmt.Fprintf(&b, "Commit with `Refs #%s`; the commit that completes the task uses `Fixes #%s`.\n\n", n, n)
	b.WriteString("Shell rules: the Claude settings block unparseable shell commands. Make plain one-command Bash calls with absolute paths. " +
		"Use the Read, Grep and Edit tools instead of shell pipelines. Never use `$(...)`, heredocs, variables, globs or `cd … &&` chains. Never redirect output, not even to `/dev/null` (no `2>/dev/null`, no `-o /dev/null`).\n\n")
	b.WriteString("Wenn du fertig bist, fasse in einem Absatz zusammen, was du getan hast und was die Tests sagen.\n")
	return b.String()
}

// ResolveChange finds the task of an OpenSpec change for arg: a task id
// ("2.3"), or an issue ("#42", or a bare number that is no task id). A bare
// value that is a task id means the task id.
func ResolveChange(c openspec.Change, arg string) (openspec.Task, error) {
	v := strings.TrimSpace(arg)
	if v == "" || strings.ContainsAny(v, " \t\n") {
		return openspec.Task{}, fmt.Errorf("invalid task %q", arg)
	}
	if !strings.HasPrefix(v, "#") {
		for _, t := range c.Tasks {
			if t.ID == v {
				return t, nil
			}
		}
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(v, "#")); err == nil && n > 0 {
		for _, t := range c.Tasks {
			if t.Issue == n {
				return t, nil
			}
		}
	}
	return openspec.Task{}, fmt.Errorf("no task %q in change %s (task id like 2.3, or issue like #42; `striker issues %s` writes the issues)", arg, c.Name, c.Name)
}

// CommandChange builds the hc argv for task t of change c: terminal
// <role>-<task id>, labels striker.task=<task id>, striker.role, striker.topic=<change>
// and, when repo is known, striker.repo=<repo> (Ruling 9: found by label even when
// hc reports a home cwd).
func CommandChange(role, repo string, c openspec.Change, t openspec.Task, cwd string) []string {
	return argv(role, t.ID, t.ID, c.Name, repo, cwd, PromptChange(role, c, t))
}

// RemoteCommandChange builds the hc argv with which Striker's relay starts task t
// of change c on machine as user, in worktree, as a sub-agent of parent (the
// coordinator's full address, e.g. remote1/t2b: it shows under it in the
// sidebar and ends with it). Name, labels and prompt are those of
// CommandChange; Claude runs through fleet.ClaudeCommand, which passes the
// worktree and prompt as arguments, so no --cwd and no quoting are needed.
func RemoteCommandChange(role, repo string, c openspec.Change, t openspec.Task, machine, user, worktree, parent string) []string {
	a := []string{"run", "--on", machine, "--bg", "--name", role + "-" + t.ID, "--parent", parent}
	a = append(a, labels(role, t.ID, c.Name, repo)...)
	a = append(a, "--")
	return append(a, fleet.ClaudeCommand(user, worktree, PromptChange(role, c, t))...)
}

// PromptChange is the first message of a task agent of an OpenSpec change.
func PromptChange(role string, c openspec.Change, t openspec.Task) string {
	var b strings.Builder
	issue := ""
	if t.Issue != 0 {
		issue = fmt.Sprintf(" (Issue #%d)", t.Issue)
	}
	fmt.Fprintf(&b, "Du bist der %s für Task %s%s des OpenSpec-Changes %s. Arbeite nur an diesem Task, in diesem Worktree.\n\n", role, t.ID, issue, c.Name)
	for _, f := range []struct{ name, path string }{
		{"Tasks", c.TasksPath}, {"Plan", c.PlanPath}, {"Design", c.Design}, {"Proposal", c.Proposal},
	} {
		if f.path != "" {
			fmt.Fprintf(&b, "%s: %s\n", f.name, f.path)
		}
	}
	b.WriteString("Lies zuerst design.md, deinen Abschnitt in plan.md und AGENTS.md/CLAUDE.md.\n\n")
	fmt.Fprintf(&b, "### Task %s: %s\n\n", t.ID, t.Title)
	if t.Body != "" {
		b.WriteString(t.Body + "\n\n")
	}
	if t.Issue != 0 {
		fmt.Fprintf(&b, "Commit with `Refs #%d`; the commit that completes the task uses `Fixes #%d`.\n", t.Issue, t.Issue)
	}
	fmt.Fprintf(&b, "Tick the task in %s (`- [x]`) and its steps in plan.md in the commit that contains its last piece of work.\n\n", c.TasksPath)
	b.WriteString("Shell rules: the Claude settings block unparseable shell commands. Make plain one-command Bash calls with absolute paths. " +
		"Use the Read, Grep and Edit tools instead of shell pipelines. Never use `$(...)`, heredocs, variables, globs or `cd … &&` chains. Never redirect output, not even to `/dev/null` (no `2>/dev/null`, no `-o /dev/null`).\n\n")
	b.WriteString("Wenn du fertig bist, fasse in einem Absatz zusammen, was du getan hast und was die Tests sagen.\n")
	return b.String()
}

// LabelCoordinator labels the terminal self (default: the terminal hc runs in,
// which `hc get` resolves on its own) with striker.coordinator=<topic> and
// returns its address. A non-empty repo is labelled striker.repo=<repo> first,
// in the same order as striker start-coordinator.
func LabelCoordinator(c hc.Client, self, repo, topic string) (string, error) {
	if self == "" {
		out, err := c.R.Run("get", "--json")
		if err != nil {
			return "", err
		}
		var v struct {
			Address string `json:"address"`
		}
		if err := json.Unmarshal(out, &v); err != nil || v.Address == "" {
			return "", fmt.Errorf("hc get: no terminal address (run striker coordinator in a terminal, or pass --terminal)")
		}
		self = v.Address
	}
	if repo != "" {
		if err := c.Label(self, "striker.repo="+repo); err != nil {
			return self, err
		}
	}
	return self, c.Label(self, "striker.coordinator="+topic)
}
