package issues

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
)

type fakeRunner struct {
	plan  string
	clean bool // git status reports no change
	calls []string
}

func (f *fakeRunner) Git(dir string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	if len(args) > 0 && args[0] == "symbolic-ref" {
		return []byte("origin/main\n"), nil
	}
	if len(args) > 0 && args[0] == "status" && !f.clean {
		return []byte(" M docs/p.md\n"), nil
	}
	return nil, nil
}

func (f *fakeRunner) Read(path string) ([]byte, error) { return []byte(f.plan), nil }

type fakeGH struct {
	next    int
	failAt  int // 1-based create call that fails; 0 never
	creates int
	calls   [][]string
	bodies  map[int]string // issue number -> current body
}

func (g *fakeGH) Run(dir string, args ...string) ([]byte, error) {
	g.calls = append(g.calls, args)
	switch {
	case args[0] == "issue" && args[1] == "view":
		n, _ := strconv.Atoi(args[2])
		b, _ := json.Marshal(map[string]string{"body": g.bodies[n]})
		return b, nil
	case args[0] == "issue" && args[1] == "create":
		g.creates++
		if g.creates == g.failAt {
			return nil, errors.New("gh: boom")
		}
		n := g.next
		g.next++
		if g.bodies == nil {
			g.bodies = map[int]string{}
		}
		g.bodies[n] = args[5]
		return []byte("https://github.com/o/r/issues/" + itoa(n) + "\n"), nil
	case args[0] == "repo":
		return []byte(`{"nameWithOwner":"o/r"}`), nil
	}
	return nil, nil
}

func itoa(n int) string { return strconv.Itoa(n) }

const plan = "# Plan\n\n### Task 1: First thing\n\nDo the first.\nStill first paragraph.\n\nMore.\n\n### Task 2: Second `x`\n\n**Depends on:** Task 1\n\nDo second.\n"

func run(t *testing.T, text string, gh *fakeGH) (map[int]int, error, []string, *fakeRunner) {
	t.Helper()
	fr := &fakeRunner{plan: text}
	var writes []string
	m, err := Create(gh, gitrepo.Repo{R: fr, Dir: "/r"}, "docs/p.md", func(rel string, b []byte) error {
		writes = append(writes, string(b))
		return nil
	})
	return m, err, writes, fr
}

func TestCreateNumbersHeadings(t *testing.T) {
	gh := &fakeGH{next: 42}
	m, err, writes, fr := run(t, plan, gh)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 || m[1] != 42 || m[2] != 43 {
		t.Fatalf("map = %v", m)
	}
	last := writes[len(writes)-1]
	for _, w := range []string{"### Task #42: First thing", "### Task #43: Second `x`", "**Depends on:** #42"} {
		if !strings.Contains(last, w) {
			t.Fatalf("missing %q in\n%s", w, last)
		}
	}
	// create body: first paragraph only plus plan line
	c := gh.calls[0]
	if c[2] != "--title" || c[3] != "First thing" || c[4] != "--body" ||
		c[5] != "Do the first.\nStill first paragraph.\n\nPlan: docs/p.md (Task 1)" {
		t.Fatalf("create args = %q", c)
	}
	// edit adds link; commit follows
	var edit []string
	for _, c := range gh.calls {
		if c[1] == "edit" {
			edit = c
		}
	}
	if edit == nil || !strings.Contains(edit[len(edit)-1], "https://github.com/o/r/blob/main/docs/p.md#task-43-second-x") {
		t.Fatalf("edit = %q", edit)
	}
	n := len(fr.calls)
	if fr.calls[n-2] != "add docs/p.md" || fr.calls[n-1] != "commit -m plan: issues for p -- docs/p.md" {
		t.Fatalf("git calls = %q", fr.calls)
	}
}

func TestCreateSkipsNumbered(t *testing.T) {
	text := "# P\n\n### Task #42: Done\n\nx\n\n### Task 2: New\n\ny\n"
	gh := &fakeGH{next: 50}
	m, err, _, _ := run(t, text, gh)
	if err != nil {
		t.Fatal(err)
	}
	if gh.creates != 1 || len(m) != 1 || m[2] != 50 {
		t.Fatalf("creates=%d map=%v", gh.creates, m)
	}
}

