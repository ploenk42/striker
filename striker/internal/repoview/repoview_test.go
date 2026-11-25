package repoview

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/plans"
)

func findPlan(t *testing.T, v View, topic string) PlanView {
	t.Helper()
	for _, p := range v.Plans {
		if p.Topic == topic {
			return p
		}
	}
	t.Fatalf("plan %q not in view", topic)
	return PlanView{}
}

func spec(topic, branch string, deps ...string) plans.Spec {
	if branch == "" {
		branch = topic
	}
	return plans.Spec{Topic: topic, Title: strings.ToUpper(topic), Branch: branch, DependsOn: deps}
}

func todoTask(n, issue int) plans.Task {
	return plans.Task{N: n, Issue: issue, Title: "t", StepsTotal: 2}
}

func TestTaskStates(t *testing.T) {
	pl := plans.Plan{Topic: "p", Tasks: []plans.Task{
		{N: 1, Issue: 42, StepsTotal: 2},
		{N: 2, Issue: 43, StepsTotal: 2},
		{N: 3, Issue: 41, StepsTotal: 2},
		{N: 4, StepsDone: 3, StepsTotal: 3},
		{N: 5, StepsDone: 1, StepsTotal: 3},
		{N: 6, Issue: 44, StepsTotal: 2},
		{N: 7},
		{N: 8, Issue: 45, StepsDone: 2, StepsTotal: 2}, // issue open, steps checked on default: not done
		{N: 9, Issue: 46, StepsTotal: 2},               // idle agent: working
	}}
	in := Inputs{
		Default:      "main",
		Specs:        []plans.Spec{spec("p", "")},
		Plans:        []plans.Plan{pl},
		DefaultPlans: map[string]plans.Plan{"p": pl},
		Closed:       map[int]bool{41: true},
		Agents: []Agent{
			{Address: "w1:t1", Status: "blocked", Role: "implementer", Task: 42},
			{Address: "w1:t3", Status: "working", Task: 42},
			{Address: "w1:t2", Status: "working", Task: 43},
			{Address: "w1:t4", Status: "done", Task: 44},
			{Address: "w1:t5", Status: "idle", Task: 0},
			{Address: "w1:t6", Status: "idle", Task: 46},
		},
	}
	p := findPlan(t, Build(in), "p")
	want := []string{"needs_you", "working", "done", "done", "todo", "todo", "todo", "todo", "working"}
	for i, w := range want {
		if got := p.Tasks[i].State; got != w {
			t.Errorf("task %d state = %q, want %q", i+1, got, w)
		}
	}
	if a := p.Tasks[0].Agents; len(a) != 2 || a[0].Address != "w1:t1" || a[0].Role != "implementer" || a[1].Address != "w1:t3" {
		t.Errorf("task 42 agents = %+v", a)
	}
	if p.Tasks[3].Issue != nil || p.Tasks[0].Issue == nil || *p.Tasks[0].Issue != 42 {
		t.Errorf("issue pointers wrong: %v %v", p.Tasks[3].Issue, p.Tasks[0].Issue)
	}
	if len(p.Tasks[3].Agents) != 0 {
		t.Errorf("unnumbered task got agents %+v", p.Tasks[3].Agents)
	}
	if p.State != "planned" {
		t.Errorf("plan state = %q, want planned", p.State)
	}
}

func TestPlanMergedWithoutBranch(t *testing.T) {
	in := Inputs{
		Default: "main",
		Trunk: []gitrepo.Commit{
			{SHA: "c3", Subject: "later"},
			{SHA: "c2", Subject: "fix", Fixes: []int{41}},
			{SHA: "c1", Subject: "start", Refs: []int{41}},
		},
		Specs:     []plans.Spec{spec("a", "")},
		Plans:     []plans.Plan{{Topic: "a", Tasks: []plans.Task{{N: 1, Issue: 41, StepsTotal: 2}, {N: 2, StepsDone: 1, StepsTotal: 1}}}},
		SpecFiles: map[string]string{"a": "docs/superpowers/specs/2026-01-01-a-design.md"},
		PlanFiles: map[string]string{"a": "docs/superpowers/plans/2026-01-01-a.md"},
		Closed:    map[int]bool{41: true},
	}
	in.DefaultPlans = map[string]plans.Plan{"a": in.Plans[0]}
	p := findPlan(t, Build(in), "a")
	if p.State != "merged" {
		t.Fatalf("state = %q, want merged", p.State)
	}
	if p.MergePoint != "c2" {
		t.Errorf("merge point = %q, want c2", p.MergePoint)
	}
	if p.Worktree != nil || p.Branch != "a" || p.Title != "A" {
		t.Errorf("plan = %+v", p)
	}
	if p.Spec != "docs/superpowers/specs/2026-01-01-a-design.md" || p.Plan != "docs/superpowers/plans/2026-01-01-a.md" {
		t.Errorf("files = %q %q", p.Spec, p.Plan)
	}
}

