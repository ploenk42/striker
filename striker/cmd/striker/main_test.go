package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ploenk42/striker/striker/internal/bridge"
	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
)

func TestHostGuard(t *testing.T) {
	h := hostGuard("7522", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for host, want := range map[string]int{
		"127.0.0.1:7522": 200,
		"localhost:7522": 200,
		"evil.com:7522":  403,
		"evil.com":       403,
		"127.0.0.1":      403,
		"127.0.0.1:80":   403,
		"":               403,
	} {
		for _, path := range []string{"/status", "/doc", "/space", "/overview"} {
			req := httptest.NewRequest("GET", path, nil)
			req.Host = host
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != want {
				t.Errorf("Host %q %s: %d, want %d", host, path, rec.Code, want)
			}
		}
	}
}

func TestStartCoordinatorArgsCheckedFirst(t *testing.T) {
	// The fleet path does not exist: any case that reached fleet.Load would
	// return 1, and nothing here may start anything.
	missing := t.TempDir() + "/none.json"
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"--help"}, 0},
		{[]string{"-h"}, 0},
		{nil, 2},
		{[]string{"alpha"}, 2},
		{[]string{"alpha", "--help"}, 2},
		{[]string{"alpha", "x", "y"}, 2},
		{[]string{"alpha", "x"}, 1},
	} {
		if got := runStartCoordinator(hc.Client{R: noHC{t}}, missing, tc.args); got != tc.want {
			t.Errorf("%q: %d, want %d", tc.args, got, tc.want)
		}
	}
}

func TestIsChange(t *testing.T) {
	top := t.TempDir()
	os.MkdirAll(filepath.Join(top, "openspec", "changes", "feat"), 0o755)
	os.WriteFile(filepath.Join(top, "openspec", "changes", "feat", "tasks.md"), []byte("- [ ] 1 a\n"), 0o644)
	os.MkdirAll(filepath.Join(top, "openspec", "changes", "empty"), 0o755)
	for arg, want := range map[string]bool{
		"feat":                        true,
		"empty":                       false, // no tasks.md
		"nope":                        false,
		"feat.md":                     false,
		"docs/superpowers/plans/x.md": false,
		"openspec/changes/feat":       false,
		"..":                          false,
		"":                            false,
	} {
		if got := isChange(top, arg); got != want {
			t.Errorf("isChange(%q) = %v, want %v", arg, got, want)
		}
	}
}

func TestLegacyBriefFilled(t *testing.T) {
	tpl := filepath.Join(t.TempDir(), "coordinator.md")
	os.WriteFile(tpl, []byte("{repo} {change} {status_file}{agents}\n"), 0o644)
	out := t.TempDir()
	p, err := legacyBrief(tpl, out, "herdr-mods", "lcars-bridge", "/h/.striker/status/herdr-mods--lcars-bridge.json")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	// Legacy coordinators run on this computer: the direct variant.
	if filepath.Dir(p) != out || !strings.HasPrefix(string(b), "herdr-mods lcars-bridge /h/.striker/status/herdr-mods--lcars-bridge.json") ||
		!strings.Contains(string(b), "striker dispatch <rolle> <task> --plan lcars-bridge") {
		t.Fatalf("%s: %q", p, b)
	}
}

func TestLegacyRepoName(t *testing.T) {
	f := fleet.Fleet{Repos: []fleet.Repo{{Name: "mods", Path: "/Users/p/herdr-mods"}}}
	for _, tc := range []struct{ top, common, want string }{
		{"/Users/p/herdr-mods", "/Users/p/herdr-mods/.git", "mods"},   // fleet name wins
		{"/Users/p/herdr-mods-x", "/Users/p/herdr-mods/.git", "mods"}, // worktree of the fleet repo
		{"/Users/p/other-wt", "/Users/p/other/.git", "other"},         // no fleet entry: main folder name
		{"/Users/p/lone", "", "lone"},                                 // no common dir: top's name
	} {
		if got := legacyRepoName(f, tc.top, tc.common); got != tc.want {
			t.Errorf("legacyRepoName(%q, %q) = %q, want %q", tc.top, tc.common, got, tc.want)
		}
	}
}

