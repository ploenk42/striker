// Package gitrepo reads git state of a repo, locally or on a remote machine via hc.
package gitrepo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ploenk42/striker/striker/internal/hc"
)

// Runner runs git in a directory and reads files.
type Runner interface {
	Git(dir string, args ...string) ([]byte, error)
	Read(path string) ([]byte, error)
}

// Local runs git on this machine.
type Local struct{}

func (Local) Git(dir string, args ...string) ([]byte, error) {
	return hc.Exec{Bin: "git"}.Run(append([]string{"-C", dir}, args...)...)
}

func (Local) Read(path string) ([]byte, error) { return os.ReadFile(path) }

// Remote runs git on another machine through hc.
type Remote struct {
	HC      hc.Runner
	Machine string
}

func (r Remote) Git(dir string, args ...string) ([]byte, error) {
	a := append([]string{"exec", "--on", r.Machine, "--cwd", dir, "git"}, args...)
	return r.HC.Run(a...)
}

func (r Remote) Read(path string) ([]byte, error) {
	return r.HC.Run("file", "read", "--on", r.Machine, path)
}

// For picks Local for an empty machine, else Remote.
func For(h hc.Runner, machine string) Runner {
	if machine == "" || machine == "local" {
		return Local{}
	}
	return Remote{HC: h, Machine: machine}
}

type Commit struct {
	SHA     string   `json:"sha"`
	Parents []string `json:"parents"`
	Subject string   `json:"subject"`
	Refs    []int    `json:"refs"`
	Fixes   []int    `json:"fixes"`
}

type Worktree struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

type Hunk struct {
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Removed  []string `json:"removed"`
	Added    []string `json:"added"`
}

// Repo is one repository directory reached through a Runner.
type Repo struct {
	R   Runner
	Dir string
}

var (
	refsRe  = regexp.MustCompile(`(?i)\brefs\s+#(\d+)`)
	fixesRe = regexp.MustCompile(`(?i)\bfixes\s+#(\d+)`)
	hunkRe  = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
)

func (r Repo) git(args ...string) ([]byte, error) { return r.R.Git(r.Dir, args...) }

func nonEmptyLines(b []byte) []string {
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// DefaultBranch is origin's HEAD branch, else "main".
func (r Repo) DefaultBranch() string {
	out, err := r.git("symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	b := strings.TrimPrefix(strings.TrimSpace(string(out)), "origin/")
	if err != nil || b == "" {
		return "main"
	}
	return b
}

func numbers(re *regexp.Regexp, s string) []int {
	var out []int
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[1])
		out = append(out, n)
	}
	return out
}

