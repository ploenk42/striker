package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/lead"
)

const (
	// answerLines: at most this many lines of an agent's last answer.
	answerLines = 40
	// tailLines: at most this many non-blank lines of a screen without an
	// answer (a blocked agent).
	tailLines = 20
	// screenLines is the history Striker reads above an agent's screen: enough
	// that a long answer's ● is usually still in it.
	screenLines = 200
)

// Watcher watches the task agents Striker started or sent to (ledger entries
// with watch and not delivered) and tells their coordinator when one is done,
// blocked or ended. One goroutine per agent address, never two for the same.
// The state lives in the ledger, so Start after a restart resumes the watches.
type Watcher struct {
	C hc.Client
	L *Ledger
	// Timeout of one hc wait in seconds; 600 when zero.
	Timeout int
	// Pause between looks at a blocked agent, and after a wait that brought
	// nothing new; 10 s when zero.
	Pause time.Duration
	// StaleAfter: if an agent Striker sent to has not begun a new turn after
	// this long, the coordinator is told once (the watch goes on); 30 min
	// when zero.
	StaleAfter time.Duration
	// Logf reports what cannot be answered to anyone (a failed ledger save);
	// may be nil.
	Logf func(format string, a ...any)

	mu      sync.Mutex
	running map[string]bool
	wg      sync.WaitGroup
}

// Start watches every agent with an open entry in the ledger that is not
// watched yet. Cheap enough to call on every relay tick.
func (w *Watcher) Start(ctx context.Context) {
	for _, addr := range openAddresses(w.L.Snapshot()) {
		w.Watch(ctx, addr)
	}
}

// Watch starts watching addr unless a watch of it runs already.
func (w *Watcher) Watch(ctx context.Context, addr string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running == nil {
		w.running = map[string]bool{}
	}
	if w.running[addr] {
		return
	}
	w.running[addr] = true
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.watch(ctx, addr)
	}()
}

// Wait blocks until all watches have ended.
func (w *Watcher) Wait() { w.wg.Wait() }

func open(e Entry) bool { return e.Watch && !e.Delivered && e.Address != "" }

func openAddresses(m map[string]Entry) []string {
	seen := map[string]bool{}
	var as []string
	for _, e := range m {
		if open(e) && !seen[e.Address] {
			seen[e.Address] = true
			as = append(as, e.Address)
		}
	}
	sort.Strings(as)
	return as
}

// waitResult is what `hc wait <addr> --json` prints (task 1): reason
// "status" with status done or blocked, "exit" with the exit code, or
// "timeout".
type waitResult struct {
	Reason   string  `json:"reason"`
	Status   *string `json:"status"`
	ExitCode *int    `json:"exit_code"`
	Turn     int     `json:"turn"`
}

func (w *Watcher) watch(ctx context.Context, addr string) {
	var (
		blockedTold bool      // this blockade was reported
		staleSince  time.Time // first wait that brought only a delivered turn
		staleTold   bool
	)
	for w.more(ctx, addr) {
		since := time.Now()
		res, err := w.wait(addr)
		if ctx.Err() != nil {
			continue // more ends it
		}
		stale := false
		switch {
		case err != nil:
			// Only hc's own word that the terminal does not exist ends the
			// watch; a lost connection to the machine is waited out.
			if notFound(err) {
				w.finish(addr, since, "ist beendet.", 0)
			} else {
				w.pause(ctx)
			}
		case res.Reason == "timeout":
			blockedTold = false // it worked all along: a blockade later is a new one
		case res.Reason == "exit":
			text := "ist beendet."
			if res.ExitCode != nil {
				text = fmt.Sprintf("ist beendet (exit %d).", *res.ExitCode)
			}
			w.finish(addr, since, text, res.Turn)
		case res.Status != nil && *res.Status == "done":
			blockedTold = false
			if last := w.lastTurn(addr); last > 0 && res.Turn <= last {
				// hc wait answers a done agent at once with its current turn:
				// the one delivered already. The agent has not begun the turn
				// for the send yet.
				stale = true
				if staleSince.IsZero() {
					staleSince = time.Now()
				} else if !staleTold && time.Since(staleSince) >= w.staleAfter() {
					w.tell(addr, "hat seit der letzten Nachricht keinen neuen Turn begonnen; vielleicht kam sie nicht an. Striker beobachtet weiter.")
					staleTold = true
				}
				w.pause(ctx)
				break
			}
			w.finish(addr, since, "ist fertig. Seine letzte Antwort: "+LastAnswer(w.screen(addr)), res.Turn)
		case res.Status != nil && *res.Status == "blocked":
			if !blockedTold {
				w.tell(addr, "wartet auf eine Eingabe: "+tail(lines(w.screen(addr))))
				blockedTold = true
			}
			if w.untilUnblocked(ctx, addr) {
				blockedTold = false
			}
		default:
			w.pause(ctx)
		}
		if !stale {
			staleSince, staleTold = time.Time{}, false
		}
	}
}

