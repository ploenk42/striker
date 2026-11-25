package crew

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/statusline"
)

type fake map[string]string

func (f fake) Run(args ...string) ([]byte, error) {
	if out, ok := f[strings.Join(args, " ")]; ok {
		return []byte(out), nil
	}
	return nil, errors.New("unexpected hc " + strings.Join(args, " "))
}

var now = time.UnixMilli(1795400000000)

var testFleet = fleet.Fleet{
	Repos:     []fleet.Repo{{Name: "alpha", Machine: "remote1", Path: "/srv/alpha", Space: "alpha-space"}},
	AgentsDir: map[string]string{"remote1": "/var/lib/striker/agents"},
}

func hcWith(ls string, gets map[string]string, find string) hc.Client {
	f := fake{"ls --json": ls}
	for a, g := range gets {
		f["get "+a+" --json"] = g
	}
	f["exec --on remote1 find /var/lib/striker/agents -name *.json -mmin -1440 -exec cat {} +"] = find
	return hc.Client{R: f}
}

const ls4 = `{"terminals":[
 {"address":"remote1/t18","agent":"claude-code","status":"working","where":"alpha-space / claude"},
 {"address":"remote1/t20","agent":"claude-code","status":"idle","where":"alpha-rb / coord rb"},
 {"address":"remote1/t21","agent":"claude-code","status":"done","where":"alpha-rb / implementer-2"},
 {"address":"t3","agent":"claude-code","status":"idle","where":"herdr-mods / 1"},
 {"address":"t4","agent":"powershell.exe","where":"herdr-mods / 2"}]}`

var gets4 = map[string]string{
	"remote1/t18": `{"address":"remote1/t18","created_ms":1,"labels":{"striker.lead":"alpha"}}`,
	"remote1/t20": `{"address":"remote1/t20","created_ms":2,"labels":{"striker.coordinator":"rb","striker.repo":"alpha"}}`,
	"remote1/t21": `{"address":"remote1/t21","created_ms":3,"labels":{"striker.role":"spec-reviewer","striker.repo":"alpha","striker.task":"2"}}`,
	"t3":          `{"address":"t3","created_ms":4,"labels":{}}`,
}

const find4 = `{"terminal":"t18","at":1795399990000,"claude":{"model":{"display_name":"Opus 5.5"},"context_window":{"total_input_tokens":1000000,"total_output_tokens":234567,"used_percentage":41}}}
{"terminal":"t21","at":1795399990000,"claude":{"model":{"display_name":"Sonnet 5.5"},"context_window":{"total_input_tokens":5000,"total_output_tokens":1000}}}
`

func byAddr(c Crew) map[string]Agent {
	m := map[string]Agent{}
	for _, a := range c.Agents {
		m[a.Address] = a
	}
	return m
}

func TestBuild(t *testing.T) {
	store := filepath.Join(t.TempDir(), "crew.json")
	local := t.TempDir()
	os.WriteFile(filepath.Join(local, "t3.json"), []byte(`{"terminal":"t3","at":1795399999000,"claude":{"model":{"display_name":"Fable 5.2"},"context_window":{"total_input_tokens":10,"total_output_tokens":2}}}`+"\n"), 0o644)
	c := Build(hcWith(ls4, gets4, find4), testFleet, store, local, now)
	m := byAddr(c)
	if len(m) != 4 {
		t.Fatalf("agents %+v", c.Agents)
	}
	lead, coord, rev, mine := m["remote1/t18"], m["remote1/t20"], m["remote1/t21"], m["t3"]
	if lead.Role != "lead" || lead.Rank != "LCDR" || lead.Repo != "alpha" || lead.Name != "PATA" || lead.Model != "Opus 5.5" ||
		lead.Tokens != 1234567 || lead.ContextPct != 41 || lead.Status != "working" {
		t.Errorf("lead %+v", lead)
	}
	if coord.Role != "coordinator" || coord.Rank != "LT" || coord.Repo != "alpha" || coord.Name != "WERF" || coord.Model != "" {
		t.Errorf("coordinator %+v", coord)
	}
	if rev.Role != "spec-reviewer" || rev.Rank != "LTJG" || rev.Name != "BEYMLA" || rev.Tokens != 6000 {
		t.Errorf("reviewer %+v", rev)
	}
	// no role: the model decides the rank; no fleet repo for the space: the space name
	if mine.Role != "" || mine.Rank != "LCDR" || mine.Repo != "herdr-mods" || mine.Name != "MARYNA" || mine.Model != "Fable 5.2" {
		t.Errorf("local agent %+v", mine)
	}
	for _, a := range c.Agents {
		if a.Name == "STRIKER" {
			t.Errorf("STRIKER handed out: %+v", a)
		}
	}
}

