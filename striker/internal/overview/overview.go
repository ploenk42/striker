// Package overview builds the OVERVIEW of a repo for the LCARS Bridge: the
// lead's briefing, the vision context, the OpenSpec inventory and the open
// items (needs_you and the REGISTER).
package overview

import (
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/openspec"
	"github.com/ploenk42/striker/striker/internal/repoview"
	"github.com/ploenk42/striker/striker/internal/status"
)

const (
	maxCommits  = 5
	maxArchived = 10
)

// ContextFiles are searched in this order for the repo's context.
var ContextFiles = []string{"docs/VISION.md", "docs/SPEC.md", "README.md"}

// RegisterFile holds the repo's decisions and open items.
const RegisterFile = "docs/REGISTER.md"

type Overview struct {
	Briefing  Briefing  `json:"briefing"`
	Context   *Context  `json:"context"`
	Inventory Inventory `json:"inventory"`
	OpenItems []Item    `json:"open_items"`
	Warnings  []string  `json:"warnings"`
}

// Briefing is the lead's status file (task, phase, progress, note,
// updated), the lead terminal and its live state (null without a lead) and
// the last commits on the default branch. Progress is {done,total}, a
// string, or null, as the lead wrote it.
type Briefing struct {
	Address  *string          `json:"address"`
	State    *string          `json:"state"`
	Task     string           `json:"task"`
	Phase    string           `json:"phase"`
	Progress json.RawMessage  `json:"progress"`
	Note     string           `json:"note"`
	Updated  string           `json:"updated"`
	Commits  []gitrepo.Commit `json:"commits"`
}

// Context is the title and first paragraph of the repo's context file.
type Context struct {
	File  string `json:"file"`
	Title string `json:"title"`
	Lead  string `json:"lead"`
}

type Inventory struct {
	Capabilities []Capability `json:"capabilities"`
	Changes      []Change     `json:"changes"`
	Archived     []Archived   `json:"archived"`
	Legacy       []Legacy     `json:"legacy"`
}

type Capability struct {
	Path         string `json:"path"`
	Requirements int    `json:"requirements"`
}

// Change is an active OpenSpec change; Agent is its coordinator terminal.
type Change struct {
	Name          string   `json:"name"`
	State         string   `json:"state"`
	Done          int      `json:"done"`
	Total         int      `json:"total"`
	Worktree      *string  `json:"worktree"`
	Agent         *string  `json:"agent"`
	InterruptedAt string   `json:"interrupted_at"`
	DependsOn     []string `json:"depends_on"`
	Branch        string   `json:"branch"`
	// Live: someone works on the change now (coordinator, task agent, or the
	// lead working on it).
	Live bool `json:"live"`
}

type Archived struct {
	Name string `json:"name"`
	Date string `json:"date"`
}

