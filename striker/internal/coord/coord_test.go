package coord

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
)

// fakeHC answers joined args from a map (string output or error); anything
// else is an error. Every call is recorded.
type fakeHC struct {
	out   map[string]any
	calls []string
}

func (f *fakeHC) Run(args ...string) ([]byte, error) {
	k := strings.Join(args, " ")
	f.calls = append(f.calls, k)
	v, ok := f.out[k]
	if !ok {
		return nil, errors.New("unexpected: " + k)
	}
	if err, isErr := v.(error); isErr {
		return nil, err
	}
	return []byte(v.(string)), nil
}

func (f *fakeHC) called(prefix string) []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

var testFleet = fleet.Fleet{
	Repos:     []fleet.Repo{{Name: "alpha", Machine: "remote1", Path: "/srv/alpha", User: "root", Space: "alpha-space"}},
	StatusDir: map[string]string{"remote1": "/var/striker"},
}

// git on remote1 runs as root (the user of the fleet entry), like Claude.
const git = "exec --on remote1 --cwd / sudo -n -i git -C /srv/alpha "

func baseHC(worktrees string) *fakeHC {
	return &fakeHC{out: map[string]any{
		"ls --label striker.coordinator=omega --json": `{"terminals":[]}`,
		// root-only repo: the change metadata is read as root too
		"exec --on remote1 --cwd / sudo -n -i cat /srv/alpha/openspec/changes/omega/.openspec.yaml": "schema: spec-driven\nx-branch: om\n",
		git + "worktree list --porcelain":                        worktrees,
		git + "symbolic-ref --short refs/remotes/origin/HEAD":    "origin/main\n",
		git + "show-ref --verify --quiet refs/heads/om":          errors.New("exit 1"),
		git + "show-ref --verify --quiet refs/remotes/origin/om": errors.New("exit 1"),
		git + "worktree add -b om /srv/alpha-omega main":         "",
		"label m/t9 striker.coordinator=omega":                   "",
		"label m/t9 striker.repo=alpha":                          "",
		"wait m/t9 --timeout 90":                                 "",
		"get m/t9 --json":                                        `{"address":"m/t9","status":{"state":"idle"}}`,
	}}
}

func briefFile(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "coordinator.md")
	os.WriteFile(p, []byte("{repo}|{change}|{status_file}\n"), 0o644)
	return p
}

func TestStartCoordinatorArgv(t *testing.T) {
	f := baseHC("worktree /srv/alpha\nbranch refs/heads/main\n")
	brief := briefFile(t)
	f.out["send m/t9 alpha|omega|/var/striker/alpha--omega.json"] = ""
	var got []string
	run := func(a ...string) ([]byte, error) {
		got = a
		return []byte(`{"space":"s1","tab":"t1","terminal":"m/t9"}`), nil
	}
	addr, err := Start(hc.Client{R: f}, run, testFleet, "alpha", "omega", brief)
	if err != nil || addr != "m/t9" {
		t.Fatalf("addr=%q err=%v calls=%q", addr, err, f.calls)
	}
	if len(f.called(git+"worktree add")) != 1 {
		t.Fatalf("worktree not created: %q", f.calls)
	}
	want := []string{"do", "space.new", "--name", "alpha-omega", "--folder", "/srv/alpha-omega", "--machine", "remote1", "--tab", "coord omega", "--argv"}
	if len(got) != len(want)+1 || !reflect.DeepEqual(got[:len(want)], want) {
		t.Fatalf("herdr-app args %q", got)
	}
	var argv []string
	if err := json.Unmarshal([]byte(got[len(got)-1]), &argv); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(argv, []string{"sudo", "--preserve-env=HC_TERMINAL_ID", "-i", "sh", "-c", "cd '/srv/alpha-omega' && exec /root/.local/bin/claude --remote-control 'coord alpha omega'"}) {
		t.Fatalf("argv %q", argv)
	}
	labels := f.called("label m/t9 ")
	if !reflect.DeepEqual(labels, []string{"label m/t9 striker.repo=alpha", "label m/t9 striker.coordinator=omega"}) || len(f.called("send m/t9 ")) != 1 {
		t.Fatalf("labels (striker.repo first) / brief: %q", f.calls)
	}
}

