package gitrepo

import (
	"reflect"
	"strings"
	"testing"
)

type fakeGit struct {
	out   map[string]string // joined args -> stdout
	calls []string
}

func (f *fakeGit) Git(dir string, args ...string) ([]byte, error) {
	k := strings.Join(args, " ")
	f.calls = append(f.calls, k)
	return []byte(f.out[k]), nil
}

func (f *fakeGit) Read(path string) ([]byte, error) { return nil, nil }

func TestLogParsesRefs(t *testing.T) {
	g := &fakeGit{out: map[string]string{}}
	g.out["log --format=%H%x1f%P%x1f%s%x1f%b%x1e -n 5 main"] =
		"abc\x1fp1 p2\x1fsubject\x1fRefs #42\nFixes #43\n\x1e\ndef\x1f\x1fother\x1f\x1e"
	cs, err := Repo{R: g, Dir: "/r"}.Log("main", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("commits = %d", len(cs))
	}
	c := cs[0]
	if c.SHA != "abc" || !reflect.DeepEqual(c.Parents, []string{"p1", "p2"}) || c.Subject != "subject" {
		t.Fatalf("commit = %+v", c)
	}
	if !reflect.DeepEqual(c.Refs, []int{42}) || !reflect.DeepEqual(c.Fixes, []int{43}) {
		t.Fatalf("refs/fixes = %v %v", c.Refs, c.Fixes)
	}
	if len(cs[1].Parents) != 0 {
		t.Fatalf("parents = %v", cs[1].Parents)
	}
}

func TestWorktreesPorcelain(t *testing.T) {
	g := &fakeGit{out: map[string]string{
		"worktree list --porcelain": "worktree /r\nHEAD aaa\nbranch refs/heads/main\n\nworktree /r-go\nHEAD bbb\nbranch refs/heads/striker-go\n",
	}}
	ws, err := Repo{R: g, Dir: "/r"}.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 || ws[1].Path != "/r-go" || ws[1].Branch != "striker-go" || ws[0].Branch != "main" {
		t.Fatalf("worktrees = %+v", ws)
	}
}

type fakeHC struct{ args []string }

func (f *fakeHC) Run(args ...string) ([]byte, error) {
	f.args = args
	return []byte("ok"), nil
}

func TestRemoteRunsThroughHC(t *testing.T) {
	h := &fakeHC{}
	if _, err := (Remote{HC: h, Machine: "remote1"}).Git("/root/x", "status"); err != nil {
		t.Fatal(err)
	}
	want := []string{"exec", "--on", "remote1", "--cwd", "/root/x", "git", "status"}
	if !reflect.DeepEqual(h.args, want) {
		t.Fatalf("args = %v", h.args)
	}
	if _, err := (Remote{HC: h, Machine: "remote1"}).Read("/root/x/f"); err != nil {
		t.Fatal(err)
	}
	want = []string{"file", "read", "--on", "remote1", "/root/x/f"}
	if !reflect.DeepEqual(h.args, want) {
		t.Fatalf("args = %v", h.args)
	}
}

func TestForLocalAndRemote(t *testing.T) {
	h := &fakeHC{}
	for _, m := range []string{"", "local"} {
		if _, ok := For(h, m).(Local); !ok {
			t.Fatalf("For(%q) is not Local", m)
		}
	}
	if r, ok := For(h, "remote1").(Remote); !ok || r.Machine != "remote1" {
		t.Fatalf("For(remote1) = %#v", For(h, "remote1"))
	}
}

func TestHunksU0(t *testing.T) {
	g := &fakeGit{out: map[string]string{
		"diff -U0 HEAD -- p.md": "diff --git a/p.md b/p.md\n--- a/p.md\n+++ b/p.md\n@@ -3,2 +3,1 @@\n-a\n-b\n+c\n",
	}}
	hs, err := Repo{R: g, Dir: "/r"}.Hunks("p.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 1 {
		t.Fatalf("hunks = %+v", hs)
	}
	h := hs[0]
	if h.OldStart != 3 || h.OldLines != 2 || h.NewStart != 3 || h.NewLines != 1 ||
		!reflect.DeepEqual(h.Removed, []string{"a", "b"}) || !reflect.DeepEqual(h.Added, []string{"c"}) {
		t.Fatalf("hunk = %+v", h)
	}
}

func TestDefaultBranchAndBranches(t *testing.T) {
	g := &fakeGit{out: map[string]string{
		"symbolic-ref --short refs/remotes/origin/HEAD":                         "origin/trunk\n",
		"for-each-ref --format=%(refname:short) refs/heads refs/remotes/origin": "main\norigin\norigin/HEAD\norigin/x\n",
	}}
	r := Repo{R: g, Dir: "/r"}
	if b := r.DefaultBranch(); b != "trunk" {
		t.Fatalf("default = %q", b)
	}
	bs, _ := r.Branches()
	if !reflect.DeepEqual(bs, []string{"main", "origin/x"}) {
		t.Fatalf("branches = %v", bs)
	}
	if b := (Repo{R: &fakeGit{out: map[string]string{}}}).DefaultBranch(); b != "main" {
		t.Fatalf("fallback = %q", b)
	}
}

func TestMergeCommitOldest(t *testing.T) {
	g := &fakeGit{out: map[string]string{
		"rev-list --merges --ancestry-path b..main": "new\nold\n",
	}}
	if m := (Repo{R: g}).MergeCommit("b", "main"); m != "old" {
		t.Fatalf("merge = %q", m)
	}
	if m := (Repo{R: &fakeGit{out: map[string]string{}}}).MergeCommit("b", "main"); m != "" {
		t.Fatalf("merge = %q", m)
	}
}
