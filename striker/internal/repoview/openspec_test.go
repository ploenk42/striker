package repoview

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/openspec"
	"github.com/ploenk42/striker/striker/internal/plans"
)

// osTasks builds tasks "1".."n" (each after the previous), the first done ones ticked.
func osTasks(n, done int) []openspec.Task {
	var ts []openspec.Task
	for i := 1; i <= n; i++ {
		t := openspec.Task{ID: string(rune('0' + i)), Title: "task", Done: i <= done}
		if i > 1 {
			t.After = []string{ts[i-2].ID}
		}
		ts = append(ts, t)
	}
	return ts
}

func osChange(name string, tasks []openspec.Task, deps ...string) openspec.Change {
	return openspec.Change{Name: name, Branch: name, DependsOn: deps, Tasks: tasks,
		Design:    "openspec/changes/" + name + "/design.md",
		TasksPath: "openspec/changes/" + name + "/tasks.md"}
}

func osInputs(cs ...openspec.Change) Inputs {
	return Inputs{Default: "main", OpenSpec: &openspec.Store{Changes: cs}}
}

func TestOpenSpecArchivedDone(t *testing.T) {
	old := osChange("old", osTasks(2, 1))
	old.Archived = "2026-11-21"
	in := osInputs(old, osChange("act", osTasks(1, 0)))
	v := Build(in)
	if len(v.Plans) != 2 || v.Plans[0].Topic != "act" || v.Plans[1].Topic != "old" {
		t.Fatalf("plans = %+v, want act then archived old", v.Plans)
	}
	p := v.Plans[1]
	if p.State != "done" || p.Archived != "2026-11-21" || p.Legacy {
		t.Errorf("old = %q archived %q legacy %v", p.State, p.Archived, p.Legacy)
	}
	for _, tv := range p.Tasks {
		if tv.State != "done" {
			t.Errorf("archived task %s = %q, want done", tv.ID, tv.State)
		}
	}
	if p.InterruptedAt != "" {
		t.Errorf("archived change interrupted at %q", p.InterruptedAt)
	}
}

func TestOpenSpecInProgressByTick(t *testing.T) {
	c := osChange("c", []openspec.Task{
		{ID: "1.1", Title: "a", Done: true, Body: "steps a"},
		{ID: "1.2", Title: "b", Issue: 42, After: []string{"1.1"}},
		{ID: "2", Title: "c", After: []string{"1.1", "1.2"}},
	})
	c.Toc = []string{"Why", "Design"}
	in := osInputs(c)
	in.Coordinators = []Coordinator{{Change: "c", Address: "w:c", State: "working"}}
	p := findPlan(t, Build(in), "c")
	if p.State != "in progress" {
		t.Fatalf("state = %q, want in progress", p.State)
	}
	if p.Spec != c.Design || p.Plan != c.TasksPath || p.Branch != "c" || p.Title != "c" ||
		!reflect.DeepEqual(p.Toc, []string{"Why", "Design"}) || p.Coordinator != "w:c" {
		t.Errorf("plan = %+v", p)
	}
	t0, t1, t2 := p.Tasks[0], p.Tasks[1], p.Tasks[2]
	if t0.ID != "1.1" || t0.N != 1 || t0.StepsDone != 1 || t0.StepsTotal != 1 || t0.State != "done" || t0.Body != "steps a" {
		t.Errorf("task 1.1 = %+v", t0)
	}
	if t1.N != 2 || t1.Issue == nil || *t1.Issue != 42 || t1.StepsDone != 0 || t1.StepsTotal != 1 ||
		!reflect.DeepEqual(t1.DependsOn, []int{1}) || t1.State != "todo" {
		t.Errorf("task 1.2 = %+v", t1)
	}
	if !reflect.DeepEqual(t2.DependsOn, []int{1, 2}) {
		t.Errorf("task 2 depends_on = %v", t2.DependsOn)
	}

	// No tick, but a worktree for the branch: in progress too.
	w := osInputs(osChange("w", osTasks(2, 0)))
	w.Branches = []Branch{{Name: "w", Worktree: "/wt", Commits: []gitrepo.Commit{{SHA: "b1"}}}}
	pw := findPlan(t, Build(w), "w")
	if pw.State != "in progress" || pw.Worktree == nil || *pw.Worktree != "/wt" || len(pw.Commits) != 1 {
		t.Errorf("worktree change = %q %v %v", pw.State, pw.Worktree, pw.Commits)
	}
	// Neither: planned.
	if pp := findPlan(t, Build(osInputs(osChange("p", osTasks(2, 0)))), "p"); pp.State != "planned" {
		t.Errorf("untouched change = %q, want planned", pp.State)
	}
	// An issue closed counts as done.
	cl := osInputs(osChange("i", []openspec.Task{{ID: "1", Issue: 7}}))
	cl.Closed = map[int]bool{7: true}
	if pi := findPlan(t, Build(cl), "i"); pi.Tasks[0].State != "done" {
		t.Errorf("closed issue task = %q, want done", pi.Tasks[0].State)
	}
}

