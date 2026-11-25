// Package bridge serves the repo, space, doc and overview endpoints of the
// LCARS Bridge.
package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/graph"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/issues"
	"github.com/ploenk42/striker/striker/internal/openspec"
	"github.com/ploenk42/striker/striker/internal/overview"
	"github.com/ploenk42/striker/striker/internal/plans"
	"github.com/ploenk42/striker/striker/internal/repoview"
	"github.com/ploenk42/striker/striker/internal/status"
)

const (
	repoTTL  = 60 * time.Second
	spaceTTL = 10 * time.Second
)

// Doc is a file of the working tree with its HEAD version and the hunks
// between them.
type Doc struct {
	Text  string         `json:"text"`
	Base  string         `json:"base"`
	Hunks []gitrepo.Hunk `json:"hunks"`
}

// Source reads repo state for the handlers. Branch is the branch checked out
// in the worktree at path ("" for a detached or unknown worktree). Parts
// reads what /overview needs beyond the repo view; Live the repo's lead and
// coordinators.
type Source interface {
	Repo(machine, path string) (repoview.View, error)
	Branch(machine, path string) (string, error)
	Doc(machine, path, rel string) (Doc, error)
	Parts(machine, path string) (overview.Parts, error)
	Live(machine, path string) status.Repo
}

type real struct {
	h  hc.Client
	gh issues.GH
	// runner overrides how git is reached (tests); nil: locally or through hc.
	runner func(machine string) gitrepo.Runner
	// extra gives the repo's fleet name, coordinator status reader and lead
	// liveness for one request (nil: none).
	extra func(machine, path string) repoview.Extra
	// live gives the repo's lead, its status file and the coordinators
	// (nil: none).
	live func(machine, path string) status.Repo
}

// NewSource reads from git (locally or through hc) and GitHub; extra and
// live (either may be nil) add what fleet.json, hc labels and the status
// files know about the repo.
func NewSource(h hc.Client, gh issues.GH, extra func(machine, path string) repoview.Extra,
	live func(machine, path string) status.Repo) Source {
	return real{h: h, gh: gh, extra: extra, live: live}
}

func (s real) repo(machine, path string) gitrepo.Repo {
	if s.runner != nil {
		return gitrepo.Repo{R: s.runner(machine), Dir: path}
	}
	return gitrepo.Repo{R: gitrepo.For(hc.Exec{}, machine), Dir: path}
}

