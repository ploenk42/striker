package status

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
)

type fakeRunner map[string]any // joined args -> string output or error

func (f fakeRunner) Run(args ...string) ([]byte, error) {
	v, ok := f[strings.Join(args, " ")]
	if !ok {
		return nil, errors.New("unexpected: " + strings.Join(args, " "))
	}
	if err, isErr := v.(error); isErr {
		return nil, err
	}
	return []byte(v.(string)), nil
}

var now = time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

func alphaFleet() fleet.Fleet {
	return fleet.Fleet{
		Repos:     []fleet.Repo{{Name: "alpha", Title: "Alpha", Machine: "remote1", Path: "/p/alpha", Space: "alpha"}},
		StatusDir: map[string]string{"remote1": "/s"},
	}
}

const alphaLs = `{"terminals":[{"address":"remote1/t3","cwd":"/p/alpha","where":"alpha / 1"}]}`
const alphaGet = `{"address":"remote1/t3","cwd":"/p/alpha","labels":{},"status":{"state":"working","msg":"go"},"status_ms":7}`

func TestBuildFleetRepoWithLead(t *testing.T) {
	r := fakeRunner{
		"ls --label striker.lead=alpha --json": alphaLs,
		"get remote1/t3 --json":               alphaGet,
		"file read --on remote1 /s/alpha.json": `{"phase":"plan"}`,
		"ls --json":                           `{"terminals":[]}`,
	}
	rep := Build(hc.Client{R: r}, alphaFleet(), t.TempDir(), now)
	if rep.Updated != "2026-11-21T10:00:00Z" {
		t.Fatalf("updated %q", rep.Updated)
	}
	got := rep.Repos[0]
	if got.Lead == nil || got.Lead.Address != "remote1/t3" || got.Lead.State == nil || *got.Lead.State != "working" || *got.Lead.Msg != "go" || got.Lead.Where != "alpha / 1" || *got.Lead.StatusMs != 7 {
		t.Fatalf("lead %+v", got.Lead)
	}
	if got.Machine == nil || *got.Machine != "remote1" || got.Title != "Alpha" {
		t.Fatalf("repo %+v", got)
	}
	if !strings.Contains(string(got.Status), "plan") {
		t.Fatalf("status %s", got.Status)
	}
}

func TestBuildDiscoversLocalLead(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".striker", "status")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "gamma.json"), []byte(`{"phase":"build"}`), 0o644)
	r := fakeRunner{
		"ls --json":     `{"terminals":[{"address":"t5","cwd":"/x","where":"gamma / 1"}]}`,
		"get t5 --json": `{"address":"t5","cwd":"/home/you/gamma","labels":{"striker.lead":"gamma"},"status":{"state":"idle","msg":""},"status_ms":1}`,
	}
	rep := Build(hc.Client{R: r}, fleet.Fleet{}, home, now)
	if len(rep.Repos) != 1 {
		t.Fatalf("repos %+v", rep.Repos)
	}
	g := rep.Repos[0]
	if g.Name != "gamma" || g.Machine != nil || g.Space != "gamma" || g.Path != "/home/you/gamma" {
		t.Fatalf("repo %+v", g)
	}
	if g.Lead == nil || g.Lead.Address != "t5" || !strings.Contains(string(g.Status), "build") {
		t.Fatalf("repo %+v status %s", g, g.Status)
	}
}

func TestBuildUnreachableMachine(t *testing.T) {
	home := t.TempDir()
	r := fakeRunner{
		"ls --label striker.lead=alpha --json": errors.New("unreachable"),
		"file read --on remote1 /s/alpha.json": errors.New("unreachable"),
		"ls --json":                           `{"terminals":[{"address":"t5","where":"gamma / 1"}]}`,
		"get t5 --json":                       `{"address":"t5","cwd":"/c","labels":{"striker.lead":"gamma"}}`,
	}
	rep := Build(hc.Client{R: r}, alphaFleet(), home, now)
	if len(rep.Repos) != 2 {
		t.Fatalf("repos %+v", rep.Repos)
	}
	if rep.Repos[0].Name != "alpha" || rep.Repos[0].Lead != nil {
		t.Fatalf("alpha %+v", rep.Repos[0])
	}
	if !strings.Contains(string(rep.Repos[0].Status), `"error"`) {
		t.Fatalf("status %s", rep.Repos[0].Status)
	}
	if rep.Repos[1].Name != "gamma" || rep.Repos[1].Lead == nil {
		t.Fatalf("local %+v", rep.Repos[1])
	}
}

