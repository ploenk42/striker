// Package serve exposes the status report (and further reports, such as the crew) over HTTP from cached copies.
package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/ploenk42/striker/striker/internal/status"
)

// report is one cached JSON answer, built at first request and then every interval.
type report struct {
	path  string
	build func() any
	first sync.Once
	mu    sync.RWMutex
	body  []byte
}

func (r *report) refresh() {
	b, err := json.Marshal(r.build())
	if err != nil {
		return
	}
	r.mu.Lock()
	r.body = b
	r.mu.Unlock()
}

// ensure runs the first build exactly once; concurrent callers block on it.
func (r *report) ensure() { r.first.Do(r.refresh) }

// Server caches its reports and serves them as JSON.
type Server struct {
	every   time.Duration
	reports []*report
}

// New serves GET /status from build.
func New(build func() status.Report, every time.Duration) *Server {
	s := &Server{every: every}
	s.Add("/status", func() any { return build() })
	return s
}

// Add serves GET path from build, cached and refreshed like /status. Call before Handler and Run.
func (s *Server) Add(path string, build func() any) {
	s.reports = append(s.reports, &report{path: path, build: build})
}

// Handler serves the reports; every other path is 404.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, r := range s.reports {
		r := r
		mux.HandleFunc("GET "+r.path, func(w http.ResponseWriter, _ *http.Request) {
			r.ensure()
			r.mu.RLock()
			b := r.body
			r.mu.RUnlock()
			if b == nil {
				http.Error(w, r.path[1:]+" unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(b)
		})
	}
	return mux
}

// Run refreshes every report every interval until ctx is done.
func (s *Server) Run(ctx context.Context) {
	for _, r := range s.reports {
		r.ensure()
	}
	t := time.NewTicker(s.every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, r := range s.reports {
				r.refresh()
			}
		}
	}
}