// Striker himself (the local Claude in the space STRIKER) is CDR STRIKER; a STRIKER space on another machine is not him.
func TestRikerHimself(t *testing.T) {
	ls := `{"terminals":[
 {"address":"t14","agent":"claude-code","status":"idle","where":"STRIKER / striker"},
 {"address":"remote1/t5","agent":"claude-code","status":"idle","where":"striker / claude"}]}`
	gets := map[string]string{
		"t14":        `{"address":"t14","created_ms":1,"labels":{}}`,
		"remote1/t5": `{"address":"remote1/t5","created_ms":2,"labels":{}}`,
	}
	// a lower-decks name kept from before Striker was known gives way
	store := filepath.Join(t.TempDir(), "crew.json")
	os.WriteFile(store, []byte(`{"t14":{"name":"BEYMLA","created_ms":1}}`), 0o644)
	m := byAddr(Build(hcWith(ls, gets, ""), testFleet, store, t.TempDir(), now))
	if r := m["t14"]; r.Name != "STRIKER" || r.Rank != "CDR" || r.Role != "striker" || r.Repo != "striker" {
		t.Errorf("striker %+v", r)
	}
	if o := m["remote1/t5"]; o.Name == "STRIKER" || o.Role != "" {
		t.Errorf("remote striker space %+v", o)
	}
}

// Striker in another space (herdr-mods, as its lead) is known by the label striker.self.
func TestRikerByLabel(t *testing.T) {
	ls := `{"terminals":[
 {"address":"t16","agent":"claude-code","status":"working","where":"herdr-mods / 1"},
 {"address":"t17","agent":"claude-code","status":"idle","where":"herdr-mods / 2"}]}`
	gets := map[string]string{
		"t16": `{"address":"t16","created_ms":1,"labels":{"striker.self":"1","striker.lead":"herdr-mods"}}`,
		"t17": `{"address":"t17","created_ms":2,"labels":{}}`,
	}
	m := byAddr(Build(hcWith(ls, gets, ""), testFleet, filepath.Join(t.TempDir(), "crew.json"), t.TempDir(), now))
	if r := m["t16"]; r.Name != "STRIKER" || r.Rank != "CDR" || r.Role != "striker" || r.Repo != "herdr-mods" {
		t.Errorf("striker by label %+v", r)
	}
	if o := m["t17"]; o.Name == "STRIKER" || o.Role == "striker" {
		t.Errorf("other terminal in the same space %+v", o)
	}
}