func TestOpenSpecBlockedByDependency(t *testing.T) {
	arch := osChange("base", osTasks(1, 1))
	arch.Archived = "2026-11-01"
	in := osInputs(osChange("a", osTasks(1, 0)), osChange("b", osTasks(1, 0), "a", "ghost"),
		osChange("c", osTasks(1, 0), "base"), arch)
	v := Build(in)
	b := findPlan(t, v, "b")
	if b.State != "blocked" || !reflect.DeepEqual(b.BlockedBy, []string{"a", "ghost?"}) ||
		!reflect.DeepEqual(b.DependsOn, []string{"a", "ghost"}) {
		t.Errorf("b = %q blocked_by %v depends_on %v", b.State, b.BlockedBy, b.DependsOn)
	}
	if c := findPlan(t, v, "c"); c.State != "planned" || len(c.BlockedBy) != 0 {
		t.Errorf("c (dep archived) = %q %v, want planned", c.State, c.BlockedBy)
	}
	// A tick beats blocked.
	in.OpenSpec.Changes[1].Tasks = osTasks(1, 1)
	if b := findPlan(t, Build(in), "b"); b.State != "in progress" {
		t.Errorf("ticked blocked change = %q, want in progress", b.State)
	}
}

func TestOpenSpecNeedsYouFromCoordinator(t *testing.T) {
	in := osInputs(osChange("c", osTasks(3, 1)))
	in.Coordinators = []Coordinator{{Change: "c", Address: "w:c", State: "working", NeedsYou: 1}}
	in.Agents = []Agent{
		{Address: "w:t2", Status: "blocked", Role: "implementer", Topic: "c", TaskID: "2"},
		{Address: "w:t3", Status: "working", Topic: "c", TaskID: "3"},
		{Address: "w:x", Status: "blocked", Topic: "other", TaskID: "3"},
	}
	p := findPlan(t, Build(in), "c")
	if p.State != "needs_you" {
		t.Errorf("state = %q, want needs_you", p.State)
	}
	if p.Tasks[1].State != "needs_you" || len(p.Tasks[1].Agents) != 1 || p.Tasks[1].Agents[0].Role != "implementer" {
		t.Errorf("task 2 = %+v", p.Tasks[1])
	}
	if p.Tasks[2].State != "working" || len(p.Tasks[2].Agents) != 1 || p.Tasks[2].Agents[0].Address != "w:t3" {
		t.Errorf("task 3 = %+v", p.Tasks[2])
	}
	// Two coordinators: the lowest address counts, whatever the input order.
	in.Coordinators = []Coordinator{{Change: "c", Address: "w:z"}, {Change: "c", Address: "w:b", NeedsYou: 1}}
	if p := findPlan(t, Build(in), "c"); p.Coordinator != "w:b" || p.State != "needs_you" {
		t.Errorf("two coordinators: %q %q, want w:b needs_you", p.Coordinator, p.State)
	}
	// A blocked coordinator terminal also means needs_you.
	in.Coordinators = []Coordinator{{Change: "c", Address: "w:c", State: "blocked"}}
	if p := findPlan(t, Build(in), "c"); p.State != "needs_you" {
		t.Errorf("blocked coordinator: state = %q", p.State)
	}
}