func TestBuildStatusFileBOMAndInvalid(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"\xEF\xBB\xBF" + `{"phase":"x"}`, `{"phase":"x"}`},
		{`{nope`, `{"error":"Statusdatei ist kein gültiges JSON"}`},
	} {
		r := fakeRunner{
			"ls --label striker.lead=alpha --json": alphaLs,
			"get remote1/t3 --json":               alphaGet,
			"file read --on remote1 /s/alpha.json": tc.body,
			"ls --json":                           `{"terminals":[]}`,
		}
		rep := Build(hc.Client{R: r}, alphaFleet(), t.TempDir(), now)
		if string(rep.Repos[0].Status) != tc.want {
			t.Fatalf("got %s want %s", rep.Repos[0].Status, tc.want)
		}
	}
}

func TestBuildNoDuplicateLocalLead(t *testing.T) {
	r := fakeRunner{
		"ls --label striker.lead=alpha --json": alphaLs,
		"get remote1/t3 --json":               alphaGet,
		"file read --on remote1 /s/alpha.json": `{}`,
		"ls --json":                           `{"terminals":[{"address":"t9","where":"alpha / 2"}]}`,
		"get t9 --json":                       `{"address":"t9","cwd":"/c","labels":{"striker.lead":"alpha"}}`,
	}
	rep := Build(hc.Client{R: r}, alphaFleet(), t.TempDir(), now)
	if len(rep.Repos) != 1 {
		t.Fatalf("repos %+v", rep.Repos)
	}
}

func TestBuildLeadWithNullStatus(t *testing.T) {
	r := fakeRunner{
		"ls --label striker.lead=alpha --json": alphaLs,
		"get remote1/t3 --json":               `{"address":"remote1/t3","labels":{},"status":null,"status_ms":null}`,
		"file read --on remote1 /s/alpha.json": `{}`,
		"ls --json":                           `{"terminals":[]}`,
	}
	rep := Build(hc.Client{R: r}, alphaFleet(), t.TempDir(), now)
	b, _ := json.Marshal(rep.Repos[0].Lead)
	if !strings.Contains(string(b), `"state":null`) || !strings.Contains(string(b), `"msg":null`) {
		t.Fatalf("lead %s", b)
	}
}

func coordRunner(statusBody any) fakeRunner {
	return fakeRunner{
		"ls --label striker.lead=alpha --json":        alphaLs,
		"get remote1/t3 --json":                       alphaGet,
		"file read --on remote1 /s/alpha.json":        `{}`,
		"ls --json":                                   `{"terminals":[{"address":"remote1/t3"},{"address":"remote1/t7","where":"alpha-omega / coord omega"}]}`,
		"get remote1/t7 --json":                       `{"address":"remote1/t7","cwd":"/srv/alpha-omega","labels":{"striker.coordinator":"omega","striker.repo":"alpha"},"status":{"state":"blocked","msg":""}}`,
		"file read --on remote1 /s/alpha--omega.json": statusBody,
	}
}

func TestBuildFindsCoordinators(t *testing.T) {
	rep := Build(hc.Client{R: coordRunner(`{"change":"omega","needs_you":[{"kind":"frage","text":"?"}]}`)}, alphaFleet(), t.TempDir(), now)
	if len(rep.Repos) != 1 {
		t.Fatalf("repos %+v", rep.Repos)
	}
	cs := rep.Repos[0].Coordinators
	if len(cs) != 1 || cs[0].Change != "omega" || cs[0].Address != "remote1/t7" || cs[0].State != "blocked" {
		t.Fatalf("coordinators %+v", cs)
	}
	if !strings.Contains(string(cs[0].Status), `"needs_you"`) {
		t.Fatalf("status %s", cs[0].Status)
	}
	b, _ := json.Marshal(rep.Repos[0])
	if !strings.Contains(string(b), `"coordinators":[{"change":"omega","address":"remote1/t7","state":"blocked","status":{`) {
		t.Fatalf("json %s", b)
	}
}

