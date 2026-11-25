// Package hc wraps the herdr `hc` command line client.
package hc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Runner runs hc with the given arguments and returns stdout.
type Runner interface {
	Run(args ...string) (stdout []byte, err error)
}

// ExecError is returned by Exec when hc exits non-zero.
type ExecError struct {
	Bin    string // binary that failed (default "hc")
	Args   []string
	Stderr string // full stderr
	Err    error

	Timeout bool
}

func (e *ExecError) Error() string {
	if e.Timeout {
		return e.Err.Error()
	}
	first, _, _ := strings.Cut(strings.TrimSpace(e.Stderr), "\n")
	bin := e.Bin
	if bin == "" {
		bin = "hc"
	}
	return fmt.Sprintf("%s %s: %v: %s", bin, strings.Join(e.Args, " "), e.Err, first)
}

func (e *ExecError) Unwrap() error { return e.Err }

// Exec runs the real hc binary (Bin, default "hc").
type Exec struct {
	Bin     string
	Timeout time.Duration // per call; 30s when zero
}

const defaultTimeout = 30 * time.Second

// timeoutFor: Timeout if set, else 30s, but a call carrying its own `--timeout N`
// (hc wait) gets N+10s so the wrapper never cuts it short.
func (x Exec) timeoutFor(args []string) time.Duration {
	if x.Timeout != 0 {
		return x.Timeout
	}
	d := defaultTimeout
	for i, a := range args {
		if a == "--timeout" && i+1 < len(args) {
			if n, err := strconv.Atoi(args[i+1]); err == nil && time.Duration(n+10)*time.Second > d {
				d = time.Duration(n+10) * time.Second
			}
		}
	}
	return d
}

func (x Exec) Run(args ...string) ([]byte, error) {
	bin := x.Bin
	if bin == "" {
		bin = "hc"
	}
	timeout := x.timeoutFor(args)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.WaitDelay = 2 * time.Second // do not hang on pipes held by grandchildren
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			first := ""
			if len(args) > 0 {
				first = args[0]
			}
			return out.Bytes(), &ExecError{Args: args, Stderr: errb.String(), Err: fmt.Errorf("%s %s: timed out after %s", bin, first, timeout), Bin: bin, Timeout: true}
		}
		return out.Bytes(), &ExecError{Bin: bin, Args: args, Stderr: errb.String(), Err: err}
	}
	return out.Bytes(), nil
}

type Terminal struct {
	Address string `json:"address"`
	Agent   string `json:"agent"` // the program herdr recognised, e.g. "claude-code"
	Status  string `json:"status"`
	Cwd     string `json:"cwd"`
	Where   string `json:"where"`
}

type Info struct {
	Address   string            `json:"address"`
	Cwd       string            `json:"cwd"`
	CreatedMs int64             `json:"created_ms"`
	Labels    map[string]string `json:"labels"`
	Status  *struct {
		State string `json:"state"`
		Msg   string `json:"msg"`
	} `json:"status"`
	StatusMs *int64 `json:"status_ms"`
}

// IsLocal reports whether addr is on this machine (no machine prefix).
func IsLocal(addr string) bool { return !strings.Contains(addr, "/") }

type Client struct{ R Runner }

func (c Client) list(args ...string) ([]Terminal, error) {
	out, err := c.R.Run(args...)
	if err != nil {
		return nil, err
	}
	var v struct {
		Terminals []Terminal `json:"terminals"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, fmt.Errorf("hc %s: parse: %w", strings.Join(args, " "), err)
	}
	return v.Terminals, nil
}

func (c Client) List() ([]Terminal, error) { return c.list("ls", "--json") }

func (c Client) ListLabel(k, v string) ([]Terminal, error) {
	return c.list("ls", "--label", k+"="+v, "--json")
}

func (c Client) Get(addr string) (Info, error) {
	var info Info
	out, err := c.R.Run("get", addr, "--json")
	if err != nil {
		return info, err
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return info, fmt.Errorf("hc get %s: parse: %w", addr, err)
	}
	return info, nil
}

func (c Client) ReadFile(machine, path string) ([]byte, error) {
	return c.R.Run("file", "read", "--on", machine, path)
}

func (c Client) Send(addr, text string) error {
	_, err := c.R.Run("send", addr, text)
	return err
}

func (c Client) Label(addr, kv string) error {
	_, err := c.R.Run("label", addr, kv)
	return err
}

func (c Client) Wait(addr string, timeout int) error {
	_, err := c.R.Run("wait", addr, "--timeout", strconv.Itoa(timeout))
	return err
}