func TestTaskNeedsYouNamedByCoordinator(t *testing.T) {
	in := osInputs(osChange("c", osTasks(3, 1)))
	in.Coordinators = []Coordinator{{Change: "c", Address: "w:c", State: "working", NeedsYou: 1, NeedsTasks: []string{"3"}}}
	in.Agents = []Agent{{Address: "w:t3", Status: "working", Topic: "c", TaskID: "3"}}
	p := findPlan(t, Build(in), "c")
	if p.State != "needs_you" || p.Tasks[2].State != "needs_you" || p.Tasks[1].State != "todo" {
		t.Errorf("change %q tasks %q %q, want needs_you, task 3 needs_you, task 2 todo", p.State, p.Tasks[1].State, p.Tasks[2].State)
	}
	// Only the coordinator of this change counts.
	in.Coordinators = []Coordinator{{Change: "other", Address: "w:o", NeedsYou: 1, NeedsTasks: []string{"3"}}}
	if p := findPlan(t, Build(in), "c"); p.Tasks[2].State != "working" {
		t.Errorf("other change's coordinator: task 3 %q, want working", p.Tasks[2].State)
	}
}

// Behind sudo -i a remote terminal's hc cwd is /root or /home/<user>: the
// striker.repo label still ties it to the repo on the same machine.
func TestGatherAgentsByRepoLabel(t *testing.T) {
	h := fakeHC{}
	h["ls --json"] = `{"terminals":[{"address":"remote1/t7"},{"address":"remote1/t8"},{"address":"t9"},{"address":"remote1/t10"}]}`
	h["get remote1/t7 --json"] = `{"address":"remote1/t7","cwd":"/root","labels":{"striker.coordinator":"rb","striker.repo":"alpha"},"status":{"state":"working"}}`
	h["get remote1/t8 --json"] = `{"address":"remote1/t8","cwd":"/root","labels":{"striker.coordinator":"x","striker.repo":"beta"}}`
	h["get t9 --json"] = `{"address":"t9","cwd":"/Users/p","labels":{"striker.coordinator":"rb","striker.repo":"alpha"}}`
	h["get remote1/t10 --json"] = `{"address":"remote1/t10","cwd":"/home/u","labels":{"striker.task":"2","striker.topic":"rb","striker.repo":"alpha"}}`
	got, err := gatherAgents(hc.Client{R: h}, "remote1", []string{"/srv/alpha"}, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	var addrs []string
	for _, a := range got {
		addrs = append(addrs, a.Address)
	}
	if !reflect.DeepEqual(addrs, []string{"remote1/t7", "remote1/t10"}) || got[0].Repo != "alpha" {
		t.Fatalf("agents %+v", got)
	}
	// Without a repo name only the cwd counts.
	if got, _ := gatherAgents(hc.Client{R: h}, "remote1", []string{"/srv/alpha"}, ""); len(got) != 0 {
		t.Fatalf("no name: %+v", got)
	}
}

func TestInterruptedWhenNotLive(t *testing.T) {
	in := osInputs(osChange("c", osTasks(4, 2)))
	p := findPlan(t, Build(in), "c")
	if p.InterruptedAt != "3" {
		t.Errorf("interrupted_at = %q, want 3", p.InterruptedAt)
	}
	want := []string{"done", "done", "interrupted", "todo"}
	for i, w := range want {
		if got := p.Tasks[i].State; got != w {
			t.Errorf("task %d = %q, want %q", i+1, got, w)
		}
	}
	// Live through any of its task agents (even idle), its coordinator: not interrupted.
	live := []Inputs{osInputs(osChange("c", osTasks(4, 2))), osInputs(osChange("c", osTasks(4, 2)))}
	live[0].Agents = []Agent{{Address: "w:t", Status: "done", Topic: "c", TaskID: "4"}}
	live[1].Coordinators = []Coordinator{{Change: "c", Address: "w:c", State: "idle"}}
	for i, l := range live {
		if p := findPlan(t, Build(l), "c"); p.InterruptedAt != "" || p.Tasks[2].State != "todo" {
			t.Errorf("live %d: interrupted_at %q task 3 %q", i, p.InterruptedAt, p.Tasks[2].State)
		}
	}
	// Done only through a closed issue (no tick in tasks.md): not interrupted.
	cl := osInputs(osChange("c", []openspec.Task{{ID: "1", Issue: 9}, {ID: "2", After: []string{"1"}}}))
	cl.Closed = map[int]bool{9: true}
	if p := findPlan(t, Build(cl), "c"); p.InterruptedAt != "" || p.Tasks[0].State != "done" || p.Tasks[1].State != "todo" {
		t.Errorf("closed issue only: interrupted_at %q tasks %q %q", p.InterruptedAt, p.Tasks[0].State, p.Tasks[1].State)
	}
	// Nothing ticked, or everything ticked: no interruption.
	for _, done := range []int{0, 4} {
		if p := findPlan(t, Build(osInputs(osChange("c", osTasks(4, done)))), "c"); p.InterruptedAt != "" {
			t.Errorf("%d/4 ticked: interrupted_at %q", done, p.InterruptedAt)
		}
	}
}

func TestNotInterruptedWhenLeadWorks(t *testing.T) {
	in := osInputs(osChange("repo-overview", osTasks(4, 2)))
	in.LeadState, in.LeadTask = "working", "Task 3 of repo-overview: repoview"
	if p := findPlan(t, Build(in), "repo-overview"); p.InterruptedAt != "" || p.Tasks[2].State != "todo" {
		t.Errorf("lead works on it: interrupted_at %q task 3 %q", p.InterruptedAt, p.Tasks[2].State)
	}
	// Lead idle, or working on another change (also one whose name merely
	// contains this one's): interrupted.
	for _, l := range [][2]string{{"idle", "repo-overview"}, {"working", "other"}, {"working", "repo-overview-2"}} {
		in.LeadState, in.LeadTask = l[0], l[1]
		if p := findPlan(t, Build(in), "repo-overview"); p.InterruptedAt != "3" {
			t.Errorf("lead %v: interrupted_at %q, want 3", l, p.InterruptedAt)
		}
	}
}

// Live says whether anyone works on the change: also when every task is ticked
// (no interruption to tell it apart) or nothing is ticked yet.
func TestLive(t *testing.T) {
	for _, done := range []int{0, 2, 4} {
		if p := findPlan(t, Build(osInputs(osChange("c", osTasks(4, done)))), "c"); p.Live {
			t.Errorf("%d/4 nobody: live", done)
		}
		lead := osInputs(osChange("c", osTasks(4, done)))
		lead.LeadState, lead.LeadTask = "working", "archive c"
		agent := osInputs(osChange("c", osTasks(4, done)))
		agent.Agents = []Agent{{Address: "w:t", Status: "working", Topic: "c", TaskID: "4"}}
		coord := osInputs(osChange("c", osTasks(4, done)))
		coord.Coordinators = []Coordinator{{Change: "c", Address: "w:c", State: "idle"}}
		for i, in := range []Inputs{lead, agent, coord} {
			if p := findPlan(t, Build(in), "c"); !p.Live {
				t.Errorf("%d/4 case %d: not live", done, i)
			}
		}
	}
}

func TestLegacyRepoUnchanged(t *testing.T) {
	in := Inputs{
		Default:  "main",
		Branches: []Branch{{Name: "striker-go", Worktree: "/w", Commits: []gitrepo.Commit{{SHA: "b1", Subject: "wip", Refs: []int{7}}}}},
		Specs:    []plans.Spec{spec("striker-go-bundle", "striker-go", "nope")},
		Plans:    []plans.Plan{{Topic: "striker-go-bundle", Tasks: []plans.Task{todoTask(1, 7)}}},
		Agents:   []Agent{{Address: "w1:c", Status: "idle", Coordinator: "striker-go-bundle"}},
		// Liveness inputs must not touch the legacy path.
		LeadState: "working", LeadTask: "striker-go-bundle",
		Now: time.Unix(0, 0),
	}
	wt, seven := "/w", 7
	want := PlanView{
		Topic: "striker-go-bundle", Title: "STRIKER-GO-BUNDLE", Branch: "striker-go",
		DependsOn: []string{"nope"}, Worktree: &wt, State: "active", BlockedBy: []string{"nope?"},
		Coordinator: "w1:c",
		Tasks: []TaskView{{Issue: &seven, N: 1, Title: "t", StepsTotal: 2, DependsOn: []int{},
			State: "todo", Agents: []AgentView{}}},
		Commits: []gitrepo.Commit{{SHA: "b1", Subject: "wip", Refs: []int{7}}},
		Toc:     []string{}, Legacy: true,
	}
	v := Build(in)
	if len(v.Plans) != 1 || !reflect.DeepEqual(v.Plans[0], want) {
		t.Errorf("legacy plan =\n%+v\nwant\n%+v", v.Plans, want)
	}
	b, _ := json.Marshal(v)
	for _, k := range []string{`"legacy":true`, `"toc":[]`, `"interrupted_at":""`, `"archived":""`, `"id":""`} {
		if !strings.Contains(string(b), k) {
			t.Errorf("missing %s in %s", k, b)
		}
	}
}

func TestOpenSpecJSONArraysNotNull(t *testing.T) {
	in := osInputs(openspec.Change{Name: "bare", Branch: "bare"})
	b, err := json.Marshal(Build(in))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "null") && !strings.Contains(s, `"worktree":null`) {
		t.Errorf("unexpected null in %s", s)
	}
	for _, k := range []string{`"tasks":[]`, `"depends_on":[]`, `"blocked_by":[]`, `"commits":[]`, `"toc":[]`, `"legacy":false`} {
		if !strings.Contains(s, k) {
			t.Errorf("missing %s in %s", k, s)
		}
	}
}

