package relay

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
)

type fakeHC struct {
	// joined args -> string output, error, or func() (string, error) (called
	// outside the lock, so it may block)
	out   map[string]any
	calls []string
	// prefixOK: a call starting with it succeeds with no output.
	prefixOK string

	mu sync.Mutex
}

func (f *fakeHC) Run(args ...string) ([]byte, error) {
	k := strings.Join(args, " ")
	f.mu.Lock()
	f.calls = append(f.calls, k)
	v, ok := f.out[k]
	prefix := f.prefixOK
	f.mu.Unlock()
	if !ok && prefix != "" && strings.HasPrefix(k, prefix) {
		return nil, nil
	}
	if !ok {
		return nil, errors.New("unexpected: " + k)
	}
	switch v := v.(type) {
	case error:
		return nil, v
	case func() (string, error):
		s, err := v()
		return []byte(s), err
	}
	return []byte(v.(string)), nil
}

// callsNow is a copy of the calls so far.
func (f *fakeHC) callsNow() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

var testFleet = fleet.Fleet{
	Repos: []fleet.Repo{
		{Name: "alpha", Machine: "remote1", Path: "/srv/alpha", User: "awayteam"},
		{Name: "lcars", Machine: "remote1", Path: "/srv/lcars", User: "awayteam"},
		{Name: "herdr-mods", Path: `C:\x\herdr-mods`},
		{Name: "plain", Machine: "kirk", Path: "/srv/plain"},
	},
	StatusDir: map[string]string{"remote1": "/var/lib/striker/status", "kirk": "/srv/status"},
}

// find expands the \n of -printf itself: the argument is a backslash and an n.
const findRemote1 = `exec --on remote1 find /var/lib/striker/status -maxdepth 1 -name *.json -mmin -1440 -printf %f\n`

