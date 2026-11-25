// Package coord starts the coordinator of an OpenSpec change: a Claude in tab
// "coord <change>" of the change's worktree space, labelled
// striker.repo=<repo> and striker.coordinator=<change>, briefed with
// briefs/coordinator.md.
package coord

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/openspec"
)

var (
	changeRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	repoRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	branchRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

// ValidChange reports whether change is a valid change name (kebab-case).
func ValidChange(change string) bool { return changeRe.MatchString(change) }

// localGit runs git on this machine (tests replace it).
var localGit gitrepo.Runner = gitrepo.Local{}

// NotReadyError: the coordinator started but is blocked/working; the brief
// was not sent. It carries the values to fill in by hand.
type NotReadyError struct {
	Address, State           string
	Repo, Change, StatusFile string
	BriefPath                string
}

func (e *NotReadyError) Error() string {
	return fmt.Sprintf("Der Koordinator ist noch nicht bereit (Status: %s). Bildschirm prüfen: hc read %s --lines 30; danach die Einweisung %s schicken mit repo=%s change=%s status_file=%s.",
		e.State, e.Address, e.BriefPath, e.Repo, e.Change, e.StatusFile)
}

// The steps for starting, waiting for and reading task agents ({agents} in
// the brief): direct coordinators call striker dispatch and hc themselves,
// relayed ones (a fleet repo with a user, no hc socket) ask Striker through
// requests in their status file.
var (
	//go:embed agents-direct.md
	agentsDirect string
	//go:embed agents-relayed.md
	agentsRelayed string
)

// FillBrief fills the coordinator brief placeholders; relayed picks the
// {agents} variant.
func FillBrief(tpl, repo, change, statusFile string, relayed bool) string {
	agents := agentsDirect
	if relayed {
		agents = agentsRelayed
	}
	tpl = strings.ReplaceAll(tpl, "{agents}", strings.TrimRight(agents, " \t\r\n"))
	return strings.NewReplacer("{repo}", repo, "{change}", change, "{status_file}", statusFile).Replace(tpl)
}

// Argv is the herdr-app argument list that opens the space <repo>-<change>
// in folder worktree on machine ("" = this computer) with the tab
// "coord <change>" running Claude there as the fleet entry's user, like
// start-lead (fleet.ClaudeArgv).
func Argv(repo, change, worktree, machine, user string) ([]string, error) {
	if !repoRe.MatchString(repo) || strings.Contains(repo, "..") {
		return nil, fmt.Errorf("ungültiger Repo-Name %q", repo)
	}
	if !changeRe.MatchString(change) {
		return nil, fmt.Errorf("ungültiger Change-Name %q (kebab-case)", change)
	}
	// a Windows path is only for this computer without a fleet user: Herdr starts Claude in the space's folder
	windows := machine == "" && user == "" && winPath.MatchString(worktree)
	if !(strings.HasPrefix(worktree, "/") || windows) || strings.ContainsAny(worktree, "'\n") {
		return nil, fmt.Errorf("ungültiger Worktree-Pfad %q", worktree)
	}
	name := "coord " + repo + " " + change
	argv := fleet.ClaudeArgv(user, worktree, name)
	if windows {
		argv = []string{"claude", "--remote-control", name}
	}
	b, _ := json.Marshal(argv)
	args := []string{"do", "space.new", "--name", repo + "-" + change, "--folder", worktree}
	if machine != "" {
		args = append(args, "--machine", machine) // Herdr refuses an empty --machine; without it: this computer
	}
	return append(args, "--tab", "coord "+change, "--argv", string(b)), nil
}

var winPath = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

// userGit runs git and reads working-tree files as the fleet entry's user
// (fleet.Sudo: fails instead of asking for a password), on another machine
// through hc exec.
type userGit struct {
	h       hc.Runner
	machine string
	user    string
}

func (g userGit) sudo(args ...string) ([]byte, error) {
	a := append(fleet.Sudo(g.user), args...)
	if g.machine == "" {
		return hc.Exec{Bin: a[0]}.Run(a[1:]...)
	}
	return g.h.Run(append([]string{"exec", "--on", g.machine, "--cwd", "/"}, a...)...)
}

func (g userGit) Git(dir string, args ...string) ([]byte, error) {
	return g.sudo(append([]string{"git", "-C", dir}, args...)...)
}

// Read reads a working-tree file as that user too (repos only it may read).
func (g userGit) Read(p string) ([]byte, error) { return g.sudo("cat", p) }

// target is the repo a coordinator works for.
type target struct {
	machine, path, user, statusDir string
}

// runner reaches git as the user that runs Claude for the repo.
func (t target) runner(h hc.Runner) gitrepo.Runner {
	if t.user != "" {
		return userGit{h: h, machine: t.machine, user: t.user}
	}
	if t.machine == "" {
		return localGit
	}
	return gitrepo.For(h, t.machine)
}

// find looks the repo up in the fleet, else as a local lead (striker.lead):
// the top of the git worktree its cwd is in.
func find(c hc.Client, f fleet.Fleet, repo string) (target, error) {
	for _, r := range f.Repos {
		if r.Name == repo {
			t := target{machine: r.Machine, path: r.Path, user: r.User}
			if r.Machine != "" {
				t.statusDir = f.StatusDir[r.Machine]
				if t.statusDir == "" {
					return t, fmt.Errorf("fleet.json hat kein status_dir für %s", r.Machine)
				}
			}
			return t, nil
		}
	}
	ts, err := c.ListLabel("striker.lead", repo)
	if err != nil {
		return target{}, fmt.Errorf("Repo '%s' steht nicht in fleet.json, lokale Leads nicht lesbar: %w", repo, err)
	}
	for _, t := range ts {
		if !hc.IsLocal(t.Address) {
			continue
		}
		info, err := c.Get(t.Address)
		if err != nil || info.Cwd == "" {
			continue
		}
		out, err := localGit.Git(info.Cwd, "rev-parse", "--show-toplevel")
		if err != nil {
			return target{}, fmt.Errorf("Lead %s von %s steht nicht in einem git-Worktree (%s): %w", t.Address, repo, info.Cwd, err)
		}
		return target{path: strings.TrimSpace(string(out))}, nil
	}
	return target{}, fmt.Errorf("Repo '%s' steht nicht in fleet.json und hat keinen lokalen Lead.", repo)
}

// changeBranch reads the change's metadata in the repo and returns its
// branch (x-branch, default the change name), checked.
func changeBranch(r gitrepo.Repo, change string) (string, error) {
	meta, err := r.ReadWorking("openspec/changes/" + change + "/.openspec.yaml")
	if err != nil {
		return "", fmt.Errorf("Change '%s' nicht gefunden (openspec/changes/%s/.openspec.yaml in %s): %w", change, change, r.Dir, err)
	}
	_, branch := openspec.ParseMeta(meta)
	if branch == "" {
		branch = change
	}
	if !branchRe.MatchString(branch) || strings.Contains(branch, "..") {
		return "", fmt.Errorf("ungültiger x-branch %q", branch)
	}
	return branch, nil
}

// registered returns the registered worktree of branch and whether its folder
// is still a git worktree ("" if none is registered).
func registered(r gitrepo.Repo, branch string) (string, bool, error) {
	wts, err := r.Worktrees()
	if err != nil {
		return "", false, fmt.Errorf("git worktree list: %w", err)
	}
	for _, w := range wts {
		if w.Branch != branch {
			continue
		}
		_, err := r.R.Git(w.Path, "rev-parse", "--show-toplevel")
		return w.Path, err == nil, nil
	}
	return "", false, nil
}

// Place is where a change is worked on: its repo's machine, user and status
// folder, the change worktree and the repo with a git runner as that user.
type Place struct {
	Machine, User, StatusDir, Worktree string
	Repo                               gitrepo.Repo
}

// FindWorktree finds the existing worktree of change's x-branch in repo,
// like Start, but never creates or prunes one: a missing worktree is an error.
func FindWorktree(c hc.Client, f fleet.Fleet, repo, change string) (Place, error) {
	if !changeRe.MatchString(change) {
		return Place{}, fmt.Errorf("ungültiger Change-Name %q (kebab-case)", change)
	}
	t, err := find(c, f, repo)
	if err != nil {
		return Place{}, err
	}
	r := gitrepo.Repo{R: t.runner(c.R), Dir: t.path}
	branch, err := changeBranch(r, change)
	if err != nil {
		return Place{}, err
	}
	wt, ok, err := registered(r, branch)
	if err != nil {
		return Place{}, err
	}
	if !ok {
		if wt != "" {
			return Place{}, fmt.Errorf("Worktree %s von Change '%s' (Branch %s) fehlt; striker start-coordinator %s %s legt ihn neu an", wt, change, branch, repo, change)
		}
		return Place{}, fmt.Errorf("Change '%s' von %s hat keinen Worktree (Branch %s); striker start-coordinator %s %s legt ihn an", change, repo, branch, repo, change)
	}
	return Place{Machine: t.machine, User: t.user, StatusDir: t.statusDir, Worktree: wt, Repo: r}, nil
}

// worktree returns the worktree of branch if it still exists, else creates
// <dir of repo>/<repo>-<change> (pruning a stale registration first), from
// the branch if it exists locally or on origin, else as a new branch from
// the default branch.
func worktree(r gitrepo.Repo, repo, change, branch string) (string, error) {
	wt, ok, err := registered(r, branch)
	if err != nil {
		return "", err
	}
	if ok {
		return wt, nil
	}
	if wt != "" {
		if out, err := r.R.Git(r.Dir, "worktree", "prune"); err != nil {
			return "", fmt.Errorf("Worktree %s von %s fehlt, git worktree prune scheiterte: %s%w", wt, branch, out, err)
		}
	}
	dir := path.Join(path.Dir(strings.ReplaceAll(r.Dir, `\`, "/")), repo+"-"+change)
	def := r.DefaultBranch()
	args := []string{"worktree", "add", "-b", branch, dir, def}
	for _, ref := range []string{"refs/heads/" + branch, "refs/remotes/origin/" + branch} {
		if _, err := r.R.Git(r.Dir, "show-ref", "--verify", "--quiet", ref); err == nil {
			args = []string{"worktree", "add", dir, branch}
			break
		}
	}
	if out, err := r.R.Git(r.Dir, args...); err != nil {
		return "", fmt.Errorf("git %s: %s%w", strings.Join(args, " "), out, err)
	}
	return dir, nil
}

func machineOf(addr string) string {
	if m, _, remote := strings.Cut(addr, "/"); remote {
		return m
	}
	return ""
}

// Start starts the coordinator of change in repo and returns its address.
// run executes herdr-app commands.
func Start(c hc.Client, run func(args ...string) ([]byte, error), f fleet.Fleet, repo, change, briefPath string) (string, error) {
	if !changeRe.MatchString(change) {
		return "", fmt.Errorf("ungültiger Change-Name %q (kebab-case)", change)
	}
	t, err := find(c, f, repo)
	if err != nil {
		return "", err
	}

	existing, err := c.ListLabel("striker.coordinator", change)
	if err != nil {
		return "", fmt.Errorf("kann nicht prüfen, ob %s schon einen Koordinator hat: %w", change, err)
	}
	for _, e := range existing {
		info, err := c.Get(e.Address)
		if err != nil {
			return "", fmt.Errorf("kann Koordinator %s nicht lesen: %w", e.Address, err)
		}
		switch r := info.Labels["striker.repo"]; {
		case r == repo:
			return "", fmt.Errorf("Change '%s' von %s hat schon einen Koordinator: %s", change, repo, e.Address)
		case r == "" && machineOf(e.Address) == t.machine:
			return "", fmt.Errorf("auf %s läuft schon ein Koordinator für '%s' ohne Label striker.repo: %s (prüfen, ggf. hc label %s striker.repo=<repo>)", e.Address, change, e.Address, e.Address)
		}
	}

	r := gitrepo.Repo{R: t.runner(c.R), Dir: t.path}
	branch, err := changeBranch(r, change)
	if err != nil {
		return "", err
	}
	wt, err := worktree(r, repo, change, branch)
	if err != nil {
		return "", err
	}

	args, err := Argv(repo, change, wt, t.machine, t.user)
	if err != nil {
		return "", err
	}
	out, err := run(args...)
	if err != nil {
		return "", fmt.Errorf("space.new scheiterte: %s%v", out, err)
	}
	var made struct {
		Terminal string `json:"terminal"`
	}
	if err := json.Unmarshal(out, &made); err != nil || made.Terminal == "" {
		return "", fmt.Errorf("space.new gab kein Terminal zurück: %s", out)
	}
	addr := made.Terminal
	fmt.Fprintf(os.Stderr, "Space '%s-%s' mit Tab 'coord %s' angelegt (Worktree %s), Koordinator ist %s.\n", repo, change, change, wt, addr)

	// striker.repo first: a terminal with striker.coordinator but no striker.repo
	// is refused as a duplicate above.
	for _, kv := range []string{"striker.repo=" + repo, "striker.coordinator=" + change} {
		if err := c.Label(addr, kv); err != nil {
			return addr, fmt.Errorf("Terminal %s läuft, hat aber kein Label %s (von Hand: hc label %s %s): %w", addr, kv, addr, kv, err)
		}
	}

	statusFile := t.statusDir + "/" + repo + "--" + change + ".json"
	if t.machine == "" {
		uh, _ := os.UserHomeDir()
		statusFile = filepath.Join(uh, ".striker", "status", repo+"--"+change+".json")
	}
	_ = c.Wait(addr, 90) // a timeout is judged by the state below
	info, err := c.Get(addr)
	if err != nil {
		return addr, err
	}
	if info.Status != nil && (info.Status.State == "blocked" || info.Status.State == "working") {
		return addr, &NotReadyError{Address: addr, State: info.Status.State, Repo: repo, Change: change, StatusFile: statusFile, BriefPath: briefPath}
	}

	tpl, err := os.ReadFile(briefPath)
	if err != nil {
		return addr, err
	}
	if err := c.Send(addr, strings.TrimRight(FillBrief(string(tpl), repo, change, statusFile, t.user != ""), " \t\r\n")); err != nil {
		return addr, fmt.Errorf("Einweisung nicht zugestellt: %w", err)
	}
	return addr, nil
}