func TestCoordinatorStatusInvalid(t *testing.T) {
	for _, body := range []any{`{nope`, errors.New("no such file")} {
		rep := Build(hc.Client{R: coordRunner(body)}, alphaFleet(), t.TempDir(), now)
		cs := rep.Repos[0].Coordinators
		if len(cs) != 1 || cs[0].Change != "omega" {
			t.Fatalf("coordinator not listed: %+v", cs)
		}
		var v map[string]any
		if err := json.Unmarshal(cs[0].Status, &v); err != nil || v["error"] == nil {
			t.Fatalf("status %s, want {\"error\":…}", cs[0].Status)
		}
	}
}

func TestCoordinatorsNeverNull(t *testing.T) {
	r := fakeRunner{
		"ls --label striker.lead=alpha --json": alphaLs,
		"get remote1/t3 --json":               alphaGet,
		"file read --on remote1 /s/alpha.json": `{}`,
		"ls --json":                           `{"terminals":[]}`,
	}
	b, _ := json.Marshal(Build(hc.Client{R: r}, alphaFleet(), t.TempDir(), now).Repos[0])
	if !strings.Contains(string(b), `"coordinators":[]`) {
		t.Fatalf("json %s", b)
	}
}

func TestSpacePathFromFleet(t *testing.T) {
	home := t.TempDir()
	r := fakeRunner{
		"ls --label striker.lead=alpha --json": alphaLs,
		"get remote1/t3 --json":               alphaGet,
		"file read --on remote1 /s/alpha.json": `{}`,
		"ls --json":                           `{"terminals":[{"address":"t5","where":"herdr-mods / 1"}]}`,
		"get t5 --json":                       `{"address":"t5","cwd":"/Users/p/herdr-mods","labels":{"striker.lead":"herdr-mods"}}`,
	}
	rep := Build(hc.Client{R: r}, alphaFleet(), home, now)
	if len(rep.Repos) != 2 || rep.Repos[0].SpacePath != "/p/alpha" || rep.Repos[1].SpacePath != "/Users/p/herdr-mods" {
		t.Fatalf("repos %+v", rep.Repos)
	}
	b, _ := json.Marshal(rep.Repos[0])
	if !strings.Contains(string(b), `"space_path":"/p/alpha"`) {
		t.Fatalf("json %s", b)
	}
}

func TestLocalCoordinatorStatusFile(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".striker", "status")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "herdr-mods--repo-overview.json"), []byte(`{"change":"repo-overview"}`), 0o644)
	r := fakeRunner{
		"ls --json":     `{"terminals":[{"address":"t5"},{"address":"t6"},{"address":"t8"}]}`,
		"get t5 --json": `{"address":"t5","cwd":"/Users/p/herdr-mods","labels":{"striker.lead":"herdr-mods"}}`,
		"get t6 --json": `{"address":"t6","cwd":"/Users/p/herdr-mods-repo-overview","labels":{"striker.coordinator":"repo-overview","striker.repo":"herdr-mods"},"status":{"state":"working"}}`,
		"get t8 --json": `{"address":"t8","labels":{"striker.coordinator":"../x","striker.repo":"herdr-mods"}}`,
	}
	rep := Build(hc.Client{R: r}, fleet.Fleet{}, home, now)
	cs := rep.Repos[0].Coordinators
	if len(cs) != 1 || cs[0].Address != "t6" || string(cs[0].Status) != `{"change":"repo-overview"}` {
		t.Fatalf("coordinators %+v", cs)
	}
}

