package dispatch

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/openspec"
	"github.com/ploenk42/striker/striker/internal/plans"
)

func TestArgvLabels(t *testing.T) {
	got := Argv("implementer", 42, "striker-go", "/w", "P")
	want := []string{"run", "--bg", "--name", "implementer-42", "--cwd", "/w",
		"--label", "striker.task=42", "--label", "striker.role=implementer",
		"--label", "striker.topic=striker-go", "claude", "P"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPromptCarriesIssueRefs(t *testing.T) {
	p := plans.Plan{Topic: "striker-go"}
	task := plans.Task{N: 1, Issue: 42, Title: "Do it", Body: "- [ ] **Step 1: write the test**\n- [ ] Step 2"}
	s := Prompt("implementer", p, task, "docs/plans/p.md", "docs/specs/s.md")
	for _, w := range []string{"Refs #42", "Fixes #42", "- [ ] **Step 1: write the test**", "docs/plans/p.md", "docs/specs/s.md", "implementer"} {
		if !strings.Contains(s, w) {
			t.Errorf("prompt lacks %q:\n%s", w, s)
		}
	}
	if !strings.Contains(s, "Bash") || !strings.Contains(s, "$(") {
		t.Errorf("prompt lacks shell rule:\n%s", s)
	}
}

func TestResolveRefusesUnnumbered(t *testing.T) {
	p := plans.Plan{Topic: "x", Tasks: []plans.Task{{N: 1, Title: "a"}, {N: 2, Title: "b"}}}
	if _, err := Resolve(p, 1, false); err == nil || !strings.Contains(err.Error(), "striker issues") {
		t.Fatalf("want refusal, got %v", err)
	}
	// Numbered plan, unknown issue: refused even in dry-run.
	q := plans.Plan{Tasks: []plans.Task{{N: 1, Issue: 7}}}
	if _, err := Resolve(q, 1, true); err == nil {
		t.Fatal("numbered plan must refuse unknown issue")
	}
}

func TestResolveRejectsNonPositive(t *testing.T) {
	p := plans.Plan{Tasks: []plans.Task{{N: 1, Title: "a"}}}
	for _, n := range []int{0, -1} {
		for _, dry := range []bool{false, true} {
			if _, err := Resolve(p, n, dry); err == nil {
				t.Errorf("Resolve(issue %d, dry %v) must fail", n, dry)
			}
		}
	}
}

func TestDryRunPlaceholders(t *testing.T) {
	p := plans.Plan{Topic: "x", Tasks: []plans.Task{{N: 1, Title: "a", Body: "- [ ] s"}}}
	task, err := Resolve(p, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	a := Command("implementer", 1, p, task, "/w", "p.md", "")
	joined := strings.Join(a, "\n")
	for _, w := range []string{"striker.task=<unassigned>", "Refs #<unassigned>", "Fixes #<unassigned>", "### Task #<unassigned>: a", "/dev/null"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing %q", w)
		}
	}
	if strings.Contains(joined, "#1") {
		t.Error("real issue ref leaked")
	}
}

func TestResolveDuplicateFirstWins(t *testing.T) {
	p := plans.Plan{Tasks: []plans.Task{{N: 1, Issue: 5, Title: "first"}, {N: 2, Issue: 5, Title: "second"}}}
	task, err := Resolve(p, 5, false)
	if err != nil || task.Title != "first" {
		t.Fatalf("got %v %v", task, err)
	}
}

func TestRoleAndTopic(t *testing.T) {
	if !ValidRole("spec-reviewer") || ValidRole("Bad Role") || ValidRole("") || ValidRole("a/b") {
		t.Error("role validation")
	}
	for _, a := range Argv("r", 1, "", "/w", "P") {
		if strings.HasPrefix(a, "striker.topic") {
			t.Error("empty topic must omit label")
		}
	}
}

func testChange() openspec.Change {
	return openspec.Change{Name: "feat", TasksPath: "openspec/changes/feat/tasks.md", PlanPath: "openspec/changes/feat/plan.md",
		Design: "openspec/changes/feat/design.md", Proposal: "openspec/changes/feat/proposal.md",
		Tasks: []openspec.Task{
			{ID: "1", Title: "Reader", Issue: 2},
			{ID: "2", Title: "Views", Issue: 42, Body: "- [ ] **Step 1: test**"},
			{ID: "2.1", Title: "Detail"},
		}}
}

func TestResolveChange(t *testing.T) {
	c := testChange()
	for arg, want := range map[string]string{
		"2":   "2",   // a task id wins over issue #2
		"2.1": "2.1", // task id
		"#2":  "1",   // explicit issue
		"42":  "2",   // bare number that is no task id: issue
		"#42": "2",
	} {
		task, err := ResolveChange(c, arg)
		if err != nil || task.ID != want {
			t.Errorf("ResolveChange(%q) = %q %v, want %q", arg, task.ID, err, want)
		}
	}
	for _, bad := range []string{"", "9", "#9", "x y"} {
		if _, err := ResolveChange(c, bad); err == nil {
			t.Errorf("ResolveChange(%q) accepted", bad)
		}
	}
}

func TestRemoteCommandChange(t *testing.T) {
	c := openspec.Change{Name: "rb", TasksPath: "openspec/changes/rb/tasks.md",
		Tasks: []openspec.Task{{ID: "2.3", Title: "Thing", Body: "- [ ] **Step 1**"}}}
	task := c.Tasks[0]
	got := RemoteCommandChange("implementer", "alpha", c, task, "remote1", "awayteam", "/srv/alpha-rb", "remote1/t2b")
	want := []string{"run", "--on", "remote1", "--bg", "--name", "implementer-2.3", "--parent", "remote1/t2b",
		"--label", "striker.task=2.3", "--label", "striker.role=implementer", "--label", "striker.topic=rb",
		"--label", "striker.repo=alpha", "--"}
	want = append(want, fleet.ClaudeCommand("awayteam", "/srv/alpha-rb", PromptChange("implementer", c, task))...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if strings.Contains(strings.Join(got, " "), "--remote-control") || strings.Contains(strings.Join(got, " "), "--cwd") {
		t.Errorf("relayed argv has --remote-control or --cwd: %q", got)
	}
}

func TestCommandChange(t *testing.T) {
	c := testChange()
	a := CommandChange("implementer", "herdr-mods", c, c.Tasks[1], "/w")
	want := []string{"run", "--bg", "--name", "implementer-2", "--cwd", "/w",
		"--label", "striker.task=2", "--label", "striker.role=implementer", "--label", "striker.topic=feat",
		"--label", "striker.repo=herdr-mods", "claude"}
	if !reflect.DeepEqual(a[:len(want)], want) || len(a) != len(want)+1 {
		t.Fatalf("argv %q", a)
	}
	p := a[len(a)-1]
	for _, w := range []string{"Task 2", "Views", "Issue #42", "Refs #42", "Fixes #42", "- [ ] **Step 1: test**",
		"openspec/changes/feat/tasks.md", "openspec/changes/feat/plan.md", "openspec/changes/feat/design.md", "Bash"} {
		if !strings.Contains(p, w) {
			t.Errorf("prompt lacks %q:\n%s", w, p)
		}
	}
	// Task without issue: no Refs/Fixes promise, name and label carry the id.
	a = CommandChange("spec-reviewer", "", c, c.Tasks[2], "/w")
	if a[3] != "spec-reviewer-2.1" || a[7] != "striker.task=2.1" || strings.Contains(a[len(a)-1], "Fixes #") {
		t.Errorf("unnumbered argv %q", a)
	}
	// Ruling 9: no repo known, no striker.repo label.
	if strings.Contains(strings.Join(a[:len(a)-1], " "), "striker.repo") {
		t.Errorf("argv without repo carries striker.repo: %q", a)
	}
}

type fake struct{ calls [][]string }

func (f *fake) Run(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if args[0] == "get" {
		return []byte(`{"address":"t9"}`), nil
	}
	return nil, nil
}

func TestCoordinatorLabel(t *testing.T) {
	f := &fake{}
	addr, err := LabelCoordinator(hc.Client{R: f}, "", "herdr-mods", "lcars-bridge")
	if err != nil || addr != "t9" {
		t.Fatalf("addr %q err %v", addr, err)
	}
	// striker.repo first, then striker.coordinator (same order as start-coordinator).
	want := [][]string{{"label", "t9", "striker.repo=herdr-mods"}, {"label", "t9", "striker.coordinator=lcars-bridge"}}
	if got := f.calls[len(f.calls)-2:]; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// An explicit address skips the lookup; no repo: only striker.coordinator.
	f = &fake{}
	addr, err = LabelCoordinator(hc.Client{R: f}, "t5", "", "x")
	if err != nil || addr != "t5" || len(f.calls) != 1 || f.calls[0][1] != "t5" {
		t.Fatalf("addr %q err %v calls %v", addr, err, f.calls)
	}
}
