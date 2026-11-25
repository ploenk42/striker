// Package repoview builds the repo view of the LCARS Bridge: every spec/plan
// topic of a repo with its plan state, task states and agents.
package repoview

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/openspec"
	"github.com/ploenk42/striker/striker/internal/plans"
)

// Branch is a plan branch of the repo (name without "origin/").
type Branch struct {
	Name, Worktree, Merge string
	Commits               []gitrepo.Commit
}

// Agent is a terminal labelled striker.task=<issue> or striker.coordinator=<topic>.
// Topic and TaskID are only set for terminals that carry striker.topic (task
// agents of an OpenSpec change): TaskID is the raw striker.task value, a task
// id ("2.3") or an issue ("#42").
type Agent struct {
	Address, Status, Role, Coordinator string
	Task                               int
	Topic, TaskID                      string
	Repo                               string // striker.repo (coordinators)
}

// Coordinator is a terminal labelled striker.coordinator=<change>. Address and
// State (live state) come from hc; NeedsYou (entries in its status file's
// needs_you), NeedsTasks (task ids those entries name with "task") and
// Status (the status file) from the coordinator status file.
type Coordinator struct {
	Change, Address, State string
	NeedsYou               int
	NeedsTasks             []string
	Status                 json.RawMessage
}

// CoordReader reads a coordinator's status file (machine "" = this machine).
type CoordReader func(machine, repo, change string) json.RawMessage

// Extra is what Gather knows from outside git: Repo, the repo's fleet name
// (terminals labelled striker.repo=<Repo> on its machine belong to it whatever
// their cwd; "" = cwd only), Read for coordinator status files (nil:
// not read), and the lead's status-file task and live state (liveness).
type Extra struct {
	Repo                string
	Read                CoordReader
	LeadTask, LeadState string
}

// Inputs is everything Build needs; Gather collects it. Plans is the display
// copy (a branch's own copy wins); DefaultPlans holds each topic's copy on
// the default branch, which decides step-based done. SpecFiles and PlanFiles
// map topic -> repo-relative path.
//
// OpenSpec, when set, is the repo's merged OpenSpec store and Build reads
// the changes from it instead of the legacy Specs/Plans. LeadTask and
// LeadState are the lead's status-file task and live state (liveness).
type Inputs struct {
	OpenSpec             *openspec.Store
	Coordinators         []Coordinator
	LeadTask, LeadState  string
	Default              string
	Trunk                []gitrepo.Commit
	Branches             []Branch
	Specs                []plans.Spec
	Plans                []plans.Plan
	DefaultPlans         map[string]plans.Plan
	SpecFiles, PlanFiles map[string]string
	Closed               map[int]bool
	Agents               []Agent
	Warnings             []string
	Now                  time.Time
}

type View struct {
	Default  string           `json:"default"`
	Trunk    []gitrepo.Commit `json:"trunk"`
	Plans    []PlanView       `json:"plans"`
	Warnings []string         `json:"warnings"`
}

type PlanView struct {
	Topic       string           `json:"topic"`
	Title       string           `json:"title"`
	Spec        string           `json:"spec"`
	Plan        string           `json:"plan"`
	Branch      string           `json:"branch"`
	DependsOn   []string         `json:"depends_on"`
	Worktree    *string          `json:"worktree"`
	State       string           `json:"state"`
	BlockedBy   []string         `json:"blocked_by"`
	MergePoint  string           `json:"merge_point"`
	Coordinator string           `json:"coordinator"`
	Tasks       []TaskView       `json:"tasks"`
	Commits     []gitrepo.Commit `json:"commits"`
	// Archived is the archive date of an archived OpenSpec change.
	Archived string `json:"archived"`
	// Toc lists the "## " headings of proposal.md then design.md.
	Toc []string `json:"toc"`
	// InterruptedAt is the id of the interrupted task, "" if none.
	InterruptedAt string `json:"interrupted_at"`
	// Legacy marks plans read from docs/superpowers (no openspec/).
	Legacy bool `json:"legacy"`
	// Live: someone works on the change now (its coordinator, a task agent of
	// its topic, or the lead working on it). OpenSpec changes only.
	Live bool `json:"live"`
}

