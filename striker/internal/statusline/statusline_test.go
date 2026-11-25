package statusline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sample = `{
  "session_id": "s1",
  "model": {"id": "claude-opus-5-5", "display_name": "Opus 5.5"},
  "context_window": {"total_input_tokens": 1100000, "total_output_tokens": 134567, "used_percentage": 41.6}
}`

var at = time.UnixMilli(1795380000000)

func TestWriteStoresOneLine(t *testing.T) {
	dir := t.TempDir()
	line := Write([]byte(sample), "t7", dir, at)
	if line != "Opus 5.5 · 1.2M" {
		t.Errorf("line = %q", line)
	}
	b, err := os.ReadFile(filepath.Join(dir, "t7.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Count(s, "\n") != 1 || !strings.HasSuffix(s, "\n") {
		t.Errorf("not one line: %q", s)
	}
	if !strings.HasPrefix(s, `{"terminal":"t7","at":1795380000000,"claude":{"session_id":"s1",`) {
		t.Errorf("file = %q", s)
	}
	e, err := Parse([]byte(strings.TrimSpace(s)))
	if err != nil {
		t.Fatal(err)
	}
	if e.Terminal != "t7" || e.At != 1795380000000 || e.Model != "Opus 5.5" || e.Tokens != 1234567 || e.ContextPct != 41 {
		t.Errorf("entry = %+v", e)
	}
}

func TestWriteWithoutTerminal(t *testing.T) {
	dir := t.TempDir()
	if line := Write([]byte(sample), "", dir, at); line != "Opus 5.5 · 1.2M" {
		t.Errorf("line = %q", line)
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("files written: %v", es)
	}
}

func TestWriteBadInput(t *testing.T) {
	dir := t.TempDir()
	if line := Write([]byte("not json"), "t7", dir, at); line != "" {
		t.Errorf("line = %q", line)
	}
	// a terminal id that is no plain name never becomes a path
	if line := Write([]byte(sample), "../t7", dir, at); line == "" {
		t.Errorf("line lost for a bad terminal id")
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("files written: %v", es)
	}
}

func TestWriteMakesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "agents")
	Write([]byte(sample), "t1b", dir, at)
	if _, err := os.Stat(filepath.Join(dir, "t1b.json")); err != nil {
		t.Fatal(err)
	}
}

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	os.WriteFile(p, []byte("{\n  \"theme\": \"dark\"\n}\n"), 0o644)
	changed, err := Install(p, `C:\x\striker.exe statusline`)
	if err != nil || !changed {
		t.Fatalf("changed %v err %v", changed, err)
	}
	b, _ := os.ReadFile(p)
	want := "{\n  \"statusLine\": {\"type\": \"command\", \"command\": \"C:\\\\x\\\\striker.exe statusline\"},\n  \"theme\": \"dark\"\n}\n"
	if string(b) != want {
		t.Errorf("got %q\nwant %q", b, want)
	}
	// a second run, or a file with its own status line: unchanged
	if changed, err := Install(p, "other"); err != nil || changed {
		t.Errorf("second run changed %v err %v", changed, err)
	}
	// no file, an empty object: made
	q := filepath.Join(dir, "new", "settings.json")
	if changed, err := Install(q, "striker statusline"); err != nil || !changed {
		t.Fatalf("new file: changed %v err %v", changed, err)
	}
	b, _ = os.ReadFile(q)
	if string(b) != "{\n  \"statusLine\": {\"type\": \"command\", \"command\": \"striker statusline\"}\n}\n" {
		t.Errorf("new file %q", b)
	}
	e := filepath.Join(dir, "empty.json")
	os.WriteFile(e, []byte("{}"), 0o644)
	if _, err := Install(e, "striker statusline"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(e)
	if string(b) != "{\n  \"statusLine\": {\"type\": \"command\", \"command\": \"striker statusline\"}\n}" {
		t.Errorf("empty object %q", b)
	}
	// not JSON: left alone, error
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("nope"), 0o644)
	if _, err := Install(bad, "x"); err == nil {
		t.Errorf("no error for a file that is not JSON")
	}
}

func TestShort(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 1000: "1K", 812345: "812K", 999999: "999K", 1000000: "1.0M", 1234567: "1.2M", 25_000_000: "25M"}
	for n, want := range cases {
		if got := Short(n); got != want {
			t.Errorf("Short(%d) = %q, want %q", n, got, want)
		}
	}
}

// A line exactly as setup/statusline.sh wrote it on remote1 (JSON kept as sent, newlines removed).
func TestParseShellLine(t *testing.T) {
	line := `{"terminal":"t99","at":1795466472000,"claude":{  "session_id": "s1",  "model": {"id": "claude-opus-5-5", "display_name": "Opus 5.5"},  "output_style": {"name": "default"},  "context_window": {"total_input_tokens": 1100000, "total_output_tokens": 134567, "used_percentage": 41.6}}}`
	e, err := Parse([]byte(line))
	if err != nil || e.Terminal != "t99" || e.Model != "Opus 5.5" || e.Tokens != 1234567 || e.ContextPct != 41 {
		t.Errorf("entry = %+v, err %v", e, err)
	}
}

func TestParseLines(t *testing.T) {
	in := `{"terminal":"t1","at":5,"claude":{"model":{"display_name":"Sonnet 5.5"},"context_window":{"total_input_tokens":10,"total_output_tokens":5}}}
garbage
{"terminal":"t2","at":6,"claude":{}}
`
	es := ParseLines([]byte(in))
	if len(es) != 2 || es[0].Terminal != "t1" || es[0].Tokens != 15 || es[0].Model != "Sonnet 5.5" || es[1].Terminal != "t2" {
		t.Errorf("entries = %+v", es)
	}
}
