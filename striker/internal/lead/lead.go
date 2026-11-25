// Package lead starts and messages repo leads.
package lead

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ploenk42/striker/striker/internal/fleet"
	"github.com/ploenk42/striker/striker/internal/hc"
)

const prefix = "[Striker] "

// Message prefixes text with "[Striker] " unless present and trims trailing whitespace.
func Message(text string) string {
	text = strings.TrimRight(text, " \t\r\n")
	if !strings.HasPrefix(text, prefix) {
		text = prefix + text
	}
	return text
}

// How a lead starts coordinators ({coordinators} in the brief): a direct
// lead calls striker start-coordinator, a relayed one (a fleet repo with a
// user, no hc socket) asks Striker through requests in its status file.
var (
	//go:embed coordinators-direct.md
	coordinatorsDirect string
	//go:embed coordinators-relayed.md
	coordinatorsRelayed string
)

// FillBrief fills the lead brief placeholders; relayed picks the
// {coordinators} variant.
func FillBrief(tpl, repo, path, machine, statusFile string, relayed bool) string {
	coordinators := coordinatorsDirect
	if relayed {
		coordinators = coordinatorsRelayed
	}
	tpl = strings.ReplaceAll(tpl, "{coordinators}", strings.TrimRight(coordinators, " \t\r\n"))
	return strings.NewReplacer("{repo}", repo, "{path}", path, "{machine}", machine, "{status_file}", statusFile).Replace(tpl)
}

// NotReadyError: the lead started but is blocked/working; the brief was not sent.
type NotReadyError struct {
	Address string
	State   string
}

func (e *NotReadyError) Error() string {
	return fmt.Sprintf("Der Lead ist noch nicht bereit (Status: %s). Bildschirm prüfen: hc read %s --lines 30; danach die Einweisung schicken.", e.State, e.Address)
}

// Start makes a lead's space and briefs it. run executes herdr-app commands.
func Start(c hc.Client, run func(args ...string) ([]byte, error), f fleet.Fleet, repo, briefPath string) (string, error) {
	var r *fleet.Repo
	for i := range f.Repos {
		if f.Repos[i].Name == repo {
			r = &f.Repos[i]
			break
		}
	}
	if r == nil {
		return "", fmt.Errorf("Repo '%s' steht nicht in fleet.json.", repo)
	}

	existing, err := c.ListLabel("striker.lead", repo)
	if err != nil {
		return "", fmt.Errorf("kann nicht prüfen, ob %s schon einen Lead hat: %w", repo, err)
	}
	if len(existing) > 0 {
		return "", fmt.Errorf("Repo '%s' hat schon einen Lead: %s", repo, existing[0].Address)
	}

	argvJSON, _ := json.Marshal(fleet.ClaudeArgv(r.User, r.Path, "lead "+repo))

	out, err := run("do", "space.new", "--name", r.Space, "--machine", r.Machine, "--tab", "claude", "--argv", string(argvJSON))
	if err != nil {
		return "", fmt.Errorf("space.new scheiterte: %s%v", out, err)
	}
	var made struct {
		Terminal string `json:"terminal"`
	}
	if err := json.Unmarshal(out, &made); err != nil || made.Terminal == "" {
		return "", fmt.Errorf("space.new gab kein Terminal zurück: %s", out)
	}
	addr := made.Terminal
	fmt.Fprintf(os.Stderr, "Space '%s' auf %s angelegt, Lead ist %s.\n", r.Space, r.Machine, addr)

	if err := c.Label(addr, "striker.lead="+repo); err != nil {
		return addr, fmt.Errorf("Terminal %s läuft, hat aber kein Label striker.lead=%s (von Hand: hc label %s striker.lead=%s): %w", addr, repo, addr, repo, err)
	}
	_ = c.Wait(addr, 90) // a timeout is judged by the state below
	info, err := c.Get(addr)
	if err != nil {
		return addr, err
	}
	if info.Status != nil && (info.Status.State == "blocked" || info.Status.State == "working") {
		return addr, &NotReadyError{Address: addr, State: info.Status.State}
	}

	tpl, err := os.ReadFile(briefPath)
	if err != nil {
		return addr, err
	}
	statusFile := f.StatusDir[r.Machine] + "/" + repo + ".json"
	brief := FillBrief(string(tpl), repo, r.Path, r.Machine, statusFile, r.User != "")
	if err := c.Send(addr, strings.TrimRight(brief, " \t\r\n")); err != nil {
		return addr, fmt.Errorf("Einweisung nicht zugestellt: %w", err)
	}
	return addr, nil
}
