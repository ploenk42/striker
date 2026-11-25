package graph

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/repoview"
)

func commit(sha string, parents ...string) gitrepo.Commit {
	return gitrepo.Commit{SHA: sha, Parents: parents, Subject: "s " + sha}
}

// trunk3 is newest first, as gitrepo.Log returns it.
func trunk3() []gitrepo.Commit {
	return []gitrepo.Commit{commit("c3", "c2"), commit("c2", "c1"), commit("c1")}
}

func task(n int, state string, deps ...int) repoview.TaskView {
	return repoview.TaskView{N: n, Title: "task", State: state, DependsOn: append([]int{}, deps...)}
}

func plan(topic, state string, deps []string, tasks ...repoview.TaskView) repoview.PlanView {
	return repoview.PlanView{Topic: topic, Title: "Plan " + topic, Branch: topic, State: state,
		DependsOn: append([]string{}, deps...), Tasks: tasks}
}

func node(t *testing.T, g Graph, id string) Node {
	t.Helper()
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("no node %q in %+v", id, g.Nodes)
	return Node{}
}

func planLane(t *testing.T, g Graph, topic string) int {
	t.Helper()
	for _, l := range g.Lanes {
		if l.Plan == topic {
			return l.Index
		}
	}
	t.Fatalf("no lane for %q in %+v", topic, g.Lanes)
	return -1
}

// checkSane: unique IDs, no two nodes on one spot, every edge end exists,
// lane indexes match their position.
func checkSane(t *testing.T, g Graph) {
	t.Helper()
	ids := map[string]bool{}
	spots := map[[2]int]string{}
	for _, n := range g.Nodes {
		if ids[n.ID] {
			t.Errorf("duplicate node id %q", n.ID)
		}
		ids[n.ID] = true
		k := [2]int{n.Col, n.Lane}
		if o, ok := spots[k]; ok {
			t.Errorf("nodes %q and %q both at col %d lane %d", o, n.ID, n.Col, n.Lane)
		}
		spots[k] = n.ID
		if n.Col < 0 || n.Col >= g.Cols {
			t.Errorf("node %q col %d outside 0..%d", n.ID, n.Col, g.Cols-1)
		}
		if n.Lane < 0 || n.Lane >= len(g.Lanes) {
			t.Errorf("node %q lane %d outside lanes", n.ID, n.Lane)
		}
		if n.Kind == "commit" && n.Col > g.Now {
			t.Errorf("commit %q at col %d right of now %d", n.ID, n.Col, g.Now)
		}
		if !n.Future && n.Col > g.Now {
			t.Errorf("solid node %q at col %d right of now %d", n.ID, n.Col, g.Now)
		}
	}
	cols := map[string]int{}
	for _, n := range g.Nodes {
		cols[n.ID] = n.Col
	}
	for _, e := range g.Edges {
		if !ids[e.From] || !ids[e.To] {
			t.Errorf("edge %s -> %s has a missing end", e.From, e.To)
			continue
		}
		if cols[e.To] < cols[e.From] {
			t.Errorf("edge %s (col %d) -> %s (col %d) goes backward", e.From, cols[e.From], e.To, cols[e.To])
		}
	}
	for i, l := range g.Lanes {
		if l.Index != i {
			t.Errorf("lane %d has index %d", i, l.Index)
		}
		if l.BlockedBy == nil {
			t.Errorf("lane %d has nil blocked_by", i)
		}
	}
}

func TestTrunkOnLaneZero(t *testing.T) {
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{}})
	checkSane(t, g)
	if g.Now != 2 {
		t.Fatalf("Now = %d, want 2", g.Now)
	}
	for i, sha := range []string{"c1", "c2", "c3"} {
		n := node(t, g, "c:"+sha)
		if n.Lane != 0 || n.Col != i || n.Kind != "commit" || n.Future {
			t.Errorf("%s = %+v, want lane 0 col %d past commit", sha, n, i)
		}
	}
	if len(g.Lanes) != 1 || g.Lanes[0].Plan != "" || g.Lanes[0].Label != "main" {
		t.Errorf("lanes = %+v", g.Lanes)
	}
	if g.Cols != 3 {
		t.Errorf("Cols = %d", g.Cols)
	}
}

