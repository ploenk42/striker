package repoview

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/openspec"
)

// buildOpenSpec builds the View from the OpenSpec store: one PlanView per
// change, active changes by name first, then archived ones newest first.
// Legacy Specs/Plans are not read on this path.
func buildOpenSpec(in Inputs) View {
	st := in.OpenSpec
	out := View{Default: in.Default, Trunk: nonNilCommits(in.Trunk), Plans: []PlanView{},
		Warnings: append([]string{}, in.Warnings...)}
	out.Warnings = append(out.Warnings, st.Warnings...)

	changes := append([]openspec.Change(nil), st.Changes...)
	sort.SliceStable(changes, func(i, j int) bool {
		a, b := changes[i], changes[j]
		if (a.Archived == "") != (b.Archived == "") {
			return a.Archived == ""
		}
		if a.Archived != b.Archived {
			return a.Archived > b.Archived
		}
		return a.Name < b.Name
	})
	archived := map[string]bool{}
	known := map[string]bool{}
	for _, c := range changes {
		known[c.Name] = true
		if c.Archived != "" {
			archived[c.Name] = true
		}
	}
	agents := append([]Agent(nil), in.Agents...)
	sort.Slice(agents, func(i, j int) bool { return agents[i].Address < agents[j].Address })

	for _, c := range changes {
		out.Warnings = append(out.Warnings, c.Warnings...)
		out.Plans = append(out.Plans, buildChange(in, c, known, archived, agents))
	}
	return out
}

func buildChange(in Inputs, c openspec.Change, known, archived map[string]bool, agents []Agent) PlanView {
	pv := PlanView{
		Topic:     c.Name,
		Title:     c.Name,
		Spec:      c.Design,
		Plan:      c.TasksPath,
		Branch:    c.Branch,
		DependsOn: append([]string{}, c.DependsOn...),
		BlockedBy: []string{},
		Tasks:     []TaskView{},
		Commits:   []gitrepo.Commit{},
		Archived:  c.Archived,
		Toc:       append([]string{}, c.Toc...),
	}
	if pv.Branch == "" {
		pv.Branch = c.Name
	}
	br := findBranch(in, pv.Branch)
	if br != nil && br.Worktree != "" {
		w := br.Worktree
		pv.Worktree = &w
	}
	// The coordinator with the lowest address counts (deterministic, like
	// legacy agents sorted by address).
	var coord *Coordinator
	for i := range in.Coordinators {
		if k := &in.Coordinators[i]; k.Change == c.Name && (coord == nil || k.Address < coord.Address) {
			coord = k
		}
	}
	if coord != nil {
		pv.Coordinator = coord.Address
	}

	ids := map[string]int{}
	issues := map[int]bool{}
	for i, t := range c.Tasks {
		ids[t.ID] = i + 1
		if t.Issue != 0 {
			issues[t.Issue] = true
		}
	}
	ticked, live := false, coord != nil
	for _, a := range agents {
		if a.Topic == c.Name {
			live = true
		}
	}
	if in.LeadState == "working" && namesChange(in.LeadTask, c.Name) {
		live = true
	}
	pv.Live = live && c.Archived == ""
	for i, t := range c.Tasks {
		tv := TaskView{ID: t.ID, N: i + 1, Title: t.Title, StepsTotal: 1, DependsOn: []int{},
			Agents: []AgentView{}, Body: t.Body}
		if t.Issue != 0 {
			n := t.Issue
			tv.Issue = &n
		}
		for _, a := range t.After {
			if n, ok := ids[a]; ok {
				tv.DependsOn = append(tv.DependsOn, n)
			}
		}
		if t.Done {
			ticked = true
		}
		blocked, running := false, false
		if c.Archived == "" {
			for _, a := range agents {
				if !runsTask(a, c.Name, t, ids) {
					continue
				}
				tv.Agents = append(tv.Agents, AgentView{Address: a.Address, Role: a.Role, Status: a.Status})
				switch a.Status {
				case "blocked":
					blocked = true
				case "working", "idle":
					running = true
				}
			}
		}
		named := false
		if coord != nil {
			for _, id := range coord.NeedsTasks {
				named = named || id == t.ID || (t.Issue != 0 && id == "#"+strconv.Itoa(t.Issue))
			}
		}
		switch {
		case c.Archived != "":
			tv.State = "done"
		case blocked, named:
			tv.State = "needs_you"
		case running:
			tv.State = "working"
		case t.Done || (t.Issue != 0 && in.Closed[t.Issue]):
			tv.State = "done"
		default:
			tv.State = "todo"
		}
		if tv.State == "done" {
			tv.StepsDone = 1
		}
		pv.Tasks = append(pv.Tasks, tv)
	}

	// interrupted: the change has ticked tasks (tasks.md ticks only, not
	// issues closed elsewhere) and open ones, and nothing runs it. The first
	// open task is interrupted.
	if c.Archived == "" && !live && ticked {
		first := -1
		for i, tv := range pv.Tasks {
			if tv.State != "done" {
				first = i
				break
			}
		}
		if first >= 0 && pv.Tasks[first].State == "todo" {
			pv.Tasks[first].State = "interrupted"
			pv.InterruptedAt = pv.Tasks[first].ID
		}
	}

	pv.Commits = planCommits(in, br, issues)
	if c.Archived != "" {
		pv.State = "done"
		pv.MergePoint = mergePoint(in, br, issues)
		return pv
	}
	for _, d := range pv.DependsOn {
		switch {
		case !known[d]:
			pv.BlockedBy = append(pv.BlockedBy, d+"?")
		case !archived[d]:
			pv.BlockedBy = append(pv.BlockedBy, d)
		}
	}
	switch {
	case coord != nil && (coord.NeedsYou > 0 || coord.State == "blocked"):
		pv.State = "needs_you"
	case ticked || pv.Worktree != nil:
		pv.State = "in progress"
	case len(pv.BlockedBy) > 0:
		pv.State = "blocked"
	default:
		pv.State = "planned"
	}
	return pv
}

// runsTask reports whether agent a is a task agent of task t in change. Its
// striker.task value is the task id, or an issue ("#42", or a bare number that
// is no task id of the change).
func runsTask(a Agent, change string, t openspec.Task, ids map[string]int) bool {
	if a.Topic != change {
		return false
	}
	v := strings.TrimSpace(a.TaskID)
	if !strings.HasPrefix(v, "#") && v == t.ID {
		return true
	}
	if _, isID := ids[v]; isID && !strings.HasPrefix(v, "#") {
		return false
	}
	return t.Issue != 0 && a.Task == t.Issue
}

// namesChange reports whether text (the lead's status task) names change as
// a whole word: "Task 3 of repo-overview" names repo-overview, not repo.
func namesChange(text, change string) bool {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '.'
	})
	for _, w := range words {
		if strings.TrimRight(w, ".") == change {
			return true
		}
	}
	return false
}
