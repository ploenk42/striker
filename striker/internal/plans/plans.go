// Package plans parses spec and plan markdown files.
package plans

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Spec is the parsed header of a design spec.
type Spec struct {
	Topic, Title, Branch string
	DependsOn            []string
	Headings             []string
}

// Task is one "### Task" section of a plan.
type Task struct {
	N          int
	Issue      int
	Title      string
	StepsDone  int
	StepsTotal int
	DependsOn  []int
	Body       string
}

// Plan is a parsed implementation plan.
type Plan struct {
	Topic, Title string
	Tasks        []Task
}

var (
	datePrefix  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-`)
	dependsLine = regexp.MustCompile(`(?i)^[\s*_>-]*depends on[\s*_]*:[\s*_]*(.*)$`)
	branchLine  = regexp.MustCompile(`(?i)^[\s*_>-]*branch[\s*_]*:[\s*_]*(.*)$`)
	taskHeading = regexp.MustCompile(`^###\s+Task\s+(#?)(\d+)\s*:?\s*(.*)$`)
	stepLine    = regexp.MustCompile(`^\s*- \[( |x|X)\]`)
	taskRef     = regexp.MustCompile(`(?i)(?:task\s+)?(#?)(\d+)`)
)

// Topic derives the topic from a spec or plan file name: base name without
// extension, date prefix and "-design" suffix.
func Topic(file string) string {
	b := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	b = datePrefix.ReplaceAllString(b, "")
	return strings.TrimSuffix(b, "-design")
}

func lines(text []byte) []string {
	s := strings.ReplaceAll(string(text), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(s, "\n")
}

func clean(s string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(s), "`*_"))
}

// ParseSpec reads title, dependencies, branch and "## " headings.
func ParseSpec(file string, text []byte) Spec {
	s := Spec{Topic: Topic(file)}
	gotDep, gotBranch := false, false
	for _, l := range lines(text) {
		switch {
		case strings.HasPrefix(l, "# ") && s.Title == "":
			s.Title = strings.TrimSpace(l[2:])
		case strings.HasPrefix(l, "## "):
			s.Headings = append(s.Headings, strings.TrimSpace(l[3:]))
		default:
			if m := dependsLine.FindStringSubmatch(l); m != nil && !gotDep {
				gotDep = true
				for _, p := range strings.Split(m[1], ",") {
					if p = clean(p); p != "" {
						s.DependsOn = append(s.DependsOn, p)
					}
				}
			} else if m := branchLine.FindStringSubmatch(l); m != nil && !gotBranch {
				gotBranch = true
				s.Branch = clean(m[1])
			}
		}
	}
	if s.Branch == "" {
		s.Branch = s.Topic
	}
	return s
}

// ParsePlan reads the plan title and its tasks.
func ParsePlan(file string, text []byte) Plan {
	p := Plan{Topic: Topic(file)}
	type raw struct {
		task Task
		body []string
	}
	var raws []*raw
	var cur *raw
	for _, l := range lines(text) {
		if strings.HasPrefix(l, "### ") || strings.HasPrefix(l, "## ") {
			cur = nil
			if m := taskHeading.FindStringSubmatch(l); m != nil {
				n, _ := strconv.Atoi(m[2])
				t := Task{N: len(raws) + 1, Title: strings.TrimSpace(m[3])}
				if m[1] == "#" {
					t.Issue = n
				}
				cur = &raw{task: t}
				raws = append(raws, cur)
			}
			continue
		}
		if cur != nil {
			cur.body = append(cur.body, l)
			continue
		}
		if strings.HasPrefix(l, "# ") && p.Title == "" {
			p.Title = strings.TrimSpace(l[2:])
		}
	}
	byIssue := map[int]int{}
	for _, r := range raws {
		if r.task.Issue != 0 {
			byIssue[r.task.Issue] = r.task.N
		}
	}
	for _, r := range raws {
		t := r.task
		t.Body = strings.TrimSpace(strings.Join(r.body, "\n"))
		hasDep := false
		for _, l := range r.body {
			if m := stepLine.FindStringSubmatch(l); m != nil {
				t.StepsTotal++
				if m[1] != " " {
					t.StepsDone++
				}
			}
			if m := dependsLine.FindStringSubmatch(l); m != nil && !hasDep {
				hasDep = true
				for _, ref := range taskRef.FindAllStringSubmatch(m[1], -1) {
					n, _ := strconv.Atoi(ref[2])
					pos := n
					if ref[1] == "#" {
						pos = byIssue[n]
					}
					if pos >= 1 && pos <= len(raws) && pos != t.N && !hasInt(t.DependsOn, pos) {
						t.DependsOn = append(t.DependsOn, pos)
					}
				}
			}
		}
		if !hasDep && t.N > 1 {
			t.DependsOn = []int{t.N - 1}
		}
		p.Tasks = append(p.Tasks, t)
	}
	return p
}

func hasInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
