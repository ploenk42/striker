package openspec

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
)

func TestParseTasksSuffix(t *testing.T) {
	text := "# Tasks\n\n## 2. Group\n\n- [x] 2.1 A\n- [ ] 2.2 Mid\n- [ ] 2.3 B (#42; after 2.1, 2.2)\n- [ ] 2.4 C\n"
	ts, warns := ParseTasks([]byte(text))
	if len(warns) != 0 {
		t.Errorf("warnings = %v", warns)
	}
	if len(ts) != 4 {
		t.Fatalf("tasks = %d", len(ts))
	}
	if !ts[0].Done || ts[0].ID != "2.1" || ts[0].Title != "A" || len(ts[0].After) != 0 {
		t.Errorf("A = %+v", ts[0])
	}
	b := ts[2]
	if b.Issue != 42 || b.Title != "B" || !reflect.DeepEqual(b.After, []string{"2.1", "2.2"}) {
		t.Errorf("B = %+v", b)
	}
	if !reflect.DeepEqual(ts[3].After, []string{"2.3"}) || ts[3].Issue != 0 {
		t.Errorf("C = %+v", ts[3])
	}
}

func TestSetIssue(t *testing.T) {
	text := "# Tasks\r\n\r\n- [ ] 1 Reader (Go)\r\n- [x] 2 Views (after 1)\r\n- [ ] 3 Done (#7)\r\n- [ ] 4 Last\r\n  - [ ] 4 nested\r\n"
	for _, tc := range []struct {
		id    string
		issue int
		line  string
	}{
		{"1", 42, "- [ ] 1 Reader (Go) (#42)\r\n"},
		{"2", 43, "- [x] 2 Views (#43; after 1)\r\n"},
		{"4", 44, "- [ ] 4 Last (#44)\r\n"},
	} {
		got, ok := SetIssue([]byte(text), tc.id, tc.issue)
		if !ok || !strings.Contains(string(got), tc.line) {
			t.Errorf("SetIssue %s: ok=%v\n%q", tc.id, ok, got)
		}
		if strings.Count(string(got), "\r\n") != strings.Count(text, "\r\n") || len(got)-len(text) != len(tc.line)-len(lineOf(text, tc.id)) {
			t.Errorf("SetIssue %s changed more than the line:\n%q", tc.id, got)
		}
		ts, _ := ParseTasks(got)
		for _, task := range ts {
			if task.ID == tc.id && task.Issue != tc.issue {
				t.Errorf("SetIssue %s: parsed issue %d", tc.id, task.Issue)
			}
		}
	}
	// Already numbered, or unknown id: unchanged.
	for _, id := range []string{"3", "9"} {
		if got, ok := SetIssue([]byte(text), id, 50); ok || string(got) != text {
			t.Errorf("SetIssue %s: ok=%v %q", id, ok, got)
		}
	}
	// The parsed implicit "after" never ends up in the file.
	got, _ := SetIssue([]byte("- [ ] 1 A\n- [ ] 2 B\n"), "2", 5)
	if string(got) != "- [ ] 1 A\n- [ ] 2 B (#5)\n" {
		t.Errorf("implicit after written: %q", got)
	}
}

func lineOf(text, id string) string {
	for _, l := range strings.SplitAfter(text, "\n") {
		if strings.HasPrefix(l, "- [ ] "+id+" ") || strings.HasPrefix(l, "- [x] "+id+" ") {
			return l
		}
	}
	return ""
}

func TestParseTasksMalformed(t *testing.T) {
	cases := map[string]string{
		"bad issue":  "- [ ] 1 A\n- [ ] 2 B (#x)\n",
		"bad after":  "- [ ] 1 A\n- [ ] 2 B (after 9.9)\n",
		"two suffix": "- [ ] 1 A\n- [ ] 2 B (#3) (after 1)\n",
	}
	for name, text := range cases {
		ts, warns := ParseTasks([]byte(text))
		if len(ts) != 2 {
			t.Errorf("%s: tasks = %d", name, len(ts))
			continue
		}
		if len(warns) == 0 {
			t.Errorf("%s: no warnings", name)
		}
		for _, w := range warns {
			if !strings.Contains(w, "line 2") {
				t.Errorf("%s: warning lacks line: %q", name, w)
			}
		}
		if !reflect.DeepEqual(ts[1].After, []string{"1"}) && name != "two suffix" {
			t.Errorf("%s: After = %v", name, ts[1].After)
		}
	}
}

func TestParseTasksNestedIgnored(t *testing.T) {
	ts, _ := ParseTasks([]byte("- [ ] 1 A\n  - [ ] 1.1 nested\n\t- [x] 1.2 tab\n- [x] 2 B\n"))
	if len(ts) != 2 || ts[0].ID != "1" || ts[1].ID != "2" {
		t.Errorf("tasks = %+v", ts)
	}
}

func TestParseMeta(t *testing.T) {
	dep, br := ParseMeta([]byte("schema: spec-driven\nx-depends_on: [a, b]\nx-branch: br\ndepends_on: [z]\nbranch: nope\n"))
	if !reflect.DeepEqual(dep, []string{"a", "b"}) || br != "br" {
		t.Errorf("dep=%v br=%q", dep, br)
	}
	dep, br = ParseMeta([]byte("depends_on: [z]\nbranch: nope\r\n"))
	if len(dep) != 0 || br != "" {
		t.Errorf("unprefixed read: dep=%v br=%q", dep, br)
	}
}