func TestDependentPlanStartsAfterMerge(t *testing.T) {
	wt := "/wt/a"
	a := plan("a", "active", nil, task(1, "done"), task(2, "working", 1), task(3, "todo", 2))
	a.Worktree = &wt
	b := plan("b", "blocked", []string{"a"}, task(1, "todo"), task(2, "todo", 1))
	b.BlockedBy = []string{"a"}
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{a, b}})
	checkSane(t, g)
	am := node(t, g, "m:a")
	if am.Kind != "merge" || !am.Future || am.Col <= g.Now {
		t.Fatalf("merge of a = %+v", am)
	}
	first := node(t, g, "t:b:1")
	if first.Col <= am.Col {
		t.Errorf("b starts at col %d, not after a's merge at %d", first.Col, am.Col)
	}
	if planLane(t, g, "a") >= planLane(t, g, "b") {
		t.Errorf("active a should come before blocked b: %+v", g.Lanes)
	}
	// a's done and working tasks are at or before now, its todo task after
	if n := node(t, g, "t:a:1"); n.Col > g.Now {
		t.Errorf("a done task 1 at col %d, now %d", n.Col, g.Now)
	}
	if n := node(t, g, "t:a:2"); n.Col != g.Now {
		t.Errorf("a working task 2 at col %d, now %d", n.Col, g.Now)
	}
	if n := node(t, g, "t:a:3"); n.Col <= g.Now {
		t.Errorf("a todo task 3 at col %d, now %d", n.Col, g.Now)
	}
}

func hasEdge(g Graph, from, to string) (Edge, bool) {
	for _, e := range g.Edges {
		if e.From == from && e.To == to {
			return e, true
		}
	}
	return Edge{}, false
}

func TestActivePlanShowsUnmergedDependency(t *testing.T) {
	wt := "/wt/a"
	a := plan("a", "active", []string{"b"}, task(1, "done"), task(2, "todo", 1), task(3, "todo", 2))
	a.Worktree = &wt
	a.BlockedBy = []string{"b"}
	b := plan("b", "planned", nil, task(1, "todo"))
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{a, b}})
	checkSane(t, g)
	l := g.Lanes[planLane(t, g, "a")]
	if !reflect.DeepEqual(l.BlockedBy, []string{"b"}) {
		t.Errorf("lane a blocked_by = %v, want [b]", l.BlockedBy)
	}
	if bl := g.Lanes[planLane(t, g, "b")]; len(bl.BlockedBy) != 0 {
		t.Errorf("lane b blocked_by = %v", bl.BlockedBy)
	}
	for _, e := range g.Edges {
		if e.From == "m:b" && strings.HasPrefix(e.To, "t:a:") {
			t.Errorf("edge %s -> %s from b's merge into a", e.From, e.To)
		}
	}
}

func TestBlockedPlanEntryOnlyIntoLaterNode(t *testing.T) {
	a := plan("a", "planned", nil, task(1, "todo"))
	// b has a closed task (solid, at or before now) and a todo task.
	b := plan("b", "blocked", []string{"a"}, task(1, "done"), task(2, "todo", 1))
	b.BlockedBy = []string{"a"}
	c := plan("c", "blocked", []string{"a"}, task(1, "done"))
	c.BlockedBy = []string{"a"}
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{a, b, c}})
	checkSane(t, g)
	if _, ok := hasEdge(g, "m:a", "t:b:2"); !ok {
		t.Errorf("no edge from a's merge to b's first todo task: %+v", g.Edges)
	}
	if _, ok := hasEdge(g, "m:a", "t:b:1"); ok {
		t.Errorf("edge from a's merge into b's done task")
	}
	if _, ok := hasEdge(g, "m:a", "m:c"); !ok {
		t.Errorf("no edge from a's merge to c's merge (c has no todo task): %+v", g.Edges)
	}
	if bl := g.Lanes[planLane(t, g, "b")]; !reflect.DeepEqual(bl.BlockedBy, []string{"a"}) {
		t.Errorf("lane b blocked_by = %v", bl.BlockedBy)
	}
}

// A change with branch commits: the commits are its past, its done tasks get no nodes, and the first open task
// starts from the newest branch commit.
func TestBranchCommitsReplaceDoneTasks(t *testing.T) {
	wt := "/wt/a"
	a := plan("a", "active", nil, task(1, "done"), task(2, "done", 1), task(3, "todo", 2), task(4, "todo", 3))
	a.Worktree = &wt
	a.Commits = []gitrepo.Commit{commit("a2", "a1"), commit("a1", "c2")}
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{a}})
	checkSane(t, g)
	for _, n := range g.Nodes {
		if n.ID == "t:a:1" || n.ID == "t:a:2" {
			t.Errorf("done task has a node: %+v", n)
		}
	}
	lanes := 0
	for _, l := range g.Lanes {
		if l.Plan == "a" {
			lanes++
		}
	}
	if lanes != 1 {
		t.Errorf("change a has %d lanes, want 1: %+v", lanes, g.Lanes)
	}
	a2, t3 := node(t, g, "c:a2"), node(t, g, "t:a:3")
	if _, ok := hasEdge(g, "c:a2", "t:a:3"); !ok {
		t.Errorf("no edge from the branch head into the first open task: %+v", g.Edges)
	}
	if t3.Lane != a2.Lane || t3.Col != g.Now+1 {
		t.Errorf("first open task %+v, branch head %+v, now %d", t3, a2, g.Now)
	}
}

