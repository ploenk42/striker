package relay

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ploenk42/striker/striker/internal/dispatch"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/openspec"
)

// as is the prefix of git and file reads as awayteam on remote1 (coord's
// userGit through fleet.Sudo).
const as = "exec --on remote1 --cwd / sudo -n -u awayteam -i "

var (
	leadFile  = File{Machine: "remote1", Path: "/var/lib/striker/status/alpha.json", Repo: "alpha"}
	coordFile = File{Machine: "remote1", Path: "/var/lib/striker/status/alpha--rb.json", Repo: "alpha", Change: "rb"}
)

func newLedger(t *testing.T) *Ledger {
	t.Helper()
	return &Ledger{Path: filepath.Join(t.TempDir(), "relay.json"), Entries: map[string]Entry{}}
}

// saved reloads the ledger file: Act writes it before it answers.
func saved(t *testing.T, l *Ledger, key string) Entry {
	t.Helper()
	got, err := LoadLedger(l.Path)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := got.Entries[key]
	if !ok {
		t.Fatalf("ledger has no %s: %+v", key, got.Entries)
	}
	return e
}

type fakeHerdr struct{ calls [][]string }

func (h *fakeHerdr) run(args ...string) ([]byte, error) {
	h.calls = append(h.calls, args)
	return []byte(`{"terminal":"remote1/t2b"}`), nil
}

func (f *fakeHC) sends() []string {
	var s []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "send ") {
			s = append(s, c)
		}
	}
	return s
}

func (f *fakeHC) started() []string {
	var s []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "run ") {
			s = append(s, c)
		}
	}
	return s
}

// worktreeOut: change rb of alpha has its worktree /srv/alpha-rb.
func worktreeOut() map[string]any {
	return map[string]any{
		as + "cat /srv/alpha/openspec/changes/rb/.openspec.yaml": "x-branch: rb\n",
		as + "git -C /srv/alpha worktree list --porcelain":       "worktree /srv/alpha\nbranch refs/heads/main\n\nworktree /srv/alpha-rb\nbranch refs/heads/rb\n",
		as + "git -C /srv/alpha-rb rev-parse --show-toplevel":    "/srv/alpha-rb\n",
	}
}

const coordLs = `{"terminals":[{"address":"remote1/t2b"}]}`

