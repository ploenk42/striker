// Package issues creates GitHub issues for the tasks of a plan.
package issues

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/openspec"
	"github.com/ploenk42/striker/striker/internal/plans"
)

// GH runs the gh command line client in dir.
type GH interface {
	Run(dir string, args ...string) ([]byte, error)
}

var (
	headingRe = regexp.MustCompile(`^(###\s+Task\s+)(#?)(\d+)`)
	dependsRe = regexp.MustCompile(`(?i)^([\s*_>-]*depends on[\s*_]*:[\s*_]*)(.*)$`)
	refRe     = regexp.MustCompile(`(?i)(task\s+)?(#?)(\d+)`)
	urlRe     = regexp.MustCompile(`/issues/(\d+)`)
	slugDrop  = regexp.MustCompile(`[^a-z0-9 _-]+`)
)

// rewrite numbers the heading of task pos (1-based, unnumbered) as #issue and
// points the first "Depends on" line of every task at it. Everything else,
// including line endings, stays byte-identical.
func rewrite(text string, pos, issue int) string {
	ls := strings.Split(text, "\n")
	idx, cur := 0, 0
	hasDep := false
	num := "#" + strconv.Itoa(issue)
	for i, l := range ls {
		if strings.HasPrefix(l, "### ") || strings.HasPrefix(l, "## ") {
			cur = 0
			if m := headingRe.FindStringSubmatch(l); m != nil {
				idx++
				cur, hasDep = idx, false
				if idx == pos && m[2] == "" {
					ls[i] = m[1] + num + l[len(m[0]):]
				}
			}
			continue
		}
		if cur == 0 || hasDep {
			continue
		}
		if m := dependsRe.FindStringSubmatch(l); m != nil {
			hasDep = true
			val := refRe.ReplaceAllStringFunc(m[2], func(s string) string {
				r := refRe.FindStringSubmatch(s)
				if n, _ := strconv.Atoi(r[3]); r[2] == "" && n == pos {
					return num
				}
				return s
			})
			ls[i] = m[1] + val
		}
	}
	return strings.Join(ls, "\n")
}

func taskBody(t plans.Task, planRel string) string {
	return fmt.Sprintf("%s\n\nPlan: %s (Task %d)", firstParagraph(t.Body), planRel, t.N)
}

func firstParagraph(body string) string {
	p, _, _ := strings.Cut(strings.TrimSpace(body), "\n\n")
	return strings.TrimSpace(p)
}

func slug(title string) string {
	s := slugDrop.ReplaceAllString(strings.ToLower(title), "")
	return strings.Trim(strings.Join(strings.Fields(s), "-"), "-")
}

// Create opens an issue for every task without one, rewriting the plan after
// each success via write. It returns plan position -> issue for the issues it
// created (also on error). Once all exist it links the issues to the plan and
// commits the plan.
func Create(gh GH, repo gitrepo.Repo, planRel string, write func(rel string, text []byte) error) (map[int]int, error) {
	raw, err := repo.ReadWorking(planRel)
	if err != nil {
		return nil, err
	}
	plan := plans.ParsePlan(planRel, raw)
	text := string(raw)
	created := map[int]int{}
	for _, t := range plan.Tasks {
		if t.Issue != 0 {
			continue
		}
		title := t.Title
		if title == "" {
			title = "Task " + strconv.Itoa(t.N)
		}
		body := taskBody(t, planRel)
		out, err := gh.Run(repo.Dir, "issue", "create", "--title", title, "--body", body)
		if err != nil {
			return created, fmt.Errorf("task %d: %w", t.N, err)
		}
		m := urlRe.FindStringSubmatch(string(out))
		if m == nil {
			return created, fmt.Errorf("task %d: no issue number in gh output %q", t.N, strings.TrimSpace(string(out)))
		}
		n, _ := strconv.Atoi(m[1])
		created[t.N] = n
		text = rewrite(text, t.N, n)
		if err := write(planRel, []byte(text)); err != nil {
			return created, fmt.Errorf("task %d: write plan (issue #%d exists): %w", t.N, n, err)
		}
	}
	out, err := gh.Run(repo.Dir, "repo", "view", "--json", "nameWithOwner")
	if err != nil {
		return created, err
	}
	var rv struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal(out, &rv); err != nil || rv.NameWithOwner == "" {
		return created, fmt.Errorf("gh repo view: cannot read nameWithOwner")
	}
	def := repo.DefaultBranch()
	for _, t := range plan.Tasks {
		// Numbered tasks too, so a rerun after a partial failure links every
		// issue striker made. The link is appended to the issue's current body
		// (never replaces it) and only when it is not there yet; issues numbered
		// by hand (no striker "Plan:" marker) are left alone.
		n := t.Issue
		if n == 0 {
			n = created[t.N]
		}
		if n == 0 {
			continue
		}
		anchor := "task-" + strconv.Itoa(n)
		if s := slug(t.Title); s != "" {
			anchor += "-" + s
		}
		link := fmt.Sprintf("https://github.com/%s/blob/%s/%s#%s", rv.NameWithOwner, def, planRel, anchor)
		vout, err := gh.Run(repo.Dir, "issue", "view", strconv.Itoa(n), "--json", "body")
		if err != nil {
			return created, fmt.Errorf("issue #%d: %w", n, err)
		}
		var iv struct {
			Body string `json:"body"`
		}
		if err := json.Unmarshal(vout, &iv); err != nil {
			return created, fmt.Errorf("issue #%d: parse body: %w", n, err)
		}
		if strings.Contains(iv.Body, link) {
			continue
		}
		if t.Issue != 0 && !strings.Contains(iv.Body, fmt.Sprintf("Plan: %s (Task %d)", planRel, t.N)) {
			continue
		}
		nb := strings.TrimRight(iv.Body, " \t\r\n")
		if nb != "" {
			nb += "\n\n"
		}
		if _, err := gh.Run(repo.Dir, "issue", "edit", strconv.Itoa(n), "--body", nb+link); err != nil {
			return created, fmt.Errorf("issue #%d: %w", n, err)
		}
	}
	st, err := repo.R.Git(repo.Dir, "status", "--porcelain", "--", planRel)
	if err != nil {
		return created, err
	}
	if strings.TrimSpace(string(st)) == "" {
		return created, nil // plan unchanged (nothing new to commit)
	}
	if _, err := repo.R.Git(repo.Dir, "add", planRel); err != nil {
		return created, err
	}
	if _, err := repo.R.Git(repo.Dir, "commit", "-m", "plan: issues for "+plan.Topic, "--", planRel); err != nil {
		return created, err
	}
	return created, nil
}