func TestCreateRerunLinksNumbered(t *testing.T) {
	text := "# P\n\n### Task #42: A\n\nx\n\n### Task #43: B\n\ny\n"
	link42 := "https://github.com/o/r/blob/main/docs/p.md#task-42-a"
	gh := &fakeGH{next: 50, bodies: map[int]string{
		// striker-made, person added a checklist, link missing: append
		42: "x\n\nPlan: docs/p.md (Task 1)\n\n- [ ] mine\n",
		// striker-made, link already there: no edit
		43: "y\n\nPlan: docs/p.md (Task 2)\n\nhttps://github.com/o/r/blob/main/docs/p.md#task-43-b",
	}}
	fr := &fakeRunner{plan: text, clean: true}
	m, err := Create(gh, gitrepo.Repo{R: fr, Dir: "/r"}, "docs/p.md", func(string, []byte) error { return nil })
	if err != nil || len(m) != 0 || gh.creates != 0 {
		t.Fatalf("m=%v err=%v creates=%d", m, err, gh.creates)
	}
	var edits [][]string
	for _, c := range gh.calls {
		if c[1] == "edit" {
			edits = append(edits, c)
		}
	}
	if len(edits) != 1 || edits[0][2] != "42" ||
		edits[0][4] != "x\n\nPlan: docs/p.md (Task 1)\n\n- [ ] mine\n\n"+link42 {
		t.Fatalf("edits = %q", edits)
	}
	for _, c := range fr.calls {
		if strings.HasPrefix(c, "commit") {
			t.Fatal("must not commit unchanged plan")
		}
	}
}