func TestParseFiles(t *testing.T) {
	f := &fakeHC{out: map[string]any{
		findRemote1: "alpha.json\nalpha--rb.json\nlcars.json\nbroken.json\nempty.json\n",
		"file read --on remote1 /var/lib/striker/status/alpha.json": "\xEF\xBB\xBF" + `{"task":"x","requests":[
			{"id":"c-rb","do":"start-coordinator","change":"rb"}]}`,
		"file read --on remote1 /var/lib/striker/status/alpha--rb.json": `{"requests":[
			{"id":"2.3-impl-1","do":"dispatch","role":"implementer","task":"2.3"},
			{"id":"5-impl-1","do":"dispatch","role":"implementer","task":5},
			{"id":"fix-1","do":"send","to":"implementer-2.3","text":"Spec review found: \"x\""}]}`,
		"file read --on remote1 /var/lib/striker/status/lcars.json":  `{"task":"idle"}`,
		"file read --on remote1 /var/lib/striker/status/broken.json": `{"requests":[`,
		"file read --on remote1 /var/lib/striker/status/empty.json":  errors.New("gone"),
	}}
	got := Read(hc.Client{R: f}, testFleet)
	want := []File{
		{Machine: "remote1", Path: "/var/lib/striker/status/alpha--rb.json", Repo: "alpha", Change: "rb", Requests: []Request{
			{ID: "2.3-impl-1", Do: "dispatch", Role: "implementer", Task: "2.3"},
			{ID: "5-impl-1", Do: "dispatch", Role: "implementer", Task: "5"},
			{ID: "fix-1", Do: "send", To: "implementer-2.3", Text: `Spec review found: "x"`},
		}},
		{Machine: "remote1", Path: "/var/lib/striker/status/alpha.json", Repo: "alpha", Requests: []Request{
			{ID: "c-rb", Do: "start-coordinator", Change: "rb"},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	// Only remote1 has relayed repos: one find there, none on kirk.
	finds := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, "exec ") {
			finds++
			if c != findRemote1 {
				t.Errorf("unexpected find %q", c)
			}
		}
	}
	if finds != 1 {
		t.Errorf("finds = %d, calls %v", finds, f.calls)
	}
}

func TestParseFilesFindFails(t *testing.T) {
	f := &fakeHC{out: map[string]any{findRemote1: errors.New("no route")}}
	if got := Read(hc.Client{R: f}, testFleet); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestFileName(t *testing.T) {
	for _, tc := range []struct{ name, repo, change string }{
		{"alpha.json", "alpha", ""},
		{"alpha--rb.json", "alpha", "rb"},
		{"herdr-mods--lcars-bridge.json", "herdr-mods", "lcars-bridge"},
	} {
		repo, change := splitName(tc.name)
		if repo != tc.repo || change != tc.change {
			t.Errorf("%s: %q %q", tc.name, repo, change)
		}
	}
}

func TestFilter(t *testing.T) {
	l := &Ledger{Entries: map[string]Entry{"remote1|alpha--rb.json|done-1": {Outcome: "sent"}}}
	lead := File{Machine: "remote1", Path: "/s/alpha.json", Repo: "alpha"}
	coordF := File{Machine: "remote1", Path: "/s/alpha--rb.json", Repo: "alpha", Change: "rb"}
	start := Request{ID: "c-rb", Do: "start-coordinator", Change: "rb"}
	disp := Request{ID: "2.3-impl-1", Do: "dispatch", Role: "implementer", Task: "2.3"}
	send := Request{ID: "fix-1", Do: "send", To: "implementer-2.3", Text: "fix it"}
	with := func(fl File, rs ...Request) File { fl.Requests = rs; return fl }
	files := []File{
		with(lead, start,
			Request{ID: "d", Do: "dispatch", Role: "implementer", Task: "1"},
			Request{ID: "bad-change", Do: "start-coordinator", Change: "Bad_Name"},
			Request{Do: "start-coordinator", Change: "rb"},
			Request{ID: "what", Do: "rm"},
		),
		with(coordF, disp, send,
			Request{ID: "2.3-impl-1", Do: "dispatch", Role: "implementer", Task: "2.3"}, // repeat: only the first counts
			Request{ID: "done-1", Do: "send", To: "implementer-2.3", Text: "x"},
			Request{ID: "sc", Do: "start-coordinator", Change: "rb"},
			Request{ID: "bad-role", Do: "dispatch", Role: "Imp Lementer", Task: "1"},
			Request{ID: "no-task", Do: "dispatch", Role: "implementer"},
			Request{ID: "no-text", Do: "send", To: "implementer-2.3"},
			Request{ID: "a|b", Do: "send", To: "implementer-2.3", Text: "x"},
		),
		with(File{Machine: "remote1", Path: "/s/other.json", Repo: "other"}, start),
		with(File{Machine: "kirk", Path: "/s/plain.json", Repo: "plain"}, start),
		with(File{Machine: "kirk", Path: "/s/alpha.json", Repo: "alpha"}, start),
		with(File{Machine: "remote1", Path: "/s/alpha--Bad.json", Repo: "alpha", Change: "Bad"}, disp),
	}
	keep, drop := Filter(testFleet, l, files)
	wantKeep := []Item{{File: files[0], Request: start}, {File: files[1], Request: disp}, {File: files[1], Request: send}}
	if !reflect.DeepEqual(keep, wantKeep) {
		t.Fatalf("keep %+v", keep)
	}
	ids := []string{}
	for _, d := range drop {
		if d.Reason == "" {
			t.Errorf("%s/%s dropped without reason", d.File.Path, d.Request.ID)
		}
		ids = append(ids, d.File.Path+":"+d.Request.ID)
	}
	wantDrop := []string{
		"/s/alpha.json:d", "/s/alpha.json:bad-change", "/s/alpha.json:what", // no id: silent
		"/s/alpha--rb.json:sc", "/s/alpha--rb.json:bad-role", "/s/alpha--rb.json:no-task", "/s/alpha--rb.json:no-text",
		"/s/alpha--rb.json:a|b",
		"/s/other.json:c-rb", "/s/plain.json:c-rb", "/s/alpha.json:c-rb", "/s/alpha--Bad.json:2.3-impl-1",
	}
	if !reflect.DeepEqual(ids, wantDrop) {
		t.Fatalf("drop %v\nwant %v", ids, wantDrop)
	}
}