// Log returns up to n commits reachable from ref, newest first.
func (r Repo) Log(ref string, n int) ([]Commit, error) {
	out, err := r.git("log", "--format=%H%x1f%P%x1f%s%x1f%b%x1e", "-n", strconv.Itoa(n), ref)
	if err != nil {
		return nil, err
	}
	var cs []Commit
	for _, rec := range strings.Split(string(out), "\x1e") {
		rec = strings.TrimLeft(rec, "\r\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		f := strings.SplitN(rec, "\x1f", 4)
		for len(f) < 4 {
			f = append(f, "")
		}
		text := f[2] + "\n" + f[3]
		cs = append(cs, Commit{
			SHA:     f[0],
			Parents: strings.Fields(f[1]),
			Subject: f[2],
			Refs:    numbers(refsRe, text),
			Fixes:   numbers(fixesRe, text),
		})
	}
	return cs, nil
}

// Worktrees parses `git worktree list --porcelain`.
func (r Repo) Worktrees() ([]Worktree, error) {
	out, err := r.git("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var ws []Worktree
	for _, l := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
		switch {
		case strings.HasPrefix(l, "worktree "):
			ws = append(ws, Worktree{Path: l[len("worktree "):]})
		case strings.HasPrefix(l, "branch ") && len(ws) > 0:
			ws[len(ws)-1].Branch = strings.TrimPrefix(l[len("branch "):], "refs/heads/")
		}
	}
	return ws, nil
}

// Branches lists local branches and origin/* remote branches (no origin/HEAD).
func (r Repo) Branches() ([]string, error) {
	out, err := r.git("for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes/origin")
	if err != nil {
		return nil, err
	}
	var bs []string
	for _, l := range nonEmptyLines(out) {
		if l == "origin/HEAD" || l == "origin" {
			continue
		}
		bs = append(bs, l)
	}
	return bs, nil
}

// MergeCommit is the oldest merge on the path from branch into `into`, or "".
func (r Repo) MergeCommit(branch, into string) string {
	out, err := r.git("rev-list", "--merges", "--ancestry-path", branch+".."+into)
	if err != nil {
		return ""
	}
	ls := nonEmptyLines(out)
	if len(ls) == 0 {
		return ""
	}
	return ls[len(ls)-1]
}

// ListFiles lists the entries of dir at ref (paths as git prints them).
func (r Repo) ListFiles(ref, dir string) ([]string, error) {
	out, err := r.git("ls-tree", "--name-only", ref, strings.TrimSuffix(dir, "/")+"/")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

// Show returns the content of path at ref.
func (r Repo) Show(ref, path string) ([]byte, error) { return r.git("show", ref+":"+path) }

// showChunk is how many files one git show reads (keeps the command line short).
var showChunk = 100

// ShowAll returns the contents of paths at ref with two git calls per showChunk files (ls-tree for the blob
// sizes, one git show for all of them, split by size) instead of one git show per file: each git process costs
// about 150 ms on Windows. Paths that are no blob at ref are left out. An error means nothing usable was read;
// callers then fall back to Show.
func (r Repo) ShowAll(ref string, paths []string) (map[string][]byte, error) {
	got := map[string][]byte{}
	for len(paths) > 0 {
		n := min(len(paths), showChunk)
		if err := r.showSome(ref, paths[:n], got); err != nil {
			return nil, err
		}
		paths = paths[n:]
	}
	return got, nil
}

func (r Repo) showSome(ref string, paths []string, got map[string][]byte) error {
	out, err := r.git(append([]string{"ls-tree", "-r", "-l", "-z", ref, "--"}, paths...)...)
	if err != nil {
		return err
	}
	size := map[string]int{}
	for _, e := range strings.Split(string(out), "\x00") {
		meta, p, ok := strings.Cut(e, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 4 || f[1] != "blob" {
			continue
		}
		if n, err := strconv.Atoi(f[3]); err == nil && n >= 0 {
			size[p] = n
		}
	}
	args := []string{"show", "--no-textconv"}
	var want []string
	total := 0
	for _, p := range paths {
		if n, ok := size[p]; ok {
			want = append(want, p)
			args = append(args, ref+":"+p)
			total += n
		}
	}
	if len(want) == 0 {
		return nil
	}
	out, err = r.git(args...)
	if err != nil {
		return err
	}
	if len(out) != total {
		return fmt.Errorf("git show %s: %d bytes for %d files, want %d", ref, len(out), len(want), total)
	}
	at := 0
	for _, p := range want {
		got[p] = out[at : at+size[p] : at+size[p]]
		at += size[p]
	}
	return nil
}

// ReadWorking reads a file of the working tree.
func (r Repo) ReadWorking(rel string) ([]byte, error) {
	return r.R.Read(filepath.ToSlash(filepath.Join(r.Dir, rel)))
}

// Hunks returns the uncommitted changes of rel against HEAD.
func (r Repo) Hunks(rel string) ([]Hunk, error) {
	out, err := r.git("diff", "-U0", "HEAD", "--", rel)
	if err != nil {
		return nil, err
	}
	var hs []Hunk
	for _, l := range strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n") {
		if m := hunkRe.FindStringSubmatch(l); m != nil {
			h := Hunk{OldLines: 1, NewLines: 1}
			h.OldStart, _ = strconv.Atoi(m[1])
			if m[2] != "" {
				h.OldLines, _ = strconv.Atoi(m[2])
			}
			h.NewStart, _ = strconv.Atoi(m[3])
			if m[4] != "" {
				h.NewLines, _ = strconv.Atoi(m[4])
			}
			hs = append(hs, h)
			continue
		}
		if len(hs) == 0 || l == "" {
			continue
		}
		h := &hs[len(hs)-1]
		switch l[0] {
		case '-':
			h.Removed = append(h.Removed, l[1:])
		case '+':
			h.Added = append(h.Added, l[1:])
		}
	}
	return hs, nil
}