// Legacy is a spec/plan topic of a repo without openspec/ (read-only).
type Legacy struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	State string `json:"state"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
	Spec  string `json:"spec"`
	Plan  string `json:"plan"`
}

// Item is an open item: source "lead" (no id), "coordinator" (id = change)
// or "register" (id = entry id, anchor into REGISTER.md).
type Item struct {
	Source string `json:"source"`
	ID     string `json:"id,omitempty"`
	Title  string `json:"title"`
	Anchor string `json:"anchor,omitempty"`
	Kind   string `json:"kind,omitempty"`
}

// Parts are the repo parts beyond the repo view: the OpenSpec store of the
// default branch (Has: openspec/ exists), the context and the REGISTER text.
type Parts struct {
	Store    openspec.Store
	Has      bool
	Context  *Context
	Register []byte
	Warnings []string
}

// leadStatus is the part of a lead or coordinator status file read here.
type leadStatus struct {
	Task     string          `json:"task"`
	Phase    string          `json:"phase"`
	Progress json.RawMessage `json:"progress"`
	Note     string          `json:"note"`
	Updated  string          `json:"updated"`
	Error    string          `json:"error"`
	NeedsYou []struct {
		Kind string `json:"kind"`
		Text string `json:"text"`
	} `json:"needs_you"`
}

func parseStatus(raw json.RawMessage) leadStatus {
	var s leadStatus
	if len(raw) > 0 {
		json.Unmarshal(raw, &s) // fields of the wrong type stay empty
	}
	return s
}

// progress keeps {done,total} and strings; anything else is null.
func progress(raw json.RawMessage) json.RawMessage {
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return nil
	}
	switch p := v.(type) {
	case string:
		return raw
	case map[string]any:
		_, d := p["done"].(float64)
		_, t := p["total"].(float64)
		if d && t {
			return raw
		}
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func doneTotal(ts []repoview.TaskView) (done, total int) {
	for _, t := range ts {
		if t.State == "done" {
			done++
		}
	}
	return done, len(ts)
}

// Build merges the repo view v (60 s), the repo parts p (60 s) and the
// live lead and coordinator state (10 s) into the Overview. It is pure.
func Build(v repoview.View, p Parts, live status.Repo) Overview {
	o := Overview{
		Context: p.Context,
		Inventory: Inventory{Capabilities: []Capability{}, Changes: []Change{}, Archived: []Archived{},
			Legacy: []Legacy{}},
		OpenItems: []Item{},
		Warnings:  []string{},
	}
	if !p.Has {
		o.Warnings = append(o.Warnings, "NO OPENSPEC")
	}
	if p.Context == nil {
		o.Warnings = append(o.Warnings, "NO VISION FILE")
	}

	// Briefing.
	ls := parseStatus(live.Status)
	b := Briefing{Task: ls.Task, Phase: ls.Phase, Progress: progress(ls.Progress), Note: ls.Note,
		Updated: ls.Updated, Commits: []gitrepo.Commit{}}
	if live.Lead != nil {
		a := live.Lead.Address
		b.Address = &a
		if live.Lead.State != nil {
			s := *live.Lead.State
			b.State = &s
		}
	}
	for i, c := range v.Trunk {
		if i == maxCommits {
			break
		}
		c.Parents = nonNil(c.Parents)
		if c.Refs == nil {
			c.Refs = []int{}
		}
		if c.Fixes == nil {
			c.Fixes = []int{}
		}
		b.Commits = append(b.Commits, c)
	}
	o.Briefing = b
	if ls.Error != "" {
		o.Warnings = append(o.Warnings, "LEAD STATUS: "+ls.Error)
	}

	// Inventory. The live coordinators (10 s) override the view's (60 s).
	coords := map[string]status.Coord{}
	for _, k := range live.Coordinators {
		if old, ok := coords[k.Change]; !ok || k.Address < old.Address {
			coords[k.Change] = k
		}
	}
	if p.Has {
		for _, c := range p.Store.Capabilities {
			o.Inventory.Capabilities = append(o.Inventory.Capabilities, Capability{Path: c.Path, Requirements: c.Requirements})
		}
	}
	for _, pv := range v.Plans {
		switch {
		case pv.Legacy:
			done, total := doneTotal(pv.Tasks)
			o.Inventory.Legacy = append(o.Inventory.Legacy, Legacy{Name: pv.Topic, Title: pv.Title, State: pv.State,
				Done: done, Total: total, Spec: pv.Spec, Plan: pv.Plan})
		case pv.Archived != "":
			o.Inventory.Archived = append(o.Inventory.Archived, Archived{Name: pv.Topic, Date: pv.Archived})
		default:
			done, total := doneTotal(pv.Tasks)
			c := Change{Name: pv.Topic, State: pv.State, Done: done, Total: total, Worktree: pv.Worktree,
				InterruptedAt: pv.InterruptedAt, DependsOn: append([]string{}, pv.DependsOn...), Branch: pv.Branch,
				Live: pv.Live}
			if pv.Coordinator != "" {
				a := pv.Coordinator
				c.Agent = &a
				c.Live = true
			}
			if k, ok := coords[pv.Topic]; ok {
				a := k.Address
				c.Agent = &a
				c.Live = true
				if n, _ := status.CoordNeeds(k.Status); n > 0 || k.State == "blocked" {
					c.State = "needs_you"
				}
			}
			o.Inventory.Changes = append(o.Inventory.Changes, c)
		}
	}
	sort.SliceStable(o.Inventory.Archived, func(i, j int) bool {
		return o.Inventory.Archived[i].Date > o.Inventory.Archived[j].Date
	})
	if len(o.Inventory.Archived) > maxArchived {
		o.Inventory.Archived = o.Inventory.Archived[:maxArchived]
	}

	// Open items: the lead's needs_you, each coordinator's, the REGISTER.
	for _, n := range ls.NeedsYou {
		o.OpenItems = append(o.OpenItems, Item{Source: "lead", Title: n.Text, Kind: n.Kind})
	}
	for _, k := range live.Coordinators {
		for _, n := range parseStatus(k.Status).NeedsYou {
			o.OpenItems = append(o.OpenItems, Item{Source: "coordinator", ID: k.Change, Title: n.Text, Kind: n.Kind})
		}
	}
	o.OpenItems = append(o.OpenItems, ParseRegister(p.Register)...)

	o.Warnings = append(o.Warnings, p.Warnings...)
	o.Warnings = append(o.Warnings, v.Warnings...)
	return o
}

// FindContext reads the first of ContextFiles that read returns; nil if
// none exists.
func FindContext(read func(file string) ([]byte, error)) *Context {
	for _, f := range ContextFiles {
		if b, err := read(f); err == nil {
			return ContextFrom(b, f)
		}
	}
	return nil
}

func mdLines(text []byte) []string {
	return strings.Split(strings.ReplaceAll(string(text), "\r\n", "\n"), "\n")
}

// ContextFrom takes the first "# " heading as title (else the file name)
// and the first paragraph of prose as lead. Front matter, headings, code,
// images, HTML and blockquote-free decoration lines are skipped.
func ContextFrom(text []byte, file string) *Context {
	c := &Context{File: file}
	ls := mdLines(text)
	i := 0
	if len(ls) > 0 && strings.TrimSpace(ls[0]) == "---" { // front matter
		for i = 1; i < len(ls) && strings.TrimSpace(ls[i]) != "---"; i++ {
		}
		i++
	}
	var para []string
	fence := false
	for ; i < len(ls); i++ {
		l := strings.TrimSpace(ls[i])
		switch {
		case strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~"):
			fence = !fence
			if len(para) > 0 {
				i = len(ls)
			}
			continue
		case fence:
			continue
		case l == "":
			if len(para) > 0 {
				i = len(ls)
			}
			continue
		case strings.HasPrefix(l, "#"):
			if len(para) > 0 {
				i = len(ls)
				continue
			}
			if c.Title == "" && strings.HasPrefix(l, "# ") {
				c.Title = plain(l[2:])
			}
			continue
		case len(para) == 0 && (strings.HasPrefix(l, "![") || strings.HasPrefix(l, "[![") ||
			strings.HasPrefix(l, "<") || strings.HasPrefix(l, "|") || strings.HasPrefix(l, "---")):
			continue
		}
		para = append(para, l)
	}
	if c.Title == "" {
		c.Title = path.Base(file)
	}
	c.Lead = plain(strings.Join(para, " "))
	return c
}

var (
	headRe    = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	anchorRe  = regexp.MustCompile(`\{#([^}\s]+)\}`)
	sectionRe = regexp.MustCompile(`(?i)open items|offene punkte|conflict`)
	doneRe    = regexp.MustCompile(`(?i)\b(resolved|decided|closed|erledigt|entschieden|retired|dissolved|withdrawn)\b`)
	statusRe  = regexp.MustCompile(`(?i)^[*_]*(status|stand)[*_]*\s*:`)
	idRe      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[0-9][A-Za-z0-9.]*$`)
	boldRe    = regexp.MustCompile(`\*\*(.+?)\*\*`)
	linkRe    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	bulletRe  = regexp.MustCompile(`^[-*+]\s+(.*)$`)
	// A ruling or decision field or column (controller ruling 13).
	rulingHeadRe = regexp.MustCompile(`(?i)^(ruling|entscheidung|decision|beschluss)\b`)
	rulingLineRe = regexp.MustCompile(`(?i)^[*_]*(ruling|entscheidung|decision|beschluss)[*_]*\s*:[*_]*\s*(.*)$`)
	// A ruling that is still owed is no ruling ("Owed by the user").
	pendingRe = regexp.MustCompile(`(?i)^(owed|open|offen|pending|tbd|todo|ausstehend|steht aus|\?)`)
	sepCellRe = regexp.MustCompile(`^:?-{3,}:?$`)
)

// hasRuling reports whether a ruling field or cell carries a decision: not
// empty, not a dash, not marked as still owed.
func hasRuling(s string) bool {
	p := plain(s)
	return p != "" && p != "—" && p != "-" && !pendingRe.MatchString(p)
}

func isSeparator(cs []string) bool {
	for _, c := range cs {
		if !sepCellRe.MatchString(c) {
			return false
		}
	}
	return len(cs) > 0
}

// plain strips markdown decoration: links keep their text; emphasis,
// strike and code marks go.
func plain(s string) string {
	s = anchorRe.ReplaceAllString(s, "")
	s = linkRe.ReplaceAllString(s, "$1")
	s = strings.NewReplacer("**", "", "__", "", "~~", "", "`", "").Replace(s)
	s = strings.Trim(strings.TrimSpace(s), "*_")
	return strings.TrimSpace(s)
}