type fakeRunner struct {
	files map[string]string // repo-relative path -> content
}

func (f fakeRunner) Git(dir string, args ...string) ([]byte, error) {
	switch args[0] {
	case "ls-tree", "ls-files":
		var out []string
		for p := range f.files {
			out = append(out, p)
		}
		return []byte(strings.Join(out, "\n")), nil
	case "show":
		i := strings.Index(args[1], ":")
		if c, ok := f.files[args[1][i+1:]]; ok {
			return []byte(c), nil
		}
	}
	return nil, fmt.Errorf("fake: unexpected git %v", args)
}

func (f fakeRunner) Read(path string) ([]byte, error) {
	if c, ok := f.files[strings.TrimPrefix(path, "/r/")]; ok {
		return []byte(c), nil
	}
	return nil, fmt.Errorf("no file %s", path)
}

func TestReadArchivedDate(t *testing.T) {
	fr := fakeRunner{files: map[string]string{
		"openspec/changes/archive/2026-11-21-lcars-bridge/tasks.md":       "- [x] 1 A\n",
		"openspec/changes/archive/2026-11-21-lcars-bridge/proposal.md":    "# P\n## Why\n## What\n",
		"openspec/changes/archive/2026-11-21-lcars-bridge/design.md":      "# D\n## Context\n```\n## not\n```\n",
		"openspec/changes/archive/2026-11-21-lcars-bridge/.openspec.yaml": "x-depends_on: [x]\n",
		"openspec/changes/live/tasks.md":                                  "- [ ] 1 A\n",
		"openspec/changes/live/plan.md":                                   "# Plan\n### Task 1: A\nbody A\n\n### Task 2: Z\nz\n",
		"README.md":                                                       "x",
	}}
	for _, ref := range []string{"main", ""} {
		s, has, err := Read(gitrepo.Repo{R: fr, Dir: "/r"}, ref)
		if err != nil || !has {
			t.Fatalf("ref %q: has=%v err=%v", ref, has, err)
		}
		if len(s.Changes) != 2 {
			t.Fatalf("ref %q: changes = %+v", ref, s.Changes)
		}
		var arch, live Change
		for _, c := range s.Changes {
			if c.Archived != "" {
				arch = c
			} else {
				live = c
			}
		}
		if arch.Name != "lcars-bridge" || arch.Archived != "2026-11-21" || len(arch.Tasks) != 1 {
			t.Errorf("archived = %+v", arch)
		}
		if !reflect.DeepEqual(arch.DependsOn, []string{"x"}) || arch.Branch != "lcars-bridge" {
			t.Errorf("meta = %v %q", arch.DependsOn, arch.Branch)
		}
		if !reflect.DeepEqual(arch.Toc, []string{"Why", "What", "Context"}) {
			t.Errorf("toc = %v", arch.Toc)
		}
		if live.Name != "live" || live.Branch != "live" || live.Tasks[0].Body != "body A" {
			t.Errorf("live = %+v", live)
		}
	}
}

func TestReadNoOpenspec(t *testing.T) {
	_, has, err := Read(gitrepo.Repo{R: fakeRunner{files: map[string]string{}}, Dir: "/r"}, "main")
	if err != nil || has {
		t.Errorf("has=%v err=%v", has, err)
	}
}

func TestMergeBranchWins(t *testing.T) {
	mk := func(done int) Store {
		ts := []Task{{ID: "1", Done: done > 0}, {ID: "2", Done: done > 1}, {ID: "3"}}
		return Store{Changes: []Change{{Name: "c", Branch: "br", Tasks: ts, TasksPath: "x"}}}
	}
	arch := Change{Name: "old", Archived: "2026-01-01", Branch: "br", Tasks: []Task{{ID: "1"}}}
	main := mk(0)
	main.Changes = append(main.Changes, arch)
	branchOnly := mk(2)
	branchOnly.Changes = append(branchOnly.Changes, Change{Name: "extra", Tasks: []Task{{ID: "1", Done: true}}})
	got := Merge(main, map[string]Store{"br": branchOnly})
	if len(got.Changes) != 2 {
		t.Fatalf("changes = %d", len(got.Changes))
	}
	n := 0
	for _, tk := range got.Changes[0].Tasks {
		if tk.Done {
			n++
		}
	}
	if n != 2 {
		t.Errorf("merged ticked = %d, want 2", n)
	}
	if got.Changes[1].Tasks[0].Done || got.Changes[1].Archived == "" {
		t.Errorf("archived touched: %+v", got.Changes[1])
	}
	if main.Changes[0].Tasks[1].Done {
		t.Error("input store mutated")
	}
}

func TestCapabilitiesCount(t *testing.T) {
	fr := fakeRunner{files: map[string]string{
		"openspec/specs/bridge/overview/spec.md": "# S\n### Requirement: A\ntext\n#### Scenario: x\n### Requirement: B\n",
		"openspec/specs/striker/spec.md":         "# S\n",
	}}
	s, has, err := Read(gitrepo.Repo{R: fr, Dir: "/r"}, "main")
	if err != nil || !has {
		t.Fatalf("has=%v err=%v", has, err)
	}
	want := []Capability{{Path: "bridge/overview", Requirements: 2}, {Path: "striker", Requirements: 0}}
	if !reflect.DeepEqual(s.Capabilities, want) {
		t.Errorf("caps = %+v", s.Capabilities)
	}
}
