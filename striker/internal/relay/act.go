package relay

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ploenk42/striker/striker/internal/coord"
	"github.com/ploenk42/striker/striker/internal/dispatch"
	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/gitrepo"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/lead"
	"github.com/ploenk42/striker/striker/internal/openspec"
)

// refuse is an error whose text is the reason given to the agent.
func refuse(format string, a ...any) error { return fmt.Errorf(format, a...) }

// Act carries out request r of file (one Filter kept): start-coordinator,
// dispatch or send. It records the outcome in the ledger and saves it, then
// answers the requester with a [Striker] message (none if the requester is
// gone). A request already in the ledger is skipped. run executes herdr-app
// commands, briefs is the folder of the brief templates. The returned error is
// only a failed ledger save: a failed request is an "error" outcome and answer.
func Act(c hc.Client, run func(...string) ([]byte, error), f fleet.Fleet, briefs string, l *Ledger, file File, r Request) error {
	key := file.Key(r.ID)
	if l.Has(key) {
		return nil
	}
	var (
		e      Entry
		answer string
		err    error
	)
	switch r.Do {
	case "start-coordinator":
		var addr string
		addr, err = coord.Start(c, run, f, file.Repo, r.Change, filepath.Join(briefs, "coordinator.md"))
		e.Address = addr
		if err == nil {
			answer = fmt.Sprintf("Anfrage %s: Koordinator für %s läuft als %s.", r.ID, r.Change, addr)
		}
	case "dispatch":
		e, err = startAgent(c, f, file, r)
		if err == nil {
			answer = fmt.Sprintf("Anfrage %s: %s läuft als %s.", r.ID, e.Name, e.Address)
		}
	case "send":
		e, err = send(c, l, file, r)
		if err == nil {
			answer = fmt.Sprintf("Anfrage %s: an %s (%s) geschickt.", r.ID, e.Name, e.Address)
		}
	default:
		err = refuse("unbekannte Anfrage %q", r.Do)
	}
	e.At = time.Now()
	if err != nil {
		reason := strings.TrimSpace(err.Error())
		e.Outcome, e.Watch = "error "+reason, false
		answer = refusedText(r.ID, reason)
	} else if e.Outcome == "" {
		e.Outcome = "started " + e.Address
	}
	l.Put(key, e)
	if err := l.Save(); err != nil {
		return err
	}
	tell(c, file, answer)
	return nil
}

// Reject records a request Filter refused and answers its requester once
// (later ticks skip it by its key). A file of no relayed repo gets the entry
// but no answer: whoever wrote it is no agent Striker relays for.
func Reject(c hc.Client, l *Ledger, f fleet.Fleet, d Dropped) error {
	if d.Request.ID == "" {
		return nil // Filter skips these; nobody to answer, no key
	}
	key := d.File.Key(d.Request.ID)
	if l.Has(key) {
		return nil
	}
	l.Put(key, Entry{At: time.Now(), Outcome: "error " + d.Reason})
	if err := l.Save(); err != nil {
		return err
	}
	if checkFile(f, d.File) == "" {
		tell(c, d.File, refusedText(d.Request.ID, d.Reason))
	}
	return nil
}

func refusedText(id, reason string) string {
	reason = strings.TrimSpace(reason)
	if !strings.HasSuffix(reason, ".") {
		reason += "."
	}
	return fmt.Sprintf("Anfrage %s abgelehnt: %s", id, reason)
}

// tell sends text to the requester of file with striker send semantics; a
// requester that is gone (or cannot be found) gets nothing.
func tell(c hc.Client, file File, text string) {
	addr, err := requester(c, file)
	if err != nil || addr == "" {
		return
	}
	_ = c.Send(addr, lead.Message(text))
}

// requester finds the agent that wrote file on its machine: the lead
// (striker.lead=<repo>) or the coordinator (striker.coordinator=<change> with
// striker.repo=<repo>). "" if none runs.
func requester(c hc.Client, file File) (string, error) {
	if file.Change != "" {
		return coordinator(c, file)
	}
	ts, err := c.ListLabel("striker.lead", file.Repo)
	if err != nil {
		return "", err
	}
	for _, t := range ts {
		if onMachine(t.Address, file.Machine) {
			return t.Address, nil
		}
	}
	return "", nil
}