func TestPlanBlockedByDependency(t *testing.T) {
	in := Inputs{
		Default: "main",
		Specs:   []plans.Spec{spec("A", ""), spec("B", "", "A")},
		Plans: []plans.Plan{
			{Topic: "A", Tasks: []plans.Task{todoTask(1, 0)}},
			{Topic: "B", Tasks: []plans.Task{todoTask(1, 0)}},
		},
	}
	v := Build(in)
	b := findPlan(t, v, "B")
	if b.State != "blocked" || !reflect.DeepEqual(b.BlockedBy, []string{"A"}) {
		t.Errorf("B = %q %v, want blocked [A]", b.State, b.BlockedBy)
	}
	if a := findPlan(t, v, "A"); a.State != "planned" {
		t.Errorf("A = %q, want planned", a.State)
	}
}

func TestUnknownAndCyclicDependency(t *testing.T) {
	in := Inputs{
		Default: "main",
		Specs:   []plans.Spec{spec("C", "", "nope"), spec("D", "", "E"), spec("E", "", "D")},
		Plans: []plans.Plan{
			{Topic: "C", Tasks: []plans.Task{todoTask(1, 0)}},
			{Topic: "D", Tasks: []plans.Task{todoTask(1, 0)}},
			{Topic: "E", Tasks: []plans.Task{todoTask(1, 0)}},
		},
	}
	v := Build(in)
	c := findPlan(t, v, "C")
	if c.State != "blocked" || !reflect.DeepEqual(c.BlockedBy, []string{"nope?"}) {
		t.Errorf("C = %q %v", c.State, c.BlockedBy)
	}
	for _, topic := range []string{"D", "E"} {
		if p := findPlan(t, v, topic); p.State != "blocked" {
			t.Errorf("%s = %q, want blocked", topic, p.State)
		}
	}
}

func TestActiveWithWorktree(t *testing.T) {
	in := Inputs{
		Default:  "main",
		Branches: []Branch{{Name: "striker-go", Worktree: "/w", Commits: []gitrepo.Commit{{SHA: "b1", Subject: "wip", Refs: []int{7}}}}},
		Specs:    []plans.Spec{spec("striker-go-bundle", "striker-go", "nope")},
		Plans:    []plans.Plan{{Topic: "striker-go-bundle", Tasks: []plans.Task{todoTask(1, 7)}}},
		Agents:   []Agent{{Address: "w1:c", Status: "idle", Coordinator: "striker-go-bundle"}},
		Now:      time.Unix(0, 0),
	}
	v := Build(in)
	p := findPlan(t, v, "striker-go-bundle")
	if p.State != "active" {
		t.Fatalf("state = %q, want active", p.State)
	}
	if p.Branch != "striker-go" || p.Worktree == nil || *p.Worktree != "/w" {
		t.Errorf("branch %q worktree %v", p.Branch, p.Worktree)
	}
	if p.Coordinator != "w1:c" {
		t.Errorf("coordinator = %q", p.Coordinator)
	}
	if len(p.Commits) != 1 || p.Commits[0].SHA != "b1" {
		t.Errorf("commits = %+v", p.Commits)
	}
}

func TestStepsOnlyOnBranchNotDone(t *testing.T) {
	branchCopy := plans.Plan{Topic: "s", Tasks: []plans.Task{{N: 1, Title: "x", StepsDone: 2, StepsTotal: 2}}}
	defCopy := plans.Plan{Topic: "s", Tasks: []plans.Task{{N: 1, Title: "x", StepsTotal: 2}}}
	in := Inputs{
		Default:      "main",
		Branches:     []Branch{{Name: "s"}},
		Specs:        []plans.Spec{spec("s", "")},
		Plans:        []plans.Plan{branchCopy},
		DefaultPlans: map[string]plans.Plan{"s": defCopy},
	}
	p := findPlan(t, Build(in), "s")
	if p.State == "merged" || p.Tasks[0].State != "todo" {
		t.Errorf("plan %q task %q, want not merged / todo", p.State, p.Tasks[0].State)
	}
	if p.Tasks[0].StepsDone != 2 {
		t.Errorf("display steps = %d, want branch copy's 2", p.Tasks[0].StepsDone)
	}
	in.DefaultPlans = nil // no default-branch copy at all: not done
	if p := findPlan(t, Build(in), "s"); p.Tasks[0].State != "todo" {
		t.Errorf("without default copy: %q", p.Tasks[0].State)
	}
}

