package relay

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ploenk42/striker/striker/internal/hc"
)

// doneScreen is `hc read --lines 60` of a done agent (task 1, Claude Code
// 2.1.331 as awayteam on remote1), with the blank padding.
const doneScreen = "❯ Rechne 2+3 und sag fertig $x\n" +
	"\n" +
	"● 2+3 ergibt 5, fertig $x\n" +
	"\n" +
	"✻ Cogitated for 2s · done 11:02 AM\n" +
	"\n\n\n\n\n\n\n\n\n\n" +
	"────────────────────────────────────────────────────────────\n" +
	"❯ \n" +
	"────────────────────────────────────────────────────────────\n" +
	"  Opus 5.5 · 43K\n" +
	"\n\n"

// workScreen: tool calls, then the final answer over several lines.
const workScreen = "❯ Implementiere Task 2.3\n" +
	"\n" +
	"● Read(openspec/changes/rb/plan.md)\n" +
	"  ⎿  Read 40 lines\n" +
	"\n" +
	"● Bash(go test ./...)\n" +
	"  ⎿  ok\n" +
	"\n" +
	"● Task 2.3 ist umgesetzt: watch.go geschrieben,\n" +
	"  Tests grün, Commit abc123.\n" +
	"\n" +
	"  Offen: nichts.\n" +
	"\n" +
	"✻ Worked for 3m · done 11:20 AM\n" +
	"\n" +
	"────────────────────────────────────────────────────────────\n" +
	"❯ \n" +
	"────────────────────────────────────────────────────────────\n" +
	"  Opus 5.5 · 80K\n"

// trustScreen: a blocked agent on the trust dialog, no answer marker.
const trustScreen = "────────────────────────────────────────────────────────────\n" +
	" Accessing workspace:\n" +
	"\n" +
	" /srv/alpha-rb\n" +
	"\n" +
	" Quick safety check: Is this a project you created or one you trust?\n" +
	"\n" +
	" ❯ 1. Yes, I trust this folder\n" +
	"   2. No, exit\n" +
	"\n" +
	" Enter to confirm · Esc to cancel\n" +
	"\n\n\n"

func TestLastAnswer(t *testing.T) {
	long := "● Anfang\n"
	for i := 1; i <= 50; i++ {
		long += "  Zeile " + string(rune('A'+i%26)) + "\n"
	}
	long += "  Schluss\n\n❯ \n"
	for _, tc := range []struct {
		name, screen, want string
	}{
		{"done", doneScreen, "2+3 ergibt 5, fertig $x"},
		{"last marker, several lines", workScreen, "Task 2.3 ist umgesetzt: watch.go geschrieben,\nTests grün, Commit abc123.\n\nOffen: nichts."},
		{"blocked: tail", trustScreen, strings.Join([]string{
			"────────────────────────────────────────────────────────────",
			" Accessing workspace:",
			" /srv/alpha-rb",
			" Quick safety check: Is this a project you created or one you trust?",
			" ❯ 1. Yes, I trust this folder",
			"   2. No, exit",
			" Enter to confirm · Esc to cancel",
		}, "\n")},
		{"no marker: tail", "\n\nnur Text\r\n\n  zweite Zeile  \n\n", "nur Text\n  zweite Zeile"},
		{"empty", "\n \n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := LastAnswer(tc.screen); got != tc.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
	t.Run("at most 40 lines, the last ones", func(t *testing.T) {
		got := strings.Split(LastAnswer(long), "\n")
		if len(got) != answerLines || got[len(got)-1] != "Schluss" {
			t.Fatalf("%d lines, last %q", len(got), got[len(got)-1])
		}
	})
	t.Run("tail at most 20 lines", func(t *testing.T) {
		s := ""
		for i := 0; i < 30; i++ {
			s += "x\n"
		}
		if got := strings.Split(LastAnswer(s), "\n"); len(got) != tailLines {
			t.Fatalf("%d lines", len(got))
		}
	})
}

// realScreen reads a real `hc read --lines 60` capture from testdata:
// screen-done.txt is a done Claude (t1z, spec reviewer of task 7, no right
// column), screen-panel.txt a working one (t1x, 200 columns) with Claude's
// diff panel as a right column behind a │ at column 110.
func realScreen(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(b), "\n")
}

// index is the first line of ls starting with prefix.
func index(t *testing.T, ls []string, prefix string) int {
	t.Helper()
	for i, l := range ls {
		if strings.HasPrefix(l, prefix) {
			return i
		}
	}
	t.Fatalf("no line %q", prefix)
	return -1
}

// noChrome: no status line, rule, prompt or footer of the screen in an
// answer (the answer text itself may name ✻ and ────).
func noChrome(t *testing.T, answer string) {
	t.Helper()
	for _, l := range strings.Split(answer, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "✻") || strings.HasPrefix(l, "────") || strings.HasPrefix(l, "❯") || strings.HasPrefix(l, "Opus 5.5 ·") || strings.HasPrefix(l, "⏵⏵") {
			t.Fatalf("chrome %q in answer:\n%s", l, answer)
		}
	}
}

