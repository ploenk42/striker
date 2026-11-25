// Package openspec reads an OpenSpec store (openspec/ in a repo) from git at any ref
// or from the working tree, without Node.
package openspec

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
)

// Task is one top-level checkbox line of tasks.md.
type Task struct {
	ID, Title string
	Done      bool
	Issue     int
	After     []string
	Body      string // the "### Task <id>:" section of plan.md, "" if absent
}

// Change is one active or archived change. Design, Proposal, TasksPath and PlanPath are
// repo-relative file paths ("" if the file is absent).
type Change struct {
	Name      string
	Archived  string // YYYY-MM-DD from the archive folder, "" for active changes
	DependsOn []string
	Branch    string
	Tasks     []Task
	Toc       []string // "## " headings of proposal.md then design.md
	Design    string
	Proposal  string
	TasksPath string
	PlanPath  string
	Warnings  []string
}

// Capability is one openspec/specs/<path>/spec.md.
type Capability struct {
	Path         string
	Requirements int
}

// Store is everything read from one openspec/ tree.
type Store struct {
	Changes      []Change
	Capabilities []Capability
	Warnings     []string
}

var (
	taskRe     = regexp.MustCompile(`^- \[([ xX])\] (.*)$`)
	idRe       = regexp.MustCompile(`^(\d+(?:\.\d+)*)\s+(.*)$`)
	suffixRe   = regexp.MustCompile(`\s*\(([^()]*)\)\s*$`)
	suffixLike = regexp.MustCompile(`^\s*(#|after\b)`)
	issueRe    = regexp.MustCompile(`^#(\d+)$`)
	taskHeadRe = regexp.MustCompile(`^### Task\s+([^\s:]+)\s*:`)
	archiveRe  = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})-(.+)$`)
)

func lines(text []byte) []string {
	return strings.Split(strings.ReplaceAll(string(text), "\r\n", "\n"), "\n")
}

// ParseTasks reads the top-level "- [ ] <id> <title> (#issue; after <id>, <id>)" lines.
// Malformed parts are ignored with a warning; the line still counts as a task.
func ParseTasks(text []byte) ([]Task, []string) {
	var ts []Task
	var warns []string
	lineOf := []int{}
	for i, l := range lines(text) {
		m := taskRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		n := i + 1
		im := idRe.FindStringSubmatch(strings.TrimSpace(m[2]))
		if im == nil {
			warns = append(warns, fmt.Sprintf("line %d: task without id ignored: %q", n, l))
			continue
		}
		t := Task{ID: im[1], Done: m[1] != " ", Title: strings.TrimSpace(im[2])}
		if sm := suffixRe.FindStringSubmatch(t.Title); sm != nil && suffixLike.MatchString(sm[1]) {
			t.Title = strings.TrimSpace(suffixRe.ReplaceAllString(t.Title, ""))
			for _, part := range strings.Split(sm[1], ";") {
				part = strings.TrimSpace(part)
				switch {
				case strings.HasPrefix(part, "after"):
					for _, a := range strings.Split(strings.TrimSpace(strings.TrimPrefix(part, "after")), ",") {
						if a = strings.TrimSpace(a); a != "" {
							t.After = append(t.After, a)
						}
					}
				case strings.HasPrefix(part, "#"):
					if x := issueRe.FindStringSubmatch(part); x != nil {
						t.Issue, _ = strconv.Atoi(x[1])
					} else {
						warns = append(warns, fmt.Sprintf("line %d: bad issue %q ignored", n, part))
					}
				default:
					warns = append(warns, fmt.Sprintf("line %d: unknown suffix part %q ignored", n, part))
				}
			}
			if sm2 := suffixRe.FindStringSubmatch(t.Title); sm2 != nil && suffixLike.MatchString(sm2[1]) {
				warns = append(warns, fmt.Sprintf("line %d: more than one suffix, ignoring (%s)", n, sm2[1]))
			}
		}
		ts = append(ts, t)
		lineOf = append(lineOf, n)
	}
	known := map[string]bool{}
	for _, t := range ts {
		known[t.ID] = true
	}
	for i := range ts {
		var ok []string
		for _, a := range ts[i].After {
			if known[a] && a != ts[i].ID {
				ok = append(ok, a)
			} else {
				warns = append(warns, fmt.Sprintf("line %d: after %q is not a task, ignored", lineOf[i], a))
			}
		}
		ts[i].After = ok
		if len(ok) == 0 && i > 0 {
			ts[i].After = []string{ts[i-1].ID}
		}
	}
	return ts, warns
}

// SetIssue writes "#<issue>" into the top-level line of task id: into an
// existing "(… after …)" suffix as "(#n; after …)", else as " (#n)" at the end.
// Only that line changes (line endings kept). A task that already has an
// issue, or an unknown id, leaves text unchanged and returns false.
func SetIssue(text []byte, id string, issue int) ([]byte, bool) {
	ls := strings.Split(string(text), "\n")
	for i, raw := range ls {
		l := strings.TrimSuffix(raw, "\r")
		m := taskRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		im := idRe.FindStringSubmatch(strings.TrimSpace(m[2]))
		if im == nil || im[1] != id {
			continue
		}
		ts, _ := ParseTasks([]byte(l))
		if len(ts) != 1 || ts[0].Issue != 0 {
			return text, false
		}
		num := "#" + strconv.Itoa(issue)
		body := strings.TrimRight(l, " \t")
		if loc := suffixRe.FindStringSubmatchIndex(body); loc != nil && suffixLike.MatchString(body[loc[2]:loc[3]]) {
			inner := strings.TrimSpace(body[loc[2]:loc[3]])
			body = body[:loc[2]] + num + "; " + inner + body[loc[3]:]
		} else {
			body += " (" + num + ")"
		}
		ls[i] = body + raw[len(l):]
		return []byte(strings.Join(ls, "\n")), true
	}
	return text, false
}

// ParseMeta reads x-depends_on and x-branch from a .openspec.yaml; other keys are ignored.
func ParseMeta(text []byte) (dependsOn []string, branch string) {
	for _, l := range lines(text) {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "x-depends_on":
			for _, d := range strings.Split(strings.Trim(v, "[]"), ",") {
				if d = strings.Trim(strings.TrimSpace(d), `"'`); d != "" {
					dependsOn = append(dependsOn, d)
				}
			}
		case "x-branch":
			branch = strings.Trim(v, `"'`)
		}
	}
	return dependsOn, branch
}

