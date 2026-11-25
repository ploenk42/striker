package relay

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLedger(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".striker", "relay.json")

	l, err := LoadLedger(p)
	if err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if len(l.Entries) != 0 {
		t.Fatalf("entries %v", l.Entries)
	}

	f := File{Machine: "remote1", Path: "/var/lib/striker/status/alpha--rb.json", Repo: "alpha", Change: "rb"}
	k := f.Key("2.3-impl-1")
	if k != "remote1|alpha--rb.json|2.3-impl-1" {
		t.Fatalf("key %q", k)
	}
	at := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	e := Entry{At: at, Outcome: "started remote1/t2a", Address: "remote1/t2a", Watch: true}
	l.Put(k, e)
	if !l.Has(k) || l.Has("remote1|alpha--rb.json|other") {
		t.Fatal("Has")
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file left: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{`"remote1|alpha--rb.json|2.3-impl-1"`, `"at"`, `"outcome"`, `"address"`, `"watch"`, `"delivered"`} {
		if !strings.Contains(string(b), s) {
			t.Errorf("file lacks %s: %s", s, b)
		}
	}

	l2, err := LoadLedger(p)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := l2.Get(k); !ok || !reflect.DeepEqual(got, e) {
		t.Fatalf("round trip %+v %v", got, ok)
	}

	// Saving again replaces the file.
	e.Delivered = true
	l2.Put(k, e)
	if err := l2.Save(); err != nil {
		t.Fatal(err)
	}
	l3, _ := LoadLedger(p)
	if got, _ := l3.Get(k); !got.Delivered {
		t.Fatalf("second save %+v", got)
	}
}

func TestLedgerBad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "relay.json")
	if err := os.WriteFile(p, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLedger(p); err == nil {
		t.Fatal("want error for invalid ledger")
	}
}