func coordinator(c hc.Client, file File) (string, error) {
	ts, err := c.ListLabel("striker.coordinator", file.Change)
	if err != nil {
		return "", err
	}
	for _, t := range ts {
		if !onMachine(t.Address, file.Machine) {
			continue
		}
		info, err := c.Get(t.Address)
		if err != nil {
			continue
		}
		if info.Labels["striker.repo"] == file.Repo {
			return t.Address, nil
		}
	}
	return "", nil
}

func onMachine(addr, machine string) bool {
	m, _, remote := strings.Cut(addr, "/")
	return remote && m == machine
}

// startAgent starts task agent r.Role for r.Task of the coordinator's change
// in its worktree, as a sub-agent of the coordinator.
func startAgent(c hc.Client, f fleet.Fleet, file File, r Request) (Entry, error) {
	parent, err := coordinator(c, file)
	if err != nil {
		return Entry{}, refuse("Koordinator von %s nicht lesbar: %v", file.Change, err)
	}
	if parent == "" {
		return Entry{}, refuse("kein laufender Koordinator für %s von %s auf %s", file.Change, file.Repo, file.Machine)
	}
	p, err := coord.FindWorktree(c, f, file.Repo, file.Change)
	if err != nil {
		return Entry{}, err
	}
	st, _, err := openspec.Read(gitrepo.Repo{R: p.Repo.R, Dir: p.Worktree}, "")
	if err != nil {
		return Entry{}, refuse("openspec im Worktree %s nicht lesbar: %v", p.Worktree, err)
	}
	var ch openspec.Change
	found := false
	for _, x := range st.Changes {
		if x.Name == file.Change && x.Archived == "" {
			ch, found = x, true
			break
		}
	}
	if !found {
		return Entry{}, refuse("kein aktiver Change %s im Worktree %s", file.Change, p.Worktree)
	}
	t, err := dispatch.ResolveChange(ch, r.Task)
	if err != nil {
		return Entry{}, refuse("kein Task %s im Change %s", strings.TrimSpace(r.Task), ch.Name)
	}
	argv := dispatch.RemoteCommandChange(r.Role, file.Repo, ch, t, p.Machine, p.User, p.Worktree, parent)
	out, err := c.R.Run(argv...)
	if err != nil {
		return Entry{}, fmt.Errorf("hc run scheiterte: %v", err)
	}
	// hc run --bg prints the new terminal as one JSON object; with --on its
	// address carries the machine already, a bare one gets it prepended so
	// ledger, answer and watch have the full address.
	var made hc.Terminal
	if err := json.Unmarshal(out, &made); err != nil || made.Address == "" {
		return Entry{}, fmt.Errorf("hc run gab keine Adresse zurück: %s", strings.TrimSpace(string(out)))
	}
	addr := made.Address
	if hc.IsLocal(addr) {
		addr = p.Machine + "/" + addr
	}
	return Entry{Outcome: "started " + addr, Address: addr, Name: r.Role + "-" + t.ID, Watch: true}, nil
}

// send passes r.Text to the task agent r.To, which Striker must have started
// for this coordinator (a dispatch entry of the same status file).
func send(c hc.Client, l *Ledger, file File, r Request) (Entry, error) {
	prefix := file.Key("")
	var addr string
	var at time.Time
	for k, e := range l.Snapshot() {
		if strings.HasPrefix(k, prefix) && e.Name == r.To && strings.HasPrefix(e.Outcome, "started ") && e.Address != "" && !e.At.Before(at) {
			addr, at = e.Address, e.At
		}
	}
	if addr == "" {
		return Entry{}, refuse("%s hat Striker nicht für diesen Koordinator gestartet", r.To)
	}
	if err := c.Send(addr, lead.Message(r.Text)); err != nil {
		return Entry{Address: addr, Name: r.To}, fmt.Errorf("an %s (%s) nicht zugestellt: %v", r.To, addr, err)
	}
	return Entry{Outcome: "sent", Address: addr, Name: r.To, Watch: true}, nil
}
