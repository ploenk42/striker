// Package crew says who is who among the Claude agents herdr knows: a Star Trek name kept for the agent's life,
// a rank from its role or model, its repo, and its model and tokens from the status line files
// (internal/statusline). Striker serves it as GET /crew; the LCARS sidebar shows it.
package crew

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
	"github.com/ploenk42/striker/striker/internal/statusline"
)

// Agent is one crew member.
type Agent struct {
	Address    string `json:"address"`
	Name       string `json:"name"`
	Rank       string `json:"rank"`
	Role       string `json:"role"`
	Repo       string `json:"repo"`
	Status     string `json:"status"`
	Model      string `json:"model"`
	Tokens     int64  `json:"tokens"`
	ContextPct int    `json:"context_pct"`
	Seen       string `json:"seen,omitempty"` // when the status line last wrote, RFC 3339
}

type Crew struct {
	Agents []Agent `json:"agents"`
}

// Leads and coordinators take bridge names, everyone else lower-decks names. STRIKER is the orchestrator's.
const orchestrator = "STRIKER"

var (
	bridgePool = []string{"PATA", "WERF", "TREY", "LE FERGE", "CRUSHA", "E'BRYEN", "YEAR", "GUYNAN", "BARCLAY", "RE", "PULESKY", "EGAWA"}
	lowerPool  = []string{"BEYMLA", "MARYNA", "TENDY", "RUTHAFERD", "FREEMAN", "RANSEM", "SHAKES", "DANNA", "BYLLUPS",
		"MYGLEEME", "JENNYFA", "BARNES", "STEVENS", "KAYSHEN"}
)

// pick is the first free name of the agent's pool, then of the other pool, then CREWMAN <n>.
func pick(bridge bool, used map[string]bool) string {
	first, second := lowerPool, bridgePool
	if bridge {
		first, second = bridgePool, lowerPool
	}
	for _, pool := range [][]string{first, second} {
		for _, n := range pool {
			if !used[n] {
				return n
			}
		}
	}
	for i := 1; ; i++ {
		if n := fmt.Sprintf("CREWMAN %d", i); !used[n] {
			return n
		}
	}
}

// isStriker: Striker himself is the Claude with the label striker.self (wherever he runs, e.g. as the lead of
// herdr-mods), else the Claude in the space STRIKER on this computer (as the LCARS mod finds him).
func isStriker(machine, where string, labels map[string]string) bool {
	if labels["striker.self"] != "" {
		return true
	}
	space, _, _ := strings.Cut(where, " / ")
	return machine == "" && strings.EqualFold(strings.TrimSpace(space), "STRIKER")
}

// rank is a US Navy rank, written without dots: from the role, else from the model.
func rank(role, model string) string {
	switch {
	case role == "striker":
		return "CDR"
	case role == "lead":
		return "LCDR"
	case role == "coordinator":
		return "LT"
	case strings.HasSuffix(role, "reviewer"):
		return "LTJG"
	case role != "":
		return "ENS"
	}
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "fable"):
		return "LCDR"
	case strings.Contains(m, "opus"):
		return "LT"
	case strings.Contains(m, "sonnet"):
		return "LTJG"
	case strings.Contains(m, "haiku"):
		return "ENS"
	}
	return "CREW"
}

func roleOf(labels map[string]string) string {
	switch {
	case labels["striker.lead"] != "":
		return "lead"
	case labels["striker.coordinator"] != "":
		return "coordinator"
	}
	return labels["striker.role"]
}

// machineOf splits "remote1/t18" into "remote1", "t18"; a local address has machine "".
func machineOf(addr string) (string, string) {
	if m, t, ok := strings.Cut(addr, "/"); ok {
		return m, t
	}
	return "", addr
}

func repoOf(f fleet.Fleet, labels map[string]string, machine, where string) string {
	if r := labels["striker.lead"]; r != "" {
		return r
	}
	if r := labels["striker.repo"]; r != "" {
		return r
	}
	space, _, _ := strings.Cut(where, " / ")
	for _, r := range f.Repos {
		if r.Space == space && r.Machine == machine {
			return r.Name
		}
	}
	return space
}