func TestInsertedTaskMatchedByIdentity(t *testing.T) {
	defCopy := plans.Plan{Topic: "m", Tasks: []plans.Task{
		{N: 1, Title: "a", StepsDone: 1, StepsTotal: 1},
		{N: 2, Issue: 43, Title: "b", StepsDone: 2, StepsTotal: 2},
		{N: 3, Title: "c", StepsDone: 1, StepsTotal: 1},
	}}
	// The branch inserts an open issue task (#50) and an unnumbered task at
	// positions whose default-branch tasks are fully checked.
	branchCopy := plans.Plan{Topic: "m", Tasks: []plans.Task{
		{N: 1, Title: "a", StepsDone: 1, StepsTotal: 1},
		{N: 2, Issue: 50, Title: "new", StepsTotal: 2},
		{N: 3, Title: "inserted", StepsTotal: 1},
		{N: 4, Issue: 43, Title: "b", StepsDone: 2, StepsTotal: 2},
	}}
	in := Inputs{
		Default:      "main",
		Branches:     []Branch{{Name: "m"}},
		Specs:        []plans.Spec{spec("m", "")},
		Plans:        []plans.Plan{branchCopy},
		DefaultPlans: map[string]plans.Plan{"m": defCopy},
		Closed:       map[int]bool{43: true},
	}
	p := findPlan(t, Build(in), "m")
	want := []string{"done", "todo", "done", "done"}
	// Position 3 on the branch is unnumbered and the default copy's
	// unnumbered task 3 is checked, so it reads done by N; #50 is open.
	for i, w := range want {
		if got := p.Tasks[i].State; got != w {
			t.Errorf("task %d state = %q, want %q", i+1, got, w)
		}
	}
	if p.State == "merged" {
		t.Error("plan must not be merged while #50 is open")
	}
	// Unnumbered branch task at a position where the default copy has an
	// issue task: never matched.
	in.Plans = []plans.Plan{{Topic: "m", Tasks: []plans.Task{
		{N: 1, Title: "a", StepsDone: 1, StepsTotal: 1},
		{N: 2, Title: "inserted", StepsTotal: 1},
	}}}
	if got := findPlan(t, Build(in), "m").Tasks[1].State; got != "todo" {
		t.Errorf("unnumbered task over default issue task = %q, want todo", got)
	}
}

type fakeGit map[string]string

func (f fakeGit) Git(dir string, args ...string) ([]byte, error) {
	k := strings.Join(args, " ")
	if v, ok := f[k]; ok {
		return []byte(v), nil
	}
	return nil, errors.New("fake git: " + k)
}

func (f fakeGit) Read(path string) ([]byte, error) { return nil, errors.New("no read") }

type fakeHC map[string]string

func (f fakeHC) Run(args ...string) ([]byte, error) {
	k := strings.Join(args, " ")
	if v, ok := f[k]; ok {
		return []byte(v), nil
	}
	return nil, errors.New("fake hc: " + k)
}

type fakeGH struct {
	out  string
	args []string
}

func (f *fakeGH) Run(dir string, args ...string) ([]byte, error) {
	f.args = args
	return []byte(f.out), nil
}