func TestStartCoordinatorStaleWorktree(t *testing.T) {
	// A registered worktree of the branch whose folder is gone: pruned, then created anew.
	f := baseHC("worktree /srv/alpha\nbranch refs/heads/main\n\nworktree /srv/om\nbranch refs/heads/om\n")
	f.out["exec --on remote1 --cwd / sudo -n -i git -C /srv/om rev-parse --show-toplevel"] = errors.New("not a git repository")
	f.out[git+"worktree prune"] = ""
	f.out["send m/t9 alpha|omega|/var/striker/alpha--omega.json"] = ""
	run := func(a ...string) ([]byte, error) { return []byte(`{"terminal":"m/t9"}`), nil }
	if _, err := Start(hc.Client{R: f}, run, testFleet, "alpha", "omega", briefFile(t)); err != nil {
		t.Fatalf("err=%v calls=%q", err, f.calls)
	}
	if len(f.called(git+"worktree prune")) != 1 || len(f.called(git+"worktree add -b om /srv/alpha-omega main")) != 1 {
		t.Fatalf("calls %q", f.calls)
	}
}

// An empty user is the user hc logs in as: no sudo at all.
func TestStartCoordinatorNonRootGit(t *testing.T) {
	fl := fleet.Fleet{Repos: []fleet.Repo{{Name: "alpha", Machine: "remote1", Path: "/srv/alpha", User: ""}},
		StatusDir: map[string]string{"remote1": "/var/striker"}}
	f := baseHC("")
	plain := "exec --on remote1 --cwd /srv/alpha git "
	f.out[plain+"worktree list --porcelain"] = "worktree /srv/alpha\nbranch refs/heads/om\n"
	f.out["file read --on remote1 /srv/alpha/openspec/changes/omega/.openspec.yaml"] = "x-branch: om\n"
	f.out[plain+"rev-parse --show-toplevel"] = "/srv/alpha\n"
	f.out["send m/t9 alpha|omega|/var/striker/alpha--omega.json"] = ""
	var got []string
	run := func(a ...string) ([]byte, error) { got = a; return []byte(`{"terminal":"m/t9"}`), nil }
	if _, err := Start(hc.Client{R: f}, run, fl, "alpha", "omega", briefFile(t)); err != nil {
		t.Fatalf("err=%v calls=%q", err, f.calls)
	}
	if !strings.Contains(got[len(got)-1], `"sh","-lc"`) || len(f.called("exec --on remote1 --cwd / sudo")) != 0 {
		t.Fatalf("non-root: argv %q calls %q", got, f.calls)
	}
}

// Another user (awayteam): git, cat and Claude run as that user through sudo -u, never as root.
func TestStartCoordinatorAwayteam(t *testing.T) {
	fl := fleet.Fleet{Repos: []fleet.Repo{{Name: "alpha", Machine: "remote1", Path: "/srv/alpha", User: "awayteam"}},
		StatusDir: map[string]string{"remote1": "/var/striker"}}
	as := "exec --on remote1 --cwd / sudo -n -u awayteam -i "
	f := &fakeHC{out: map[string]any{
		"ls --label striker.coordinator=omega --json":                             `{"terminals":[]}`,
		as + "cat /srv/alpha/openspec/changes/omega/.openspec.yaml":               "x-branch: om\n",
		as + "git -C /srv/alpha worktree list --porcelain":                        "worktree /srv/alpha\nbranch refs/heads/main\n",
		as + "git -C /srv/alpha symbolic-ref --short refs/remotes/origin/HEAD":    "origin/main\n",
		as + "git -C /srv/alpha show-ref --verify --quiet refs/heads/om":          errors.New("exit 1"),
		as + "git -C /srv/alpha show-ref --verify --quiet refs/remotes/origin/om": errors.New("exit 1"),
		as + "git -C /srv/alpha worktree add -b om /srv/alpha-omega main":         "",
		"label m/t9 striker.coordinator=omega":                                    "",
		"label m/t9 striker.repo=alpha":                                           "",
		"wait m/t9 --timeout 90":                                                  "",
		"get m/t9 --json":                                                         `{"address":"m/t9","status":{"state":"idle"}}`,
	}}
	// A repo with a user is relayed: the brief tells to ask Striker.
	brief := filepath.Join(t.TempDir(), "coordinator.md")
	os.WriteFile(brief, []byte("{repo}|{agents}\n"), 0o644)
	sent := "send m/t9 " + strings.TrimRight(FillBrief("alpha|{agents}", "alpha", "omega", "/var/striker/alpha--omega.json", true), " \t\r\n")
	if !strings.Contains(sent, `"do": "dispatch"`) {
		t.Fatalf("relayed brief %q", sent)
	}
	f.out[sent] = ""
	var got []string
	run := func(a ...string) ([]byte, error) { got = a; return []byte(`{"terminal":"m/t9"}`), nil }
	if _, err := Start(hc.Client{R: f}, run, fl, "alpha", "omega", brief); err != nil {
		t.Fatalf("err=%v calls=%q", err, f.calls)
	}
	var argv []string
	if err := json.Unmarshal([]byte(got[len(got)-1]), &argv); err != nil {
		t.Fatal(err)
	}
	want := []string{"sudo", "--preserve-env=HC_TERMINAL_ID", "-u", "awayteam", "-i", "sh", "-c", `cd '/srv/alpha-omega' && exec "$HOME/.local/bin/claude" --remote-control 'coord alpha omega'`}
	if !reflect.DeepEqual(argv, want) || len(f.called("exec --on remote1 --cwd / sudo -n -i")) != 0 {
		t.Fatalf("argv %q calls %q", argv, f.calls)
	}
}

