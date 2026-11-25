package lead

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
)

type fakeHC struct {
	args     [][]string
	lsOut    string
	state    string
	lsErr    error
	labelErr error
}

func (f *fakeHC) Run(args ...string) ([]byte, error) {
	f.args = append(f.args, args)
	switch args[0] {
	case "ls":
		if f.lsErr != nil {
			return nil, f.lsErr
		}
		if f.lsOut != "" {
			return []byte(f.lsOut), nil
		}
		return []byte(`{"terminals":[]}`), nil
	case "get":
		return []byte(`{"address":"m/t9","status":{"state":"` + f.state + `"}}`), nil
	case "label":
		return nil, f.labelErr
	}
	return nil, nil
}

func TestMessagePrefix(t *testing.T) {
	if got := Message("hi"); got != "[Striker] hi" {
		t.Fatal(got)
	}
	if got := Message("[Striker] x\n"); got != "[Striker] x" {
		t.Fatal(got)
	}
}

func TestSendPassesTextAsOneArg(t *testing.T) {
	f := &fakeHC{}
	text := "say \"hi\" C:\\path\\" + "\n" + "line2"
	if err := (hc.Client{R: f}).Send("t3", Message(text)); err != nil {
		t.Fatal(err)
	}
	want := []string{"send", "t3", "[Striker] say \"hi\" C:\\path\\\nline2"}
	if !reflect.DeepEqual(f.args[0], want) {
		t.Fatalf("got %q", f.args[0])
	}
}

var testFleet = fleet.Fleet{
	Repos:     []fleet.Repo{{Name: "beta", Machine: "remote1", Path: "/root/beta", User: "root", Space: "beta"}},
	StatusDir: map[string]string{"remote1": "/var/striker"},
}

// {coordinators}: a direct lead starts coordinators itself, a relayed one
// asks Striker through requests in its status file.
func TestFillBriefCoordinators(t *testing.T) {
	direct := FillBrief("{repo}|{path}|{machine}|{status_file}|{coordinators}", "beta", "/srv/beta", "remote1", "/s/beta.json", false)
	if !strings.HasPrefix(direct, "beta|/srv/beta|remote1|/s/beta.json|") ||
		!strings.Contains(direct, "striker start-coordinator beta <change>") || strings.Contains(direct, "requests") {
		t.Errorf("direct:\n%s", direct)
	}
	relayed := FillBrief("{coordinators}", "beta", "/srv/beta", "remote1", "/s/beta.json", true)
	for _, want := range []string{
		"/s/beta.json", "requests",
		`{ "id": "c-<change>-1", "do": "start-coordinator", "change": "<change>" }`,
		"beende deinen Zug; Strikers Nachricht bringt das Ergebnis",
		"nie mit `sleep`", "nicht wiederholt",
	} {
		if !strings.Contains(relayed, want) {
			t.Errorf("relayed lacks %q:\n%s", want, relayed)
		}
	}
	if strings.Contains(relayed, "striker start-coordinator") || strings.Contains(relayed, "{status_file}") {
		t.Errorf("relayed tells to call striker or keeps a placeholder:\n%s", relayed)
	}
}

// The shipped brief has {coordinators} in rule 5.
func TestBriefFileHasCoordinators(t *testing.T) {
	tpl, err := os.ReadFile(filepath.Join("..", "..", "briefs", "lead.md"))
	if err != nil {
		t.Fatal(err)
	}
	// Issues and Refs/Fixes exist only for direct leads: the fixed text says so.
	for _, line := range strings.Split(string(tpl), "\n") {
		if (strings.Contains(line, "striker issues") || strings.Contains(line, "Fixes #")) && !strings.Contains(line, "Nur direkt") {
			t.Errorf("unconditional issue line: %s", line)
		}
	}
	direct := FillBrief(string(tpl), "beta", "/srv/beta", "remote1", "/s/beta.json", false)
	relayed := FillBrief(string(tpl), "beta", "/srv/beta", "remote1", "/s/beta.json", true)
	if !strings.Contains(relayed, "`/s/beta.json`") {
		t.Errorf("relayed status file not in backticks:\n%s", relayed)
	}
	if strings.Contains(direct, "{coordinators}") || !strings.Contains(direct, "   - Zur Umsetzung startest du pro Change einen Koordinator mit `striker start-coordinator beta <change>`") ||
		!strings.Contains(relayed, "   - Zur Umsetzung startet Striker pro Change einen Koordinator für dich") {
		t.Errorf("direct:\n%s\nrelayed:\n%s", direct, relayed)
	}
}

func TestStartLeadRelayedBrief(t *testing.T) {
	f := &fakeHC{state: "idle"}
	run := func(a ...string) ([]byte, error) { return []byte(`{"terminal":"m/t9"}`), nil }
	fl := fleet.Fleet{Repos: []fleet.Repo{{Name: "beta", Machine: "remote1", Path: "/srv/beta", User: "awayteam", Space: "beta"}},
		StatusDir: map[string]string{"remote1": "/var/lib/striker/status"}}
	brief := filepath.Join(t.TempDir(), "lead.md")
	os.WriteFile(brief, []byte("{coordinators}\n"), 0o644)
	if _, err := Start(hc.Client{R: f}, run, fl, "beta", brief); err != nil {
		t.Fatal(err)
	}
	last := f.args[len(f.args)-1]
	if last[0] != "send" || !strings.Contains(last[2], `"do": "start-coordinator"`) || !strings.Contains(last[2], "/var/lib/striker/status/beta.json") {
		t.Fatalf("brief %q", last)
	}
}