func TestCreateLeavesHandNumberedIssue(t *testing.T) {
	text := "# P\n\n### Task #7: Existing\n\nx\n"
	gh := &fakeGH{bodies: map[int]string{7: "someone else's issue"}}
	fr := &fakeRunner{plan: text, clean: true}
	if _, err := Create(gh, gitrepo.Repo{R: fr, Dir: "/r"}, "docs/p.md", func(string, []byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	for _, c := range gh.calls {
		if c[1] == "edit" {
			t.Fatalf("hand-numbered issue edited: %q", c)
		}
	}
}

func TestCreatePartialFailure(t *testing.T) {
	gh := &fakeGH{next: 42, failAt: 2}
	m, err, writes, fr := run(t, plan, gh)
	if err == nil {
		t.Fatal("want error")
	}
	if len(m) != 1 || m[1] != 42 {
		t.Fatalf("map = %v", m)
	}
	last := writes[len(writes)-1]
	if !strings.Contains(last, "### Task #42:") || !strings.Contains(last, "### Task 2:") {
		t.Fatalf("last write:\n%s", last)
	}
	for _, c := range fr.calls {
		if strings.HasPrefix(c, "commit") {
			t.Fatal("must not commit after failure")
		}
	}
}

func TestCreatePreservesCRLF(t *testing.T) {
	text := strings.ReplaceAll(plan, "\n", "\r\n")
	gh := &fakeGH{next: 42}
	_, err, writes, _ := run(t, text, gh)
	if err != nil {
		t.Fatal(err)
	}
	last := writes[len(writes)-1]
	want := strings.ReplaceAll(strings.NewReplacer(
		"### Task 1:", "### Task #42:", "### Task 2:", "### Task #43:", "Task 1\n", "#42\n").Replace(plan), "\n", "\r\n")
	if last != want {
		t.Fatalf("got %q\nwant %q", last, want)
	}
}

// osRunner is a working tree with files (repo-relative path -> content).
type osRunner struct {
	files map[string]string
	calls []string
}

func (f *osRunner) Git(dir string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	switch args[0] {
	case "ls-files":
		var b strings.Builder
		for p := range f.files {
			b.WriteString(p + "\n")
		}
		return []byte(b.String()), nil
	case "symbolic-ref":
		return []byte("origin/main\n"), nil
	case "status":
		return []byte(" M openspec/changes/feat/tasks.md\n"), nil
	}
	return nil, nil
}

func (f *osRunner) Read(path string) ([]byte, error) {
	if s, ok := f.files[strings.TrimPrefix(path, "/r/")]; ok {
		return []byte(s), nil
	}
	return nil, errors.New("no file " + path)
}

func changeRepo() *osRunner {
	return &osRunner{files: map[string]string{
		"openspec/changes/feat/.openspec.yaml": "schema: spec-driven\n",
		"openspec/changes/feat/tasks.md":       "# Tasks\n\n- [ ] 1 Reader (Go)\n- [ ] 2 Views (after 1)\n- [ ] 3 Old (#7)\n",
		"openspec/changes/feat/plan.md":        "# Plan\n\n### Task 1: Reader (Go)\n\nFirst para.\nStill.\n\nMore.\n",
	}}
}

func TestCreateChange(t *testing.T) {
	fr := changeRepo()
	gh := &fakeGH{next: 42}
	var writes []string
	m, err := CreateChange(gh, gitrepo.Repo{R: fr, Dir: "/r"}, "feat", func(rel string, b []byte) error {
		if rel != "openspec/changes/feat/tasks.md" {
			t.Errorf("write %s", rel)
		}
		writes = append(writes, string(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 || m["1"] != 42 || m["2"] != 43 {
		t.Fatalf("map = %v", m)
	}
	last := writes[len(writes)-1]
	if last != "# Tasks\n\n- [ ] 1 Reader (Go) (#42)\n- [ ] 2 Views (#43; after 1)\n- [ ] 3 Old (#7)\n" {
		t.Fatalf("tasks.md = %q", last)
	}
	var create []string
	for _, c := range gh.calls {
		if c[0] == "issue" && c[1] == "create" {
			create = c
			break
		}
	}
	if create[3] != "Reader (Go)" || !strings.HasPrefix(create[5], "First para.\nStill.\n\nChange: openspec/changes/feat (Task 1)") ||
		!strings.Contains(create[5], "https://github.com/o/r/blob/main/openspec/changes/feat/plan.md#task-1-reader-go") {
		t.Fatalf("create = %q", create)
	}
	n := len(fr.calls)
	if fr.calls[n-2] != "add openspec/changes/feat/tasks.md" || fr.calls[n-1] != "commit -m openspec: issues for feat -- openspec/changes/feat/tasks.md" {
		t.Fatalf("git calls = %q", fr.calls)
	}
}

func TestCreateChangePartialFailure(t *testing.T) {
	fr := changeRepo()
	gh := &fakeGH{next: 42, failAt: 2}
	var writes []string
	m, err := CreateChange(gh, gitrepo.Repo{R: fr, Dir: "/r"}, "feat", func(rel string, b []byte) error {
		writes = append(writes, string(b))
		return nil
	})
	if err == nil || len(m) != 1 || m["1"] != 42 || len(writes) != 1 || !strings.Contains(writes[0], "(#42)") {
		t.Fatalf("err=%v m=%v writes=%q", err, m, writes)
	}
	for _, c := range fr.calls {
		if strings.HasPrefix(c, "commit") {
			t.Fatal("must not commit after failure")
		}
	}
}

func TestCreateChangeUnknown(t *testing.T) {
	gh := &fakeGH{}
	if _, err := CreateChange(gh, gitrepo.Repo{R: changeRepo(), Dir: "/r"}, "nope", nil); err == nil || len(gh.calls) != 0 {
		t.Fatalf("err=%v gh=%q", err, gh.calls)
	}
}

func TestClosed(t *testing.T) {
	gh := &closedGH{}
	m, err := Closed(gh, "o/r")
	if err != nil || !m[3] || !m[9] || m[4] {
		t.Fatalf("m=%v err=%v", m, err)
	}
	if strings.Join(gh.args, " ") != "issue list -R o/r --state closed --json number --limit 1000" {
		t.Fatalf("args = %v", gh.args)
	}
	if gh.dir != "" {
		t.Fatalf("dir = %q, want empty (gh must not depend on the repo path)", gh.dir)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:o/r.git\n":         "o/r",
		"https://github.com/o/r.git":       "o/r",
		"https://github.com/o/r":           "o/r",
		"https://github.com/o/r/":          "o/r",
		"ssh://git@github.com/o/r.git":     "o/r",
		"https://gitlab.com/o/r.git":       "",
		"":                                 "",
		"https://github.com/o.dot/r.dot.x": "o.dot/r.dot.x",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

type closedGH struct {
	dir  string
	args []string
}

func (c *closedGH) Run(dir string, args ...string) ([]byte, error) {
	c.dir, c.args = dir, args
	return []byte(`[{"number":3},{"number":9}]`), nil
}
