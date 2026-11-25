package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/repoview"
	"github.com/ploenk42/striker/striker/internal/status"
)

// mapGit answers git from a map of joined args (top /r) and reads working
// tree files from files (keyed by path).
type mapGit struct {
	git   map[string]string
	files map[string]string
}

func (g mapGit) Git(dir string, args ...string) ([]byte, error) {
	if s, ok := g.git[strings.Join(args, " ")]; ok {
		return []byte(s), nil
	}
	return nil, errors.New("fake git: " + strings.Join(args, " "))
}

func (g mapGit) Read(path string) ([]byte, error) {
	if s, ok := g.files[path]; ok {
		return []byte(s), nil
	}
	return nil, fmt.Errorf("no file %s: %w", path, fs.ErrNotExist)
}

// permGit refuses to read the REGISTER (it exists but cannot be read).
type permGit struct{ mapGit }

func (g permGit) Read(path string) ([]byte, error) {
	if path == "/r/docs/REGISTER.md" {
		return nil, fmt.Errorf("open %s: %w", path, fs.ErrPermission)
	}
	return g.mapGit.Read(path)
}

// An unreadable REGISTER is named in warnings and gives no register items; a
// missing one stays silent (spec delta striker/overview-endpoint).
func TestOverviewRegisterUnreadable(t *testing.T) {
	for _, tc := range []struct {
		name string
		git  gitrepo.Runner
		warn bool
	}{
		{"unreadable", permGit{overviewRepo()}, true},
		{"missing", func() gitrepo.Runner { g := overviewRepo(); delete(g.files, "/r/docs/REGISTER.md"); return g }(), false},
		{"remote missing", remoteMissing{overviewRepo()}, false},
	} {
		src := real{h: hc.Client{R: noTerms{}}, runner: func(string) gitrepo.Runner { return tc.git }}
		rec := httptest.NewRecorder()
		Handler(src).ServeHTTP(rec, httptest.NewRequest("GET", "/overview?machine=&path=/r", nil))
		var ov struct {
			OpenItems []struct{ Source string } `json:"open_items"`
			Warnings  []string                  `json:"warnings"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &ov); err != nil {
			t.Fatalf("%s: %s: %v", tc.name, rec.Body.String(), err)
		}
		named := false
		for _, w := range ov.Warnings {
			named = named || strings.Contains(w, "REGISTER")
		}
		if named != tc.warn {
			t.Errorf("%s: warnings %q, want REGISTER named %v", tc.name, ov.Warnings, tc.warn)
		}
		for _, i := range ov.OpenItems {
			if i.Source == "register" {
				t.Errorf("%s: register item listed: %s", tc.name, rec.Body.String())
			}
		}
	}
}

// remoteMissing answers a missing REGISTER the way `hc file read` does: an
// error whose text says the file does not exist (no fs.ErrNotExist).
type remoteMissing struct{ mapGit }

func (g remoteMissing) Read(path string) ([]byte, error) {
	if path == "/r/docs/REGISTER.md" {
		return nil, errors.New("hc file read --on remote1 " + path + `: exit status 3: {"error":{"hint":"check the path","kind":"NOT_FOUND","message":"no file ` + path + `"}}`)
	}
	return g.mapGit.Read(path)
}

type noTerms struct{}

func (noTerms) Run(args ...string) ([]byte, error) {
	if strings.Join(args, " ") == "ls --json" {
		return []byte(`{"terminals":[]}`), nil
	}
	return nil, errors.New("fake hc: " + strings.Join(args, " "))
}

// overviewRepo is a repo at /r whose change c has 2 of 4 tasks ticked and
// nobody but (maybe) the lead working on it.
func overviewRepo() mapGit {
	cd := "openspec/changes/c/"
	return mapGit{git: map[string]string{
		"rev-parse --show-toplevel":                                             "/r\n",
		"symbolic-ref --short refs/remotes/origin/HEAD":                         "origin/main\n",
		"log --format=%H%x1f%P%x1f%s%x1f%b%x1e -n 30 main":                      "c2\x1fc1\x1fsecond\x1f\x1ec1\x1f\x1finit\x1f\x1e",
		"worktree list --porcelain":                                             "worktree /r\nbranch refs/heads/main\n",
		"for-each-ref --format=%(refname:short) refs/heads refs/remotes/origin": "main\norigin/main\n",
		"ls-tree -r --name-only main openspec/":                                 cd + ".openspec.yaml\n" + cd + "tasks.md\nopenspec/specs/bridge/spec.md\n",
		"show main:" + cd + ".openspec.yaml":                                    "schema: spec-driven\n",
		"show main:" + cd + "tasks.md":                                          "- [x] 1 a\n- [x] 2 b\n- [ ] 3 c\n- [ ] 4 d\n",
		"show main:openspec/specs/bridge/spec.md":                               "### Requirement: A\n### Requirement: B\n",
	}, files: map[string]string{
		"/r/README.md":        "# r\n\nThe r repo.\n",
		"/r/docs/REGISTER.md": "## Open items\n\n### OI-1 First\n",
	}}
}

func interruptedAt(t *testing.T, body string) string {
	t.Helper()
	var rb struct {
		View repoview.View `json:"view"`
	}
	if err := json.Unmarshal([]byte(body), &rb); err != nil || len(rb.View.Plans) != 1 {
		t.Fatalf("body %s: %v", body, err)
	}
	return rb.View.Plans[0].InterruptedAt
}

// Ruling 7: the lead's status task and live state reach /repo, so a change
// only the lead works on is not interrupted.
func TestRepoLeadWorkingNotInterrupted(t *testing.T) {
	for _, tc := range []struct{ state, task, want string }{
		{"working", "Task 3 of c", ""},
		{"idle", "Task 3 of c", "3"},
		{"working", "other", "3"},
	} {
		x := repoview.Extra{LeadTask: tc.task, LeadState: tc.state}
		src := real{h: hc.Client{R: noTerms{}}, runner: func(string) gitrepo.Runner { return overviewRepo() },
			extra: func(machine, path string) repoview.Extra { return x }}
		rec := httptest.NewRecorder()
		Handler(src).ServeHTTP(rec, httptest.NewRequest("GET", "/repo?machine=&path=/r", nil))
		if got := interruptedAt(t, rec.Body.String()); got != tc.want {
			t.Errorf("lead %s %q: interrupted_at %q, want %q", tc.state, tc.task, got, tc.want)
		}
	}
}

// wtGit answers rev-parse --show-toplevel with the directory asked, so any
// worktree of the fake repo is a worktree top.
type wtGit struct{ mapGit }

func (g wtGit) Git(dir string, args ...string) ([]byte, error) {
	if strings.Join(args, " ") == "rev-parse --show-toplevel" {
		return []byte(dir + "\n"), nil
	}
	return g.mapGit.Git(dir, args...)
}

// In a worktree space the lead (and the fleet entry) belong to the main
// checkout: extra and live are asked for it, and /space of the change's
// worktree shows the change live while the lead works on it.
func TestSpaceWorktreeLeadFromMainCheckout(t *testing.T) {
	g := overviewRepo()
	g.git["rev-parse --path-format=absolute --git-common-dir"] = "/r/.git\n"
	g.git["worktree list --porcelain"] = "worktree /r\nbranch refs/heads/main\n\nworktree /w\nbranch refs/heads/c\n"
	g.git["for-each-ref --format=%(refname:short) refs/heads refs/remotes/origin"] = "main\nc\norigin/main\n"
	cd := "openspec/changes/c/"
	g.git["ls-tree -r --name-only c openspec/"] = cd + ".openspec.yaml\n" + cd + "tasks.md\n"
	g.git["show c:"+cd+".openspec.yaml"] = "schema: spec-driven\n"
	g.git["show c:"+cd+"tasks.md"] = "- [x] 1 a\n- [x] 2 b\n- [ ] 3 c\n- [ ] 4 d\n"
	var asked []string
	src := real{h: hc.Client{R: noTerms{}}, runner: func(string) gitrepo.Runner { return wtGit{g} },
		extra: func(machine, path string) repoview.Extra {
			asked = append(asked, "extra "+path)
			return repoview.Extra{LeadTask: "Task 3 of c", LeadState: "working"}
		},
		live: func(machine, path string) status.Repo {
			asked = append(asked, "live "+path)
			return status.Repo{}
		}}
	h := Handler(src)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/space?machine=&path=/w", nil))
	var sb struct {
		Plan *repoview.PlanView `json:"plan"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sb); err != nil || sb.Plan == nil {
		t.Fatalf("space %s: %v", rec.Body.String(), err)
	}
	if sb.Plan.InterruptedAt != "" || sb.Plan.Tasks[2].State != "todo" {
		t.Errorf("lead works on c: interrupted_at %q task 3 %q", sb.Plan.InterruptedAt, sb.Plan.Tasks[2].State)
	}
	g.files["/w/README.md"] = "# worktree copy\n\nnot this.\n"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/overview?machine=&path=/w", nil))
	if want := []string{"extra /r", "live /r"}; strings.Join(asked, ",") != strings.Join(want, ",") {
		t.Errorf("asked %q, want %q", asked, want)
	}
	// Context and REGISTER come from the main checkout's working tree.
	body := rec.Body.String()
	if !strings.Contains(body, `"context":{"file":"README.md","title":"r"`) || !strings.Contains(body, `"id":"OI-1"`) {
		t.Errorf("overview of /w: %s", body)
	}
}

func TestOverviewRealSource(t *testing.T) {
	addr, state := "t1", "working"
	src := real{h: hc.Client{R: noTerms{}}, runner: func(string) gitrepo.Runner { return overviewRepo() },
		live: func(machine, path string) status.Repo {
			return status.Repo{Name: "r", Lead: &status.Lead{Address: addr, State: &state}, Status: json.RawMessage(`{"task":"Task 3 of c"}`)}
		}}
	rec := httptest.NewRecorder()
	Handler(src).ServeHTTP(rec, httptest.NewRequest("GET", "/overview?machine=&path=/r", nil))
	body := rec.Body.String()
	for _, want := range []string{
		`"context":{"file":"README.md","title":"r","lead":"The r repo."}`,
		`"capabilities":[{"path":"bridge","requirements":2}]`,
		`"name":"c","state":"in progress","done":2,"total":4`,
		`{"source":"register","id":"OI-1","title":"First","anchor":"oi-1-first"}`,
		`"task":"Task 3 of c"`,
		`"sha":"c2"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
	if strings.Contains(body, "NO OPENSPEC") || strings.Contains(body, "NO VISION FILE") {
		t.Errorf("unexpected warning in %s", body)
	}
}