// CreateChange opens an issue for every task of the active OpenSpec change
// (openspec/changes/<change>/tasks.md of the working tree) without one and
// writes "#n" into the task line via write after each success. It returns
// task id -> issue for the issues it created (also on error). Once all exist
// it commits tasks.md.
func CreateChange(gh GH, repo gitrepo.Repo, change string, write func(rel string, text []byte) error) (map[string]int, error) {
	st, has, err := openspec.Read(repo, "")
	if err != nil {
		return nil, err
	}
	var c *openspec.Change
	for i := range st.Changes {
		if has && st.Changes[i].Name == change && st.Changes[i].Archived == "" {
			c = &st.Changes[i]
			break
		}
	}
	if c == nil || c.TasksPath == "" {
		return nil, fmt.Errorf("no active change %q with tasks.md in openspec/changes", change)
	}
	out, err := gh.Run(repo.Dir, "repo", "view", "--json", "nameWithOwner")
	if err != nil {
		return nil, err
	}
	var rv struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal(out, &rv); err != nil || rv.NameWithOwner == "" {
		return nil, fmt.Errorf("gh repo view: cannot read nameWithOwner")
	}
	def := repo.DefaultBranch()
	text, err := repo.ReadWorking(c.TasksPath)
	if err != nil {
		return nil, err
	}
	created := map[string]int{}
	for _, t := range c.Tasks {
		if t.Issue != 0 {
			continue
		}
		title := t.Title
		if title == "" {
			title = "Task " + t.ID
		}
		body := fmt.Sprintf("Change: openspec/changes/%s (Task %s)", change, t.ID)
		if p := firstParagraph(t.Body); p != "" {
			body = p + "\n\n" + body
		}
		if c.PlanPath != "" {
			body += fmt.Sprintf("\nPlan: https://github.com/%s/blob/%s/%s#%s", rv.NameWithOwner, def, c.PlanPath, slug("Task "+t.ID+" "+t.Title))
		}
		out, err := gh.Run(repo.Dir, "issue", "create", "--title", title, "--body", body)
		if err != nil {
			return created, fmt.Errorf("task %s: %w", t.ID, err)
		}
		m := urlRe.FindStringSubmatch(string(out))
		if m == nil {
			return created, fmt.Errorf("task %s: no issue number in gh output %q", t.ID, strings.TrimSpace(string(out)))
		}
		n, _ := strconv.Atoi(m[1])
		created[t.ID] = n
		next, ok := openspec.SetIssue(text, t.ID, n)
		if !ok {
			return created, fmt.Errorf("task %s: issue #%d exists, but its line in %s was not found", t.ID, n, c.TasksPath)
		}
		text = next
		if err := write(c.TasksPath, text); err != nil {
			return created, fmt.Errorf("task %s: write %s (issue #%d exists): %w", t.ID, c.TasksPath, n, err)
		}
	}
	if len(created) == 0 {
		return created, nil
	}
	stt, err := repo.R.Git(repo.Dir, "status", "--porcelain", "--", c.TasksPath)
	if err != nil {
		return created, err
	}
	if strings.TrimSpace(string(stt)) == "" {
		return created, nil
	}
	if _, err := repo.R.Git(repo.Dir, "add", c.TasksPath); err != nil {
		return created, err
	}
	if _, err := repo.R.Git(repo.Dir, "commit", "-m", "openspec: issues for "+change, "--", c.TasksPath); err != nil {
		return created, err
	}
	return created, nil
}

var slugRe = regexp.MustCompile(`github\.com[:/]+([^/\s]+)/([^/\s]+?)(?:\.git)?/?$`)

// Slug returns "owner/repo" for a GitHub remote URL (scp-style, ssh:// or
// https), or "" for anything else.
func Slug(remoteURL string) string {
	m := slugRe.FindStringSubmatch(strings.TrimSpace(remoteURL))
	if m == nil {
		return ""
	}
	return m[1] + "/" + m[2]
}

// Closed returns the numbers of closed issues of the GitHub repo slug
// ("owner/repo"). gh runs without a directory, so it works for repos that
// live on another machine.
func Closed(gh GH, slug string) (map[int]bool, error) {
	out, err := gh.Run("", "issue", "list", "-R", slug, "--state", "closed", "--json", "number", "--limit", "1000")
	if err != nil {
		return nil, err
	}
	var list []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("gh issue list: parse: %w", err)
	}
	m := map[int]bool{}
	for _, i := range list {
		m[i.Number] = true
	}
	return m, nil
}