func TestActStartCoordinator(t *testing.T) {
	briefs := t.TempDir()
	if err := os.WriteFile(filepath.Join(briefs, "coordinator.md"), []byte("Brief {repo} {change}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := worktreeOut()
	for k, v := range map[string]any{
		"ls --label striker.coordinator=rb --json": `{"terminals":[]}`,
		"label remote1/t2b striker.repo=alpha":      "",
		"label remote1/t2b striker.coordinator=rb": "",
		"wait remote1/t2b --timeout 90":            "",
		"get remote1/t2b --json":                   `{"address":"remote1/t2b","status":{"state":"idle"}}`,
		"send remote1/t2b Brief alpha rb":           "",
		"ls --label striker.lead=alpha --json":      `{"terminals":[{"address":"kirk/t1"},{"address":"remote1/t1"}]}`,
		"send remote1/t1 [Striker] Anfrage c-rb: Koordinator für rb läuft als remote1/t2b.": "",
	} {
		out[k] = v
	}
	f := &fakeHC{out: out}
	h := &fakeHerdr{}
	l := newLedger(t)
	r := Request{ID: "c-rb", Do: "start-coordinator", Change: "rb"}
	if err := Act(hc.Client{R: f}, h.run, testFleet, briefs, l, leadFile, r); err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 1 || h.calls[0][1] != "space.new" || !strings.Contains(strings.Join(h.calls[0], " "), "--folder /srv/alpha-rb") {
		t.Fatalf("herdr calls %q", h.calls)
	}
	want := []string{"send remote1/t2b Brief alpha rb", "send remote1/t1 [Striker] Anfrage c-rb: Koordinator für rb läuft als remote1/t2b."}
	if !reflect.DeepEqual(f.sends(), want) {
		t.Fatalf("sends %q", f.sends())
	}
	e := saved(t, l, "remote1|alpha.json|c-rb")
	if e.Outcome != "started remote1/t2b" || e.Address != "remote1/t2b" || e.Watch || e.At.IsZero() {
		t.Fatalf("entry %+v", e)
	}
}

// dispatchOut: the coordinator of rb runs as remote1/t2b, the worktree holds
// change rb with task 2.3.
func dispatchOut() map[string]any {
	out := worktreeOut()
	for k, v := range map[string]any{
		"ls --label striker.coordinator=rb --json": `{"terminals":[{"address":"remote1/t9"},{"address":"remote1/t2b"}]}`,
		"get remote1/t9 --json":                    `{"address":"remote1/t9","labels":{"striker.coordinator":"rb","striker.repo":"lcars"}}`,
		"get remote1/t2b --json":                   `{"address":"remote1/t2b","labels":{"striker.coordinator":"rb","striker.repo":"alpha"}}`,
		as + "git -C /srv/alpha-rb ls-files --cached --others --exclude-standard -- openspec": "openspec/changes/rb/.openspec.yaml\nopenspec/changes/rb/tasks.md\nopenspec/changes/rb/plan.md\n",
		as + "cat /srv/alpha-rb/openspec/changes/rb/.openspec.yaml":                           "x-branch: rb\n",
		as + "cat /srv/alpha-rb/openspec/changes/rb/tasks.md":                                 "- [x] 1 First\n- [ ] 2.3 Do it\n",
		as + "cat /srv/alpha-rb/openspec/changes/rb/plan.md":                                  "### Task 2.3: Do it\n\nSay \"hi\" $x.\n",
	} {
		out[k] = v
	}
	return out
}

func TestActDispatch(t *testing.T) {
	// hc run --on remote1 --bg prints one JSON object; checked on remote1 it
	// carries the full address. A bare one gets the machine prepended.
	for name, run := range map[string]string{
		"full address": `{"address":"remote1/t2c","agent":"claude","cwd":"/srv/alpha-rb","name":"implementer-2.3","parent":"remote1/t2b","state":"live","turn":0}` + "\n",
		"bare address": `{"address":"t2c","agent":"claude","cwd":"/srv/alpha-rb","name":"implementer-2.3","parent":"t2b","state":"live","turn":0}` + "\n",
	} {
		t.Run(name, func(t *testing.T) { testActDispatch(t, run) })
	}
}

func testActDispatch(t *testing.T, runOut string) {
	out := dispatchOut()
	ch := openspec.Change{Name: "rb", Branch: "rb", TasksPath: "openspec/changes/rb/tasks.md", PlanPath: "openspec/changes/rb/plan.md"}
	task := openspec.Task{ID: "2.3", Title: "Do it", Body: "Say \"hi\" $x."}
	argv := dispatch.RemoteCommandChange("implementer", "alpha", ch, task, "remote1", "awayteam", "/srv/alpha-rb", "remote1/t2b")
	out[strings.Join(argv, " ")] = runOut
	answer := "send remote1/t2b [Striker] Anfrage 2.3-impl-1: implementer-2.3 läuft als remote1/t2c."
	out[answer] = ""
	f := &fakeHC{out: out}
	l := newLedger(t)
	r := Request{ID: "2.3-impl-1", Do: "dispatch", Role: "implementer", Task: "2.3"}
	if err := Act(hc.Client{R: f}, (&fakeHerdr{}).run, testFleet, t.TempDir(), l, coordFile, r); err != nil {
		t.Fatal(err)
	}
	if got := f.started(); len(got) != 1 || got[0] != strings.Join(argv, " ") {
		t.Fatalf("started %q\nwant %q", got, strings.Join(argv, " "))
	}
	if !reflect.DeepEqual(f.sends(), []string{answer}) {
		t.Fatalf("sends %q", f.sends())
	}
	e := saved(t, l, "remote1|alpha--rb.json|2.3-impl-1")
	if e.Outcome != "started remote1/t2c" || e.Address != "remote1/t2c" || e.Name != "implementer-2.3" || !e.Watch || e.Delivered {
		t.Fatalf("entry %+v", e)
	}
}

func TestActSend(t *testing.T) {
	l := newLedger(t)
	l.Put("remote1|alpha--rb.json|2.3-impl-1", Entry{Outcome: "started remote1/t2c", Address: "remote1/t2c", Name: "implementer-2.3", Watch: true, Delivered: true})
	// Another coordinator's agent with the same name is not this one's.
	l.Put("remote1|lcars--rb.json|2.3-impl-1", Entry{Outcome: "started remote1/t7", Address: "remote1/t7", Name: "implementer-2.3", Watch: true})
	text := `Spec review found: "x" and $y`
	toAgent := "send remote1/t2c [Striker] " + text
	answer := "send remote1/t2b [Striker] Anfrage fix-1: an implementer-2.3 (remote1/t2c) geschickt."
	f := &fakeHC{out: map[string]any{
		toAgent: "",
		answer:  "",
		"ls --label striker.coordinator=rb --json": coordLs,
		"get remote1/t2b --json":                   `{"address":"remote1/t2b","labels":{"striker.coordinator":"rb","striker.repo":"alpha"}}`,
	}}
	r := Request{ID: "fix-1", Do: "send", To: "implementer-2.3", Text: text}
	if err := Act(hc.Client{R: f}, (&fakeHerdr{}).run, testFleet, t.TempDir(), l, coordFile, r); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.sends(), []string{toAgent, answer}) {
		t.Fatalf("sends %q", f.sends())
	}
	e := saved(t, l, "remote1|alpha--rb.json|fix-1")
	if e.Outcome != "sent" || e.Address != "remote1/t2c" || e.Name != "implementer-2.3" || !e.Watch || e.Delivered {
		t.Fatalf("entry %+v", e)
	}
}

func TestActRefused(t *testing.T) {
	coordAnswer := func(id, reason string) string {
		return "send remote1/t2b [Striker] Anfrage " + id + " abgelehnt: " + reason
	}
	for _, tc := range []struct {
		name   string
		r      Request
		out    func() map[string]any
		reason string // start of the reason
	}{
		{"unknown task", Request{ID: "9-impl-1", Do: "dispatch", Role: "implementer", Task: "9"}, dispatchOut, "kein Task 9 im Change rb."},
		{"missing worktree", Request{ID: "2.3-impl-1", Do: "dispatch", Role: "implementer", Task: "2.3"}, func() map[string]any {
			out := dispatchOut()
			out[as+"git -C /srv/alpha worktree list --porcelain"] = "worktree /srv/alpha\nbranch refs/heads/main\n"
			return out
		}, "Change 'rb' von alpha hat keinen Worktree"},
		{"foreign terminal", Request{ID: "fix-1", Do: "send", To: "implementer-2.3", Text: "x"}, func() map[string]any {
			return map[string]any{
				"ls --label striker.coordinator=rb --json": coordLs,
				"get remote1/t2b --json":                   `{"address":"remote1/t2b","labels":{"striker.coordinator":"rb","striker.repo":"alpha"}}`,
			}
		}, "implementer-2.3 hat Striker nicht für diesen Koordinator gestartet."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := tc.out()
			f := &fakeHC{out: out}
			// The answer is whatever starts with the expected reason.
			l := newLedger(t)
			l.Put("remote1|lcars--rb.json|x", Entry{Outcome: "started remote1/t7", Address: "remote1/t7", Name: "implementer-2.3"})
			f.prefixOK = coordAnswer(tc.r.ID, tc.reason)
			if err := Act(hc.Client{R: f}, (&fakeHerdr{}).run, testFleet, t.TempDir(), l, coordFile, tc.r); err != nil {
				t.Fatal(err)
			}
			if s := f.sends(); len(s) != 1 || !strings.HasPrefix(s[0], coordAnswer(tc.r.ID, tc.reason)) {
				t.Fatalf("sends %q", s)
			}
			if s := f.started(); len(s) != 0 {
				t.Fatalf("started %q", s)
			}
			e := saved(t, l, coordFile.Key(tc.r.ID))
			if !strings.HasPrefix(e.Outcome, "error "+strings.TrimSuffix(tc.reason, ".")) || e.Watch {
				t.Fatalf("entry %+v", e)
			}
		})
	}
}

