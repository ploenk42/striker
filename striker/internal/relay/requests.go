// Package relay does hc work for agents that cannot run hc themselves
// (fleet repos with a user): they write requests into their own status file,
// Striker carries each out once and keeps what it did in its ledger.
package relay

import (
	"bytes"
	"encoding/json"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/ploenk42/striker/striker/internal/coord"
	"github.com/ploenk42/striker/striker/internal/dispatch"
	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
)

// Request is one entry of "requests" in a status file.
type Request struct {
	ID     string `json:"id"`
	Do     string `json:"do"`
	Role   string `json:"role,omitempty"`
	Task   string `json:"task,omitempty"`
	Change string `json:"change,omitempty"`
	To     string `json:"to,omitempty"`
	Text   string `json:"text,omitempty"`
}

// UnmarshalJSON accepts a task id written as a number (5, 2.3) as well.
func (r *Request) UnmarshalJSON(b []byte) error {
	type plain Request
	var v struct {
		plain
		Task json.RawMessage `json:"task"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = Request(v.plain)
	r.Task = ""
	if len(v.Task) > 0 {
		var s string
		var n json.Number
		if json.Unmarshal(v.Task, &s) == nil {
			r.Task = s
		} else if json.Unmarshal(v.Task, &n) == nil {
			r.Task = n.String()
		}
	}
	return nil
}

// File is a status file with requests: <repo>.json of a lead (Change "")
// or <repo>--<change>.json of a coordinator.
type File struct {
	Machine  string
	Path     string
	Repo     string
	Change   string
	Requests []Request
}

// Key is the ledger key of request id in f: <machine>|<file name>|<id>.
func (f File) Key(id string) string {
	return f.Machine + "|" + path.Base(f.Path) + "|" + id
}

// splitName gives repo and change of a status file name.
func splitName(name string) (repo, change string) {
	base := strings.TrimSuffix(name, ".json")
	repo, change, _ = strings.Cut(base, "--")
	return repo, change
}

// relayedMachines are the machines with at least one relayed fleet repo
// (one with a user), sorted.
func relayedMachines(f fleet.Fleet) []string {
	seen := map[string]bool{}
	var ms []string
	for _, r := range f.Repos {
		if r.User != "" && r.Machine != "" && !seen[r.Machine] {
			seen[r.Machine] = true
			ms = append(ms, r.Machine)
		}
	}
	sort.Strings(ms)
	return ms
}

// Read lists, per machine with relayed repos, the status files of its
// status_dir changed within a day and returns those with requests, sorted by
// machine and name. Errors (find, read, invalid JSON) yield no file: the
// status report shows them already.
func Read(c hc.Client, f fleet.Fleet) []File {
	var files []File
	for _, m := range relayedMachines(f) {
		dir, ok := f.StatusDir[m]
		if !ok {
			continue
		}
		out, err := c.R.Run("exec", "--on", m, "find", dir, "-maxdepth", "1", "-name", "*.json", "-mmin", "-1440", "-printf", `%f\n`)
		if err != nil {
			continue
		}
		var names []string
		for _, n := range strings.Split(string(out), "\n") {
			n = strings.TrimSpace(n)
			if strings.HasSuffix(n, ".json") && !strings.ContainsAny(n, `/\`) {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		for _, n := range names {
			p := dir + "/" + n
			b, err := c.ReadFile(m, p)
			if err != nil {
				continue
			}
			rs := parseRequests(b)
			if len(rs) == 0 {
				continue
			}
			repo, change := splitName(n)
			files = append(files, File{Machine: m, Path: p, Repo: repo, Change: change, Requests: rs})
		}
	}
	return files
}

// parseRequests reads "requests" of a status file body; an entry that is no
// object is skipped.
func parseRequests(b []byte) []Request {
	b = bytes.TrimSpace(bytes.TrimPrefix(b, []byte("\xEF\xBB\xBF")))
	var v struct {
		Requests []json.RawMessage `json:"requests"`
	}
	if json.Unmarshal(b, &v) != nil {
		return nil
	}
	var rs []Request
	for _, raw := range v.Requests {
		var r Request
		if json.Unmarshal(raw, &r) == nil {
			rs = append(rs, r)
		}
	}
	return rs
}

// Item is a request to carry out.
type Item struct {
	File    File
	Request Request
}

// Dropped is a request Striker refuses, with the reason (German, for the
// answer to the agent).
type Dropped struct {
	File    File
	Request Request
	Reason  string
}

// Filter splits the requests of files into those to carry out and those
// refused with a reason, in file order. Left out silently: requests whose
// key is in the ledger, requests without an id (no key to answer them once
// by) and repeats of an id within a file (only the first counts).
func Filter(f fleet.Fleet, l *Ledger, files []File) (keep []Item, drop []Dropped) {
	for _, fl := range files {
		fileReason := checkFile(f, fl)
		seen := map[string]bool{}
		for _, r := range fl.Requests {
			if r.ID == "" || seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			if l != nil && l.Has(fl.Key(r.ID)) {
				continue
			}
			reason := fileReason
			if reason == "" {
				reason = checkRequest(fl, r)
			}
			if reason != "" {
				drop = append(drop, Dropped{File: fl, Request: r, Reason: reason})
				continue
			}
			keep = append(keep, Item{File: fl, Request: r})
		}
	}
	return keep, drop
}

// checkFile: the file must belong to a relayed fleet repo on its machine,
// a coordinator's file must name a valid change.
func checkFile(f fleet.Fleet, fl File) string {
	relayed := false
	for _, r := range f.Repos {
		if r.Name == fl.Repo && r.Machine == fl.Machine && r.User != "" {
			relayed = true
			break
		}
	}
	if !relayed {
		return "kein weitergeleitetes Fleet-Repo " + fl.Repo + " auf " + fl.Machine
	}
	if fl.Change != "" && !coord.ValidChange(fl.Change) {
		return "ungültiger Change-Name " + strconv.Quote(fl.Change) + " im Dateinamen"
	}
	return ""
}

func checkRequest(fl File, r Request) string {
	switch {
	case strings.ContainsAny(r.ID, "|\n\r\t") || len(r.ID) > 100:
		return "ungültige id " + strconv.Quote(r.ID)
	}
	lead := fl.Change == ""
	switch r.Do {
	case "start-coordinator":
		if !lead {
			return "start-coordinator darf nur der Lead anfragen"
		}
		if !coord.ValidChange(r.Change) {
			return "ungültiger Change-Name " + strconv.Quote(r.Change)
		}
	case "dispatch":
		if lead {
			return "dispatch darf nur ein Koordinator anfragen"
		}
		if !dispatch.ValidRole(r.Role) {
			return "ungültige Rolle " + strconv.Quote(r.Role)
		}
		if strings.TrimSpace(r.Task) == "" {
			return "dispatch ohne task"
		}
	case "send":
		if lead {
			return "send darf nur ein Koordinator anfragen"
		}
		if r.To == "" {
			return "send ohne to"
		}
		if strings.TrimSpace(r.Text) == "" {
			return "send ohne text"
		}
	default:
		return "unbekannte Anfrage " + strconv.Quote(r.Do)
	}
	return ""
}
