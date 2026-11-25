// Package home locates the Striker directory (fleet.json, briefs).
package home

import (
	"os"
	"path/filepath"
)

// Dir returns STRIKER_HOME or <user home>/striker/striker.
func Dir() string {
	if d := os.Getenv("STRIKER_HOME"); d != "" {
		return d
	}
	h, err := os.UserHomeDir()
	if err != nil {
		h = "."
	}
	return filepath.Join(h, "striker", "striker")
}