func TestActRequesterGone(t *testing.T) {
	out := dispatchOut()
	out["ls --label striker.coordinator=rb --json"] = `{"terminals":[]}`
	f := &fakeHC{out: out}
	l := newLedger(t)
	r := Request{ID: "2.3-impl-1", Do: "dispatch", Role: "implementer", Task: "2.3"}
	if err := Act(hc.Client{R: f}, (&fakeHerdr{}).run, testFleet, t.TempDir(), l, coordFile, r); err != nil {
		t.Fatal(err)
	}
	if s := f.sends(); len(s) != 0 {
		t.Fatalf("sends %q", s)
	}
	if s := f.started(); len(s) != 0 {
		t.Fatalf("started without a coordinator: %q", s)
	}
	if e := saved(t, l, coordFile.Key(r.ID)); !strings.HasPrefix(e.Outcome, "error ") {
		t.Fatalf("entry %+v", e)
	}
}

func TestActDoneOnce(t *testing.T) {
	l := newLedger(t)
	l.Put(coordFile.Key("2.3-impl-1"), Entry{Outcome: "started remote1/t2c", Address: "remote1/t2c"})
	f := &fakeHC{out: map[string]any{}}
	r := Request{ID: "2.3-impl-1", Do: "dispatch", Role: "implementer", Task: "2.3"}
	if err := Act(hc.Client{R: f}, (&fakeHerdr{}).run, testFleet, t.TempDir(), l, coordFile, r); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("carried out twice: %q", f.calls)
	}
}

