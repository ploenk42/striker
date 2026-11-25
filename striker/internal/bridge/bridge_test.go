package bridge

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/overview"
	"github.com/ploenk42/striker/striker/internal/repoview"
	"github.com/ploenk42/striker/striker/internal/status"
)

type fake struct {
	view       repoview.View
	repoErr    error
	repoCalls  int
	branch     string
	doc        Doc
	docs       map[string]string
	parts      overview.Parts
	partsCalls int
	live       status.Repo
	liveCalls  int
}

func (f *fake) Repo(machine, path string) (repoview.View, error) {
	f.repoCalls++
	return f.view, f.repoErr
}

func (f *fake) Parts(machine, path string) (overview.Parts, error) {
	f.partsCalls++
	return f.parts, nil
}

func (f *fake) Live(machine, path string) status.Repo {
	f.liveCalls++
	return f.live
}

func (f *fake) Branch(machine, path string) (string, error) { return f.branch, nil }

func (f *fake) Doc(machine, path, rel string) (Doc, error) {
	if t, ok := f.docs[rel]; ok {
		return Doc{Text: t}, nil
	}
	return f.doc, nil
}

func get(t *testing.T, h http.Handler, url string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil && rec.Code == 200 {
		t.Fatalf("bad json %q: %v", rec.Body.String(), err)
	}
	return rec.Code, m
}

func TestSpaceNoPlan(t *testing.T) {
	f := &fake{branch: "hotfix", view: repoview.View{Plans: []repoview.PlanView{{Topic: "x", Branch: "feat"}}}}
	code, m := get(t, Handler(f), "/space?machine=&path=/r")
	if code != 200 || m["branch"] != "hotfix" || m["plan"] != nil || m["topic"] != nil || m["coordinator"] != nil {
		t.Fatalf("got %d %v", code, m)
	}
	if h, ok := m["spec_headings"].([]any); !ok || len(h) != 0 {
		t.Fatalf("spec_headings = %v", m["spec_headings"])
	}
}

func TestSpaceWithPlan(t *testing.T) {
	f := &fake{branch: "feat", docs: map[string]string{"s.md": "# T\n\n## Alpha\n\n## Beta\n"},
		view: repoview.View{Plans: []repoview.PlanView{{Topic: "x", Branch: "feat", Spec: "s.md", Coordinator: "herdr/1"}}}}
	_, m := get(t, Handler(f), "/space?machine=&path=/r")
	if m["topic"] != "x" || m["coordinator"] != "herdr/1" || m["plan"] == nil {
		t.Fatalf("got %v", m)
	}
	h, _ := m["spec_headings"].([]any)
	if len(h) == 0 {
		t.Fatalf("no headings: %v", m)
	}
}

func TestSpaceCarriesWarnings(t *testing.T) {
	f := &fake{branch: "x", view: repoview.View{Warnings: []string{"closed issues unavailable: boom"}}}
	_, m := get(t, Handler(f), "/space?machine=&path=/r")
	w, _ := m["warnings"].([]any)
	if len(w) != 1 || w[0] != "closed issues unavailable: boom" {
		t.Fatalf("warnings = %v", m["warnings"])
	}
	_, m = get(t, Handler(&fake{}), "/space?machine=&path=/r")
	if w, ok := m["warnings"].([]any); !ok || len(w) != 0 {
		t.Fatalf("warnings = %v, want []", m["warnings"])
	}
}