func TestOpenSpecWarningsInView(t *testing.T) {
	c := osChange("c", osTasks(1, 0))
	c.Warnings = []string{"openspec/changes/c/tasks.md: line 3: bad issue"}
	in := osInputs(c)
	in.OpenSpec.Warnings = []string{"store warning"}
	in.Warnings = []string{"gather warning"}
	v := Build(in)
	want := []string{"gather warning", "store warning", "openspec/changes/c/tasks.md: line 3: bad issue"}
	if !reflect.DeepEqual(v.Warnings, want) {
		t.Errorf("warnings = %v, want %v", v.Warnings, want)
	}
}

func TestGatherOpenSpec(t *testing.T) {
	g := fakeGit{}
	g["symbolic-ref --short refs/remotes/origin/HEAD"] = "origin/main\n"
	g["log --format=%H%x1f%P%x1f%s%x1f%b%x1e -n 30 main"] = "c1\x1f\x1finit\x1f\x1e"
	g["worktree list --porcelain"] = "worktree /r\nbranch refs/heads/main\n\nworktree /w\nbranch refs/heads/feat\n"
	g["for-each-ref --format=%(refname:short) refs/heads refs/remotes/origin"] = "main\nfeat\norigin/main\norigin/pushed\n"
	cd := "openspec/changes/feat/"
	g["ls-tree -r --name-only main openspec/"] = cd + ".openspec.yaml\n" + cd + "tasks.md\n"
	g["show main:"+cd+".openspec.yaml"] = "schema: spec-driven\n"
	g["show main:"+cd+"tasks.md"] = "- [ ] 1 a\n- [ ] 2 b\n"
	g["ls-tree -r --name-only feat openspec/"] = cd + ".openspec.yaml\n" + cd + "tasks.md\n"
	g["show feat:"+cd+".openspec.yaml"] = "schema: spec-driven\n"
	g["show feat:"+cd+"tasks.md"] = "- [x] 1 a\n- [ ] 2 b\n"
	// origin/pushed: the fake has no ls-tree answer, so the read fails:
	// skipped with a warning.
	g["rev-list --merges --ancestry-path feat..main"] = ""
	h := fakeHC{}
	h["ls --json"] = `{"terminals":[{"address":"w:c"},{"address":"w:t"},{"address":"w:old"}]}`
	h["get w:c --json"] = `{"address":"w:c","cwd":"/w","labels":{"striker.coordinator":"feat"},"status":{"state":"working"}}`
	h["get w:t --json"] = `{"address":"w:t","cwd":"/w","labels":{"striker.task":"2","striker.topic":"feat","striker.role":"implementer"},"status":{"state":"blocked"}}`
	h["get w:old --json"] = `{"address":"w:old","cwd":"/r","labels":{"striker.task":"#5"},"status":{"state":"idle"}}`
	in, err := Gather(gitrepo.Repo{R: g, Dir: "/r"}, hc.Client{R: h}, &fakeGH{}, time.Unix(1, 0), Extra{})
	if err != nil {
		t.Fatal(err)
	}
	if in.OpenSpec == nil || len(in.OpenSpec.Changes) != 1 || !in.OpenSpec.Changes[0].Tasks[0].Done {
		t.Fatalf("openspec = %+v, want feat with the branch's tick", in.OpenSpec)
	}
	if len(in.Warnings) != 1 || !strings.HasPrefix(in.Warnings[0], "openspec on origin/pushed unreadable: ") {
		t.Errorf("warnings = %v, want the origin/pushed read warning", in.Warnings)
	}
	wantC := []Coordinator{{Change: "feat", Address: "w:c", State: "working"}}
	if !reflect.DeepEqual(in.Coordinators, wantC) {
		t.Errorf("coordinators = %+v", in.Coordinators)
	}
	wantA := []Agent{
		{Address: "w:c", Status: "working", Coordinator: "feat"},
		{Address: "w:t", Status: "blocked", Role: "implementer", Task: 2, Topic: "feat", TaskID: "2"},
		{Address: "w:old", Status: "idle", Task: 5},
	}
	if !reflect.DeepEqual(in.Agents, wantA) {
		t.Errorf("agents = %+v", in.Agents)
	}
	p := findPlan(t, Build(in), "feat")
	if p.State != "in progress" || p.Coordinator != "w:c" || p.Worktree == nil || p.Tasks[1].State != "needs_you" {
		t.Errorf("feat = %+v", p)
	}

	// With a reader: the coordinator's status file fills NeedsYou, NeedsTasks
	// and Status; one without striker.repo gets an error status, unread.
	h["ls --json"] = `{"terminals":[{"address":"w:c"},{"address":"w:n"}]}`
	h["get w:c --json"] = `{"address":"w:c","cwd":"/w","labels":{"striker.coordinator":"feat","striker.repo":"r"},"status":{"state":"working"}}`
	h["get w:n --json"] = `{"address":"w:n","cwd":"/w","labels":{"striker.coordinator":"feat"},"status":{"state":"idle"}}`
	var asked []string
	read := func(machine, repo, change string) json.RawMessage {
		asked = append(asked, machine+"|"+repo+"|"+change)
		return json.RawMessage(`{"needs_you":[{"text":"a","task":"1"},{"text":"b"}]}`)
	}
	in, err = Gather(gitrepo.Repo{R: g, Dir: "/r"}, hc.Client{R: h}, &fakeGH{}, time.Unix(1, 0), Extra{Read: read})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asked, []string{"|r|feat"}) {
		t.Errorf("reader asked %q", asked)
	}
	if len(in.Coordinators) != 2 {
		t.Fatalf("coordinators = %+v", in.Coordinators)
	}
	c := in.Coordinators[0]
	if c.NeedsYou != 2 || !reflect.DeepEqual(c.NeedsTasks, []string{"1"}) || !strings.Contains(string(c.Status), "needs_you") {
		t.Errorf("coordinator = %+v", c)
	}
	if n := in.Coordinators[1]; n.NeedsYou != 0 || !strings.Contains(string(n.Status), `"error"`) {
		t.Errorf("unlabelled coordinator = %+v %s", n, n.Status)
	}
	if p := findPlan(t, Build(in), "feat"); p.State != "needs_you" || p.Tasks[0].State != "needs_you" {
		t.Errorf("feat with status = %q task 1 %q", p.State, p.Tasks[0].State)
	}

	// The lead's task and live state from Extra reach the Inputs.
	in, err = Gather(gitrepo.Repo{R: g, Dir: "/r"}, hc.Client{R: h}, &fakeGH{}, time.Unix(1, 0), Extra{LeadTask: "Task 2 of feat", LeadState: "working"})
	if err != nil {
		t.Fatal(err)
	}
	if in.LeadTask != "Task 2 of feat" || in.LeadState != "working" {
		t.Errorf("lead task %q state %q", in.LeadTask, in.LeadState)
	}
}