// Done tasks of a change with branch commits do not push Now to the right.
func TestDoneTasksBesideCommitsKeepNow(t *testing.T) {
	wt := "/wt/a"
	a := plan("a", "active", nil, task(1, "done"), task(2, "done"), task(3, "done"), task(4, "done"), task(5, "done"), task(6, "todo"))
	a.Worktree = &wt
	a.Commits = []gitrepo.Commit{commit("a1", "c2")}
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{a}})
	checkSane(t, g)
	if g.Now != 2 {
		t.Errorf("now = %d, want 2 (trunk head and branch head)", g.Now)
	}
}

// Without branch commits the done tasks stay, on the change's own lane.
func TestDoneTasksWithoutCommitsOnPlanLane(t *testing.T) {
	p := plan("p", "planned", nil, task(1, "done"), task(2, "done", 1), task(3, "todo", 2))
	g := Layout(repoview.View{Plans: []repoview.PlanView{p}})
	checkSane(t, g)
	pl := planLane(t, g, "p")
	d1, d2 := node(t, g, "t:p:1"), node(t, g, "t:p:2")
	if d1.Lane != pl || d2.Lane != pl || d2.Col <= d1.Col || d2.Col > g.Now {
		t.Errorf("done tasks %+v %+v, plan lane %d, now %d", d1, d2, pl, g.Now)
	}
	if n := node(t, g, "t:p:3"); n.Col <= g.Now {
		t.Errorf("todo task %+v, now %d", n, g.Now)
	}
}

func TestNoEdgeToForkOutsideWindow(t *testing.T) {
	wt := "/wt/a"
	a := plan("a", "active", nil, task(1, "todo"))
	a.Worktree = &wt
	a.Commits = []gitrepo.Commit{commit("a1", "far")}
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{a}})
	checkSane(t, g)
	for _, e := range g.Edges {
		if e.To == "c:a1" {
			t.Errorf("edge into a1 whose fork is outside the window: %+v", e)
		}
	}
}

func TestParallelTasksOwnLane(t *testing.T) {
	p := plan("p", "planned", nil, task(1, "todo"), task(2, "todo", 1), task(3, "todo", 2), task(4, "todo", 2), task(5, "todo", 3, 4))
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{p}})
	checkSane(t, g)
	n3, n4 := node(t, g, "t:p:3"), node(t, g, "t:p:4")
	if n3.Lane == n4.Lane {
		t.Fatalf("tasks 3 and 4 share lane %d", n3.Lane)
	}
	if n3.Col != n4.Col {
		t.Errorf("tasks 3 and 4 at cols %d and %d", n3.Col, n4.Col)
	}
	pl := planLane(t, g, "p")
	if n3.Lane != pl || n4.Lane != pl+1 {
		t.Errorf("lanes 3=%d 4=%d, plan lane %d", n3.Lane, n4.Lane, pl)
	}
	if g.Lanes[pl+1].Plan != "p" {
		t.Errorf("extra lane = %+v", g.Lanes[pl+1])
	}
	if n5 := node(t, g, "t:p:5"); n5.Lane != pl || n5.Col != n3.Col+1 {
		t.Errorf("task 5 = %+v", n5)
	}
	if n2 := node(t, g, "t:p:2"); n2.Lane != pl {
		t.Errorf("task 2 off the main strand: %+v", n2)
	}
}

func TestFutureFlags(t *testing.T) {
	wt := "/wt/p"
	p := plan("p", "active", nil, task(1, "done"), task(2, "needs_you", 1), task(3, "todo", 2))
	p.Worktree = &wt
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{p}})
	checkSane(t, g)
	if n := node(t, g, "t:p:1"); n.Future || n.State != "done" || n.Kind != "task" {
		t.Errorf("done task = %+v", n)
	}
	if n := node(t, g, "t:p:2"); n.Future {
		t.Errorf("needs_you task = %+v", n)
	}
	if n := node(t, g, "t:p:3"); !n.Future || n.State != "todo" {
		t.Errorf("todo task = %+v", n)
	}
	for _, e := range g.Edges {
		if e.From == "t:p:1" && e.To == "t:p:2" && e.Future {
			t.Errorf("edge between past tasks is future")
		}
		if e.From == "t:p:2" && e.To == "t:p:3" && !e.Future {
			t.Errorf("edge into a todo task is not future")
		}
	}
}