// Slug is GitHub's heading anchor: lower case, punctuation dropped, spaces
// to hyphens.
func Slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

func struck(s string) bool { return strings.Contains(s, "~~") }

// startsDone reports whether a table cell or bullet text begins with a
// decision word ("*Retired 2026-10-18*: …"): prose that merely mentions
// "closed" further on stays open. Headings are matched anywhere.
func startsDone(cell string) bool {
	c := strings.TrimLeft(strings.TrimSpace(cell), "*_ ")
	m := doneRe.FindStringIndex(c)
	return m != nil && m[0] == 0
}

func anyStartsDone(cs []string) bool {
	for _, c := range cs {
		if startsDone(c) {
			return true
		}
	}
	return false
}

// rowTitle is the first bold phrase of the cells, else the first cell with
// text (a dash counts as empty).
func rowTitle(cs []string) string {
	for _, c := range cs {
		if m := boldRe.FindStringSubmatch(c); m != nil {
			return plain(m[1])
		}
	}
	for _, c := range cs {
		if p := plain(c); p != "" && p != "—" && p != "-" {
			return p
		}
	}
	return ""
}

// anchorOf is a heading's explicit {#id}, else its GitHub slug.
func anchorOf(h string) string {
	if m := anchorRe.FindStringSubmatch(h); m != nil {
		return m[1]
	}
	return Slug(plain(h))
}