func TestCoordinatorWithoutRepoEntryKept(t *testing.T) {
	r := fakeRunner{
		"ls --label striker.lead=alpha --json":     alphaLs,
		"get remote1/t3 --json":                   alphaGet,
		"file read --on remote1 /s/alpha.json":     `{}`,
		"ls --json":                               `{"terminals":[{"address":"remote1/t7"}]}`,
		"get remote1/t7 --json":                   `{"address":"remote1/t7","labels":{"striker.coordinator":"x","striker.repo":"beta"},"status":{"state":"idle"}}`,
		"file read --on remote1 /s/beta--x.json": `{"change":"x"}`,
	}
	rep := Build(hc.Client{R: r}, alphaFleet(), t.TempDir(), now)
	if len(rep.Repos) != 2 {
		t.Fatalf("repos %+v", rep.Repos)
	}
	b := rep.Repos[1]
	if b.Name != "beta" || b.Lead != nil || b.Machine == nil || *b.Machine != "remote1" || len(b.Coordinators) != 1 || b.Coordinators[0].Change != "x" {
		t.Fatalf("beta %+v", b)
	}
}

func TestReadCoordMissingLocal(t *testing.T) {
	got := ReadCoord(t.TempDir(), hc.Client{R: fakeRunner{}}, fleet.Fleet{}, "", "herdr-mods", "x")
	if !strings.Contains(string(got), `"error"`) {
		t.Fatalf("got %s", got)
	}
	if got := ReadCoord(t.TempDir(), hc.Client{R: fakeRunner{}}, fleet.Fleet{}, "", "a/b", "x"); !strings.Contains(string(got), `"error"`) {
		t.Fatalf("unsafe repo: %s", got)
	}
}

func TestCoordNeeds(t *testing.T) {
	n, tasks := CoordNeeds([]byte(`{"needs_you":[{"kind":"frage","text":"a","task":"2.3"},{"text":"b"},{"text":"c","task":" 4 "}]}`))
	if n != 3 || len(tasks) != 2 || tasks[0] != "2.3" || tasks[1] != "4" {
		t.Fatalf("n=%d tasks=%q", n, tasks)
	}
	if n, tasks := CoordNeeds([]byte(`{"error":"x"}`)); n != 0 || tasks != nil {
		t.Fatalf("error status: %d %q", n, tasks)
	}
}

func TestForFleetRepo(t *testing.T) {
	r := coordRunner(`{"change":"omega"}`)
	r["file read --on remote1 /s/alpha.json"] = `{"task":"Task 2 of omega","phase":"implement"}`
	got := For(hc.Client{R: r}, alphaFleet(), t.TempDir(), "remote1", "/p/alpha/")
	if got.Name != "alpha" || got.Lead == nil || got.Lead.Address != "remote1/t3" || *got.Lead.State != "working" {
		t.Fatalf("repo %+v", got)
	}
	if task, state := LeadTask(got); task != "Task 2 of omega" || state != "working" {
		t.Fatalf("lead task %q state %q", task, state)
	}
	if len(got.Coordinators) != 1 || got.Coordinators[0].Address != "remote1/t7" || got.Coordinators[0].State != "blocked" {
		t.Fatalf("coordinators %+v", got.Coordinators)
	}
	// LeadAt alone reads no coordinators.
	if l := LeadAt(hc.Client{R: r}, alphaFleet(), t.TempDir(), "remote1", "/p/alpha"); l.Lead == nil || len(l.Coordinators) != 0 {
		t.Fatalf("lead at %+v", l)
	}
}