func TestStartCoordinatorReusesWorktree(t *testing.T) {
	f := baseHC("worktree /srv/alpha\nbranch refs/heads/main\n\nworktree /srv/om\nbranch refs/heads/om\n")
	f.out["exec --on remote1 --cwd / sudo -n -i git -C /srv/om rev-parse --show-toplevel"] = "/srv/om\n"
	f.out["send m/t9 alpha|omega|/var/striker/alpha--omega.json"] = ""
	var got []string
	run := func(a ...string) ([]byte, error) { got = a; return []byte(`{"terminal":"m/t9"}`), nil }
	if _, err := Start(hc.Client{R: f}, run, testFleet, "alpha", "omega", briefFile(t)); err != nil {
		t.Fatal(err)
	}
	if n := f.called(git + "worktree add"); len(n) != 0 {
		t.Fatalf("worktree added again: %q", n)
	}
	if !strings.Contains(got[len(got)-1], "cd '/srv/om'") {
		t.Fatalf("argv %q", got)
	}
}

func TestStartCoordinatorExistingBranch(t *testing.T) {
	f := baseHC("worktree /srv/alpha\nbranch refs/heads/main\n")
	f.out[git+"show-ref --verify --quiet refs/remotes/origin/om"] = ""
	f.out[git+"worktree add /srv/alpha-omega om"] = ""
	f.out["send m/t9 alpha|omega|/var/striker/alpha--omega.json"] = ""
	run := func(a ...string) ([]byte, error) { return []byte(`{"terminal":"m/t9"}`), nil }
	if _, err := Start(hc.Client{R: f}, run, testFleet, "alpha", "omega", briefFile(t)); err != nil {
		t.Fatalf("err=%v calls=%q", err, f.calls)
	}
}

func TestStartCoordinatorRefusesDuplicate(t *testing.T) {
	f := baseHC("")
	f.out["ls --label striker.coordinator=omega --json"] = `{"terminals":[{"address":"remote1/t1"}]}`
	f.out["get remote1/t1 --json"] = `{"address":"remote1/t1","labels":{"striker.coordinator":"omega","striker.repo":"alpha"}}`
	ran := false
	_, err := Start(hc.Client{R: f}, func(a ...string) ([]byte, error) { ran = true; return nil, nil }, testFleet, "alpha", "omega", "")
	if err == nil || ran || !strings.Contains(err.Error(), "remote1/t1") {
		t.Fatalf("err=%v ran=%v", err, ran)
	}
	if n := f.called(git + "worktree add"); len(n) != 0 {
		t.Fatalf("worktree touched: %q", n)
	}
	// A coordinator for the change on the repo's machine without striker.repo: refuse.
	f.out["get remote1/t1 --json"] = `{"address":"remote1/t1","labels":{"striker.coordinator":"omega"}}`
	if _, err := Start(hc.Client{R: f}, func(a ...string) ([]byte, error) { ran = true; return nil, nil }, testFleet, "alpha", "omega", ""); err == nil || ran || !strings.Contains(err.Error(), "remote1/t1") {
		t.Fatalf("unlabelled same machine: err=%v ran=%v", err, ran)
	}
	// hc unreachable: refuse too.
	f.out["ls --label striker.coordinator=omega --json"] = errors.New("hc down")
	if _, err := Start(hc.Client{R: f}, func(a ...string) ([]byte, error) { ran = true; return nil, nil }, testFleet, "alpha", "omega", ""); err == nil || ran {
		t.Fatalf("ls error: err=%v ran=%v", err, ran)
	}
}