func TestReject(t *testing.T) {
	answer := "send remote1/t1 [Striker] Anfrage d abgelehnt: dispatch darf nur ein Koordinator anfragen."
	f := &fakeHC{out: map[string]any{
		"ls --label striker.lead=alpha --json": `{"terminals":[{"address":"remote1/t1"}]}`,
		answer:                                "",
	}}
	l := newLedger(t)
	d := Dropped{File: leadFile, Request: Request{ID: "d", Do: "dispatch"}, Reason: "dispatch darf nur ein Koordinator anfragen"}
	if err := Reject(hc.Client{R: f}, l, testFleet, d); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.sends(), []string{answer}) {
		t.Fatalf("sends %q", f.sends())
	}
	if e := saved(t, l, "remote1|alpha.json|d"); e.Outcome != "error dispatch darf nur ein Koordinator anfragen" {
		t.Fatalf("entry %+v", e)
	}
	// Filter now leaves it out: answered once.
	fl := leadFile
	fl.Requests = []Request{d.Request}
	if keep, drop := Filter(testFleet, l, []File{fl}); len(keep)+len(drop) != 0 {
		t.Fatalf("again: %+v %+v", keep, drop)
	}

	// No id: nobody to answer, nothing recorded.
	f.calls = nil
	if err := Reject(hc.Client{R: f}, l, testFleet, Dropped{File: leadFile, Request: Request{Do: "x"}, Reason: "Anfrage ohne id"}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 || len(l.Snapshot()) != 1 {
		t.Fatalf("calls %q ledger %+v", f.calls, l.Snapshot())
	}

	// A file of no relayed repo: recorded once, never answered (its writer is
	// no agent Striker relays for).
	other := File{Machine: "remote1", Path: "/var/lib/striker/status/other.json", Repo: "other"}
	if err := Reject(hc.Client{R: f}, l, testFleet, Dropped{File: other, Request: Request{ID: "c", Do: "start-coordinator"}, Reason: "kein weitergeleitetes Fleet-Repo"}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("calls %q", f.calls)
	}
	if e := saved(t, l, "remote1|other.json|c"); !strings.HasPrefix(e.Outcome, "error ") {
		t.Fatalf("entry %+v", e)
	}
}
