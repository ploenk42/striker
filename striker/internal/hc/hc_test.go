package hc

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fake struct {
	out  string
	err  error
	args [][]string
}

func (f *fake) Run(args ...string) ([]byte, error) {
	f.args = append(f.args, args)
	return []byte(f.out), f.err
}

func TestListParsesTerminals(t *testing.T) {
	f := &fake{out: `{"hidden_finished":0,"machines":[],"terminals":[{"address":"t1","agent":"zsh","cwd":"/x","state":"live","turn":0,"where":"gamma / 1"}]}`}
	ts, err := Client{R: f}.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 || ts[0].Address != "t1" || ts[0].Cwd != "/x" || ts[0].Where != "gamma / 1" {
		t.Fatalf("got %+v", ts)
	}
	if !reflect.DeepEqual(f.args[0], []string{"ls", "--json"}) {
		t.Fatalf("args %v", f.args[0])
	}
}

func TestListLabelArgs(t *testing.T) {
	f := &fake{out: `{"terminals":[]}`}
	if _, err := (Client{R: f}).ListLabel("striker.lead", "alpha"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.args[0], []string{"ls", "--label", "striker.lead=alpha", "--json"}) {
		t.Fatalf("args %v", f.args[0])
	}
}

// Trimmed from real `hc get t1 --json` output.
const getSample = `{"address":"t1","cwd":"/Users/you","labels":{"herdr.seen":"0","herdr.space":"sp5e038549d2da6239a315a146a2cb9545","herdr.tab":"tb12bff785b3e63d9afec54c5f37a01145","where":"gamma / 1"},"state":"live","status":null,"status_ms":null}`

func TestGetParsesLabels(t *testing.T) {
	f := &fake{out: getSample}
	info, err := Client{R: f}.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	if info.Labels["herdr.space"] == "" || info.Cwd != "/Users/you" {
		t.Fatalf("got %+v", info)
	}
	if info.Status != nil || info.StatusMs != nil {
		t.Fatalf("status should be nil: %+v", info)
	}
	if !reflect.DeepEqual(f.args[0], []string{"get", "t1", "--json"}) {
		t.Fatalf("args %v", f.args[0])
	}
}

func TestGetParsesStatus(t *testing.T) {
	f := &fake{out: `{"address":"t1","status":{"state":"working","msg":"hi"},"status_ms":5}`}
	info, err := Client{R: f}.Get("t1")
	if err != nil {
		t.Fatal(err)
	}
	if info.Status == nil || info.Status.State != "working" || info.Status.Msg != "hi" || *info.StatusMs != 5 {
		t.Fatalf("got %+v", info)
	}
}

func TestIsLocal(t *testing.T) {
	if !IsLocal("t3") || IsLocal("remote1/t3") {
		t.Fatal("IsLocal wrong")
	}
}

func TestCommandArgs(t *testing.T) {
	f := &fake{}
	c := Client{R: f}
	if _, err := c.ReadFile("remote1", "/a/b"); err != nil {
		t.Fatal(err)
	}
	c.Send("t1", "hello")
	c.Label("t1", "k=v")
	c.Wait("t1", 30)
	want := [][]string{
		{"file", "read", "--on", "remote1", "/a/b"},
		{"send", "t1", "hello"},
		{"label", "t1", "k=v"},
		{"wait", "t1", "--timeout", "30"},
	}
	if !reflect.DeepEqual(f.args, want) {
		t.Fatalf("got %v", f.args)
	}
}

func TestErrorsPropagate(t *testing.T) {
	f := &fake{err: errors.New("boom")}
	if _, err := (Client{R: f}).List(); err == nil {
		t.Fatal("want error")
	}
	if err := (Client{R: f}).Send("t1", "x"); err == nil {
		t.Fatal("want error")
	}
}

// helperEnv makes the re-executed test binary act as a fake hc (no sh needed).
func helperExec(t *testing.T, timeout time.Duration) Exec {
	t.Helper()
	t.Setenv("STRIKER_HELPER_PROCESS", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Exec{Bin: exe, Timeout: timeout}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("STRIKER_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	switch args[0] {
	case "stderr":
		fmt.Fprint(os.Stderr, "oops\nmore\n")
		os.Exit(3)
	case "sleep":
		time.Sleep(10 * time.Second)
	}
	os.Exit(0)
}

func TestExecNonZeroCarriesStderr(t *testing.T) {
	_, err := helperExec(t, 0).Run("-test.run=TestHelperProcess", "--", "stderr")
	var ee *ExecError
	if !errors.As(err, &ee) || ee.Stderr != "oops\nmore\n" {
		t.Fatalf("got %v", err)
	}
	if err.Error() == "" || !strings.HasSuffix(err.Error(), ": oops") {
		t.Fatalf("message %q", err.Error())
	}
}

func TestExecTimesOut(t *testing.T) {
	start := time.Now()
	_, err := helperExec(t, 100*time.Millisecond).Run("-test.run=TestHelperProcess", "--", "sleep")
	if err == nil || !strings.Contains(err.Error(), "timed out after 100ms") || !strings.HasPrefix(err.Error(), helperExec(t, 0).Bin+" -test.run=TestHelperProcess: ") {
		t.Fatalf("got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("did not stop the child")
	}
}

func TestTimeoutForHonoursOwnTimeout(t *testing.T) {
	x := Exec{}
	if d := x.timeoutFor([]string{"wait", "t1", "--timeout", "90"}); d != 100*time.Second {
		t.Fatalf("wait 90: %v", d)
	}
	if d := x.timeoutFor([]string{"wait", "t1", "--timeout", "5"}); d != 30*time.Second {
		t.Fatalf("wait 5: %v", d)
	}
	if d := x.timeoutFor([]string{"ls", "--json"}); d != 30*time.Second {
		t.Fatalf("ls: %v", d)
	}
	if d := (Exec{Timeout: time.Second}).timeoutFor([]string{"ls"}); d != time.Second {
		t.Fatalf("explicit: %v", d)
	}
}

func TestExecErrorNamesBinary(t *testing.T) {
	_, err := Exec{Bin: "definitely-not-a-real-binary-xyz"}.Run("do", "x")
	if err == nil || !strings.HasPrefix(err.Error(), "definitely-not-a-real-binary-xyz do x:") {
		t.Fatalf("got %v", err)
	}
}
