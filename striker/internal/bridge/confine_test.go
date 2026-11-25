package bridge

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/hc"
)

// confGit answers git for a repo whose top is /r with one tracked file.
type confGit struct{}

func (confGit) Git(dir string, args ...string) ([]byte, error) {
	k := strings.Join(args, " ")
	switch {
	case k == "rev-parse --show-toplevel":
		if dir == "/r" || dir == "/r/" {
			return []byte("/r\n"), nil
		}
		if dir == "/r/sub" {
			return []byte("/r\n"), nil
		}
		return nil, errors.New("not a git repository")
	case k == "ls-files --error-unmatch -- docs/a.md":
		return []byte("docs/a.md\n"), nil
	case strings.HasPrefix(k, "ls-files --error-unmatch"):
		return nil, errors.New("pathspec did not match")
	case strings.HasPrefix(k, "show HEAD:"):
		return []byte("base"), nil
	case strings.HasPrefix(k, "diff"):
		return nil, nil
	}
	return nil, errors.New("fake git: " + k)
}

func (confGit) Read(path string) ([]byte, error) { return []byte("text"), nil }

func realSrc() Source {
	return real{h: hc.Client{}, runner: func(string) gitrepo.Runner { return confGit{} }}
}

func TestDocConfinement(t *testing.T) {
	h := Handler(realSrc())
	rec := func(url string) (int, string) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		return w.Code, w.Body.String()
	}
	rejected := []string{
		"/doc?path=/r&file=../x.md",
		"/doc?path=/r&file=a/../../x.md",
		"/doc?path=/r&file=/etc/passwd.md",
		"/doc?path=/r&file=C:/x.md",
		"/doc?path=/r&file=docs/a.txt",
		"/doc?path=/r&file=docs/untracked.md",
		"/doc?path=/r/sub&file=docs/a.md",
		"/doc?path=/nowhere&file=docs/a.md",
	}
	for _, u := range rejected {
		code, body := rec(u)
		if code == 200 && !strings.Contains(body, `"error"`) {
			t.Errorf("%s: accepted: %d %s", u, code, body)
		}
	}
	code, body := rec("/doc?path=/r&file=docs/a.md")
	if code != 200 || strings.Contains(body, `"error"`) || !strings.Contains(body, `"text":"text"`) {
		t.Errorf("good request: %d %s", code, body)
	}
	for _, u := range []string{"/space?path=/r/sub", "/repo?path=/r/sub", "/overview?path=/r/sub"} {
		if code, body := rec(u); code != 200 || !strings.Contains(body, "not a worktree top") {
			t.Errorf("%s: %d %s", u, code, body)
		}
	}
}