func TestStartLeadRefusesExisting(t *testing.T) {
	f := &fakeHC{lsOut: `{"terminals":[{"address":"m/t1"}]}`}
	ran := false
	_, err := Start(hc.Client{R: f}, func(a ...string) ([]byte, error) { ran = true; return nil, nil }, testFleet, "beta", "")
	if err == nil || !strings.Contains(err.Error(), "m/t1") || ran {
		t.Fatalf("err=%v ran=%v", err, ran)
	}
}

func TestStartLeadRootArgv(t *testing.T) {
	f := &fakeHC{state: "idle"}
	var got []string
	run := func(a ...string) ([]byte, error) {
		got = a
		return []byte(`{"terminal":"m/t9"}`), nil
	}
	brief := filepath.Join(t.TempDir(), "lead.md")
	os.WriteFile(brief, []byte("{repo}|{path}|{machine}|{status_file}\n"), 0o644)
	addr, err := Start(hc.Client{R: f}, run, testFleet, "beta", brief)
	if err != nil || addr != "m/t9" {
		t.Fatalf("addr=%q err=%v", addr, err)
	}
	var argv []string
	if err := json.Unmarshal([]byte(got[len(got)-1]), &argv); err != nil {
		t.Fatal(err)
	}
	want := []string{"sudo", "--preserve-env=HC_TERMINAL_ID", "-i", "sh", "-c", "cd '/root/beta' && exec /root/.local/bin/claude --remote-control 'lead beta'"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv %q", argv)
	}
	if !reflect.DeepEqual(got[:9], []string{"do", "space.new", "--name", "beta", "--machine", "remote1", "--tab", "claude", "--argv"}) {
		t.Fatalf("args %q", got)
	}
	last := f.args[len(f.args)-1]
	if last[0] != "send" || last[2] != "beta|/root/beta|remote1|/var/striker/beta.json" {
		t.Fatalf("brief %q", last)
	}
}

func TestStartLeadAwayteamArgv(t *testing.T) {
	f := &fakeHC{state: "idle"}
	var got []string
	run := func(a ...string) ([]byte, error) { got = a; return []byte(`{"terminal":"m/t9"}`), nil }
	fl := fleet.Fleet{Repos: []fleet.Repo{{Name: "beta", Machine: "remote1", Path: "/srv/beta", User: "awayteam", Space: "beta"}},
		StatusDir: map[string]string{"remote1": "/var/lib/striker/status"}}
	brief := filepath.Join(t.TempDir(), "lead.md")
	os.WriteFile(brief, []byte("{repo}\n"), 0o644)
	if _, err := Start(hc.Client{R: f}, run, fl, "beta", brief); err != nil {
		t.Fatal(err)
	}
	var argv []string
	if err := json.Unmarshal([]byte(got[len(got)-1]), &argv); err != nil {
		t.Fatal(err)
	}
	want := []string{"sudo", "--preserve-env=HC_TERMINAL_ID", "-u", "awayteam", "-i", "sh", "-c", `cd '/srv/beta' && exec "$HOME/.local/bin/claude" --remote-control 'lead beta'`}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv %q", argv)
	}
}

func TestStartLeadTabNamedClaude(t *testing.T) {
	f := &fakeHC{state: "blocked"}
	var got []string
	run := func(a ...string) ([]byte, error) { got = a; return []byte(`{"terminal":"m/t9"}`), nil }
	Start(hc.Client{R: f}, run, testFleet, "beta", "")
	tab := ""
	for i := 0; i+1 < len(got); i++ {
		if got[i] == "--tab" {
			tab = got[i+1]
		}
	}
	if len(got) < 2 || got[1] != "space.new" || tab != "claude" {
		t.Fatalf("args %q", got)
	}
}

func TestStartLeadNotReady(t *testing.T) {
	f := &fakeHC{state: "blocked"}
	run := func(a ...string) ([]byte, error) { return []byte(`{"terminal":"m/t9"}`), nil }
	addr, err := Start(hc.Client{R: f}, run, testFleet, "beta", "")
	var nr *NotReadyError
	if !errors.As(err, &nr) || nr.State != "blocked" || addr != "m/t9" {
		t.Fatalf("addr=%q err=%v", addr, err)
	}
	for _, a := range f.args {
		if a[0] == "send" {
			t.Fatal("brief sent")
		}
	}
}

func TestStartLeadListLabelErrorStops(t *testing.T) {
	f := &fakeHC{lsErr: errors.New("hc down")}
	ran := false
	_, err := Start(hc.Client{R: f}, func(a ...string) ([]byte, error) { ran = true; return nil, nil }, testFleet, "beta", "")
	if err == nil || ran || !strings.Contains(err.Error(), "kann nicht prüfen, ob beta schon einen Lead hat") || !errors.Is(err, f.lsErr) {
		t.Fatalf("err=%v ran=%v", err, ran)
	}
}

func TestStartLeadLabelErrorReported(t *testing.T) {
	f := &fakeHC{state: "idle", labelErr: errors.New("label failed")}
	run := func(a ...string) ([]byte, error) { return []byte(`{"terminal":"m/t9"}`), nil }
	addr, err := Start(hc.Client{R: f}, run, testFleet, "beta", "")
	if err == nil || addr != "m/t9" || !errors.Is(err, f.labelErr) || !strings.Contains(err.Error(), "m/t9") {
		t.Fatalf("addr=%q err=%v", addr, err)
	}
}