func TestNamesKeptAndFreed(t *testing.T) {
	store := filepath.Join(t.TempDir(), "crew.json")
	local := t.TempDir()
	Build(hcWith(ls4, gets4, ""), testFleet, store, local, now)
	// t18 is gone, t20 was replaced by a new terminal at the same address, t21 stays; a new lead t30 comes
	ls := `{"terminals":[
 {"address":"remote1/t20","agent":"claude-code","where":"alpha-rb / coord rb"},
 {"address":"remote1/t21","agent":"claude-code","where":"alpha-rb / implementer-2"},
 {"address":"remote1/t30","agent":"claude-code","where":"alpha-space / claude"}]}`
	gets := map[string]string{
		"remote1/t20": `{"address":"remote1/t20","created_ms":99,"labels":{"striker.coordinator":"rb","striker.repo":"alpha"}}`,
		"remote1/t21": gets4["remote1/t21"],
		"remote1/t30": `{"address":"remote1/t30","created_ms":5,"labels":{"striker.lead":"alpha"}}`,
	}
	m := byAddr(Build(hcWith(ls, gets, ""), testFleet, store, local, now))
	if m["remote1/t21"].Name != "BEYMLA" {
		t.Errorf("t21 lost its name: %+v", m["remote1/t21"])
	}
	// PATA (t18) is free again and goes to the first new bridge agent in address order: the new t20
	if m["remote1/t20"].Name != "PATA" || m["remote1/t30"].Name != "WERF" {
		t.Errorf("t20 %+v, t30 %+v", m["remote1/t20"], m["remote1/t30"])
	}
}

func TestPoolsRunOut(t *testing.T) {
	used := map[string]bool{}
	for _, n := range bridgePool {
		used[n] = true
	}
	if got := pick(true, used); got != lowerPool[0] {
		t.Errorf("bridge pool empty: got %q", got)
	}
	for _, n := range lowerPool {
		used[n] = true
	}
	if got := pick(false, used); got != "CREWMAN 1" {
		t.Errorf("both empty: got %q", got)
	}
	used["CREWMAN 1"] = true
	if got := pick(false, used); got != "CREWMAN 2" {
		t.Errorf("got %q", got)
	}
}

func TestPools(t *testing.T) {
	bridge := []string{"PATA", "WERF", "TREY", "LE FERGE", "CRUSHA", "E'BRYEN", "YEAR", "GUYNAN", "BARCLAY", "RE", "PULESKY", "EGAWA"}
	lower := []string{"BEYMLA", "MARYNA", "TENDY", "RUTHAFERD", "FREEMAN", "RANSEM", "SHAKES", "DANNA", "BYLLUPS",
		"MYGLEEME", "JENNYFA", "BARNES", "STEVENS", "KAYSHEN"}
	if strings.Join(bridgePool, ",") != strings.Join(bridge, ",") {
		t.Errorf("bridge pool %q", bridgePool)
	}
	if strings.Join(lowerPool, ",") != strings.Join(lower, ",") {
		t.Errorf("lower pool %q", lowerPool)
	}
}

func TestRank(t *testing.T) {
	cases := []struct{ role, model, want string }{
		{"striker", "", "CDR"},
		{"lead", "Haiku 5.5", "LCDR"}, {"coordinator", "", "LT"}, {"reviewer", "", "LTJG"}, {"spec-reviewer", "", "LTJG"},
		{"implementer", "Opus 5.5", "ENS"},
		{"", "Fable 5.2", "LCDR"}, {"", "Opus 5.5", "LT"}, {"", "Sonnet 5.5", "LTJG"}, {"", "Haiku 5.5", "ENS"}, {"", "", "CREW"},
	}
	for _, c := range cases {
		if got := rank(c.role, c.model); got != c.want {
			t.Errorf("rank(%q, %q) = %q, want %q", c.role, c.model, got, c.want)
		}
	}
}

func TestStaleStatusLineIgnored(t *testing.T) {
	local := t.TempDir()
	p := filepath.Join(local, "t3.json")
	os.WriteFile(p, []byte(`{"terminal":"t3","at":1,"claude":{"model":{"display_name":"Opus 5.5"}}}`+"\n"), 0o644)
	old := now.Add(-25 * time.Hour)
	os.Chtimes(p, old, old)
	es := readLocal(local, now)
	if len(es) != 0 {
		t.Errorf("stale entries read: %+v", es)
	}
	_ = statusline.Entry{}
}