func TestForLocalLeadByCwd(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".striker", "status")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "herdr-mods.json"), []byte(`{"task":"Task 5 of repo-overview"}`), 0o644)
	r := fakeRunner{
		"ls --json":             `{"terminals":[{"address":"t5","where":"herdr-mods / claude"},{"address":"t6"},{"address":"remote1/t1"}]}`,
		"get t5 --json":         `{"address":"t5","cwd":"/Users/p/herdr-mods","labels":{"striker.lead":"herdr-mods"},"status":{"state":"working","msg":""}}`,
		"get t6 --json":         `{"address":"t6","cwd":"/Users/p/herdr-mods-x","labels":{"striker.coordinator":"x","striker.repo":"herdr-mods"},"status":{"state":"idle"}}`,
		"get remote1/t1 --json": `{"address":"remote1/t1","cwd":"/Users/p/herdr-mods","labels":{"striker.lead":"other","striker.coordinator":"y","striker.repo":"herdr-mods"}}`,
	}
	got := For(hc.Client{R: r}, fleet.Fleet{}, home, "", "/Users/p/herdr-mods")
	if got.Name != "herdr-mods" || got.Lead == nil || got.Lead.Address != "t5" || got.Lead.Where != "herdr-mods / claude" {
		t.Fatalf("repo %+v", got)
	}
	if task, state := LeadTask(got); task != "Task 5 of repo-overview" || state != "working" {
		t.Fatalf("lead task %q state %q", task, state)
	}
	// Only coordinators on the repo's machine count.
	if len(got.Coordinators) != 1 || got.Coordinators[0].Change != "x" || !strings.Contains(string(got.Coordinators[0].Status), "error") {
		t.Fatalf("coordinators %+v", got.Coordinators)
	}
}

type countRunner struct {
	r     fakeRunner
	calls map[string]int
}

func (c countRunner) Run(args ...string) ([]byte, error) {
	c.calls[strings.Join(args, " ")]++
	return c.r.Run(args...)
}

// For lists and reads the terminals once for both the lead and the
// coordinators.
func TestForOneTerminalPass(t *testing.T) {
	r := countRunner{r: fakeRunner{
		"ls --json":     `{"terminals":[{"address":"t5"},{"address":"t6"}]}`,
		"get t5 --json": `{"address":"t5","cwd":"/Users/p/herdr-mods","labels":{"striker.lead":"herdr-mods"},"status":{"state":"working","msg":""}}`,
		"get t6 --json": `{"address":"t6","cwd":"/x","labels":{"striker.coordinator":"x","striker.repo":"herdr-mods"},"status":{"state":"idle"}}`,
	}, calls: map[string]int{}}
	got := For(hc.Client{R: r}, fleet.Fleet{}, t.TempDir(), "", "/Users/p/herdr-mods")
	if got.Lead == nil || len(got.Coordinators) != 1 {
		t.Fatalf("repo %+v", got)
	}
	for k, n := range r.calls {
		if n != 1 {
			t.Errorf("%s called %d times", k, n)
		}
	}
}

func TestForNoLead(t *testing.T) {
	r := fakeRunner{"ls --json": `{"terminals":[]}`}
	got := For(hc.Client{R: r}, fleet.Fleet{}, t.TempDir(), "", "/Users/p/herdr-mods")
	if got.Name != "herdr-mods" || got.Lead != nil || got.Status != nil || got.Coordinators == nil {
		t.Fatalf("repo %+v", got)
	}
	if task, state := LeadTask(got); task != "" || state != "" {
		t.Fatalf("lead task %q state %q", task, state)
	}
	// hc down: still an answer.
	if got := For(hc.Client{R: fakeRunner{}}, fleet.Fleet{}, t.TempDir(), "", "/x/y"); got.Name != "y" || got.Coordinators == nil {
		t.Fatalf("hc down %+v", got)
	}
}

func TestBuildSkipsUnsafeLocalLeadName(t *testing.T) {
	r := fakeRunner{
		"ls --json":     `{"terminals":[{"address":"t5"},{"address":"t6"}]}`,
		"get t5 --json": `{"address":"t5","labels":{"striker.lead":"../x"},"status":null}`,
		"get t6 --json": `{"address":"t6","labels":{"striker.lead":"a/b"},"status":null}`,
	}
	rep := Build(hc.Client{R: r}, fleet.Fleet{}, t.TempDir(), now)
	if len(rep.Repos) != 0 {
		t.Fatalf("repos %+v", rep.Repos)
	}
}
