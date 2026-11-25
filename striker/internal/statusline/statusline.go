// Package statusline stores what Claude Code's status line command receives (model, tokens) in one file per
// herdr terminal, and reads those files back for Striker's crew. The file is <dir>/<terminal>.json, one line:
// {"terminal":"t7","at":<unix ms>,"claude":<the status line JSON as Claude sent it>}.
package statusline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Entry is what Striker needs from one file.
type Entry struct {
	Terminal   string
	At         int64 // unix ms
	Model      string
	Tokens     int64 // input + output
	ContextPct int
}

type claude struct {
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		In   int64   `json:"total_input_tokens"`
		Out  int64   `json:"total_output_tokens"`
		Used float64 `json:"used_percentage"`
	} `json:"context_window"`
}

var plainID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Write stores stdin (Claude's status line JSON) for terminal in dir and returns the line to print. Bad JSON:
// nothing stored, "". No or odd terminal id: nothing stored, the line still returned. Write never fails loudly:
// a status line must not break Claude's screen.
func Write(stdin []byte, terminal, dir string, now time.Time) string {
	var c claude
	if err := json.Unmarshal(stdin, &c); err != nil {
		return ""
	}
	line := Line(c.Model.DisplayName, c.ContextWindow.In+c.ContextWindow.Out)
	if !plainID.MatchString(terminal) || dir == "" {
		return line
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, stdin); err != nil {
		return line
	}
	tid, _ := json.Marshal(terminal)
	body := fmt.Sprintf(`{"terminal":%s,"at":%d,"claude":%s}`+"\n", tid, now.UnixMilli(), compact.Bytes())
	if err := os.MkdirAll(dir, 0o775); err != nil {
		return line
	}
	tmp := filepath.Join(dir, "."+terminal+".tmp")
	if err := os.WriteFile(tmp, []byte(body), 0o664); err != nil {
		return line
	}
	_ = os.Rename(tmp, filepath.Join(dir, terminal+".json"))
	return line
}

// Install sets Claude Code's status line to command in the settings file at path, only when the file has no
// "statusLine" yet; it reports whether it changed the file. The line is inserted as text right after the opening
// brace, so the rest of the file (order, formatting) stays as it is. A missing file is made; a file that is not a
// JSON object is left alone with an error.
func Install(path, command string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if os.IsNotExist(err) {
		b = []byte("{}\n")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimPrefix(b, []byte("\xEF\xBB\xBF")), &obj); err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if _, ok := obj["statusLine"]; ok {
		return false, nil
	}
	cmd, _ := json.Marshal(command)
	entry := `"statusLine": {"type": "command", "command": ` + string(cmd) + `}`
	i := bytes.IndexByte(b, '{')
	rest := b[i+1:]
	var out []byte
	if len(obj) == 0 {
		out = append(append(append([]byte{}, b[:i+1]...), "\n  "+entry+"\n"...), bytes.TrimLeft(rest, " \t\r\n")...)
	} else {
		out = append(append(append([]byte{}, b[:i+1]...), "\n  "+entry+","...), rest...)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, out, 0o644)
}

// Line is the status bar text: "<model> · <tokens>", either part left out when unknown.
func Line(model string, tokens int64) string {
	switch {
	case model != "" && tokens > 0:
		return model + " · " + Short(tokens)
	case model != "":
		return model
	case tokens > 0:
		return Short(tokens)
	}
	return ""
}

// Short writes a token count briefly: 999, 812K, 1.2M, 25M.
func Short(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprint(n)
	case n < 1_000_000:
		return fmt.Sprintf("%dK", n/1000)
	case n < 10_000_000:
		return fmt.Sprintf("%.1fM", float64(n/100_000)/10)
	}
	return fmt.Sprintf("%dM", n/1_000_000)
}

// Parse reads one stored file.
func Parse(b []byte) (Entry, error) {
	var v struct {
		Terminal string `json:"terminal"`
		At       int64  `json:"at"`
		Claude   claude `json:"claude"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return Entry{}, err
	}
	if v.Terminal == "" {
		return Entry{}, fmt.Errorf("no terminal")
	}
	return Entry{Terminal: v.Terminal, At: v.At, Model: v.Claude.Model.DisplayName,
		Tokens: v.Claude.ContextWindow.In + v.Claude.ContextWindow.Out, ContextPct: int(v.Claude.ContextWindow.Used)}, nil
}

// ParseLines reads several stored files given as one per line (what `cat` of a directory prints); lines that
// do not parse are skipped.
func ParseLines(b []byte) []Entry {
	var out []Entry
	for _, l := range bytes.Split(b, []byte("\n")) {
		if l = bytes.TrimSpace(l); len(l) == 0 {
			continue
		}
		if e, err := Parse(l); err == nil {
			out = append(out, e)
		}
	}
	return out
}