// headings returns the "## " headings of text, outside code fences.
func headings(text []byte) []string {
	var out []string
	fence := false
	for _, l := range lines(text) {
		if strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~") {
			fence = !fence
			continue
		}
		if !fence && strings.HasPrefix(l, "## ") {
			out = append(out, strings.TrimSpace(l[3:]))
		}
	}
	return out
}

// planBodies maps task id to the text under its "### Task <id>:" heading in plan.md.
func planBodies(text []byte) map[string]string {
	out := map[string]string{}
	id := ""
	var buf []string
	flush := func() {
		if id != "" {
			out[id] = strings.TrimSpace(strings.Join(buf, "\n"))
		}
		id, buf = "", nil
	}
	for _, l := range lines(text) {
		if m := taskHeadRe.FindStringSubmatch(l); m != nil {
			flush()
			id = m[1]
			continue
		}
		if strings.HasPrefix(l, "## ") || strings.HasPrefix(l, "### ") || strings.HasPrefix(l, "# ") {
			flush()
			continue
		}
		if id != "" {
			buf = append(buf, l)
		}
	}
	flush()
	return out
}

func countRequirements(text []byte) int {
	n := 0
	for _, l := range lines(text) {
		if strings.HasPrefix(l, "### Requirement:") {
			n++
		}
	}
	return n
}

// list returns every file under openspec/ (repo-relative, slash separated). Working tree
// (ref "") uses git ls-files with untracked files; a ref uses a recursive git ls-tree.
// Both go through the Runner, so Local and Remote behave the same.
func list(r gitrepo.Repo, ref string) ([]string, error) {
	var out []byte
	var err error
	if ref == "" {
		out, err = r.R.Git(r.Dir, "ls-files", "--cached", "--others", "--exclude-standard", "--", "openspec")
	} else {
		out, err = r.R.Git(r.Dir, "ls-tree", "-r", "--name-only", ref, "openspec/")
	}
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var fs []string
	for _, l := range lines(out) {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "openspec/") && !seen[l] {
			seen[l] = true
			fs = append(fs, l)
		}
	}
	sort.Strings(fs)
	return fs, nil
}