func TestRepoErrorIs200(t *testing.T) {
	f := &fake{repoErr: errors.New("hc: machine remote1 unreachable\nmore")}
	rec := httptest.NewRecorder()
	Handler(f).ServeHTTP(rec, httptest.NewRequest("GET", "/repo?machine=remote1&path=/r", nil))
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"error":"hc: machine remote1 unreachable"}` {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestRepoCached(t *testing.T) {
	f := &fake{}
	h := Handler(f)
	for i := 0; i < 2; i++ {
		code, m := get(t, h, "/repo?machine=&path=/r")
		if code != 200 || m["view"] == nil || m["graph"] == nil {
			t.Fatalf("got %d %v", code, m)
		}
	}
	if f.repoCalls != 1 {
		t.Fatalf("repo calls = %d", f.repoCalls)
	}
}

func TestDocHunks(t *testing.T) {
	f := &fake{doc: Doc{Text: "a", Base: "b", Hunks: []gitrepo.Hunk{{Added: []string{"a"}}}}}
	_, m := get(t, Handler(f), "/doc?machine=&path=/r&file=x.md")
	hs, _ := m["hunks"].([]any)
	if len(hs) != 1 || m["text"] != "a" || m["base"] != "b" {
		t.Fatalf("got %v", m)
	}
	if added := hs[0].(map[string]any)["added"].([]any); added[0] != "a" {
		t.Fatalf("added = %v", added)
	}
}

func TestMissingParam400(t *testing.T) {
	h := Handler(&fake{})
	for _, u := range []string{"/repo?machine=", "/space?machine=", "/doc?machine=&path=/r", "/doc?path=&file=x", "/overview?machine="} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", u, nil))
		if rec.Code != 400 {
			t.Errorf("%s: code %d", u, rec.Code)
		}
	}
}

func TestOverviewArraysNotNull(t *testing.T) {
	code, m := get(t, Handler(&fake{}), "/overview?machine=&path=/r")
	if code != 200 || len(m) != 5 {
		t.Fatalf("got %d %v", code, m)
	}
	for _, k := range []string{"open_items", "warnings"} {
		if _, ok := m[k].([]any); !ok {
			t.Errorf("%s = %v, want an array", k, m[k])
		}
	}
	inv, _ := m["inventory"].(map[string]any)
	for _, k := range []string{"capabilities", "changes", "archived", "legacy"} {
		if _, ok := inv[k].([]any); !ok {
			t.Errorf("inventory.%s = %v, want an array", k, inv[k])
		}
	}
}

func TestOverviewMergesAndCaches(t *testing.T) {
	addr, state := "t1", "working"
	f := &fake{
		view:  repoview.View{Plans: []repoview.PlanView{{Topic: "c", State: "planned"}}},
		parts: overview.Parts{Has: true, Context: &overview.Context{File: "README.md", Title: "R", Lead: "x"}},
		live:  status.Repo{Lead: &status.Lead{Address: addr, State: &state}, Status: json.RawMessage(`{"task":"c"}`)},
	}
	now := time.Unix(1000, 0)
	h := newHandlerBG(f, func() time.Time { return now }, func(g func()) { g() })
	_, m := get(t, h, "/overview?machine=&path=/r")
	b, _ := m["briefing"].(map[string]any)
	ctx, _ := m["context"].(map[string]any)
	inv, _ := m["inventory"].(map[string]any)
	if b["task"] != "c" || b["address"] != "t1" || ctx["file"] != "README.md" || len(inv["changes"].([]any)) != 1 {
		t.Fatalf("got %v", m)
	}
	// Live part (lead, coordinators) 10 s, repo parts 60 s; /repo shares the view.
	now = now.Add(11 * time.Second)
	get(t, h, "/overview?machine=&path=/r")
	get(t, h, "/repo?machine=&path=/r")
	if f.repoCalls != 1 || f.partsCalls != 1 || f.liveCalls != 2 {
		t.Fatalf("calls repo %d parts %d live %d", f.repoCalls, f.partsCalls, f.liveCalls)
	}
	now = now.Add(50 * time.Second)
	get(t, h, "/overview?machine=&path=/r")
	if f.repoCalls != 2 || f.partsCalls != 2 || f.liveCalls != 3 {
		t.Fatalf("after 61 s: calls repo %d parts %d live %d", f.repoCalls, f.partsCalls, f.liveCalls)
	}
}

// A stale view answers at once (the old one) and is rebuilt in the background, one rebuild at a time; the next
// ask sees the new one. A failed rebuild drops the cache, so the ask after it shows the error.
func TestStaleRepoAnswersAtOnce(t *testing.T) {
	f := &fake{view: repoview.View{Default: "old"}}
	now := time.Unix(1000, 0)
	var later []func()
	h := newHandlerBG(f, func() time.Time { return now }, func(g func()) { later = append(later, g) })
	_, m := get(t, h, "/repo?machine=&path=/r")
	if f.repoCalls != 1 || len(later) != 0 {
		t.Fatalf("first ask: calls %d, background %d", f.repoCalls, len(later))
	}
	f.view = repoview.View{Default: "new"}
	now = now.Add(61 * time.Second)
	_, m = get(t, h, "/repo?machine=&path=/r")
	get(t, h, "/repo?machine=&path=/r")
	if v, _ := m["view"].(map[string]any); v["default"] != "old" || f.repoCalls != 1 || len(later) != 1 {
		t.Fatalf("stale ask: %v calls %d background %d", m["view"], f.repoCalls, len(later))
	}
	later[0]()
	_, m = get(t, h, "/repo?machine=&path=/r")
	if v, _ := m["view"].(map[string]any); v["default"] != "new" || f.repoCalls != 2 {
		t.Fatalf("after rebuild: %v calls %d", m["view"], f.repoCalls)
	}
	f.repoErr = errors.New("gone")
	now = now.Add(61 * time.Second)
	get(t, h, "/repo?machine=&path=/r")
	later[len(later)-1]()
	if _, m = get(t, h, "/repo?machine=&path=/r"); m["error"] != "gone" {
		t.Fatalf("after failed rebuild: %v", m)
	}
}

func TestOverviewRepoError(t *testing.T) {
	f := &fake{repoErr: errors.New("boom\nmore")}
	rec := httptest.NewRecorder()
	Handler(f).ServeHTTP(rec, httptest.NewRequest("GET", "/overview?machine=&path=/r", nil))
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"error":"boom"}` || f.partsCalls != 0 || f.liveCalls != 0 {
		t.Fatalf("got %d %q parts %d live %d", rec.Code, rec.Body.String(), f.partsCalls, f.liveCalls)
	}
}