func TestCycleTerminates(t *testing.T) {
	d := plan("d", "blocked", []string{"e"}, task(1, "todo"))
	e := plan("e", "blocked", []string{"d"}, task(1, "todo"))
	done := make(chan Graph, 1)
	go func() {
		done <- Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{d, e}})
	}()
	select {
	case g := <-done:
		checkSane(t, g)
		planLane(t, g, "d")
		planLane(t, g, "e")
	case <-time.After(5 * time.Second):
		t.Fatal("Layout did not return on a dependency cycle")
	}
}

func TestMergedPlanLaneOnlyInWindow(t *testing.T) {
	// m's branch commit b1 is in the trunk window (off the first-parent line).
	trunk := []gitrepo.Commit{commit("c3", "c2", "b1"), commit("b1", "c1"), commit("c2", "c1"), commit("c1")}
	m := plan("m", "merged", nil, task(1, "done"))
	m.Commits = []gitrepo.Commit{commit("b1", "c1")}
	m.MergePoint = "c3"
	old := plan("old", "merged", nil, task(1, "done"))
	old.Commits = []gitrepo.Commit{commit("gone", "x")}
	g := Layout(repoview.View{Default: "main", Trunk: trunk, Plans: []repoview.PlanView{m, old}})
	checkSane(t, g)
	ml := planLane(t, g, "m")
	if n := node(t, g, "c:b1"); n.Lane != ml || n.Future || n.Plan != "m" {
		t.Errorf("b1 = %+v, want lane %d", n, ml)
	}
	for _, sha := range []string{"c1", "c2", "c3"} {
		if n := node(t, g, "c:"+sha); n.Lane != 0 {
			t.Errorf("%s left the trunk: %+v", sha, n)
		}
	}
	for _, l := range g.Lanes {
		if l.Plan == "old" {
			t.Errorf("merged plan outside the window got a lane")
		}
	}
	for _, n := range g.Nodes {
		if n.Plan == "m" && n.Kind != "commit" {
			t.Errorf("merged plan has a %s node %q", n.Kind, n.ID)
		}
	}
}

func TestActiveBranchCommitsBeforeTasks(t *testing.T) {
	wt := "/wt/a"
	a := plan("a", "active", nil, task(1, "working"))
	a.Worktree = &wt
	a.Commits = []gitrepo.Commit{commit("a2", "a1"), commit("a1", "c2")}
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{a}})
	checkSane(t, g)
	l := planLane(t, g, "a")
	a1, a2, t1 := node(t, g, "c:a1"), node(t, g, "c:a2"), node(t, g, "t:a:1")
	if a1.Lane != l || a2.Lane != l || a1.Future || a2.Future {
		t.Fatalf("branch commits = %+v %+v", a1, a2)
	}
	if !(1 < a1.Col && a1.Col < a2.Col && a2.Col <= g.Now) {
		t.Errorf("cols: fork c2=1, a1=%d a2=%d now=%d", a1.Col, a2.Col, g.Now)
	}
	if g.Now != 3 {
		t.Errorf("now = %d, want 3 (last branch commit)", g.Now)
	}
	if t1.Col != g.Now || t1.Lane == a2.Lane {
		t.Errorf("working task %+v should sit at now beside a2 %+v", t1, a2)
	}
	found := false
	for _, e := range g.Edges {
		if e.From == "c:c2" && e.To == "c:a1" {
			found = true
		}
	}
	if !found {
		t.Errorf("no fork edge c2 -> a1 in %+v", g.Edges)
	}
}

func TestDeterministicAndNonNil(t *testing.T) {
	v := repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{
		plan("x", "planned", nil, task(1, "todo"), task(2, "todo")),
		plan("y", "blocked", []string{"x", "unknown"}, task(1, "todo")),
		plan("z", "planned", nil),
	}}
	g1, g2 := Layout(v), Layout(v)
	checkSane(t, g1)
	if !reflect.DeepEqual(g1, g2) {
		t.Errorf("Layout is not deterministic")
	}
	// two dep-less tasks are parallel
	if node(t, g1, "t:x:1").Lane == node(t, g1, "t:x:2").Lane {
		t.Errorf("dep-less tasks share a lane")
	}
	// a plan without tasks still has a lane and a merge node
	planLane(t, g1, "z")
	node(t, g1, "m:z")

	b, err := json.Marshal(Layout(repoview.View{}))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, k := range []string{`"lanes":[`, `"nodes":[]`, `"edges":[]`, `"cols":`, `"now":`} {
		if !strings.Contains(s, k) {
			t.Errorf("empty layout %s lacks %s", s, k)
		}
	}
}