func TestStartCoordinatorSameChangeOtherRepo(t *testing.T) {
	f := baseHC("worktree /srv/alpha\nbranch refs/heads/main\n")
	f.out["ls --label striker.coordinator=omega --json"] = `{"terminals":[{"address":"remote1/t1"}]}`
	f.out["get remote1/t1 --json"] = `{"address":"remote1/t1","labels":{"striker.coordinator":"omega","striker.repo":"beta"}}`
	f.out["send m/t9 alpha|omega|/var/striker/alpha--omega.json"] = ""
	run := func(a ...string) ([]byte, error) { return []byte(`{"terminal":"m/t9"}`), nil }
	if _, err := Start(hc.Client{R: f}, run, testFleet, "alpha", "omega", briefFile(t)); err != nil {
		t.Fatal(err)
	}
}

func TestStartCoordinatorUnknown(t *testing.T) {
	f := baseHC("")
	f.out["ls --label striker.lead=nope --json"] = `{"terminals":[]}`
	if _, err := Start(hc.Client{R: f}, nil, testFleet, "nope", "omega", ""); err == nil {
		t.Fatal("unknown repo accepted")
	}
	delete(f.out, "exec --on remote1 --cwd / sudo -n -i cat /srv/alpha/openspec/changes/omega/.openspec.yaml")
	if _, err := Start(hc.Client{R: f}, nil, testFleet, "alpha", "omega", ""); err == nil || !strings.Contains(err.Error(), "omega") {
		t.Fatalf("missing change: %v", err)
	}
}

func TestStartCoordinatorNotReady(t *testing.T) {
	f := baseHC("worktree /srv/alpha\nbranch refs/heads/main\n")
	f.out["get m/t9 --json"] = `{"address":"m/t9","status":{"state":"blocked"}}`
	run := func(a ...string) ([]byte, error) { return []byte(`{"terminal":"m/t9"}`), nil }
	addr, err := Start(hc.Client{R: f}, run, testFleet, "alpha", "omega", briefFile(t))
	var nr *NotReadyError
	if !errors.As(err, &nr) || addr != "m/t9" || len(f.called("send ")) != 0 {
		t.Fatalf("addr=%q err=%v", addr, err)
	}
	// The message carries the filled values, so the person can send the brief by hand.
	for _, w := range []string{"repo=alpha", "change=omega", "status_file=/var/striker/alpha--omega.json", "m/t9"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("message lacks %q: %s", w, err)
		}
	}
	// An unlabelled local coordinator terminal of the same change is on another machine: no refusal.
	g := baseHC("worktree /srv/alpha\nbranch refs/heads/main\n")
	g.out["ls --label striker.coordinator=omega --json"] = `{"terminals":[{"address":"t1"}]}`
	g.out["get t1 --json"] = `{"address":"t1","labels":{"striker.coordinator":"omega"}}`
	g.out["send m/t9 alpha|omega|/var/striker/alpha--omega.json"] = ""
	if _, err := Start(hc.Client{R: g}, run, testFleet, "alpha", "omega", briefFile(t)); err != nil {
		t.Fatalf("other machine refused: %v", err)
	}
}

