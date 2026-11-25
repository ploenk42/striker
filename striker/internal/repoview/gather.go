package repoview

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/issues"
	"github.com/ploenk42/striker/striker/internal/openspec"
	"github.com/ploenk42/striker/striker/internal/plans"
	"github.com/ploenk42/striker/striker/internal/status"
)

const (
	specDir  = "docs/superpowers/specs"
	planDir  = "docs/superpowers/plans"
	trunkLen = 30
)

// docs are the specs and plans read from one ref.
type docs struct {
	specs     map[string]plans.Spec
	plans     map[string]plans.Plan
	specFiles map[string]string
	planFiles map[string]string
}

func readDocs(r gitrepo.Repo, ref string) docs {
	d := docs{map[string]plans.Spec{}, map[string]plans.Plan{}, map[string]string{}, map[string]string{}}
	// A ref without the directory makes ls-tree fail: treat as empty.
	specFiles, _ := r.ListFiles(ref, specDir)
	planFiles, _ := r.ListFiles(ref, planDir)
	// one batch for both folders; a failed batch falls back to one git show per file
	var md []string
	for _, f := range append(append([]string{}, specFiles...), planFiles...) {
		if strings.HasSuffix(f, ".md") {
			md = append(md, f)
		}
	}
	batch := map[string][]byte{}
	if len(md) > 0 {
		if b, err := r.ShowAll(ref, md); err == nil {
			batch = b
		}
	}
	show := func(f string) ([]byte, error) {
		if b, ok := batch[f]; ok {
			return b, nil
		}
		return r.Show(ref, f)
	}
	for _, f := range specFiles {
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		if b, err := show(f); err == nil {
			s := plans.ParseSpec(f, b)
			d.specs[s.Topic], d.specFiles[s.Topic] = s, f
		}
	}
	for _, f := range planFiles {
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		if b, err := show(f); err == nil {
			p := plans.ParsePlan(f, b)
			d.plans[p.Topic], d.planFiles[p.Topic] = p, f
		}
	}
	return d
}

// Gather collects the Inputs of the repo r. Plan branches are the branches
// (other than the default) that have a worktree or an origin/ counterpart.
// x carries the repo's fleet name and the coordinator status reader.
func Gather(r gitrepo.Repo, h hc.Client, gh issues.GH, now time.Time, x Extra) (Inputs, error) {
	read := x.Read
	in := Inputs{Now: now, SpecFiles: map[string]string{}, PlanFiles: map[string]string{},
		LeadTask: x.LeadTask, LeadState: x.LeadState}
	def := r.DefaultBranch()
	in.Default = def
	trunk, err := r.Log(def, trunkLen)
	if err != nil {
		return in, fmt.Errorf("log %s: %w", def, err)
	}
	in.Trunk = trunk

	wts, err := r.Worktrees()
	if err != nil {
		return in, fmt.Errorf("worktrees: %w", err)
	}
	refs, err := r.Branches()
	if err != nil {
		return in, fmt.Errorf("branches: %w", err)
	}
	local := map[string]bool{}
	names := map[string]bool{}
	for _, b := range refs {
		if n, ok := strings.CutPrefix(b, "origin/"); ok {
			names[n] = true
		} else {
			local[b] = true
		}
	}
	worktree := map[string]string{}
	for _, w := range wts {
		if w.Branch != "" { // detached worktrees have no branch
			worktree[w.Branch] = w.Path
			names[w.Branch] = true
		}
	}
	delete(names, def)
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	base := readDocs(r, def)
	specs, pls := base.specs, map[string]plans.Plan{}
	in.DefaultPlans = map[string]plans.Plan{}
	for t, p := range base.plans {
		pls[t], in.DefaultPlans[t] = p, p
	}
	for t, f := range base.specFiles {
		in.SpecFiles[t] = f
	}
	for t, f := range base.planFiles {
		in.PlanFiles[t] = f
	}

	branchRef := map[string]string{}
	for _, name := range sorted {
		ref := name
		if !local[name] {
			ref = "origin/" + name
		}
		branchRef[name] = ref
		b := Branch{Name: name, Worktree: worktree[name], Merge: r.MergeCommit(ref, def)}
		if cs, err := r.Log(def+".."+ref, trunkLen); err == nil {
			b.Commits = cs
		}
		in.Branches = append(in.Branches, b)

		d := readDocs(r, ref)
		// own reports whether topic is built on this branch: its copy then
		// wins over the default branch's. Other topics are only added when
		// not known yet.
		own := func(topic string) bool {
			if s, ok := d.specs[topic]; ok {
				return s.Branch == name
			}
			if s, ok := specs[topic]; ok {
				return s.Branch == name
			}
			return topic == name
		}
		for t, s := range d.specs {
			if _, known := specs[t]; !known || own(t) {
				specs[t], in.SpecFiles[t] = s, d.specFiles[t]
			}
		}
		for t, p := range d.plans {
			if _, known := pls[t]; !known || own(t) {
				pls[t], in.PlanFiles[t] = p, d.planFiles[t]
			}
		}
	}
	for _, s := range specs {
		in.Specs = append(in.Specs, s)
	}
	for _, p := range pls {
		in.Plans = append(in.Plans, p)
	}
	sort.Slice(in.Specs, func(i, j int) bool { return in.Specs[i].Topic < in.Specs[j].Topic })
	sort.Slice(in.Plans, func(i, j int) bool { return in.Plans[i].Topic < in.Plans[j].Topic })

	// OpenSpec: like the legacy docs, the default branch is read at its ref
	// (not the working tree); each plan branch's copy of a change wins. A
	// read that fails on the default branch means no store (legacy path).
	if st, has, err := openspec.Read(r, def); err == nil && has {
		branches := map[string]openspec.Store{}
		for _, name := range sorted {
			bs, ok, err := openspec.Read(r, branchRef[name])
			switch {
			case err != nil:
				msg, _, _ := strings.Cut(err.Error(), "\n")
				in.Warnings = append(in.Warnings, "openspec on "+branchRef[name]+" unreadable: "+msg)
			case ok:
				branches[name] = bs
			}
		}
		merged := openspec.Merge(st, branches)
		in.OpenSpec = &merged
	}

	// No origin or a non-GitHub origin: no issues to read, not an error.
	in.Closed = map[int]bool{}
	if url, err := r.R.Git(r.Dir, "remote", "get-url", "origin"); err == nil {
		if slug := issues.Slug(string(url)); slug != "" {
			closed, err := issues.Closed(gh, slug)
			if err != nil {
				msg, _, _ := strings.Cut(err.Error(), "\n")
				in.Warnings = append(in.Warnings, "closed issues unavailable: "+msg)
			} else {
				in.Closed = closed
			}
		}
	}

	machine := ""
	if rm, ok := r.R.(gitrepo.Remote); ok {
		machine = rm.Machine
	}
	roots := []string{r.Dir}
	for _, w := range wts {
		roots = append(roots, w.Path)
	}
	agents, err := gatherAgents(h, machine, roots, x.Repo)
	if err != nil {
		return in, fmt.Errorf("agents: %w", err)
	}
	in.Agents = agents
	// Coordinators from their labels; with a reader, NeedsYou, NeedsTasks
	// and Status from their status files (striker.repo names the file).
	for _, a := range agents {
		if a.Coordinator == "" {
			continue
		}
		k := Coordinator{Change: a.Coordinator, Address: a.Address, State: a.Status}
		if read != nil {
			if a.Repo == "" {
				k.Status = json.RawMessage(`{"error":"Terminal ohne Label striker.repo"}`)
			} else {
				k.Status = read(machine, a.Repo, a.Coordinator)
				k.NeedsYou, k.NeedsTasks = status.CoordNeeds(k.Status)
			}
		}
		in.Coordinators = append(in.Coordinators, k)
	}
	return in, nil
}

