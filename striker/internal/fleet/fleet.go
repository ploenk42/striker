// Package fleet loads fleet.json.
package fleet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

type Repo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Machine string `json:"machine"`
	Path    string `json:"path"`
	User    string `json:"user"`
	Space   string `json:"space"`
}

type Fleet struct {
	Repos     []Repo            `json:"repos"`
	StatusDir map[string]string `json:"status_dir"`
	// AgentsDir: per machine, where the status line files of its agents are (<terminal>.json); a machine
	// without an entry uses ~/.striker/agents of the user hc logs in as.
	AgentsDir map[string]string `json:"agents_dir"`
}

// RepoAt returns the name of the fleet repo at path on machine ("" or
// "local" = this computer), or "".
func (f Fleet) RepoAt(machine, path string) string {
	if machine == "local" {
		machine = ""
	}
	norm := func(p string) string {
		p = strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
		if len(p) > 1 {
			p = strings.TrimRight(p, "/")
		}
		return p
	}
	for _, r := range f.Repos {
		if r.Machine == machine && norm(r.Path) == norm(path) {
			return r.Name
		}
	}
	return ""
}

// Load reads a fleet file; a leading UTF-8 BOM is accepted.
func Load(path string) (Fleet, error) {
	var f Fleet
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, fmt.Errorf("%w; kopier fleet.example.json daneben nach fleet.json und trag deine Repos ein", err)
	}
	if err != nil {
		return f, err
	}
	b = bytes.TrimPrefix(b, []byte("\xEF\xBB\xBF"))
	if err := json.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}