// FindWorktree finds the existing worktree of the change's x-branch and the
// git runner of the fleet user; it never creates one.
func TestFindWorktree(t *testing.T) {
	fl := fleet.Fleet{Repos: []fleet.Repo{{Name: "alpha", Machine: "remote1", Path: "/srv/alpha", User: "awayteam"}},
		StatusDir: map[string]string{"remote1": "/var/striker"}}
	as := "exec --on remote1 --cwd / sudo -n -u awayteam -i "
	f := &fakeHC{out: map[string]any{
		as + "cat /srv/alpha/openspec/changes/om/.openspec.yaml": "x-branch: om\n",
		as + "git -C /srv/alpha worktree list --porcelain":       "worktree /srv/alpha\nbranch refs/heads/main\n\nworktree /srv/alpha-om\nbranch refs/heads/om\n",
		as + "git -C /srv/alpha-om rev-parse --show-toplevel":    "/srv/alpha-om\n",
	}}
	p, err := FindWorktree(hc.Client{R: f}, fl, "alpha", "om")
	if err != nil {
		t.Fatalf("err=%v calls=%q", err, f.calls)
	}
	if p.Worktree != "/srv/alpha-om" || p.Machine != "remote1" || p.User != "awayteam" || p.StatusDir != "/var/striker" || p.Repo.Dir != "/srv/alpha" {
		t.Fatalf("place %+v", p)
	}
	n := len(f.calls)
	if _, err := p.Repo.R.Git("/srv/alpha-om", "rev-parse", "--show-toplevel"); err != nil || len(f.called(as+"git -C /srv/alpha-om")) != 2 || len(f.calls) != n+1 {
		t.Fatalf("runner not through sudo -u awayteam: err=%v calls=%q", err, f.calls)
	}
	if len(f.called(as+"git -C /srv/alpha worktree add")) != 0 {
		t.Fatalf("worktree created: %q", f.calls)
	}

	// No worktree of the branch: an error naming the change, nothing created.
	f.out[as+"git -C /srv/alpha worktree list --porcelain"] = "worktree /srv/alpha\nbranch refs/heads/main\n"
	if _, err := FindWorktree(hc.Client{R: f}, fl, "alpha", "om"); err == nil || !strings.Contains(err.Error(), "om") {
		t.Fatalf("missing worktree: %v", err)
	}
	if len(f.called(as+"git -C /srv/alpha worktree add")) != 0 || len(f.called(as+"git -C /srv/alpha worktree prune")) != 0 {
		t.Fatalf("worktree touched: %q", f.calls)
	}
	// Unknown change: an error naming it.
	if _, err := FindWorktree(hc.Client{R: f}, fl, "alpha", "nope"); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("unknown change: %v", err)
	}
}

type fakeGit struct{ out map[string]string }

func (g fakeGit) Git(dir string, args ...string) ([]byte, error) {
	k := dir + " " + strings.Join(args, " ")
	if v, ok := g.out[k]; ok {
		return []byte(v), nil
	}
	return nil, errors.New("unexpected git " + k)
}
func (fakeGit) Read(string) ([]byte, error) { return nil, errors.New("no") }

func TestFindLocalLeadTopLevel(t *testing.T) {
	old := localGit
	defer func() { localGit = old }()
	localGit = fakeGit{out: map[string]string{"/Users/p/herdr-mods/striker rev-parse --show-toplevel": "/Users/p/herdr-mods\n"}}
	f := &fakeHC{out: map[string]any{
		"ls --label striker.lead=herdr-mods --json": `{"terminals":[{"address":"t5"}]}`,
		"get t5 --json": `{"address":"t5","cwd":"/Users/p/herdr-mods/striker","labels":{"striker.lead":"herdr-mods"}}`,
	}}
	got, err := find(hc.Client{R: f}, fleet.Fleet{}, "herdr-mods")
	if err != nil || got.path != "/Users/p/herdr-mods" || got.machine != "" || got.user != "" {
		t.Fatalf("target %+v err %v", got, err)
	}
}

func TestFillBrief(t *testing.T) {
	if got := FillBrief("{repo} {change} {status_file} {repo}", "alpha", "om", "/s/alpha--om.json", false); got != "alpha om /s/alpha--om.json alpha" {
		t.Fatalf("got %q", got)
	}
}

