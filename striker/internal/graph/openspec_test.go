package graph

import (
	"testing"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/repoview"
)

// OpenSpec states: an archived change ("done") acts as merged.
func TestOpenSpecArchivedClaimsCommits(t *testing.T) {
	trunk := []gitrepo.Commit{commit("c3", "c2", "b1"), commit("b1", "c1"), commit("c2", "c1"), commit("c1")}
	d := plan("d", "done", nil, task(1, "done"))
	d.Archived = "2026-11-21"
	d.Commits = []gitrepo.Commit{commit("b1", "c1")}
	d.MergePoint = "c3"
	g := Layout(repoview.View{Default: "main", Trunk: trunk, Plans: []repoview.PlanView{d}})
	checkSane(t, g)
	if n := node(t, g, "c:b1"); n.Lane != planLane(t, g, "d") || n.Future || n.Plan != "d" || n.State != "done" {
		t.Errorf("b1 = %+v, want on d's lane with raw state done", n)
	}
	for _, n := range g.Nodes {
		if n.Plan == "d" && n.Kind != "commit" {
			t.Errorf("archived change has a %s node %q", n.Kind, n.ID)
		}
	}
}

// "in progress" (and "needs_you") with a worktree or branch commits acts as active.
func TestOpenSpecInProgressBranchCommits(t *testing.T) {
	for _, state := range []string{"in progress", "needs_you"} {
		a := plan("a", state, []string{"b"}, task(1, "working"))
		a.Commits = []gitrepo.Commit{commit("a2", "a1"), commit("a1", "c2")}
		b := plan("b", "planned", nil, task(1, "todo"))
		g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{a, b}})
		checkSane(t, g)
		l := planLane(t, g, "a")
		a1, a2 := node(t, g, "c:a1"), node(t, g, "c:a2")
		if a1.Lane != l || a2.Lane != l || a1.Future || a2.State != state {
			t.Errorf("%s: branch commits = %+v %+v", state, a1, a2)
		}
		if g.Now != 3 {
			t.Errorf("%s: now = %d, want 3", state, g.Now)
		}
		if l >= planLane(t, g, "b") {
			t.Errorf("%s: a should come first: %+v", state, g.Lanes)
		}
		// Active: no entry edge from b's planned merge.
		if _, ok := hasEdge(g, "m:b", "t:a:1"); ok {
			t.Errorf("%s: active change got a dependency-merge edge", state)
		}
	}
	// Ticked only (no worktree, no commits): stays on the non-active path,
	// so a dependency's planned merge leads into it.
	b := plan("b", "planned", nil, task(1, "todo"))
	p := plan("p", "in progress", []string{"b"}, task(1, "done"), task(2, "todo", 1))
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{b, p}})
	checkSane(t, g)
	if _, ok := hasEdge(g, "m:b", "t:p:2"); !ok {
		t.Errorf("ticked-only change: no edge m:b -> t:p:2 in %+v", g.Edges)
	}
}

// An interrupted task is placed like a todo task: in the future.
func TestOpenSpecInterruptedTaskInFuture(t *testing.T) {
	a := plan("a", "planned", nil, task(1, "todo"))
	c := plan("c", "in progress", []string{"a"}, task(1, "done"), task(2, "interrupted", 1), task(3, "todo", 2))
	c.InterruptedAt = "2"
	g := Layout(repoview.View{Default: "main", Trunk: trunk3(), Plans: []repoview.PlanView{a, c}})
	checkSane(t, g)
	n2 := node(t, g, "t:c:2")
	if !n2.Future || n2.Col <= g.Now || n2.State != "interrupted" {
		t.Errorf("interrupted task = %+v (now %d), want future after now", n2, g.Now)
	}
	if n3 := node(t, g, "t:c:3"); n3.Col <= n2.Col {
		t.Errorf("task 3 col %d not after interrupted task col %d", n3.Col, n2.Col)
	}
	// The dependency's merge leads into the first future task, the interrupted one.
	if am := node(t, g, "m:a"); n2.Col <= am.Col {
		t.Errorf("interrupted task col %d not after dependency merge col %d", n2.Col, am.Col)
	}
	if _, ok := hasEdge(g, "m:a", "t:c:2"); !ok {
		t.Errorf("no edge m:a -> t:c:2 in %+v", g.Edges)
	}
}
