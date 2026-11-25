package overview

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/openspec"
	"github.com/ploenk42/striker/striker/internal/repoview"
	"github.com/ploenk42/striker/striker/internal/status"
)

func ids(items []Item) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.ID+"#"+it.Anchor)
	}
	return out
}

func TestRegisterOpenItems(t *testing.T) {
	text := "# REGISTER\n\n## 12 Not used {#12}\n\n### OI-99 Not an open item\n\n" +
		"## 13 Open items {#13}\n\n*Kind: checkpoints not yet met.*\n\n" +
		"### OI-29 Is a sensor's reading the net's firing?\n\nText.\n\n" +
		"### ~~OI-28~~ Dissolved remnant\n\n" +
		"### OI-31 Own anchor {#oi-31}\n\n" +
		"## 15 Next {#15}\n\n### OI-40 After the section\n"
	got := ParseRegister([]byte(text))
	want := []Item{
		{Source: "register", ID: "OI-29", Title: "Is a sensor's reading the net's firing?", Anchor: "oi-29-is-a-sensors-reading-the-nets-firing"},
		{Source: "register", ID: "OI-31", Title: "Own anchor", Anchor: "oi-31"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items =\n%+v\nwant\n%+v", got, want)
	}
	// Bullet entries without an own anchor point at the section.
	got = ParseRegister([]byte("## 13 Open items {#13}\n\n- **OI-29** sensor price\n- ~~**OI-28** gone~~\n"))
	if !reflect.DeepEqual(ids(got), []string{"OI-29#13"}) || got[0].Title != "sensor price" {
		t.Fatalf("bullets = %+v", got)
	}
}

func TestRegisterGermanAndConflicts(t *testing.T) {
	text := "## Offene Punkte\n\n### OP-1 Wohin mit den Logs\n\n### OP-2 Backup (erledigt)\n\n### OP-3 Port\n\nStatus: entschieden am 3.11.\n\n" +
		"## Conflict register {#14}\n\n### CR-1 Python vs C++\n\n### CR-2 Resolved: clocks\n"
	got := ParseRegister([]byte(text))
	if want := []string{"OP-1#op-1-wohin-mit-den-logs", "CR-1#cr-1-python-vs-c"}; !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("items = %v (%+v), want %v", ids(got), got, want)
	}
}

// A trimmed excerpt of beta's real REGISTER: entries are table rows.
const betaRegister = "## 13 Open items {#13}\n\n*Kind: checkpoints not yet met.*\n\nWhat the law demands.\n\n" +
	"| ID | Owed under | Item |\n|---|---|---|\n" +
	"| **OI-10**{#OI-10} | — | *Retired 2026-10-18*: it rested on rules struck. The number is retired |\n" +
	"| **OI-12**{#OI-12} | [REQ-PRC-063](SPEC.md#REQ-PRC-063) | **What drives the pulse rate.** That is scaffolding and needs its retirement condition |\n" +
	"| **OI-13**{#OI-13} | — | *Dissolved 2026-09-13*: ruled for the branch |\n" +
	"| **OI-26**{#OI-26} | x | **A net that falls silent for good.** a gate that does not close for good |\n" +
	"| **OI-28**{#OI-28} | — | *Dissolved 2026-11-15*: ruled for a remnant |\n" +
	"| **OI-29**{#OI-29} | [REQ-SUB-171](SUBSTRATE.md#REQ-SUB-171) | **Is a sensor's reading the net's firing?** Input neurons are neurons |\n" +
	"| **OI-30**{#OI-30} | [REQ-PRC-043](SPEC.md#REQ-PRC-043) | **What a deployed viewer shows.** The image serves the viewer |\n\n---\n\n" +
	"## 14 Conflict register {#14}\n\n### 14.1 Conflicts within the document {#14.1}\n\nTwo statements.\n\n" +
	"| ID | Between | Conflict | Ruling |\n|---|---|---|---|\n" +
	"| **CR-03**{#CR-03} | `HW-1.md` [§11](HW-1.md#11) vs `PROCESS.md` [§9](PROCESS.md#9) | The toolchain is C++/ROS 2; the simulated one is Python | Applicability: not a defect but a configuration split. |\n" +
	"| **CR-10**{#CR-10} | — | *Retired 2026-10-18*: one side was struck | — |\n\n" +
	"### 14.2 Conflicts between measurement and the document {#14.2}\n\nOnly three rulings are permitted:\n\n" +
	"- **The requirement is corrected** — it demanded something.\n- **The measurement is found flawed** — named.\n\n" +
	"| ID | Requirement | Measurement and harness | Ruling |\n|---|---|---|---|\n" +
	"| **MC-01**{#MC-01} | [REQ-SUB-190](SUBSTRATE.md#REQ-SUB-190): \"death is the largest fall\" | In a world with drain | Owed by the user |\n\n" +
	"## Appendix A — Legacy resolver\n\n| **OI-77**{#OI-77} | not in a register section | **x** |\n"

