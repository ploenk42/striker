package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ploenk42/striker/striker/internal/status"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func TestStatusWaitsForFirstBuild(t *testing.T) {
	s := New(func() status.Report {
		time.Sleep(50 * time.Millisecond)
		return status.Report{Repos: []status.Repo{{Name: "a"}}}
	}, time.Hour)
	rec := get(t, s.Handler(), "/status")
	if rec.Code != 200 {
		t.Fatalf("code %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type %q", ct)
	}
	var r status.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Repos) != 1 {
		t.Fatalf("repos %d", len(r.Repos))
	}
}

func TestServesCachedReport(t *testing.T) {
	var n int32
	s := New(func() status.Report {
		atomic.AddInt32(&n, 1)
		return status.Report{}
	}, time.Hour)
	get(t, s.Handler(), "/status")
	get(t, s.Handler(), "/status")
	if got := atomic.LoadInt32(&n); got != 1 {
		t.Fatalf("builds %d", got)
	}
}

func TestAddedReport(t *testing.T) {
	s := New(func() status.Report { return status.Report{} }, time.Hour)
	var n int32
	s.Add("/crew", func() any {
		atomic.AddInt32(&n, 1)
		return map[string]any{"agents": []string{"PATA"}}
	})
	h := s.Handler()
	get(t, h, "/crew")
	rec := get(t, h, "/crew")
	if rec.Code != 200 || rec.Body.String() != `{"agents":["PATA"]}` || atomic.LoadInt32(&n) != 1 {
		t.Fatalf("code %d body %q builds %d", rec.Code, rec.Body.String(), n)
	}
	if rec := get(t, h, "/status"); rec.Code != 200 {
		t.Fatalf("status code %d", rec.Code)
	}
}

func TestUnknownPath404(t *testing.T) {
	s := New(func() status.Report { return status.Report{} }, time.Hour)
	if rec := get(t, s.Handler(), "/nope"); rec.Code != 404 {
		t.Fatalf("code %d", rec.Code)
	}
}