func TestExtraFor(t *testing.T) {
	f := fleet.Fleet{Repos: []fleet.Repo{{Name: "alpha", Machine: "remote1", Path: "/srv/alpha"}}, StatusDir: map[string]string{"remote1": "/s"}}
	x := extraFor(hc.Client{R: fileHC{}}, f, t.TempDir(), "remote1", "/srv/alpha")
	if x.Repo != "alpha" || string(x.Read("remote1", "alpha", "rb")) != `{"change":"rb"}` {
		t.Fatalf("extra %+v", x)
	}
}

// extraFor finds a local lead by its cwd (no fleet entry, as herdr-mods)
// and passes its status task and live state on (ruling 7).
func TestExtraForLocalLead(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".striker", "status"), 0o755)
	os.WriteFile(filepath.Join(home, ".striker", "status", "herdr-mods.json"), []byte(`{"task":"Task 5 of repo-overview"}`), 0o644)
	r := mapHC{
		"ls --json":     `{"terminals":[{"address":"t5"}]}`,
		"get t5 --json": `{"address":"t5","cwd":"/Users/p/herdr-mods","labels":{"striker.lead":"herdr-mods"},"status":{"state":"working","msg":""}}`,
	}
	x := extraFor(hc.Client{R: r}, fleet.Fleet{}, home, "", "/Users/p/herdr-mods")
	if x.Repo != "" || x.LeadTask != "Task 5 of repo-overview" || x.LeadState != "working" {
		t.Fatalf("extra %+v", x)
	}
}

type mapHC map[string]string

func (m mapHC) Run(args ...string) ([]byte, error) {
	if s, ok := m[strings.Join(args, " ")]; ok {
		return []byte(s), nil
	}
	return nil, os.ErrNotExist
}

// /overview is mounted behind the Host guard and keeps the worktree-top
// guard of the bridge.
func TestOverviewEndpointGuards(t *testing.T) {
	bh := bridge.Handler(bridge.NewSource(hc.Client{R: mapHC{}}, nil, nil, nil))
	h := routes("7522", http.NotFoundHandler(), bh)
	notRepo := t.TempDir()
	for _, tc := range []struct {
		host string
		want int
	}{{"evil.com:7522", 403}, {"127.0.0.1:7522", 200}} {
		req := httptest.NewRequest("GET", "/overview?machine=&path="+notRepo, nil)
		req.Host = tc.host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("Host %s: %d %s, want %d", tc.host, rec.Code, rec.Body.String(), tc.want)
		}
		if tc.want == 200 && !strings.Contains(rec.Body.String(), `"error"`) {
			t.Fatalf("not a worktree: %s", rec.Body.String())
		}
	}
}

type fileHC struct{}

func (fileHC) Run(args ...string) ([]byte, error) {
	if len(args) == 5 && args[0] == "file" && args[4] == "/s/alpha--rb.json" {
		return []byte(`{"change":"rb"}`), nil
	}
	return nil, os.ErrNotExist
}

type noHC struct{ t *testing.T }

func (n noHC) Run(args ...string) ([]byte, error) {
	n.t.Fatalf("hc called: %q", args)
	return nil, nil
}

func TestOutside(t *testing.T) {
	for rel, want := range map[string]bool{
		"..":                 true,
		"../x.md":            true,
		"..foo.md":           false,
		"docs/../x.md":       false,
		"docs/plans/a.md":    false,
		"..hidden/plan.md":   false,
		"../../docs/plan.md": true,
	} {
		if got := outside(rel); got != want {
			t.Errorf("outside(%q) = %v, want %v", rel, got, want)
		}
	}
}