func TestRegisterBetaTables(t *testing.T) {
	got := ParseRegister([]byte(betaRegister))
	want := []string{"OI-12#OI-12", "OI-26#OI-26", "OI-29#OI-29", "OI-30#OI-30", "MC-01#MC-01"}
	if !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("items = %v, want %v", ids(got), want)
	}
	if got[2].Title != "Is a sensor's reading the net's firing?" || got[2].Source != "register" {
		t.Errorf("OI-29 = %+v", got[2])
	}
	if got[4].Title != `REQ-SUB-190: "death is the largest fall"` {
		t.Errorf("MC-01 title = %q", got[4].Title)
	}
}

// An entry heading whose title contains a section keyword stays an entry and
// does not move the section anchor.
func TestRegisterEntryHeadingWithKeyword(t *testing.T) {
	text := "## 13 Open items {#13}\n\n### OI-31 Conflict between A and B\n\n### OI-32 Next\n\n" +
		"## 14 Rest\n\n## 15 Open items {#15}\n\n- **OI-33** bullet\n\n### Conflicts within {#15.1}\n\n- **CR-1** sub\n"
	got := ParseRegister([]byte(text))
	want := []string{"OI-31#oi-31-conflict-between-a-and-b", "OI-32#oi-32-next", "OI-33#15", "CR-1#15.1"}
	if !reflect.DeepEqual(ids(got), want) || got[0].Title != "Conflict between A and B" {
		t.Fatalf("items = %v (%+v), want %v", ids(got), got, want)
	}
}

// Controller ruling 13: an entry carrying a ruling/decision is decided; one
// whose ruling is still owed stays open.
func TestRegisterRulings(t *testing.T) {
	text := "## 14 Conflict register {#14}\n\n" +
		"| ID | Between | Conflict | Ruling |\n|---|---|---|---|\n" +
		"| **CR-03**{#CR-03} | A vs B | C++ vs Python | Applicability: not a defect. |\n" +
		"| **CR-04**{#CR-04} | A vs C | x | — |\n" +
		"| **MC-01**{#MC-01} | REQ-1 | measured | Owed by the user |\n\n" +
		"## Offene Punkte\n\n### OP-4 Logs\n\nEntscheidung: Loki bleibt.\n\n" +
		"### OP-5 Port\n\n- **Ruling:** owed\n\n### OP-6 Backup\n\n- **Decision:** nightly\n"
	got := ParseRegister([]byte(text))
	want := []string{"CR-04#CR-04", "MC-01#MC-01", "OP-5#op-5-port"}
	if !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("items = %v, want %v", ids(got), want)
	}
}

