package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is what Striker did for one request.
type Entry struct {
	At time.Time `json:"at"`
	// Outcome: "started <address>", "sent" or "error <text>".
	Outcome string `json:"outcome"`
	// Address of the agent started or sent to, if any.
	Address string `json:"address"`
	// Name is the terminal name of that agent (<role>-<task>), if Striker
	// started or sent to a task agent.
	Name string `json:"name,omitempty"`
	// Watch: Striker watches the agent and tells the requester its result.
	Watch bool `json:"watch"`
	// Delivered: the watched agent's result reached the requester.
	Delivered bool `json:"delivered"`
	// Turn is the agent's turn (hc wait) whose result was delivered, so a
	// later watch of the same agent does not deliver it again.
	Turn int `json:"turn,omitempty"`
}

// Ledger is Striker's record of carried-out requests (~/.striker/relay.json),
// keyed <machine>|<status file>|<id> (File.Key). Striker never writes into the
// agents' status files; this is the only place that says what was done.
// Safe for use by several goroutines.
type Ledger struct {
	Path    string
	Entries map[string]Entry

	mu sync.Mutex
}

// LoadLedger reads the ledger at path; a missing file is an empty ledger,
// an invalid one an error (so nothing is carried out twice).
func LoadLedger(path string) (*Ledger, error) {
	l := &Ledger{Path: path, Entries: map[string]Entry{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	b = bytes.TrimPrefix(b, []byte("\xEF\xBB\xBF"))
	if err := json.Unmarshal(b, &l.Entries); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if l.Entries == nil {
		l.Entries = map[string]Entry{}
	}
	return l, nil
}

func (l *Ledger) Has(key string) bool {
	_, ok := l.Get(key)
	return ok
}

func (l *Ledger) Get(key string) (Entry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.Entries[key]
	return e, ok
}

func (l *Ledger) Put(key string, e Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.Entries == nil {
		l.Entries = map[string]Entry{}
	}
	l.Entries[key] = e
}

// Snapshot is a copy of all entries.
func (l *Ledger) Snapshot() map[string]Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	m := make(map[string]Entry, len(l.Entries))
	for k, e := range l.Entries {
		m[k] = e
	}
	return m
}

// Save writes the ledger to a temporary file beside Path and renames it
// over Path, so a crash never leaves half a ledger.
func (l *Ledger) Save() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, err := json.MarshalIndent(l.Entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o700); err != nil {
		return err
	}
	tmp := l.Path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, l.Path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