// splitID cuts a leading entry id ("OI-29", "**OI-29**{#OI-29}") off s.
func splitID(s string) (id, anchor, rest string) {
	s = strings.TrimSpace(s)
	if m := anchorRe.FindStringSubmatchIndex(s); m != nil {
		anchor = s[m[2]:m[3]]
	}
	t := strings.TrimSpace(anchorRe.ReplaceAllString(s, ""))
	if strings.HasPrefix(t, "**") {
		if end := strings.Index(t[2:], "**"); end >= 0 {
			cand := strings.TrimSpace(t[2 : 2+end])
			if idRe.MatchString(cand) {
				return cand, anchor, strings.TrimSpace(t[4+end:])
			}
		}
	}
	f := strings.Fields(plain(t))
	if len(f) > 0 && idRe.MatchString(strings.TrimRight(f[0], ":")) {
		id = strings.TrimRight(f[0], ":")
		rest = strings.TrimSpace(strings.TrimPrefix(plain(t), f[0]))
		return id, anchor, rest
	}
	return "", anchor, t
}

func cells(row string) []string {
	r := strings.TrimSpace(row)
	r = strings.TrimSuffix(strings.TrimPrefix(r, "|"), "|")
	out := strings.Split(r, "|")
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

// ParseRegister returns the unresolved entries of a REGISTER: under a
// heading containing "Open items", "Offene Punkte" or "Conflict" (any case),
// until a heading of the same or a higher level that does not. An entry is
// a sub-heading (unless entry rows or bullets sit under it: then it is a
// sub-section), or a table row or top-level bullet that starts with an id
// (bold or plain, like "OI-29"). Struck-through entries
// and those marked resolved/decided/closed/erledigt/entschieden (also
// retired/dissolved/withdrawn) are left out, as are entries carrying a
// ruling (a filled Ruling/Entscheidung/Decision column, or such a field
// under an entry heading) unless it is still owed ("Owed by the user").
// A sub-heading with an id is an entry even if its title contains a
// section keyword. Anchor: the entry's explicit
// {#id}, else the heading's GitHub slug, else (rows, bullets) the section's.
func ParseRegister(text []byte) []Item {
	out := []Item{}
	if len(text) == 0 {
		return out
	}
	level := 0 // level of the open register section, 0 = none
	sectionAnchor := ""
	// cur is the position in out of the last entry heading (-1: none or
	// left out); rows or bullets under it make it a sub-section instead.
	cur := -1
	demote := func() {
		if cur >= 0 {
			out = append(out[:cur], out[cur+1:]...)
			cur = -1
		}
	}
	// header holds the cells of the current table's header row (the id-less
	// row before a |---| line).
	var header, lastRow []string
	fence := false
	for _, raw := range mdLines(text) {
		l := strings.TrimSpace(raw)
		if strings.HasPrefix(l, "```") || strings.HasPrefix(l, "~~~") {
			fence = !fence
			continue
		}
		if fence {
			continue
		}
		if m := headRe.FindStringSubmatch(l); m != nil {
			lv, h := len(m[1]), m[2]
			id, anchor, rest := splitID(h)
			header, lastRow = nil, nil
			switch {
			// A sub-heading with an id is an entry even when its title
			// contains a keyword ("### OI-31 Conflict between A and B").
			case !(level != 0 && lv > level && id != "") && sectionRe.MatchString(plain(h)):
				if level == 0 || lv <= level {
					level = lv
				}
				sectionAnchor = anchorOf(h)
				cur = -1
			case level != 0 && lv <= level:
				level, cur = 0, -1
			case level != 0:
				cur = -1
				if struck(h) || doneRe.MatchString(plain(h)) {
					break
				}
				it := Item{Source: "register", ID: id, Title: plain(rest), Anchor: anchor}
				if it.Anchor == "" {
					it.Anchor = Slug(plain(h))
				}
				cur = len(out)
				out = append(out, it)
			}
			continue
		}
		if level == 0 {
			continue
		}
		// "Ruling: …" / "Entscheidung: …" (also as a bullet) under an entry
		// heading decides it, unless the ruling is still owed.
		if cur >= 0 {
			field := l
			if b := bulletRe.FindStringSubmatch(l); b != nil {
				field = b[1]
			}
			if rm := rulingLineRe.FindStringSubmatch(field); rm != nil && hasRuling(rm[2]) {
				demote()
				continue
			}
		}
		// Rows and bullets are entries only when they start with an id
		// ("**OI-29**{#OI-29}", "OP-1"): header rows and explanatory lists
		// are not, and do not end an entry heading.
		var id, anchor, title string
		done := false
		switch {
		case strings.HasPrefix(l, "|"):
			cs := cells(l)
			if isSeparator(cs) {
				header, lastRow = lastRow, nil
				continue
			}
			id, anchor, _ = splitID(cs[0])
			if id == "" {
				lastRow = cs // maybe the header, if a separator follows
				continue
			}
			title = rowTitle(cs[1:])
			done = struck(cs[0]) || anyStartsDone(cs[1:])
			// A filled Ruling/Entscheidung column decides the entry.
			for i, hcell := range header {
				if i > 0 && i < len(cs) && rulingHeadRe.MatchString(plain(hcell)) && hasRuling(cs[i]) {
					done = true
				}
			}
		case bulletRe.MatchString(l) && raw == strings.TrimLeft(raw, " \t"):
			body := bulletRe.FindStringSubmatch(l)[1]
			var rest string
			id, anchor, rest = splitID(body)
			title = plain(rest)
			done = struck(body) || startsDone(body) || startsDone(rest)
		case cur >= 0 && statusRe.MatchString(l) && doneRe.MatchString(l):
			demote() // "Status: decided …" under an entry heading
			continue
		default:
			continue
		}
		if id == "" {
			continue
		}
		demote()
		if done {
			continue
		}
		if anchor == "" {
			anchor = sectionAnchor
		}
		out = append(out, Item{Source: "register", ID: id, Title: title, Anchor: anchor})
	}
	return out
}