// normPath makes paths comparable: forward slashes, no trailing slash.
func normPath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

// machineOf is the machine prefix of a terminal address ("" = local).
func machineOf(addr string) string {
	if m, _, remote := strings.Cut(addr, "/"); remote {
		return m
	}
	return ""
}

// inRepo reports whether a terminal at addr with working dir cwd belongs to
// this repo: same machine (local: no "/" in the address; remote: address
// prefix "<machine>/") and cwd inside the repo dir or one of its worktrees.
func inRepo(addr, cwd, machine string, roots []string) bool {
	if machineOf(addr) != machine || cwd == "" {
		return false
	}
	c := normPath(cwd)
	for _, root := range roots {
		r := normPath(root)
		if r != "" && (c == r || strings.HasPrefix(c, strings.TrimSuffix(r, "/")+"/")) {
			return true
		}
	}
	return false
}

// gatherAgents reads every terminal the local hc knows (remote ones carry a
// machine/ prefix) and keeps those labelled striker.task or striker.coordinator
// whose terminal belongs to this repo (task numbers are per repo): its cwd
// is inside the repo or a worktree, or (repo != "") it carries
// striker.repo=<repo> and runs on the repo's machine — behind sudo -i the cwd
// hc reports is the user's home.
func gatherAgents(h hc.Client, machine string, roots []string, repo string) ([]Agent, error) {
	terms, err := h.List()
	if err != nil {
		return nil, err
	}
	var out []Agent
	for _, t := range terms {
		info, err := h.Get(t.Address)
		if err != nil {
			continue // terminal vanished between ls and get
		}
		labelled := repo != "" && info.Labels["striker.repo"] == repo && machineOf(t.Address) == machine
		if !labelled && !inRepo(t.Address, info.Cwd, machine, roots) {
			continue
		}
		task, hasTask := info.Labels["striker.task"]
		coord := info.Labels["striker.coordinator"]
		if !hasTask && coord == "" {
			continue
		}
		a := Agent{Address: t.Address, Role: info.Labels["striker.role"], Coordinator: coord}
		a.Repo = info.Labels["striker.repo"]
		if topic := info.Labels["striker.topic"]; topic != "" && hasTask {
			a.Topic, a.TaskID = topic, strings.TrimSpace(task)
		}
		if info.Status != nil {
			a.Status = info.Status.State
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(task), "#")); err == nil {
			a.Task = n
		}
		out = append(out, a)
	}
	return out, nil
}