func TestGather(t *testing.T) {
	sp := "docs/superpowers/specs/"
	pl := "docs/superpowers/plans/"
	g := fakeGit{}
	g["symbolic-ref --short refs/remotes/origin/HEAD"] = "origin/main\n"
	g["log --format=%H%x1f%P%x1f%s%x1f%b%x1e -n 30 main"] = "c1\x1f\x1finit\x1f\x1e"
	g["worktree list --porcelain"] = "worktree /r\nbranch refs/heads/main\n\nworktree /w\nbranch refs/heads/feat\n\nworktree /d\ndetached\n"
	g["for-each-ref --format=%(refname:short) refs/heads refs/remotes/origin"] = "main\nfeat\nlonely\norigin/HEAD\norigin/main\norigin/pushed\n"
	g["ls-tree --name-only main "+sp] = sp + "2026-01-01-feat-design.md\n" + sp + "2026-01-01-old-design.md\n"
	g["ls-tree --name-only main "+pl] = pl + "2026-01-01-feat.md\n"
	g["show main:"+sp+"2026-01-01-feat-design.md"] = "# Feat main\n"
	g["show main:"+sp+"2026-01-01-old-design.md"] = "# Old\n"
	g["show main:"+pl+"2026-01-01-feat.md"] = "# P\n### Task #5: a\n- [ ] x\n"
	g["ls-tree --name-only feat "+sp] = sp + "2026-01-01-feat-design.md\n" + sp + "2026-01-01-old-design.md\n"
	g["show feat:"+sp+"2026-01-01-feat-design.md"] = "# Feat branch\n"
	g["show feat:"+sp+"2026-01-01-old-design.md"] = "# Old on feat\n"
	g["ls-tree --name-only origin/pushed "+pl] = pl + "2026-01-02-pushed.md\n"
	g["show origin/pushed:"+pl+"2026-01-02-pushed.md"] = "# Pushed\n### Task 1: b\n"
	g["rev-list --merges --ancestry-path feat..main"] = ""
	g["rev-list --merges --ancestry-path origin/pushed..main"] = "m1\n"
	g["log --format=%H%x1f%P%x1f%s%x1f%b%x1e -n 30 main..feat"] = "b1\x1fc1\x1fwip\x1fRefs #5\x1e"
	g["remote get-url origin"] = "git@github.com:o/r.git\n"
	h := fakeHC{}
	h["ls --json"] = `{"terminals":[{"address":"w:t1"},{"address":"remote1/w:t2"},{"address":"w:t3"},{"address":"w:gone"},{"address":"w:t4"},{"address":"w:t5"},{"address":"w:t6"}]}`
	h["get w:t1 --json"] = `{"address":"w:t1","cwd":"/w/sub","labels":{"striker.task":"5","striker.role":"implementer"},"status":{"state":"blocked"}}`
	h["get remote1/w:t2 --json"] = `{"address":"remote1/w:t2","cwd":"/r","labels":{"striker.coordinator":"feat"}}`
	h["get w:t3 --json"] = `{"address":"w:t3","cwd":"/r","labels":{"other":"x"}}`
	h["get w:t4 --json"] = `{"address":"w:t4","cwd":"/r","labels":{"striker.coordinator":"feat"}}`
	h["get w:t5 --json"] = `{"address":"w:t5","cwd":"/other","labels":{"striker.task":"5"},"status":{"state":"working"}}`
	h["get w:t6 --json"] = `{"address":"w:t6","cwd":"/rother","labels":{"striker.task":"5"},"status":{"state":"working"}}`
	gh := &fakeGH{out: `[{"number":9}]`}
	in, err := Gather(gitrepo.Repo{R: g, Dir: "/r"}, hc.Client{R: h}, gh, time.Unix(1, 0), Extra{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(gh.args, " ") != "issue list -R o/r --state closed --json number --limit 1000" {
		t.Errorf("gh args = %v", gh.args)
	}
	if len(in.DefaultPlans) != 1 || in.DefaultPlans["feat"].Title != "P" {
		t.Errorf("default plans = %+v", in.DefaultPlans)
	}
	if in.Default != "main" || len(in.Trunk) != 1 || !in.Closed[9] {
		t.Errorf("default %q trunk %v closed %v", in.Default, in.Trunk, in.Closed)
	}
	if len(in.Branches) != 2 || in.Branches[0].Name != "feat" || in.Branches[0].Worktree != "/w" ||
		len(in.Branches[0].Commits) != 1 || in.Branches[1].Name != "pushed" || in.Branches[1].Merge != "m1" {
		t.Errorf("branches = %+v", in.Branches)
	}
	titles := map[string]string{}
	for _, s := range in.Specs {
		titles[s.Topic] = s.Title
	}
	if titles["feat"] != "Feat branch" || titles["old"] != "Old" {
		t.Errorf("spec titles = %v (own branch copy wins, others not)", titles)
	}
	if len(in.Plans) != 2 || in.PlanFiles["pushed"] != pl+"2026-01-02-pushed.md" {
		t.Errorf("plans = %+v files %v", in.Plans, in.PlanFiles)
	}
	want := []Agent{
		{Address: "w:t1", Status: "blocked", Role: "implementer", Task: 5},
		{Address: "w:t4", Coordinator: "feat"},
	}
	if !reflect.DeepEqual(in.Agents, want) {
		t.Errorf("agents = %+v", in.Agents)
	}
	v := Build(in)
	p := findPlan(t, v, "feat")
	if p.State != "active" || p.Coordinator != "w:t4" || p.Tasks[0].State != "needs_you" {
		t.Errorf("feat = %q %q %+v", p.State, p.Coordinator, p.Tasks)
	}
}

// remoteGit serves fakeGit through hc's "exec --on <m> --cwd <dir> git ...".
type remoteGit struct {
	prefix string
	g      fakeGit
}

func (r remoteGit) Run(args ...string) ([]byte, error) {
	k, ok := strings.CutPrefix(strings.Join(args, " "), r.prefix)
	if !ok {
		return nil, errors.New("not git: " + strings.Join(args, " "))
	}
	return r.g.Git("", strings.Fields(k)...)
}

func TestGatherRemoteNoOrigin(t *testing.T) {
	g := fakeGit{}
	g["symbolic-ref --short refs/remotes/origin/HEAD"] = "origin/main\n"
	g["log --format=%H%x1f%P%x1f%s%x1f%b%x1e -n 30 main"] = ""
	g["worktree list --porcelain"] = "worktree /srv/r\nbranch refs/heads/main\n"
	g["for-each-ref --format=%(refname:short) refs/heads refs/remotes/origin"] = "main\n"
	repo := gitrepo.Repo{R: gitrepo.Remote{HC: remoteGit{prefix: "exec --on remote1 --cwd /srv/r git ", g: g}, Machine: "remote1"}, Dir: "/srv/r"}
	h := fakeHC{}
	h["ls --json"] = `{"terminals":[{"address":"remote1/w:t1"},{"address":"w:t2"},{"address":"other/w:t3"}]}`
	h["get remote1/w:t1 --json"] = `{"address":"remote1/w:t1","cwd":"/srv/r","labels":{"striker.task":"5"}}`
	h["get w:t2 --json"] = `{"address":"w:t2","cwd":"/srv/r","labels":{"striker.task":"5"}}`
	h["get other/w:t3 --json"] = `{"address":"other/w:t3","cwd":"/srv/r","labels":{"striker.task":"5"}}`
	gh := &fakeGH{out: "boom"}
	in, err := Gather(repo, hc.Client{R: h}, gh, time.Unix(1, 0), Extra{})
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Closed) != 0 || gh.args != nil {
		t.Errorf("no origin: closed %v, gh called with %v", in.Closed, gh.args)
	}
	if len(in.Agents) != 1 || in.Agents[0].Address != "remote1/w:t1" {
		t.Errorf("agents = %+v, want only the remote1 one", in.Agents)
	}
}

type failGH struct{}

func (failGH) Run(dir string, args ...string) ([]byte, error) {
	return nil, errors.New("gh: not logged in")
}

func TestGatherSurvivesFailingGH(t *testing.T) {
	g := fakeGit{}
	g["symbolic-ref --short refs/remotes/origin/HEAD"] = "origin/main\n"
	g["log --format=%H%x1f%P%x1f%s%x1f%b%x1e -n 30 main"] = ""
	g["worktree list --porcelain"] = "worktree /r\nbranch refs/heads/main\n"
	g["for-each-ref --format=%(refname:short) refs/heads refs/remotes/origin"] = "main\n"
	g["remote get-url origin"] = "git@github.com:o/r.git\n"
	h := fakeHC{"ls --json": `{"terminals":[]}`}
	in, err := Gather(gitrepo.Repo{R: g, Dir: "/r"}, hc.Client{R: h}, failGH{}, time.Unix(1, 0), Extra{})
	if err != nil {
		t.Fatalf("gh failure must not fail Gather: %v", err)
	}
	if len(in.Closed) != 0 || len(in.Warnings) != 1 || !strings.Contains(in.Warnings[0], "gh: not logged in") {
		t.Errorf("closed %v warnings %v", in.Closed, in.Warnings)
	}
	v := Build(in)
	if len(v.Warnings) != 1 {
		t.Errorf("view warnings = %v", v.Warnings)
	}
	b, _ := json.Marshal(Build(Inputs{Default: "main"}))
	if !strings.Contains(string(b), `"warnings":[]`) {
		t.Errorf("warnings not an empty array: %s", b)
	}
}

func TestJSONArraysNotNull(t *testing.T) {
	v := Build(Inputs{Default: "main", Specs: []plans.Spec{spec("x", "")}})
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "null") && !strings.Contains(string(b), `"worktree":null`) {
		t.Errorf("unexpected null in %s", b)
	}
	s := string(b)
	for _, k := range []string{`"trunk":[]`, `"tasks":[]`, `"depends_on":[]`, `"blocked_by":[]`, `"commits":[]`} {
		if !strings.Contains(s, k) {
			t.Errorf("missing %s in %s", k, s)
		}
	}
	if p := findPlan(t, v, "x"); p.State != "planned" {
		t.Errorf("spec without plan state = %q, want planned", p.State)
	}
}