type kept struct {
	Name      string `json:"name"`
	CreatedMs int64  `json:"created_ms"`
}

func loadStore(path string) map[string]kept {
	m := map[string]kept{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func saveStore(path string, m map[string]kept) {
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}

// readLocal reads the status line files in dir written within the last day.
func readLocal(dir string, now time.Time) []statusline.Entry {
	es, _ := os.ReadDir(dir)
	var out []statusline.Entry
	for _, e := range es {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) > 24*time.Hour {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, statusline.ParseLines(b)...)
	}
	return out
}

// readRemote reads a machine's status line files of the last day with one hc call.
func readRemote(c hc.Client, machine, dir string) []statusline.Entry {
	out, err := c.R.Run("exec", "--on", machine, "find", dir, "-name", "*.json", "-mmin", "-1440", "-exec", "cat", "{}", "+")
	if err != nil && len(out) == 0 {
		return nil
	}
	return statusline.ParseLines(out)
}

// Build lists the crew. storePath keeps the names (crew.json), localAgents is this machine's status line dir.
func Build(c hc.Client, f fleet.Fleet, storePath, localAgents string, now time.Time) Crew {
	crew := Crew{Agents: []Agent{}}
	terms, err := c.List()
	if err != nil {
		return crew
	}
	type member struct {
		t     hc.Terminal
		info  hc.Info
		agent Agent
	}
	var ms []member
	machines := map[string]bool{}
	for _, t := range terms {
		if t.Agent != "claude-code" {
			continue
		}
		info, err := c.Get(t.Address)
		if err != nil {
			continue
		}
		machine, _ := machineOf(t.Address)
		machines[machine] = true
		a := Agent{Address: t.Address, Role: roleOf(info.Labels), Status: t.Status, Repo: repoOf(f, info.Labels, machine, t.Where)}
		if isStriker(machine, t.Where, info.Labels) {
			a.Role, a.Repo = "striker", "striker"
			if r := info.Labels["striker.lead"]; r != "" {
				a.Repo = r // Striker as the lead of his own repo
			}
		}
		ms = append(ms, member{t: t, info: info, agent: a})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].t.Address < ms[j].t.Address })

	// model and tokens, by <machine>/<terminal>
	lines := map[string]statusline.Entry{}
	for m := range machines {
		var es []statusline.Entry
		if m == "" {
			es = readLocal(localAgents, now)
		} else if dir := f.AgentsDir[m]; dir != "" {
			es = readRemote(c, m, dir)
		}
		for _, e := range es {
			key := e.Terminal
			if m != "" {
				key = m + "/" + e.Terminal
			}
			lines[key] = e
		}
	}

	// names: keep those of agents still here (same terminal, same creation), free the rest, then hand out
	store := loadStore(storePath)
	next := map[string]kept{}
	used := map[string]bool{orchestrator: true}
	for _, m := range ms {
		if k, ok := store[m.t.Address]; ok && m.agent.Role != "striker" && k.CreatedMs == m.info.CreatedMs && !used[k.Name] {
			next[m.t.Address] = k
			used[k.Name] = true
		}
	}
	for _, m := range ms {
		if _, ok := next[m.t.Address]; ok {
			continue
		}
		if m.agent.Role == "striker" {
			next[m.t.Address] = kept{Name: orchestrator, CreatedMs: m.info.CreatedMs}
			continue
		}
		n := pick(m.agent.Role == "lead" || m.agent.Role == "coordinator", used)
		used[n] = true
		next[m.t.Address] = kept{Name: n, CreatedMs: m.info.CreatedMs}
	}
	if !sameStore(store, next) {
		saveStore(storePath, next)
	}

	for _, m := range ms {
		a := m.agent
		a.Name = next[a.Address].Name
		if e, ok := lines[a.Address]; ok {
			a.Model, a.Tokens, a.ContextPct = e.Model, e.Tokens, e.ContextPct
			if e.At > 0 {
				a.Seen = time.UnixMilli(e.At).UTC().Format(time.RFC3339)
			}
		}
		a.Rank = rank(a.Role, a.Model)
		crew.Agents = append(crew.Agents, a)
	}
	return crew
}

func sameStore(a, b map[string]kept) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