func normTop(p string) string {
	p = strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(p), `\`, "/"), "/")
	if len(p) >= 2 && p[1] == ':' { // Windows drive letters are case-insensitive
		p = strings.ToLower(p[:1]) + p[1:]
	}
	return p
}

// checkTop accepts only the top directory of a worktree, so a request cannot
// make striker read an arbitrary directory.
func checkTop(r gitrepo.Repo) error {
	out, err := r.R.Git(r.Dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return fmt.Errorf("%s is not a git worktree", r.Dir)
	}
	if normTop(string(out)) != normTop(r.Dir) {
		return fmt.Errorf("%s is not a worktree top", r.Dir)
	}
	return nil
}

// mainPath is the main checkout of the worktree r (its git common dir
// without /.git), so a worktree space finds the repo's fleet entry and lead;
// path itself when that cannot be told.
func mainPath(r gitrepo.Repo) string {
	out, err := r.R.Git(r.Dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return r.Dir
	}
	common := strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(string(out)), `\`, "/"), "/")
	if m, ok := strings.CutSuffix(common, "/.git"); ok && m != "" {
		return m
	}
	return r.Dir
}

func (s real) Repo(machine, path string) (repoview.View, error) {
	r := s.repo(machine, path)
	if err := checkTop(r); err != nil {
		return repoview.View{}, err
	}
	var x repoview.Extra
	if s.extra != nil {
		x = s.extra(machine, mainPath(r))
	}
	in, err := repoview.Gather(r, s.h, s.gh, time.Now(), x)
	if err != nil {
		return repoview.View{}, err
	}
	return repoview.Build(in), nil
}

func (s real) Branch(machine, path string) (string, error) {
	r := s.repo(machine, path)
	if err := checkTop(r); err != nil {
		return "", err
	}
	wts, err := r.Worktrees()
	if err != nil {
		return "", err
	}
	want := filepath.Clean(path)
	for _, w := range wts {
		if filepath.Clean(w.Path) == want {
			return w.Branch, nil
		}
	}
	return "", nil
}

func (s real) Doc(machine, path, rel string) (Doc, error) {
	r := s.repo(machine, path)
	if err := checkTop(r); err != nil {
		return Doc{}, err
	}
	if _, err := r.R.Git(r.Dir, "ls-files", "--error-unmatch", "--", rel); err != nil {
		return Doc{}, fmt.Errorf("%s is not tracked", rel)
	}
	text, err := r.ReadWorking(rel)
	if err != nil {
		return Doc{}, err
	}
	d := Doc{Text: string(text), Hunks: []gitrepo.Hunk{}}
	if base, err := r.Show("HEAD", rel); err == nil { // new file: no base
		d.Base = string(base)
	}
	if hs, err := r.Hunks(rel); err == nil && hs != nil {
		d.Hunks = hs
	}
	return d, nil
}

// Parts reads the OpenSpec store of the default branch (capabilities, and
// whether openspec/ exists), the context file and docs/REGISTER.md from
// the working tree of the repo's main checkout.
func (s real) Parts(machine, path string) (overview.Parts, error) {
	r := s.repo(machine, path)
	if err := checkTop(r); err != nil {
		return overview.Parts{}, err
	}
	var p overview.Parts
	if st, has, err := openspec.Read(r, r.DefaultBranch()); err != nil {
		msg, _, _ := strings.Cut(err.Error(), "\n")
		p.Warnings = append(p.Warnings, "openspec unreadable: "+msg)
	} else {
		p.Store, p.Has = st, has
	}
	// Context and REGISTER belong to the repo: read the main checkout's
	// working tree, also from a worktree space.
	m := s.repo(machine, mainPath(r))
	p.Context = overview.FindContext(m.ReadWorking)
	// A missing REGISTER is normal; one that exists but cannot be read is
	// named, so its open items are not lost in silence.
	if b, err := m.ReadWorking(overview.RegisterFile); err == nil {
		p.Register = b
	} else if !missing(err) {
		msg, _, _ := strings.Cut(err.Error(), "\n")
		p.Warnings = append(p.Warnings, "REGISTER UNREADABLE: "+msg)
	}
	return p, nil
}

// missing reports whether a read error means the file does not exist: locally
// fs.ErrNotExist, through `hc file read` only the error text says so.
func missing(err error) bool {
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	s := strings.ToLower(err.Error())
	// hc file read: {"kind":"NOT_FOUND","message":"no file …"}
	for _, w := range []string{"not_found", "no file", "no such file", "not found", "does not exist"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func (s real) Live(machine, path string) status.Repo {
	if s.live == nil {
		return status.Repo{Coordinators: []status.Coord{}}
	}
	return s.live(machine, mainPath(s.repo(machine, path)))
}

type entry struct {
	at   time.Time
	body []byte
}

type partsEntry struct {
	at time.Time
	p  overview.Parts
}

type liveEntry struct {
	at time.Time
	r  status.Repo
}

type handler struct {
	src Source
	now func() time.Time

	mu    sync.Mutex
	repos map[string]entry
	space map[string]entry
	parts map[string]partsEntry
	live  map[string]liveEntry

	refreshing map[string]bool // background refreshes running, by kind and key
	bg         func(func())    // runs a background refresh (go f(); tests run it in place)
}

// Handler serves GET /repo, /space, /doc and /overview. Source errors
// answer 200 with {"error": first line}; a missing parameter answers 400.
func Handler(src Source) http.Handler { return newHandler(src, time.Now) }

func newHandler(src Source, now func() time.Time) http.Handler {
	return newHandlerBG(src, now, func(f func()) { go f() })
}

// newHandlerBG is newHandler with its own runner for background refreshes (tests run them in place or later).
func newHandlerBG(src Source, now func() time.Time, bg func(func())) http.Handler {
	h := &handler{src: src, now: now, repos: map[string]entry{}, space: map[string]entry{},
		parts: map[string]partsEntry{}, live: map[string]liveEntry{}, refreshing: map[string]bool{}, bg: bg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repo", h.serveRepo)
	mux.HandleFunc("GET /space", h.serveSpace)
	mux.HandleFunc("GET /doc", h.serveDoc)
	mux.HandleFunc("GET /overview", h.serveOverview)
	return mux
}

func write(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func writeErr(w http.ResponseWriter, err error) {
	msg, _, _ := strings.Cut(err.Error(), "\n")
	b, _ := json.Marshal(map[string]string{"error": msg})
	write(w, b)
}

// params returns machine and path; path is required.
func params(w http.ResponseWriter, r *http.Request) (machine, path string, ok bool) {
	q := r.URL.Query()
	machine, path = q.Get("machine"), q.Get("path")
	if path == "" {
		http.Error(w, "missing parameter: path", http.StatusBadRequest)
		return "", "", false
	}
	return machine, path, true
}

func key(machine, path string) string { return machine + "\x00" + path }

// cached returns the entry stored under k in m if younger than ttl.
func (h *handler) cached(m map[string]entry, k string, ttl time.Duration) ([]byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	e, ok := m[k]
	if !ok || h.now().Sub(e.at) >= ttl {
		return nil, false
	}
	return e.body, true
}

func (h *handler) store(m map[string]entry, k string, b []byte) {
	h.mu.Lock()
	m[k] = entry{h.now(), b}
	h.mu.Unlock()
}

type repoBody struct {
	View  repoview.View `json:"view"`
	Graph graph.Graph   `json:"graph"`
}

// claim takes the background refresh of what/k, once at a time; release gives it back.
func (h *handler) claim(what, k string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.refreshing[what+k] {
		return false
	}
	h.refreshing[what+k] = true
	return true
}

func (h *handler) release(what, k string) {
	h.mu.Lock()
	delete(h.refreshing, what+k)
	h.mu.Unlock()
}

// repoBytes answers from the cache at once, also when it is older than repoTTL: then the view is rebuilt in the
// background (a repo view costs seconds of git calls, and the Bridge asks about every repoTTL, so waiting would
// make nearly every ask slow). Only a repo never read waits for its first build.
func (h *handler) repoBytes(machine, path string) ([]byte, repoview.View, error) {
	k := key(machine, path)
	h.mu.Lock()
	e, ok := h.repos[k]
	h.mu.Unlock()
	if ok {
		var rb repoBody
		if err := json.Unmarshal(e.body, &rb); err == nil {
			if h.now().Sub(e.at) >= repoTTL && h.claim("repo", k) {
				h.bg(func() {
					defer h.release("repo", k)
					h.buildRepo(machine, path)
				})
			}
			return e.body, rb.View, nil
		}
	}
	return h.buildRepo(machine, path)
}

// buildRepo reads the repo view and caches it; an error drops the cached one, so the next ask shows the error.
func (h *handler) buildRepo(machine, path string) ([]byte, repoview.View, error) {
	k := key(machine, path)
	v, err := h.src.Repo(machine, path)
	if err != nil {
		h.mu.Lock()
		delete(h.repos, k)
		h.mu.Unlock()
		return nil, v, err
	}
	b, err := json.Marshal(repoBody{View: v, Graph: graph.Layout(v)})
	if err != nil {
		return nil, v, err
	}
	h.store(h.repos, k, b)
	return b, v, nil
}

func (h *handler) serveRepo(w http.ResponseWriter, r *http.Request) {
	machine, path, ok := params(w, r)
	if !ok {
		return
	}
	b, _, err := h.repoBytes(machine, path)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, b)
}

type spaceBody struct {
	Topic        *string            `json:"topic"`
	Branch       string             `json:"branch"`
	Plan         *repoview.PlanView `json:"plan"`
	SpecHeadings []string           `json:"spec_headings"`
	Coordinator  *string            `json:"coordinator"`
	Warnings     []string           `json:"warnings"`
}

func (h *handler) serveSpace(w http.ResponseWriter, r *http.Request) {
	machine, path, ok := params(w, r)
	if !ok {
		return
	}
	k := key(machine, path)
	if b, ok := h.cached(h.space, k, spaceTTL); ok {
		write(w, b)
		return
	}
	branch, err := h.src.Branch(machine, path)
	if err != nil {
		writeErr(w, err)
		return
	}
	// The plan comes from the repo view, itself cached for repoTTL.
	_, v, err := h.repoBytes(machine, path)
	if err != nil {
		writeErr(w, err)
		return
	}
	body := spaceBody{Branch: branch, SpecHeadings: []string{}, Warnings: append([]string{}, v.Warnings...)}
	if branch != "" {
		for i := range v.Plans {
			p := v.Plans[i]
			if p.Branch != branch {
				continue
			}
			body.Plan, body.Topic = &p, &p.Topic
			if p.Coordinator != "" {
				body.Coordinator = &p.Coordinator
			}
			if p.Spec != "" {
				if d, err := h.src.Doc(machine, path, p.Spec); err == nil {
					if hs := plans.ParseSpec(p.Spec, []byte(d.Text)).Headings; hs != nil {
						body.SpecHeadings = hs
					}
				}
			}
			break
		}
	}
	b, err := json.Marshal(body)
	if err != nil {
		writeErr(w, err)
		return
	}
	h.store(h.space, k, b)
	write(w, b)
}

// buildParts reads the repo parts and caches them; an error drops the cached ones.
func (h *handler) buildParts(machine, path string) (partsEntry, error) {
	k := key(machine, path)
	p, err := h.src.Parts(machine, path)
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		delete(h.parts, k)
		return partsEntry{}, err
	}
	pe := partsEntry{h.now(), p}
	h.parts[k] = pe
	return pe, nil
}

// serveOverview merges the repo view (shared with /repo, repoTTL), the repo
// parts (repoTTL) and the lead/coordinator state (spaceTTL) per response.
// The view comes first: its read applies the worktree-top guard.
func (h *handler) serveOverview(w http.ResponseWriter, r *http.Request) {
	machine, path, ok := params(w, r)
	if !ok {
		return
	}
	k := key(machine, path)
	_, v, err := h.repoBytes(machine, path)
	if err != nil {
		writeErr(w, err)
		return
	}
	h.mu.Lock()
	pe, okP := h.parts[k]
	le, okL := h.live[k]
	h.mu.Unlock()
	// like the view: a stale entry answers at once and is rebuilt in the background
	if !okP {
		p, err := h.buildParts(machine, path)
		if err != nil {
			writeErr(w, err)
			return
		}
		pe = p
	} else if h.now().Sub(pe.at) >= repoTTL && h.claim("parts", k) {
		h.bg(func() {
			defer h.release("parts", k)
			h.buildParts(machine, path)
		})
	}
	if !okL || h.now().Sub(le.at) >= spaceTTL {
		le = liveEntry{h.now(), h.src.Live(machine, path)}
		h.mu.Lock()
		h.live[k] = le
		h.mu.Unlock()
	}
	b, err := json.Marshal(overview.Build(v, pe.p, le.r))
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, b)
}

func (h *handler) serveDoc(w http.ResponseWriter, r *http.Request) {
	machine, path, ok := params(w, r)
	if !ok {
		return
	}
	file := filepath.ToSlash(r.URL.Query().Get("file"))
	if !strings.HasSuffix(file, ".md") || strings.HasPrefix(file, "/") || filepath.IsAbs(file) ||
		strings.Contains(file, `\`) || (len(file) >= 2 && file[1] == ':') || file == ".." ||
		strings.HasPrefix(file, "../") || strings.Contains(file, "/../") || strings.HasSuffix(file, "/..") {
		http.Error(w, "missing or invalid parameter: file", http.StatusBadRequest)
		return
	}
	d, err := h.src.Doc(machine, path, file)
	if err != nil {
		writeErr(w, err)
		return
	}
	if d.Hunks == nil {
		d.Hunks = []gitrepo.Hunk{}
	}
	b, err := json.Marshal(d)
	if err != nil {
		writeErr(w, err)
		return
	}
	write(w, b)
}
