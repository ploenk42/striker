package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// a real repo: ShowAll must give each file's bytes exactly, as one git show per file would
func TestShowAllRealRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	files := map[string]string{
		"openspec/config.yaml":          "schema: spec-driven\n",
		"openspec/changes/a/tasks.md":   "- [ ] 1 one\r\n- [x] 2 two\n",
		"openspec/changes/a/design.md":  "no newline at end",
		"openspec/changes/b c/empty.md": "",
		"openspec/specs/x/spec.md":      "# X\n\nÄnderung ✓\n",
	}
	for p, s := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "core.autocrlf=false", "add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "x"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	r := Repo{R: Local{}, Dir: dir}
	paths := []string{"openspec/config.yaml", "openspec/changes/a/tasks.md", "openspec/changes/a/design.md",
		"openspec/changes/b c/empty.md", "openspec/specs/x/spec.md", "openspec/missing.md"}
	got, err := r.ShowAll("main", paths)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]byte{}
	for p, s := range files {
		want[p] = []byte(s)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d files, want %d: %q", len(got), len(want), got)
	}
	for p, b := range want {
		if !reflect.DeepEqual(got[p], b) && !(len(b) == 0 && len(got[p]) == 0) {
			t.Errorf("%s: %q, want %q", p, got[p], b)
		}
		if _, ok := got[p]; !ok {
			t.Errorf("%s missing", p)
		}
	}
	// chunks: the same with a tiny chunk size
	old := showChunk
	showChunk = 2
	defer func() { showChunk = old }()
	got2, err := r.ShowAll("main", paths)
	if err != nil || !reflect.DeepEqual(len(got2), len(got)) || string(got2["openspec/specs/x/spec.md"]) != files["openspec/specs/x/spec.md"] {
		t.Fatalf("chunked: %v %q", err, got2)
	}
}

// a runner whose show output does not add up is an error, so callers fall back to one show per file
func TestShowAllBadSizes(t *testing.T) {
	f := &fakeGit{out: map[string]string{
		"ls-tree -r -l -z main -- a.md": "100644 blob 0123 10\ta.md\x00",
		"show --no-textconv main:a.md":  "short",
	}}
	if _, err := (Repo{R: f, Dir: "/x"}).ShowAll("main", []string{"a.md"}); err == nil {
		t.Fatal("want an error")
	}
}
