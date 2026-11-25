// Package graph lays out the REPO tree of the LCARS Bridge: trunk commits,
// plan branches with their task nodes and planned merges, on a grid of
// columns (time, left to right) and lanes (rows, 0 = trunk).
package graph

import (
	"sort"
	"strconv"
	"strings"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/repoview"
)

// Graph is the laid-out tree. Columns 0..Now are the past (trunk commits),
// columns after Now the planned future.
type Graph struct {
	Cols  int    `json:"cols"`
	Now   int    `json:"now"`
	Lanes []Lane `json:"lanes"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Lane is one row. Lane 0 is the trunk (Plan ""); a plan's lane is followed
// directly by its extra lanes for parallel tasks (same Plan, Label "").
// BlockedBy (on a plan's first lane only) lists its unmerged dependencies,
// drawn as a marker at the lane start.
type Lane struct {
	Index     int      `json:"index"`
	Plan      string   `json:"plan"`
	Label     string   `json:"label"`
	BlockedBy []string `json:"blocked_by"`
}

// Node is a commit, a task or a (planned) merge.
type Node struct {
	ID     string `json:"id"`
	Col    int    `json:"col"`
	Lane   int    `json:"lane"`
	Kind   string `json:"kind"`
	Plan   string `json:"plan"`
	Issue  *int   `json:"issue"`
	State  string `json:"state"`
	Future bool   `json:"future"`
	Label  string `json:"label"`
}

// Edge joins two nodes; Future edges are drawn dashed.
type Edge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Future bool   `json:"future"`
}

// group is a plan's block of lanes; trunk is group -1.
type group struct {
	plan, label string
	subs        int
	blockedBy   []string
}

type builder struct {
	nodes  []Node
	groups []group
	nodeG  []int // group per node; Node.Lane holds the sub-lane until done
	ids    map[string]int
	seen   map[[2]string]bool
	edges  []Edge
	trunk  map[int]bool // columns used on lane 0
}

func (b *builder) add(n Node, g int) bool {
	if _, ok := b.ids[n.ID]; ok {
		return false
	}
	b.ids[n.ID] = len(b.nodes)
	b.nodes = append(b.nodes, n)
	b.nodeG = append(b.nodeG, g)
	if g < 0 {
		b.trunk[n.Col] = true
	}
	return true
}

func (b *builder) edge(from, to string) {
	i, ok1 := b.ids[from]
	j, ok2 := b.ids[to]
	k := [2]string{from, to}
	// Never draw an edge backward in time; equal columns (vertical) are fine.
	if !ok1 || !ok2 || from == to || b.seen[k] || b.nodes[j].Col < b.nodes[i].Col {
		return
	}
	b.seen[k] = true
	b.edges = append(b.edges, Edge{From: from, To: to, Future: b.nodes[i].Future || b.nodes[j].Future})
}

func (b *builder) newGroup(plan, label string, blockedBy []string) int {
	b.groups = append(b.groups, group{plan: plan, label: label, subs: 1, blockedBy: append([]string{}, blockedBy...)})
	return len(b.groups) - 1
}

func issueOf(c gitrepo.Commit) *int {
	var n int
	switch {
	case len(c.Fixes) > 0:
		n = c.Fixes[0]
	case len(c.Refs) > 0:
		n = c.Refs[0]
	default:
		return nil
	}
	return &n
}

func commitID(sha string) string { return "c:" + sha }

// merged: a legacy merged plan or an archived OpenSpec change ("done").
func merged(p *repoview.PlanView) bool { return p.State == "merged" || p.State == "done" }

// active: a legacy active plan, or an OpenSpec change in progress (or
// needing you) that has a worktree or branch commits. Planned and blocked
// changes stay on the non-active path.
func active(p *repoview.PlanView) bool {
	if p.State == "active" {
		return true
	}
	return (p.State == "in progress" || p.State == "needs_you") && (p.Worktree != nil || len(p.Commits) > 0)
}

// future: a task still to do; interrupted tasks are placed like todo ones.
func future(state string) bool { return state == "todo" || state == "interrupted" }

// Layout places the repo view on the grid. It is pure and deterministic.
func Layout(v repoview.View) Graph {
	b := &builder{ids: map[string]int{}, seen: map[[2]string]bool{}, trunk: map[int]bool{}}

	// Trunk: v.Trunk is newest first; oldest goes to column 0.
	nt := len(v.Trunk)
	now := nt - 1
	trunkCol := map[string]int{}
	byID := map[string]gitrepo.Commit{}
	for i := 0; i < nt; i++ {
		c := v.Trunk[nt-1-i]
		trunkCol[c.SHA] = i
		byID[c.SHA] = c
	}
	head := ""
	chain := map[string]bool{} // first-parent line of the window
	if nt > 0 {
		head = commitID(v.Trunk[0].SHA)
		for sha := v.Trunk[0].SHA; !chain[sha]; {
			c, ok := byID[sha]
			if !ok {
				break
			}
			chain[sha] = true
			if len(c.Parents) == 0 {
				break
			}
			sha = c.Parents[0]
		}
	}

	plans := append([]repoview.PlanView(nil), v.Plans...)
	sort.SliceStable(plans, func(i, j int) bool { return plans[i].Topic < plans[j].Topic })
	byTopic := map[string]*repoview.PlanView{}
	for i := range plans {
		byTopic[plans[i].Topic] = &plans[i]
	}

	// Merged plans claim their branch commits that are in the window and off
	// the first-parent line: listed commits, and the side of their merge.
	owner := map[string]string{}
	claim := func(sha, topic string) bool {
		if _, in := trunkCol[sha]; !in || chain[sha] || owner[sha] != "" {
			return false
		}
		owner[sha] = topic
		return true
	}
	mergedG := map[string]int{}
	for i := range plans {
		p := &plans[i]
		if !merged(p) {
			continue
		}
		got := false
		for _, c := range p.Commits {
			got = claim(c.SHA, p.Topic) || got
		}
		if mc, ok := byID[p.MergePoint]; ok && len(mc.Parents) > 1 {
			for _, sha := range mc.Parents[1:] {
				for claim(sha, p.Topic) {
					got = true
					ps := byID[sha].Parents
					if len(ps) == 0 {
						break
					}
					sha = ps[0]
				}
			}
		}
		if got {
			mergedG[p.Topic] = b.newGroup(p.Topic, p.Title, nil)
		}
	}
	for i := 0; i < nt; i++ {
		c := v.Trunk[nt-1-i]
		n := Node{ID: commitID(c.SHA), Col: i, Kind: "commit", Issue: issueOf(c), Label: c.Subject}
		g := -1
		if t := owner[c.SHA]; t != "" {
			n.Plan, n.State, g = t, byTopic[t].State, mergedG[t]
		}
		b.add(n, g)
	}
	for _, c := range v.Trunk {
		for _, p := range c.Parents {
			b.edge(commitID(p), commitID(c.SHA))
		}
	}

	// Plan order: active plans, then the others in dependency order. Edges
	// into a plan that is being visited (a cycle) are ignored.
	var order []string
	mark := map[string]int{} // 1 visiting, 2 placed
	for i := range plans {
		if p := &plans[i]; active(p) {
			order = append(order, p.Topic)
			mark[p.Topic] = 2
		}
	}
	var visit func(t string)
	visit = func(t string) {
		if mark[t] != 0 {
			return
		}
		mark[t] = 1
		for _, d := range byTopic[t].DependsOn {
			if dp, ok := byTopic[d]; ok && !merged(dp) {
				visit(d)
			}
		}
		mark[t] = 2
		order = append(order, t)
	}
	for i := range plans {
		if !merged(&plans[i]) {
			visit(plans[i].Topic)
		}
	}

	// Pass 1: branch commits of active plans. They sit after their fork (if
	// it is in the window), right-aligned to the trunk head, and together
	// with the trunk head they fix Now.
	headCol := nt - 1
	groupOf := map[string]int{}
	entriesOf := map[string][]string{}
	branchCols := map[string][]int{}
	for _, t := range order {
		p := byTopic[t]
		if !active(p) {
			continue
		}
		g := b.newGroup(p.Topic, p.Title, p.BlockedBy)
		groupOf[t] = g
		cs := p.Commits
		k := len(cs)
		start := headCol + 1 - k
		if k > 0 {
			oldest := cs[k-1]
			if len(oldest.Parents) > 0 {
				if fc, ok := trunkCol[oldest.Parents[0]]; ok && fc+1 > start {
					start = fc + 1
				}
			}
		}
		if start < 0 {
			start = 0
		}
		for i := k - 1; i >= 0; i-- {
			c := cs[i]
			col := start + (k - 1 - i)
			if b.add(Node{ID: commitID(c.SHA), Col: col, Kind: "commit", Plan: t, Issue: issueOf(c), State: p.State, Label: c.Subject}, g) {
				branchCols[t] = append(branchCols[t], col)
			}
			if col > now {
				now = col
			}
			// A fork outside the window gets no edge.
			for _, par := range c.Parents {
				b.edge(commitID(par), commitID(c.SHA))
			}
		}
		switch {
		case k > 0:
			entriesOf[t] = []string{commitID(cs[0].SHA)}
		case head != "":
			entriesOf[t] = []string{head}
		}
	}
	// Done tasks end at Now: make room for the longest run of them. A change
	// with branch commits shows no done tasks (its commits are its past).
	for i := range plans {
		p := &plans[i]
		if merged(p) || len(branchCols[p.Topic]) > 0 {
			continue
		}
		done := 0
		for _, tv := range p.Tasks {
			if tv.State == "done" {
				done++
			}
		}
		if done-1 > now {
			now = done - 1
		}
	}

	mergeCol := map[string]int{}
	var merges []string
	for _, t := range order {
		p := byTopic[t]
		g, ok := groupOf[t]
		if !ok {
			g = b.newGroup(p.Topic, p.Title, p.BlockedBy)
		}
		entries := entriesOf[t] // into dep-less tasks
		var depMerges []string  // merges of placed dependencies
		base := now             // todo tasks start after base
		if !active(p) {
			for _, d := range p.DependsOn {
				if mc, ok := mergeCol[d]; ok {
					if mc > base {
						base = mc
					}
					depMerges = append(depMerges, "m:"+d)
				}
			}
			if len(depMerges) == 0 && head != "" {
				entries = []string{head}
			}
		}
		tails := append(append([]string{}, entries...), depMerges...)
		last := base

		// Tasks. With branch commits the commits are the change's past and
		// its done tasks get no nodes (a later task that depended on them
		// starts from the branch head); without, done tasks fill the columns
		// ending at Now on the change's first lane. Working and needs_you
		// tasks sit at Now; todo tasks start after base, one column after
		// their latest earlier dependency. A todo task whose predecessors an
		// earlier task shares takes the first free sub-lane in its column;
		// otherwise it continues its latest predecessor's strand.
		col, sub := map[int]int{}, map[int]int{}
		occ := map[int]map[int]bool{}
		for _, bc := range branchCols[t] {
			occ[bc] = map[int]bool{0: true}
		}
		hideDone := len(branchCols[t]) > 0
		nDone := 0
		for _, tv := range p.Tasks {
			if tv.State == "done" {
				nDone++
			}
		}
		di := 0
		shared := map[string]bool{}
		hasNext := map[int]bool{}
		id := func(n int) string { return "t:" + t + ":" + strconv.Itoa(n) }
		for _, tv := range p.Tasks {
			if hideDone && tv.State == "done" {
				continue
			}
			c, pref := base+1, 0
			var deps []int
			for _, d := range tv.DependsOn {
				if _, ok := col[d]; ok && d < tv.N {
					deps = append(deps, d)
					if col[d]+1 > c {
						c, pref = col[d]+1, sub[d]
					}
				}
			}
			sort.Ints(deps)
			keyParts := make([]string, len(deps))
			for i, d := range deps {
				keyParts[i] = strconv.Itoa(d)
			}
			key := strings.Join(keyParts, ",")
			free := shared[key]
			switch tv.State {
			case "done":
				c, pref, free = now-nDone+1+di, 0, false
				di++
			case "todo", "interrupted":
			default:
				c, free = now, true
			}
			if occ[c] == nil {
				occ[c] = map[int]bool{}
			}
			s := pref
			if free || occ[c][s] {
				for s = 0; occ[c][s]; s++ {
				}
			}
			shared[key] = true
			occ[c][s] = true
			col[tv.N], sub[tv.N] = c, s
			if s+1 > b.groups[g].subs {
				b.groups[g].subs = s + 1
			}
			if c > last {
				last = c
			}
			var issue *int
			if tv.Issue != nil {
				n := *tv.Issue
				issue = &n
			}
			n := Node{ID: id(tv.N), Col: c, Lane: s, Kind: "task", Plan: t, Issue: issue,
				State: tv.State, Future: future(tv.State), Label: tv.Title}
			if !b.add(n, g) {
				continue
			}
			if len(deps) == 0 {
				for _, e := range entries {
					b.edge(e, n.ID)
				}
			}
			for _, d := range deps {
				b.edge(id(d), n.ID)
				hasNext[d] = true
			}
		}
		if len(p.Tasks) > 0 {
			tails = nil
			for _, tv := range p.Tasks {
				if _, placed := col[tv.N]; placed && !hasNext[tv.N] {
					tails = append(tails, id(tv.N))
				}
			}
		}

		// Planned merge on the trunk lane, after the plan's last node.
		mc := last + 1
		for b.trunk[mc] {
			mc++
		}
		mid := "m:" + t
		b.add(Node{ID: mid, Col: mc, Kind: "merge", Plan: t, State: p.State, Future: true, Label: p.Title}, -1)
		for _, e := range tails {
			b.edge(e, mid)
		}
		// A dependency's merge leads into the plan's first node right of it:
		// its first todo task, else its own merge.
		for _, dm := range depMerges {
			to := mid
			for _, tv := range p.Tasks {
				if c, ok := col[tv.N]; ok && future(tv.State) && c > mergeCol[dm[2:]] {
					to = id(tv.N)
					break
				}
			}
			b.edge(dm, to)
		}
		mergeCol[t] = mc
		merges = append(merges, mid)
	}

	// The future trunk runs through the planned merges in column order.
	sort.SliceStable(merges, func(i, j int) bool { return mergeCol[merges[i][2:]] < mergeCol[merges[j][2:]] })
	prev := head
	for _, m := range merges {
		if prev != "" {
			b.edge(prev, m)
		}
		prev = m
	}

	// Lanes: trunk, then each group's lanes in creation order (merged plans
	// with a past part, active plans, the rest in dependency order).
	g := Graph{Now: now, Lanes: []Lane{{Index: 0, Plan: "", Label: v.Default, BlockedBy: []string{}}}, Nodes: []Node{}, Edges: []Edge{}}
	offset := make([]int, len(b.groups))
	for i, gr := range b.groups {
		offset[i] = len(g.Lanes)
		for s := 0; s < gr.subs; s++ {
			l := Lane{Index: len(g.Lanes), Plan: gr.plan, BlockedBy: []string{}}
			if s == 0 {
				l.Label, l.BlockedBy = gr.label, gr.blockedBy
			}
			g.Lanes = append(g.Lanes, l)
		}
	}
	for i, n := range b.nodes {
		if gi := b.nodeG[i]; gi >= 0 {
			n.Lane += offset[gi]
		} else {
			n.Lane = 0
		}
		if n.Col+1 > g.Cols {
			g.Cols = n.Col + 1
		}
		g.Nodes = append(g.Nodes, n)
	}
	g.Edges = append(g.Edges, b.edges...)
	return g
}
