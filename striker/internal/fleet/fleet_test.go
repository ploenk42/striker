package fleet

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFleetWithBOM(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fleet.json")
	body := "\xEF\xBB\xBF" + `{"repos":[{"name":"alpha","machine":"remote1"}],"status_dir":{"remote1":"/s"}}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Repos) != 1 || f.Repos[0].Name != "alpha" || f.Repos[0].Machine != "remote1" || f.StatusDir["remote1"] != "/s" {
		t.Fatalf("got %+v", f)
	}
}

func TestRepoAt(t *testing.T) {
	f := Fleet{Repos: []Repo{{Name: "alpha", Machine: "remote1", Path: "/srv/alpha"}, {Name: "loc", Path: "/Users/p/loc"}}}
	for _, tc := range []struct{ machine, path, want string }{
		{"remote1", "/srv/alpha", "alpha"},
		{"remote1", "/srv/alpha/", "alpha"},
		{"", "/srv/alpha", ""},
		{"", "/Users/p/loc", "loc"},
		{"local", "/Users/p/loc", "loc"},
		{"remote1", "/srv/beta", ""},
	} {
		if got := f.RepoAt(tc.machine, tc.path); got != tc.want {
			t.Errorf("RepoAt(%q, %q) = %q, want %q", tc.machine, tc.path, got, tc.want)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "fleet.json"))
	if err == nil {
		t.Fatal("want error")
	}
	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "fleet.example.json") {
		t.Fatalf("error does not point to the example: %v", err)
	}
}

func TestExampleFleetLoads(t *testing.T) {
	f, err := Load(filepath.Join("..", "..", "fleet.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Repos) != 5 || f.StatusDir["remote1"] == "" || f.AgentsDir["remote1"] == "" {
		t.Fatalf("got %+v", f)
	}
}
