package relay

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
)

func TestRelayTick(t *testing.T) {
	l := newLedger(t)
	l.Put("remote1|alpha--rb.json|2.3-impl-1", Entry{Outcome: "started remote1/t2c", Address: "remote1/t2c", Name: "implementer-2.3", Watch: true, Delivered: true})
	first := "send remote1/t2c [Striker] erst"
	refused := "send remote1/t2b [Striker] Anfrage bad abgelehnt: unbekannte Anfrage \"nope\"."
	second := "send remote1/t2c [Striker] dann"
	f := &fakeHC{out: map[string]any{
		findRemote1: "alpha--rb.json\n",
		"file read --on remote1 /var/lib/striker/status/alpha--rb.json": `{"requests":[
			{"id":"fix-1","do":"send","to":"implementer-2.3","text":"erst"},
			{"id":"bad","do":"nope"},
			{"id":"fix-2","do":"send","to":"implementer-2.3","text":"dann"}]}`,
		first:   "",
		second:  "",
		refused: "",
		"send remote1/t2b [Striker] Anfrage fix-1: an implementer-2.3 (remote1/t2c) geschickt.": "",
		"send remote1/t2b [Striker] Anfrage fix-2: an implementer-2.3 (remote1/t2c) geschickt.": "",
		"ls --label striker.coordinator=rb --json":                                              coordLs,
		"get remote1/t2b --json": `{"address":"remote1/t2b","labels":{"striker.coordinator":"rb","striker.repo":"alpha"}}`,
	}}
	r := &Relay{
		C:      hc.Client{R: f},
		Herdr:  (&fakeHerdr{}).run,
		Fleet:  func() fleet.Fleet { return testFleet },
		Briefs: t.TempDir(),
		L:      l,
	}
	if err := r.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		first,
		"send remote1/t2b [Striker] Anfrage fix-1: an implementer-2.3 (remote1/t2c) geschickt.",
		refused,
		second,
		"send remote1/t2b [Striker] Anfrage fix-2: an implementer-2.3 (remote1/t2c) geschickt.",
	}
	if !reflect.DeepEqual(f.sends(), want) {
		t.Fatalf("sends %q", f.sends())
	}
	for _, id := range []string{"fix-1", "bad", "fix-2"} {
		saved(t, l, "remote1|alpha--rb.json|"+id)
	}
	if e := saved(t, l, "remote1|alpha--rb.json|bad"); e.Outcome != `error unbekannte Anfrage "nope"` {
		t.Fatalf("bad %+v", e)
	}

	// The same files again: nothing new to do.
	n := len(f.sends())
	if err := r.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.sends()) != n {
		t.Fatalf("second tick sent %q", f.sends()[n:])
	}
}

func TestRelayRunTicksAtOnce(t *testing.T) {
	f := &fakeHC{out: map[string]any{findRemote1: ""}}
	r := &Relay{C: hc.Client{R: f}, Fleet: func() fleet.Fleet { return testFleet }, L: newLedger(t)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx, time.Hour); close(done) }()
	for len(f.callsNow()) == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if got := f.callsNow(); got[0] != findRemote1 {
		t.Fatalf("calls %q", got)
	}
}