func TestLastAnswerRealScreens(t *testing.T) {
	done := realScreen(t, "screen-done.txt")
	wantDone := strings.Join([]string{
		"Ergebnis: Task 7 erfüllt den Plan bis auf eine Abweichung. LastAnswer passt nicht zu einem echten Bildschirm, und die Bildschirm-Beispiele in den Tests sind nachgebaut, nicht wie verlangt aus Task 1",
		"übernommen. go test ./... läuft grün, auch relay. Ich habe nichts geändert und nichts committet. Die Haken in tasks.md und plan.md stehen im Task-Commit d2d7e9d.",
	}, "\n")
	t.Run("done", func(t *testing.T) {
		got := LastAnswer(strings.Join(done, "\n"))
		if !strings.HasPrefix(got, wantDone+"\n\nAbgleich mit dem Plan:") || !strings.HasSuffix(got, "\n\nWie sich ein frisch gestarteter Agent in Turn 0 bei hc wait verhält, konnte ich hier nicht prüfen; das zeigt erst Task 10.") {
			t.Fatalf("got\n%s", got)
		}
		noChrome(t, got)
	})

	// The answer's ● scrolled out of the window: the text before the ✻
	// line, without the status, rules and footer.
	t.Run("marker out of window", func(t *testing.T) {
		cut := done[index(t, done, "  Abgleich mit dem Plan:"):]
		got := LastAnswer(strings.Join(cut, "\n"))
		if !strings.HasPrefix(got, "Abgleich mit dem Plan:") || !strings.HasSuffix(got, "konnte ich hier nicht prüfen; das zeigt erst Task 10.") {
			t.Fatalf("got\n%s", got)
		}
		noChrome(t, got)
	})

	// t1x as it looked when done: the screen up to its ✻ line, then its
	// rules, prompt and footer.
	panel := realScreen(t, "screen-panel.txt")
	brewed := index(t, panel, "✻ Brewed for 4m 57s")
	rule := index(t, panel, "────")
	doneP := append(append([]string{}, panel[:brewed+2]...), panel[rule:rule+5]...)
	t.Run("right column", func(t *testing.T) {
		got := LastAnswer(strings.Join(doneP, "\n"))
		want := strings.Join([]string{
			"Task 7 ist fertig und als d2d7e9d striker: relay watches task agents committet; er ist in tasks.md und in",
			"plan.md abgehakt. Neu ist striker/internal/relay/watch.go mit Watcher und LastAnswer. Pro Adresse läuft",
		}, "\n")
		if !strings.HasPrefix(got, want+"\n") || !strings.HasSuffix(got, "\nFall ab, das zeigt sich erst in Task 10.") {
			t.Fatalf("got\n%s", got)
		}
		if strings.Contains(got, "│") || strings.Contains(got, "No changes this session") {
			t.Fatalf("right column in answer:\n%s", got)
		}
		// a line that fills the left column up to the │
		if !strings.Contains(got, "\nzusätzlich turn, damit eine erneute Beobachtung nach einem send nicht die alte Antwort noch einmal zustellt.\n") {
			t.Fatalf("full line lost:\n%s", got)
		}
	})
	t.Run("right column, tail", func(t *testing.T) {
		got := LastAnswer(strings.Join(panel[:index(t, panel, "● Green.")], "\n"))
		if got != "" {
			t.Fatalf("got %q", got)
		}
		got = LastAnswer(strings.Join(panel[index(t, panel, "  Read 1 file, ran 4"):index(t, panel, "● Now tick")], "\n"))
		if got != "  Read 1 file, ran 4 shell commands · 1 failed" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("│ in the answer stays", func(t *testing.T) {
		s := "● Tabelle:\n  │ a │ b │\n  │ 1 │ 2 │\n\n✻ Worked for 1s · done 12:00\n"
		if got := LastAnswer(s); got != "Tabelle:\n│ a │ b │\n│ 1 │ 2 │" {
			t.Fatalf("got %q", got)
		}
	})
}

// seq answers each call with the next output; after the last, the last again.
func seq(outs ...string) func() (string, error) {
	var mu sync.Mutex
	i := 0
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		s := outs[i]
		if i < len(outs)-1 {
			i++
		}
		return s, nil
	}
}

const (
	waitT2a    = "wait remote1/t2a --timeout 600 --json"
	readT2a    = "read remote1/t2a --lines 200"
	waitTO     = `{"reason":"timeout","status":null,"ended":false,"exit_code":null,"turn":0}`
	waitDone1  = `{"reason":"status","status":"done","ended":true,"exit_code":null,"turn":1}`
	waitDone2  = `{"reason":"status","status":"done","ended":true,"exit_code":null,"turn":2}`
	waitBlock  = `{"reason":"status","status":"blocked","ended":true,"exit_code":null,"turn":0}`
	waitExit   = `{"reason":"exit","status":null,"ended":true,"exit_code":129,"turn":1}`
	implKey    = "remote1|alpha--rb.json|2.3-impl-1"
	doneToCoor = "send remote1/t2b [Striker] implementer-2.3 (remote1/t2a) ist fertig. Seine letzte Antwort: 2+3 ergibt 5, fertig $x"
)

// watchOut: the coordinator of rb (alpha) runs as remote1/t2b.
func watchOut(extra map[string]any) map[string]any {
	out := map[string]any{
		"ls --label striker.coordinator=rb --json": coordLs,
		"get remote1/t2b --json":                   `{"address":"remote1/t2b","labels":{"striker.coordinator":"rb","striker.repo":"alpha"}}`,
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func started(l *Ledger) {
	l.Put(implKey, Entry{At: time.Now().Add(-time.Minute), Outcome: "started remote1/t2a", Address: "remote1/t2a", Name: "implementer-2.3", Watch: true})
}

func count(calls []string, c string) int {
	n := 0
	for _, x := range calls {
		if x == c {
			n++
		}
	}
	return n
}

func newWatcher(f *fakeHC, l *Ledger) *Watcher {
	return &Watcher{C: hc.Client{R: f}, L: l, Pause: time.Millisecond}
}

func TestWatchDone(t *testing.T) {
	f := &fakeHC{out: watchOut(map[string]any{
		waitT2a:    seq(waitTO, waitDone1), // a timeout just waits again
		readT2a:    doneScreen,
		doneToCoor: "",
	})}
	l := newLedger(t)
	started(l)
	w := newWatcher(f, l)
	w.Watch(context.Background(), "remote1/t2a")
	w.Wait()
	if !reflect.DeepEqual(f.sends(), []string{doneToCoor}) {
		t.Fatalf("sends %q", f.sends())
	}
	if n := count(f.calls, waitT2a); n != 2 {
		t.Fatalf("%d waits", n)
	}
	if e := saved(t, l, implKey); !e.Delivered || !e.Watch || e.Turn != 1 || e.Outcome != "started remote1/t2a" {
		t.Fatalf("entry %+v", e)
	}
}

func TestWatchBlockedThenSend(t *testing.T) {
	blockedMsg := "send remote1/t2b [Striker] implementer-2.3 (remote1/t2a) wartet auf eine Eingabe: " + LastAnswer(trustScreen)
	f := &fakeHC{out: watchOut(map[string]any{
		waitT2a: seq(waitBlock, waitDone1),
		readT2a: seq(trustScreen, doneScreen),
		// still blocked on the first look, then answered
		"get remote1/t2a --json": seq(
			`{"address":"remote1/t2a","status":{"state":"blocked"}}`,
			`{"address":"remote1/t2a","status":{"state":"working"}}`),
		blockedMsg: "",
		doneToCoor: "",
	})}
	l := newLedger(t)
	started(l)
	w := newWatcher(f, l)
	w.Watch(context.Background(), "remote1/t2a")
	w.Wait()
	if !reflect.DeepEqual(f.sends(), []string{blockedMsg, doneToCoor}) {
		t.Fatalf("sends %q", f.sends())
	}
	if e := saved(t, l, implKey); !e.Delivered || e.Turn != 1 {
		t.Fatalf("entry %+v", e)
	}

	// The coordinator sends a fix round: the agent is watched again, its old
	// answer (turn 1) is not delivered twice.
	answer2 := "send remote1/t2b [Striker] implementer-2.3 (remote1/t2a) ist fertig. Seine letzte Antwort: Task 2.3 ist umgesetzt: watch.go geschrieben,\nTests grün, Commit abc123.\n\nOffen: nichts."
	f.mu.Lock()
	f.calls = nil
	f.out[waitT2a] = seq(waitDone1, waitDone2)
	f.out[readT2a] = workScreen
	f.out[answer2] = ""
	f.mu.Unlock()
	l.Put("remote1|alpha--rb.json|fix-1", Entry{At: time.Now(), Outcome: "sent", Address: "remote1/t2a", Name: "implementer-2.3", Watch: true})
	w.Start(context.Background())
	w.Wait()
	if !reflect.DeepEqual(f.sends(), []string{answer2}) {
		t.Fatalf("sends %q", f.sends())
	}
	if e := saved(t, l, "remote1|alpha--rb.json|fix-1"); !e.Delivered || e.Turn != 2 {
		t.Fatalf("entry %+v", e)
	}
}

func TestWatchEnded(t *testing.T) {
	msg := "send remote1/t2b [Striker] implementer-2.3 (remote1/t2a) ist beendet (exit 129)."
	f := &fakeHC{out: watchOut(map[string]any{waitT2a: waitExit, msg: ""})}
	l := newLedger(t)
	started(l)
	w := newWatcher(f, l)
	w.Watch(context.Background(), "remote1/t2a")
	w.Wait()
	if !reflect.DeepEqual(f.sends(), []string{msg}) {
		t.Fatalf("sends %q", f.sends())
	}
	if e := saved(t, l, implKey); !e.Delivered {
		t.Fatalf("entry %+v", e)
	}
}

// steps answers each call with the next item (a string output or an error);
// after the last, the last again.
func steps(items ...any) func() (string, error) {
	var mu sync.Mutex
	i := 0
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		v := items[i]
		if i < len(items)-1 {
			i++
		}
		if err, ok := v.(error); ok {
			return "", err
		}
		return v.(string), nil
	}
}

// noTerminal is how hc answers for a terminal that does not exist (checked:
// `hc get remote1/t9999 --json` and `hc wait remote1/t9999`, exit 3, the JSON
// on stderr).
func noTerminal(args ...string) error {
	return &hc.ExecError{Args: args, Err: errors.New("exit status 3"),
		Stderr: `{"error":{"hint":"see ` + "`hc ls --all`" + `","kind":"not_found","message":"no terminal \"t2a\""}}` + "\n"}
}

// lost is a failed connection to the machine: no answer of hc about the
// terminal.
func lost(args ...string) error {
	return &hc.ExecError{Args: args, Err: errors.New("exit status 255"), Stderr: "ssh: connect to host remote1 port 22: Connection timed out\n"}
}

func TestWatchConnectionLost(t *testing.T) {
	f := &fakeHC{out: watchOut(map[string]any{
		waitT2a: steps(
			lost("wait", "remote1/t2a"),
			&hc.ExecError{Args: []string{"wait"}, Err: errors.New("exit status 4"), Stderr: `{"error":{"kind":"machine_unknown","message":"unknown machine \"remote1\""}}`},
			errors.New("hc wait: timed out after 610s"),
			waitDone1),
		"get remote1/t2a --json": lost("get", "remote1/t2a"),
		readT2a:                  doneScreen,
		doneToCoor:               "",
	})}
	l := newLedger(t)
	started(l)
	w := newWatcher(f, l)
	w.Watch(context.Background(), "remote1/t2a")
	w.Wait()
	if !reflect.DeepEqual(f.sends(), []string{doneToCoor}) {
		t.Fatalf("sends %q", f.sends())
	}
	if n := count(f.calls, waitT2a); n != 4 {
		t.Fatalf("%d waits", n)
	}
	if e := saved(t, l, implKey); !e.Delivered || e.Turn != 1 {
		t.Fatalf("entry %+v", e)
	}
}

func TestWatchBlockedOnce(t *testing.T) {
	blockedMsg := "send remote1/t2b [Striker] implementer-2.3 (remote1/t2a) wartet auf eine Eingabe: " + LastAnswer(trustScreen)
	f := &fakeHC{out: watchOut(map[string]any{
		waitT2a: seq(waitBlock, waitBlock, waitBlock, waitBlock, waitDone1),
		readT2a: seq(trustScreen, trustScreen, doneScreen),
		"get remote1/t2a --json": steps(
			lost("get", "remote1/t2a"),                               // same blockade, hc not reachable
			`{"address":"remote1/t2a"}`,                              // same blockade, no status
			`{"address":"remote1/t2a","status":{"state":"working"}}`, // answered
			`{"address":"remote1/t2a","status":{"state":"working"}}`),
		blockedMsg: "",
		doneToCoor: "",
	})}
	l := newLedger(t)
	started(l)
	w := newWatcher(f, l)
	w.Watch(context.Background(), "remote1/t2a")
	w.Wait()
	// The first blockade once, the second (after it was answered) once more.
	if !reflect.DeepEqual(f.sends(), []string{blockedMsg, blockedMsg, doneToCoor}) {
		t.Fatalf("sends %q", f.sends())
	}
}

func TestWatchStaleTurn(t *testing.T) {
	stale := "send remote1/t2b [Striker] implementer-2.3 (remote1/t2a) hat seit der letzten Nachricht keinen neuen Turn begonnen; vielleicht kam sie nicht an. Striker beobachtet weiter."
	done2 := "send remote1/t2b [Striker] implementer-2.3 (remote1/t2a) ist fertig. Seine letzte Antwort: 2+3 ergibt 5, fertig $x"
	f := &fakeHC{out: watchOut(map[string]any{
		waitT2a: seq(waitDone1, waitDone1, waitDone1, waitDone1, waitDone2),
		readT2a: doneScreen,
		stale:   "",
		done2:   "",
	})}
	l := newLedger(t)
	l.Put(implKey, Entry{At: time.Now().Add(-time.Hour), Outcome: "started remote1/t2a", Address: "remote1/t2a", Name: "implementer-2.3", Watch: true, Delivered: true, Turn: 1})
	l.Put("remote1|alpha--rb.json|fix-1", Entry{At: time.Now().Add(-time.Minute), Outcome: "sent", Address: "remote1/t2a", Name: "implementer-2.3", Watch: true})
	w := newWatcher(f, l)
	w.StaleAfter = time.Nanosecond
	w.Watch(context.Background(), "remote1/t2a")
	w.Wait()
	if !reflect.DeepEqual(f.sends(), []string{stale, done2}) {
		t.Fatalf("sends %q", f.sends())
	}
}

func TestWatchGone(t *testing.T) {
	msg := "send remote1/t2b [Striker] implementer-2.3 (remote1/t2a) ist beendet."
	f := &fakeHC{out: watchOut(map[string]any{
		waitT2a:                  noTerminal("wait", "remote1/t2a"),
		"get remote1/t2a --json": noTerminal("get", "remote1/t2a"),
		msg:                      "",
	})}
	l := newLedger(t)
	started(l)
	w := newWatcher(f, l)
	w.Watch(context.Background(), "remote1/t2a")
	w.Wait()
	if !reflect.DeepEqual(f.sends(), []string{msg}) {
		t.Fatalf("sends %q", f.sends())
	}
}

func TestWatchCoordinatorGone(t *testing.T) {
	f := &fakeHC{out: map[string]any{
		waitT2a: waitExit,
		"ls --label striker.coordinator=rb --json": `{"terminals":[]}`,
	}}
	l := newLedger(t)
	started(l)
	w := newWatcher(f, l)
	w.Watch(context.Background(), "remote1/t2a")
	w.Wait()
	if s := f.sends(); len(s) != 0 {
		t.Fatalf("sends %q", s)
	}
	if e := saved(t, l, implKey); e.Watch || e.Delivered {
		t.Fatalf("entry %+v", e)
	}
}

func TestWatchOnce(t *testing.T) {
	release := make(chan struct{})
	f := &fakeHC{out: watchOut(map[string]any{
		waitT2a: func() (string, error) {
			<-release
			return waitDone1, nil
		},
		readT2a:    doneScreen,
		doneToCoor: "",
	})}
	l := newLedger(t)
	started(l)
	w := newWatcher(f, l)
	w.Watch(context.Background(), "remote1/t2a")
	w.Watch(context.Background(), "remote1/t2a")
	w.Start(context.Background())
	close(release)
	w.Wait()
	if n := count(f.calls, waitT2a); n != 1 {
		t.Fatalf("%d waits: %q", n, f.calls)
	}
	if len(f.sends()) != 1 {
		t.Fatalf("sends %q", f.sends())
	}
}

func TestResume(t *testing.T) {
	f := &fakeHC{out: watchOut(map[string]any{
		waitT2a:    waitDone1,
		readT2a:    doneScreen,
		doneToCoor: "",
	})}
	l := newLedger(t)
	started(l)
	l.Put("remote1|alpha--rb.json|1-impl-1", Entry{Outcome: "started remote1/t2c", Address: "remote1/t2c", Name: "implementer-1", Watch: true, Delivered: true})
	l.Put("remote1|alpha.json|c-rb", Entry{Outcome: "started remote1/t2b", Address: "remote1/t2b"})
	l.Put("remote1|alpha--rb.json|x", Entry{Outcome: "error kein Task", Watch: false})
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	l2, err := LoadLedger(l.Path) // as after a restart of striker serve
	if err != nil {
		t.Fatal(err)
	}
	w := newWatcher(f, l2)
	w.Start(context.Background())
	w.Wait()
	for _, c := range f.calls {
		if strings.HasPrefix(c, "wait ") && c != waitT2a {
			t.Errorf("watched %q", c)
		}
	}
	if count(f.calls, waitT2a) != 1 || !reflect.DeepEqual(f.sends(), []string{doneToCoor}) {
		t.Fatalf("calls %q", f.calls)
	}
	if e := saved(t, l2, implKey); !e.Delivered {
		t.Fatalf("entry %+v", e)
	}
}