type TaskView struct {
	ID         string      `json:"id"`
	Issue      *int        `json:"issue"`
	N          int         `json:"n"`
	Title      string      `json:"title"`
	StepsDone  int         `json:"steps_done"`
	StepsTotal int         `json:"steps_total"`
	DependsOn  []int       `json:"depends_on"`
	State      string      `json:"state"`
	Agents     []AgentView `json:"agents"`
	Body       string      `json:"body"`
}

type AgentView struct {
	Address string `json:"address"`
	Role    string `json:"role"`
	Status  string `json:"status"`
}

// Build turns Inputs into the View. It is pure and its output is ordered
// (plans by topic, agents by address) so equal inputs give equal answers.
func Build(in Inputs) View {
	if in.OpenSpec != nil {
		return buildOpenSpec(in)
	}
	specs := map[string]plans.Spec{}
	pls := map[string]plans.Plan{}
	topicSet := map[string]bool{}
	for _, s := range in.Specs {
		specs[s.Topic] = s
		topicSet[s.Topic] = true
	}
	for _, p := range in.Plans {
		pls[p.Topic] = p
		topicSet[p.Topic] = true
	}
	topics := make([]string, 0, len(topicSet))
	for t := range topicSet {
		topics = append(topics, t)
	}
	sort.Strings(topics)

	agents := append([]Agent(nil), in.Agents...)
	sort.Slice(agents, func(i, j int) bool { return agents[i].Address < agents[j].Address })

	views := map[string]*PlanView{}
	merged := map[string]bool{}
	for _, topic := range topics {
		pv := buildPlan(in, topic, specs, pls, agents)
		views[topic] = pv
		merged[topic] = pv.State == "merged"
	}

	out := View{Default: in.Default, Trunk: nonNilCommits(in.Trunk), Plans: []PlanView{},
		Warnings: append([]string{}, in.Warnings...)}
	for _, topic := range topics {
		pv := views[topic]
		if pv.State == "" {
			// merged only depends on a plan's own tasks, so dependency cycles
			// need no special handling: each side is simply not merged.
			for _, d := range pv.DependsOn {
				if _, known := views[d]; !known {
					pv.BlockedBy = append(pv.BlockedBy, d+"?")
				} else if !merged[d] {
					pv.BlockedBy = append(pv.BlockedBy, d)
				}
			}
			switch {
			case pv.Worktree != nil:
				// active wins; blocked_by still lists unmerged dependencies.
				pv.State = "active"
			case len(pv.BlockedBy) > 0:
				pv.State = "blocked"
			default:
				pv.State = "planned"
			}
		}
		out.Plans = append(out.Plans, *pv)
	}
	return out
}

// buildPlan fills everything of a plan except the non-merged states.
func buildPlan(in Inputs, topic string, specs map[string]plans.Spec, pls map[string]plans.Plan, agents []Agent) *PlanView {
	s, hasSpec := specs[topic]
	p := pls[topic]
	pv := &PlanView{
		Topic:     topic,
		Spec:      in.SpecFiles[topic],
		Plan:      in.PlanFiles[topic],
		Branch:    topic,
		DependsOn: []string{},
		BlockedBy: []string{},
		Tasks:     []TaskView{},
		Commits:   []gitrepo.Commit{},
		Toc:       []string{},
		Legacy:    true,
	}
	switch {
	case hasSpec && s.Title != "":
		pv.Title = s.Title
	case p.Title != "":
		pv.Title = p.Title
	default:
		pv.Title = topic
	}
	if hasSpec {
		if s.Branch != "" {
			pv.Branch = s.Branch
		}
		pv.DependsOn = append(pv.DependsOn, s.DependsOn...)
	}
	br := findBranch(in, pv.Branch)
	if br != nil && br.Worktree != "" {
		w := br.Worktree
		pv.Worktree = &w
	}
	for _, a := range agents {
		if a.Coordinator == topic {
			pv.Coordinator = a.Address
			break
		}
	}

	issues := map[int]bool{}
	allDone := len(p.Tasks) > 0
	// Step-based done applies only to tasks without an issue (a task with an
	// issue is done iff its issue is closed) and is read from the default
	// branch's copy (spec "Data"): steps ticked only on an unmerged branch do
	// not finish a task. Unnumbered tasks are matched by position N against
	// the default copy's unnumbered tasks only, so a default task with an
	// issue never lends its steps to a branch task.
	byN := map[int]bool{}
	for _, t := range in.DefaultPlans[topic].Tasks {
		if t.Issue == 0 {
			byN[t.N] = t.StepsTotal > 0 && t.StepsDone == t.StepsTotal
		}
	}
	for _, t := range p.Tasks {
		tv := buildTask(in, t, t.Issue == 0 && byN[t.N], agents)
		if t.Issue != 0 {
			issues[t.Issue] = true
		}
		if tv.State != "done" {
			allDone = false
		}
		pv.Tasks = append(pv.Tasks, tv)
	}

	pv.Commits = planCommits(in, br, issues)
	// A plan without tasks (spec only, or empty plan) is never merged.
	if allDone {
		pv.State = "merged"
		pv.MergePoint = mergePoint(in, br, issues)
	}
	return pv
}