// The shipped brief has {agents} where steps 2 and 3 belong.
func TestBriefFileHasAgents(t *testing.T) {
	tpl, err := os.ReadFile(filepath.Join("..", "..", "briefs", "coordinator.md"))
	if err != nil {
		t.Fatal(err)
	}
	// Issues and Refs/Fixes exist only for direct coordinators: the fixed text says so.
	for _, line := range strings.Split(string(tpl), "\n") {
		if (strings.Contains(line, "striker issues") || strings.Contains(line, "Fixes #")) && !strings.Contains(line, "Nur direkt") {
			t.Errorf("unconditional issue line: %s", line)
		}
	}
	for _, relayed := range []bool{false, true} {
		got := FillBrief(string(tpl), "alpha", "om", "/s/alpha--om.json", relayed)
		if strings.Contains(got, "{agents}") || !strings.Contains(got, "\n2. ") || !strings.Contains(got, "\n3. ") || !strings.Contains(got, "\n4. ") {
			t.Errorf("relayed=%v:\n%s", relayed, got)
		}
	}
}

// {agents}: a direct coordinator starts and reads task agents itself, a
// relayed one asks Striker through requests in its status file.
func TestFillBriefAgents(t *testing.T) {
	direct := FillBrief("{agents}", "alpha", "om", "/s/alpha--om.json", false)
	for _, want := range []string{"striker dispatch <rolle> <task> --plan om", "hc wait <adresse>", "hc read <adresse>", "sleep"} {
		if !strings.Contains(direct, want) {
			t.Errorf("direct lacks %q:\n%s", want, direct)
		}
	}
	if strings.Contains(direct, "requests") || strings.Contains(direct, "{") {
		t.Errorf("direct mentions requests or a placeholder:\n%s", direct)
	}
	relayed := FillBrief("{agents}", "alpha", "om", "/s/alpha--om.json", true)
	for _, want := range []string{
		"/s/alpha--om.json", "requests",
		`{ "id": "2.3-impl-1", "do": "dispatch", "role": "implementer", "task": "2.3" }`,
		`"do": "send", "to": "implementer-2.3"`,
		"beende deinen Zug; Strikers Nachricht bringt das Ergebnis",
		"nie mit `sleep`", "nicht wiederholt",
	} {
		if !strings.Contains(relayed, want) {
			t.Errorf("relayed lacks %q:\n%s", want, relayed)
		}
	}
	if strings.Contains(relayed, "striker dispatch") || strings.Contains(relayed, "hc wait") || strings.Contains(relayed, "{status_file}") {
		t.Errorf("relayed tells to call striker/hc or keeps a placeholder:\n%s", relayed)
	}
}

func TestArgvLocalUserAndValidation(t *testing.T) {
	a, err := Argv("herdr-mods", "repo-overview", "/Users/p/herdr-mods-repo-overview", "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"do", "space.new", "--name", "herdr-mods-repo-overview", "--folder", "/Users/p/herdr-mods-repo-overview", "--tab", "coord repo-overview", "--argv"}
	var argv []string
	if len(a) != len(want)+1 || !reflect.DeepEqual(a[:len(want)], want) || json.Unmarshal([]byte(a[len(want)]), &argv) != nil ||
		!reflect.DeepEqual(argv, []string{"sh", "-lc", "cd '/Users/p/herdr-mods-repo-overview' && exec claude --remote-control 'coord herdr-mods repo-overview'"}) {
		t.Fatalf("argv %q", a)
	}
	// a worktree on this Windows computer: Herdr starts Claude in the folder itself (no sh), and no --machine
	// (Herdr refuses an empty one)
	w, err := Argv("herdr-mods", "striker-relay", "C:/Users/p/herdr-mods-striker-relay", "", "")
	if err != nil {
		t.Fatal(err)
	}
	wantW := []string{"do", "space.new", "--name", "herdr-mods-striker-relay", "--folder", "C:/Users/p/herdr-mods-striker-relay",
		"--tab", "coord striker-relay", "--argv", `["claude","--remote-control","coord herdr-mods striker-relay"]`}
	if !reflect.DeepEqual(w, wantW) {
		t.Fatalf("windows argv %q", w)
	}
	for _, bad := range [][3]string{
		{"alpha", "../x", "/srv/x"},
		{"alpha", "Bad Name", "/srv/x"},
		{"alpha", "ok", "/srv/it's"},
		{"alpha", "ok", "relative/x"},
		{"a/b", "ok", "/srv/x"},
	} {
		if _, err := Argv(bad[0], bad[1], bad[2], "remote1", "root"); err == nil {
			t.Errorf("Argv%q accepted", bad)
		}
	}
}