// Read reads the store at ref ("" = working tree). The bool reports whether openspec/ exists.
func Read(r gitrepo.Repo, ref string) (Store, bool, error) {
	var s Store
	files, err := list(r, ref)
	if err != nil {
		return s, false, err
	}
	if len(files) == 0 {
		return s, false, nil
	}
	have := map[string]bool{}
	for _, f := range files {
		have[f] = true
	}
	// a ref is read in one batch (ShowAll); a failed batch falls back to one git show per file
	var batch map[string][]byte
	if ref != "" {
		batch, _ = r.ShowAll(ref, files)
	}
	read := func(p string) []byte {
		if b, ok := batch[p]; ok {
			return b
		}
		var b []byte
		var err error
		if ref == "" {
			b, err = r.ReadWorking(p)
		} else {
			b, err = r.Show(ref, p)
		}
		if err != nil {
			s.Warnings = append(s.Warnings, fmt.Sprintf("%s: unreadable: %v", p, err))
			return nil
		}
		return b
	}

	changeDirs := map[string]bool{}
	for _, f := range files {
		rest := strings.TrimPrefix(f, "openspec/")
		switch {
		case strings.HasPrefix(rest, "specs/") && path.Base(rest) == "spec.md":
			dir := strings.TrimPrefix(path.Dir(rest), "specs")
			dir = strings.TrimPrefix(dir, "/")
			if dir == "" {
				continue
			}
			s.Capabilities = append(s.Capabilities, Capability{Path: dir, Requirements: countRequirements(read(f))})
		case strings.HasPrefix(rest, "changes/"):
			parts := strings.Split(strings.TrimPrefix(rest, "changes/"), "/")
			switch {
			case parts[0] == "archive" && len(parts) >= 3:
				changeDirs["openspec/changes/archive/"+parts[1]] = true
			case parts[0] != "archive" && len(parts) >= 2:
				changeDirs["openspec/changes/"+parts[0]] = true
			}
		}
	}

	var dirs []string
	for d := range changeDirs {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		c := Change{Name: path.Base(d)}
		if path.Base(path.Dir(d)) == "archive" {
			if m := archiveRe.FindStringSubmatch(c.Name); m != nil {
				c.Archived, c.Name = m[1], m[2]
			} else {
				c.Archived = "unknown"
			}
		}
		if p := d + "/.openspec.yaml"; have[p] {
			c.DependsOn, c.Branch = ParseMeta(read(p))
		}
		if c.Branch == "" {
			c.Branch = c.Name
		}
		for _, f := range []struct {
			name string
			dst  *string
		}{{"proposal.md", &c.Proposal}, {"design.md", &c.Design}, {"tasks.md", &c.TasksPath}, {"plan.md", &c.PlanPath}} {
			if p := d + "/" + f.name; have[p] {
				*f.dst = p
			}
		}
		for _, p := range []string{c.Proposal, c.Design} {
			if p != "" {
				c.Toc = append(c.Toc, headings(read(p))...)
			}
		}
		if c.TasksPath != "" {
			var warns []string
			c.Tasks, warns = ParseTasks(read(c.TasksPath))
			for _, w := range warns {
				c.Warnings = append(c.Warnings, c.TasksPath+": "+w)
			}
		}
		if c.PlanPath != "" && len(c.Tasks) > 0 {
			bodies := planBodies(read(c.PlanPath))
			for i := range c.Tasks {
				c.Tasks[i].Body = bodies[c.Tasks[i].ID]
			}
		}
		s.Changes = append(s.Changes, c)
	}
	// active changes first (by name), archived after (by date)
	sort.SliceStable(s.Changes, func(i, j int) bool {
		a, b := s.Changes[i], s.Changes[j]
		if (a.Archived == "") != (b.Archived == "") {
			return a.Archived == ""
		}
		return false
	})
	return s, true, nil
}

// Merge returns main with, for each active change, the tasks.md/plan.md content of the
// same change in the store of the branch named by the change's Branch. Archived changes
// and branch-only changes are left alone. Inputs are not modified.
func Merge(main Store, branches map[string]Store) Store {
	out := main
	out.Changes = make([]Change, len(main.Changes))
	copy(out.Changes, main.Changes)
	for i, c := range out.Changes {
		if c.Archived != "" {
			continue
		}
		bs, ok := branches[c.Branch]
		if !ok {
			continue
		}
		for _, bc := range bs.Changes {
			if bc.Name != c.Name || bc.Archived != "" {
				continue
			}
			out.Changes[i].Tasks = append([]Task(nil), bc.Tasks...)
			out.Changes[i].TasksPath = bc.TasksPath
			out.Changes[i].PlanPath = bc.PlanPath
			out.Changes[i].Warnings = append([]string(nil), bc.Warnings...)
			break
		}
	}
	return out
}