func TestContextVisionFirst(t *testing.T) {
	files := map[string]string{
		"docs/VISION.md": "---\nstatus: x\n---\n# Beta vision\n\n![badge](b.svg)\n\nA robot that learns\nby itself.\n\nSecond paragraph.\n",
		"docs/SPEC.md":   "# Spec\n\nspec text\n",
		"README.md":      "# herdr-mods\n\nMods for Herdr.\n",
	}
	read := func(f string) ([]byte, error) {
		if s, ok := files[f]; ok {
			return []byte(s), nil
		}
		return nil, errors.New("missing")
	}
	got := FindContext(read)
	if want := (&Context{File: "docs/VISION.md", Title: "Beta vision", Lead: "A robot that learns by itself."}); !reflect.DeepEqual(got, want) {
		t.Fatalf("vision: %+v", got)
	}
	delete(files, "docs/VISION.md")
	if got := FindContext(read); got == nil || got.File != "docs/SPEC.md" || got.Lead != "spec text" {
		t.Fatalf("spec: %+v", got)
	}
	delete(files, "docs/SPEC.md")
	if got := FindContext(read); got == nil || got.File != "README.md" || got.Title != "herdr-mods" || got.Lead != "Mods for Herdr." {
		t.Fatalf("readme: %+v", got)
	}
	// A file without a heading is titled by its name.
	if c := ContextFrom([]byte("Just text.\n"), "README.md"); c.Title != "README.md" || c.Lead != "Just text." {
		t.Fatalf("no heading: %+v", c)
	}
	// Markdown marks and links are dropped from title and lead.
	c := ContextFrom([]byte("# SPEC — **Beta** (`docs/SPEC.md`) {#top}\n\nSee [§9](PROCESS.md#9) and **this**.\n"), "docs/SPEC.md")
	if c.Title != "SPEC — Beta (docs/SPEC.md)" || c.Lead != "See §9 and this." {
		t.Fatalf("markdown: %+v", c)
	}
	delete(files, "README.md")
	if got := FindContext(read); got != nil {
		t.Fatalf("none: %+v", got)
	}
	o := Build(repoview.View{}, Parts{Has: true}, status.Repo{})
	if o.Context != nil || !contains(o.Warnings, "NO VISION FILE") {
		t.Fatalf("overview without context: %+v", o)
	}
	o = Build(repoview.View{}, Parts{Has: true, Context: &Context{File: "README.md"}}, status.Repo{})
	if contains(o.Warnings, "NO VISION FILE") {
		t.Fatalf("warning with context: %v", o.Warnings)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func TestOverviewLegacyRepo(t *testing.T) {
	v := repoview.View{Plans: []repoview.PlanView{
		{Topic: "striker-go-bundle", Title: "STRIKER GO", State: "active", Spec: "docs/superpowers/specs/a-design.md",
			Plan: "docs/superpowers/plans/a.md", Legacy: true,
			Tasks: []repoview.TaskView{{State: "done"}, {State: "todo"}}},
	}}
	o := Build(v, Parts{Has: false, Context: &Context{File: "README.md"}}, status.Repo{})
	if !contains(o.Warnings, "NO OPENSPEC") {
		t.Errorf("warnings = %v", o.Warnings)
	}
	want := []Legacy{{Name: "striker-go-bundle", Title: "STRIKER GO", State: "active", Done: 1, Total: 2,
		Spec: "docs/superpowers/specs/a-design.md", Plan: "docs/superpowers/plans/a.md"}}
	if !reflect.DeepEqual(o.Inventory.Legacy, want) {
		t.Errorf("legacy = %+v", o.Inventory.Legacy)
	}
	if len(o.Inventory.Changes) != 0 || len(o.Inventory.Archived) != 0 {
		t.Errorf("changes %+v archived %+v", o.Inventory.Changes, o.Inventory.Archived)
	}
}

func TestOverviewProgressText(t *testing.T) {
	addr, state := "remote1/t13", "idle"
	live := status.Repo{Lead: &status.Lead{Address: addr, State: &state},
		Status: json.RawMessage(`{"task":"GDELT","phase":"betrieb","progress":"Archiv bis 21.08.","note":"n","updated":"2026-11-15T21:24:58Z","needs_you":[{"kind":"frage","text":"Ja?"}]}`)}
	o := Build(repoview.View{}, Parts{Has: true}, live)
	b := o.Briefing
	if b.Task != "GDELT" || b.Phase != "betrieb" || b.Note != "n" || b.Updated != "2026-11-15T21:24:58Z" ||
		b.Address == nil || *b.Address != addr || b.State == nil || *b.State != "idle" {
		t.Fatalf("briefing = %+v", b)
	}
	if string(b.Progress) != `"Archiv bis 21.08."` {
		t.Errorf("progress = %s", b.Progress)
	}
	if len(o.OpenItems) != 1 || o.OpenItems[0] != (Item{Source: "lead", Title: "Ja?", Kind: "frage"}) {
		t.Errorf("open items = %+v", o.OpenItems)
	}
	live.Status = json.RawMessage(`{"progress":{"done":4,"total":7}}`)
	if p := Build(repoview.View{}, Parts{Has: true}, live).Briefing.Progress; string(p) != `{"done":4,"total":7}` {
		t.Errorf("progress object = %s", p)
	}
	live.Status = json.RawMessage(`{"progress":null}`)
	if p := Build(repoview.View{}, Parts{Has: true}, live).Briefing.Progress; p != nil {
		t.Errorf("progress null = %s", p)
	}
}

func TestOverviewArraysNotNull(t *testing.T) {
	b, err := json.Marshal(Build(repoview.View{}, Parts{}, status.Repo{}))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	keys := []string{}
	for k := range m {
		keys = append(keys, k)
	}
	for _, k := range []string{"briefing", "context", "inventory", "open_items", "warnings"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %s in %s", k, b)
		}
	}
	if len(m) != 5 {
		t.Errorf("keys = %v", keys)
	}
	s := string(b)
	for _, k := range []string{`"open_items":[]`, `"capabilities":[]`, `"changes":[]`, `"archived":[]`, `"legacy":[]`, `"commits":[]`} {
		if !strings.Contains(s, k) {
			t.Errorf("missing %s in %s", k, s)
		}
	}
	// With a change: its arrays are never null either.
	v := repoview.View{Plans: []repoview.PlanView{{Topic: "c", State: "planned"}}}
	b, _ = json.Marshal(Build(v, Parts{Has: true}, status.Repo{}))
	if !strings.Contains(string(b), `"depends_on":[]`) {
		t.Errorf("change depends_on in %s", b)
	}
}

func TestOverviewInventory(t *testing.T) {
	wt := "/w/repo-overview"
	trunk := make([]gitrepo.Commit, 8)
	for i := range trunk {
		trunk[i] = gitrepo.Commit{SHA: string(rune('a' + i))}
	}
	plans := []repoview.PlanView{
		{Topic: "repo-overview", Branch: "repo-overview", State: "in progress", Worktree: &wt, Coordinator: "t9",
			InterruptedAt: "", DependsOn: []string{"lcars-bridge"},
			Tasks: []repoview.TaskView{{State: "done"}, {State: "done"}, {State: "todo"}}},
		{Topic: "striker-go-bundle", Branch: "striker-go", State: "in progress", InterruptedAt: "7.2",
			Tasks: []repoview.TaskView{{State: "done"}, {State: "interrupted"}}},
	}
	for i := 0; i < 12; i++ { // oldest first: Build sorts newest first
		plans = append(plans, repoview.PlanView{Topic: "old" + string(rune('a'+i)), State: "done", Archived: "2026-10-" + string(rune('a'+i))})
	}
	v := repoview.View{Trunk: trunk, Plans: plans, Warnings: []string{"closed issues unavailable: x"}}
	p := Parts{Has: true, Store: openspec.Store{Capabilities: []openspec.Capability{{Path: "bridge", Requirements: 3}}},
		Context: &Context{File: "README.md"}, Register: []byte("## Open items\n\n### OI-1 x\n")}
	live := status.Repo{Coordinators: []status.Coord{
		{Change: "striker-go-bundle", Address: "t7", State: "idle", Status: json.RawMessage(`{"needs_you":[{"kind":"frage","text":"Windows ok?"}]}`)},
	}}
	o := Build(v, p, live)
	if len(o.Briefing.Commits) != 5 || o.Briefing.Commits[0].SHA != "a" {
		t.Errorf("commits = %+v", o.Briefing.Commits)
	}
	if bs, _ := json.Marshal(o.Briefing.Commits[0]); strings.Contains(string(bs), "null") {
		t.Errorf("commit json %s", bs)
	}
	if !reflect.DeepEqual(o.Inventory.Capabilities, []Capability{{Path: "bridge", Requirements: 3}}) {
		t.Errorf("capabilities = %+v", o.Inventory.Capabilities)
	}
	ch := o.Inventory.Changes
	if len(ch) != 2 {
		t.Fatalf("changes = %+v", ch)
	}
	if c := ch[0]; c.Name != "repo-overview" || c.State != "in progress" || c.Done != 2 || c.Total != 3 ||
		c.Worktree == nil || *c.Worktree != wt || c.Agent == nil || *c.Agent != "t9" ||
		!reflect.DeepEqual(c.DependsOn, []string{"lcars-bridge"}) || c.Branch != "repo-overview" {
		t.Errorf("repo-overview = %+v", c)
	}
	// The live coordinator's needs_you makes the change needs_you now.
	if c := ch[1]; c.State != "needs_you" || c.InterruptedAt != "7.2" || c.Agent == nil || *c.Agent != "t7" || c.Done != 1 || c.Total != 2 {
		t.Errorf("striker-go-bundle = %+v", c)
	}
	if len(o.Inventory.Archived) != 10 || o.Inventory.Archived[0] != (Archived{Name: "oldl", Date: "2026-10-l"}) ||
		o.Inventory.Archived[9] != (Archived{Name: "oldc", Date: "2026-10-c"}) {
		t.Errorf("archived = %+v", o.Inventory.Archived)
	}
	wantItems := []Item{
		{Source: "coordinator", ID: "striker-go-bundle", Title: "Windows ok?", Kind: "frage"},
		{Source: "register", ID: "OI-1", Title: "x", Anchor: "oi-1-x"},
	}
	if !reflect.DeepEqual(o.OpenItems, wantItems) {
		t.Errorf("open items = %+v", o.OpenItems)
	}
	if !contains(o.Warnings, "closed issues unavailable: x") || contains(o.Warnings, "NO OPENSPEC") {
		t.Errorf("warnings = %v", o.Warnings)
	}
}

// A change is live when the view says so (a task agent, the lead) or a coordinator runs it.
func TestOverviewChangeLive(t *testing.T) {
	v := repoview.View{Plans: []repoview.PlanView{
		{Topic: "a", State: "in progress", Live: true, Tasks: []repoview.TaskView{{State: "todo"}}},
		{Topic: "b", State: "in progress", Tasks: []repoview.TaskView{{State: "done"}}},
		{Topic: "c", State: "in progress", Coordinator: "t3", Tasks: []repoview.TaskView{{State: "done"}}},
		{Topic: "d", State: "in progress", Tasks: []repoview.TaskView{{State: "done"}}},
	}}
	live := status.Repo{Coordinators: []status.Coord{{Change: "d", Address: "t4", State: "working"}}}
	got := map[string]bool{}
	for _, c := range Build(v, Parts{Has: true}, live).Inventory.Changes {
		got[c.Name] = c.Live
	}
	if !reflect.DeepEqual(got, map[string]bool{"a": true, "b": false, "c": true, "d": true}) {
		t.Errorf("live = %v", got)
	}
	b, _ := json.Marshal(Change{Name: "x"})
	if !strings.Contains(string(b), `"live":false`) {
		t.Errorf("change json %s", b)
	}
}

func TestOverviewLeadStatusError(t *testing.T) {
	o := Build(repoview.View{}, Parts{Has: true, Context: &Context{}}, status.Repo{Status: json.RawMessage(`{"error":"no file"}`)})
	if !contains(o.Warnings, "LEAD STATUS: no file") {
		t.Errorf("warnings = %v", o.Warnings)
	}
}