func (w *Watcher) staleAfter() time.Duration {
	if w.StaleAfter == 0 {
		return 30 * time.Minute
	}
	return w.StaleAfter
}

// notFound reports whether err is hc's answer that the terminal does not
// exist: exit 3 with {"error":{"kind":"not_found",...}} on stderr. Any other
// failure (no connection to the machine, an unknown machine, a timeout) says
// nothing about the terminal.
func notFound(err error) bool {
	var xe *hc.ExecError
	if !errors.As(err, &xe) {
		return false
	}
	var v struct {
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	return json.Unmarshal([]byte(strings.TrimSpace(xe.Stderr)), &v) == nil && v.Error.Kind == "not_found"
}

// more reports whether addr still has an open entry; if not (or ctx is
// done) the watch ends and a later Watch may start a new one.
func (w *Watcher) more(ctx context.Context, addr string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ctx.Err() == nil {
		for _, e := range w.L.Snapshot() {
			if open(e) && e.Address == addr {
				return true
			}
		}
	}
	delete(w.running, addr)
	return false
}

func (w *Watcher) wait(addr string) (waitResult, error) {
	timeout := w.Timeout
	if timeout == 0 {
		timeout = 600
	}
	var res waitResult
	out, err := w.C.R.Run("wait", addr, "--timeout", strconv.Itoa(timeout), "--json")
	if err != nil {
		return res, err
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return res, fmt.Errorf("hc wait %s: parse: %w", addr, err)
	}
	return res, nil
}

func (w *Watcher) pause(ctx context.Context) {
	d := w.Pause
	if d == 0 {
		d = 10 * time.Second
	}
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// untilUnblocked waits until addr is no longer blocked: true once hc get
// shows another state (someone answered it, in its terminal or with a send).
// It returns false without that word (hc get failed or shows no status); the
// next hc wait then tells what holds, and the blockade counts as the same.
func (w *Watcher) untilUnblocked(ctx context.Context, addr string) bool {
	for ctx.Err() == nil {
		w.pause(ctx)
		info, err := w.C.Get(addr)
		if err != nil || info.Status == nil {
			return false
		}
		if info.Status.State != "blocked" {
			return true
		}
	}
	return false
}

func (w *Watcher) screen(addr string) string {
	out, err := w.C.R.Run("read", addr, "--lines", strconv.Itoa(screenLines))
	if err != nil {
		return fmt.Sprintf("(Bildschirm nicht lesbar: %v)", err)
	}
	return string(out)
}

// lastTurn is the highest turn of addr whose result was delivered.
func (w *Watcher) lastTurn(addr string) int {
	last := 0
	for _, e := range w.L.Snapshot() {
		if e.Address == addr && e.Delivered && e.Turn > last {
			last = e.Turn
		}
	}
	return last
}

// current is the newest open entry of addr: its key names the coordinator's
// status file, its name the agent.
func (w *Watcher) current(addr string) (string, Entry, bool) {
	var (
		key   string
		entry Entry
		found bool
	)
	for k, e := range w.L.Snapshot() {
		if open(e) && e.Address == addr && (!found || e.At.After(entry.At)) {
			key, entry, found = k, e, true
		}
	}
	return key, entry, found
}

// tell sends the coordinator of addr "[Striker] <name> (<addr>) <text>";
// false if the coordinator is gone.
func (w *Watcher) tell(addr, text string) bool {
	key, e, ok := w.current(addr)
	if !ok {
		return false
	}
	file, ok := keyFile(key)
	if !ok {
		return false
	}
	to, err := coordinator(w.C, file)
	if err != nil || to == "" {
		return false
	}
	who := addr
	if e.Name != "" {
		who = e.Name + " (" + addr + ")"
	}
	return w.C.Send(to, lead.Message(who+" "+text)) == nil
}

// finish tells the coordinator the agent's result and closes the entries of
// addr written before the wait that brought it began (a send that came in
// meanwhile stays open: its answer is the next turn). Delivered if the
// coordinator got it; if it is gone, nobody is left to tell and the watch
// stops.
func (w *Watcher) finish(addr string, since time.Time, text string, turn int) {
	delivered := w.tell(addr, text)
	for k, e := range w.L.Snapshot() {
		if !open(e) || e.Address != addr || e.At.After(since) {
			continue
		}
		if delivered {
			e.Delivered, e.Turn = true, turn
		} else {
			e.Watch = false
		}
		w.L.Put(k, e)
	}
	if err := w.L.Save(); err != nil && w.Logf != nil {
		w.Logf("relay: Ledger nicht gespeichert: %v", err)
	}
}

// keyFile is the status file of a ledger key <machine>|<file>|<id> (its
// path is only the file name, enough for File.Key and the requester).
func keyFile(key string) (File, bool) {
	parts := strings.SplitN(key, "|", 3)
	if len(parts) != 3 || !strings.Contains(parts[1], "--") {
		return File{}, false
	}
	repo, change := splitName(parts[1])
	return File{Machine: parts[0], Path: parts[1], Repo: repo, Change: change}, true
}

// LastAnswer is the agent's last answer on screen: the lines from the last
// line starting with the answer marker ● up to the end of the answer (a line
// starting with ✻ or ──── or a ❯ prompt), without the marker and the
// two-space indent of its continuation lines, trailing blanks dropped, at
// most the last 40 lines. If the ● scrolled out of the screen, the answer is
// the text before the last ✻ status line (back to a ❯ prompt or the top). A
// screen with neither (a blocked agent's dialog) gives its last non-blank
// lines. A right column beside the chat (Claude's diff panel) is cut off.
func LastAnswer(screen string) string {
	ls := lines(screen)
	start, status := -1, -1
	for i := len(ls) - 1; i >= 0; i-- {
		t := strings.TrimLeft(ls[i], " ")
		if strings.HasPrefix(t, "●") {
			start = i
			break
		}
		if strings.HasPrefix(t, "✻") {
			status = i
		}
	}
	var block []string
	switch {
	case start >= 0:
		block = []string{strings.TrimSpace(strings.TrimPrefix(strings.TrimLeft(ls[start], " "), "●"))}
		for _, l := range ls[start+1:] {
			if answerEnd(l) {
				break
			}
			block = append(block, strings.TrimPrefix(l, "  "))
		}
	case status >= 0:
		from := 0
		for i := status - 1; i >= 0; i-- {
			if answerEnd(ls[i]) {
				from = i + 1
				break
			}
		}
		for _, l := range ls[from:status] {
			block = append(block, strings.TrimPrefix(l, "  "))
		}
		for len(block) > 0 && strings.TrimSpace(block[0]) == "" {
			block = block[1:]
		}
	default:
		return tail(ls)
	}
	for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
		block = block[:len(block)-1]
	}
	if len(block) > answerLines {
		block = block[len(block)-answerLines:]
	}
	return strings.Join(block, "\n")
}

// answerEnd: a ✻ status line, a ──── rule or a ❯ prompt ends an answer.
func answerEnd(l string) bool {
	t := strings.TrimSpace(l)
	return strings.HasPrefix(t, "✻") || strings.HasPrefix(t, "────") || strings.HasPrefix(t, "❯")
}

// lines splits a screen into lines without trailing white space and without
// a right column (see panelColumn).
func lines(screen string) []string {
	ls := strings.Split(screen, "\n")
	for i, l := range ls {
		ls[i] = strings.TrimRight(l, " \t\r")
	}
	c := panelColumn(ls)
	if c < 0 {
		return ls
	}
	for i, l := range ls {
		rs := []rune(l)
		if len(rs) > c && rs[c] == '│' {
			ls[i] = strings.TrimRight(string(rs[:c]), " ")
		}
	}
	return ls
}

// panelMinColumn: a right column starts at least this far right; a │ further
// left (a table in an answer) is never taken for one.
const panelMinColumn = 40

// panelColumn finds the separator of a right column beside the chat, as
// Claude draws its diff panel in a wide terminal: a │ after a space in the
// same column (in runes) of at least half the non-blank lines, at least two,
// and at least panelMinColumn from the left. -1 if there is none. A line
// whose text fills the chat up to the column has no space before its │; it is
// cut all the same.
func panelColumn(ls []string) int {
	count := map[int]int{}
	nonBlank := 0
	for _, l := range ls {
		if strings.TrimSpace(l) == "" {
			continue
		}
		nonBlank++
		rs := []rune(l)
		for i := panelMinColumn; i < len(rs); i++ {
			if rs[i] == '│' && rs[i-1] == ' ' {
				count[i]++
			}
		}
	}
	best, n := -1, 0
	for c, k := range count {
		if k > n || (k == n && c < best) {
			best, n = c, k
		}
	}
	if n < 2 || 2*n < nonBlank {
		return -1
	}
	return best
}

// tail is the last non-blank lines, at most tailLines.
func tail(ls []string) string {
	var keep []string
	for _, l := range ls {
		if strings.TrimSpace(l) != "" {
			keep = append(keep, l)
		}
	}
	if len(keep) > tailLines {
		keep = keep[len(keep)-tailLines:]
	}
	return strings.Join(keep, "\n")
}