// findBranch returns the plan branch called name, nil if there is none.
func findBranch(in Inputs, name string) *Branch {
	for i := range in.Branches {
		if in.Branches[i].Name == name {
			return &in.Branches[i]
		}
	}
	return nil
}

// hitsIssues reports whether commit c refers to or fixes one of issues.
func hitsIssues(c gitrepo.Commit, issues map[int]bool) bool {
	for _, n := range c.Refs {
		if issues[n] {
			return true
		}
	}
	for _, n := range c.Fixes {
		if issues[n] {
			return true
		}
	}
	return false
}

// planCommits: the branch's own commits, else the trunk commits that name
// one of the plan's issues.
func planCommits(in Inputs, br *Branch, issues map[int]bool) []gitrepo.Commit {
	if br != nil {
		return nonNilCommits(br.Commits)
	}
	out := []gitrepo.Commit{}
	for _, c := range in.Trunk {
		if hitsIssues(c, issues) {
			out = append(out, c)
		}
	}
	return out
}

// mergePoint: the branch's merge commit, else the newest trunk commit that
// names one of the plan's issues ("" if none).
func mergePoint(in Inputs, br *Branch, issues map[int]bool) string {
	if br != nil && br.Merge != "" {
		return br.Merge
	}
	for _, c := range in.Trunk { // newest first
		if hitsIssues(c, issues) {
			return c.SHA
		}
	}
	return ""
}

// buildTask: defStepsDone tells whether this task has no issue and all its
// steps are checked in the default branch's copy of the plan.
func buildTask(in Inputs, t plans.Task, defStepsDone bool, agents []Agent) TaskView {
	tv := TaskView{
		N:          t.N,
		Title:      t.Title,
		StepsDone:  t.StepsDone,
		StepsTotal: t.StepsTotal,
		DependsOn:  append([]int{}, t.DependsOn...),
		Agents:     []AgentView{},
		Body:       t.Body,
	}
	blocked, running := false, false
	if t.Issue != 0 {
		n := t.Issue
		tv.Issue = &n
		for _, a := range agents {
			if a.Task != n {
				continue
			}
			tv.Agents = append(tv.Agents, AgentView{Address: a.Address, Role: a.Role, Status: a.Status})
			switch a.Status {
			case "blocked":
				blocked = true
			case "working", "idle":
				// A live labelled terminal that is working or idle runs the
				// task. "done" and "" (no status) do not count as running: the
				// task falls through to the issue and step checks.
				running = true
			}
		}
	}
	switch {
	case blocked:
		tv.State = "needs_you"
	case running:
		tv.State = "working"
	case t.Issue != 0 && in.Closed[t.Issue]:
		tv.State = "done"
	case defStepsDone:
		tv.State = "done"
	default:
		tv.State = "todo"
	}
	return tv
}

func nonNilCommits(cs []gitrepo.Commit) []gitrepo.Commit {
	if cs == nil {
		return []gitrepo.Commit{}
	}
	return cs
}
