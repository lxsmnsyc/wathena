// Package reference loads the data recorded from rAthena in reference/rathena.
// Parity tests use it to compare wAthena formulas against rAthena.
package reference

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Mode is the rAthena build mode the data was recorded in.
type Mode string

const (
	Renewal    Mode = "renewal"
	PreRenewal Mode = "pre-renewal"
)

// Modes lists every recorded mode.
var Modes = []Mode{Renewal, PreRenewal}

// DataPath returns the rAthena database folder used by the mode.
func (m Mode) DataPath() string {
	if m == Renewal {
		return "re/"
	}
	return "pre-re/"
}

// Systems lists the formula files recorded for each mode.
var Systems = []string{
	"pathfinding",
	"distance",
	"elements",
	"mob_status",
	"mob_damage",
	"pc_status",
	"pc_damage",
	"skill_damage",
	"item_bonuses",
	"refine",
	"status_effects",
	"level_penalty",
}

// Header is the start of every formula file.
type Header struct {
	System         string `json:"system"`
	Mode           Mode   `json:"mode"`
	RathenaGitHash string `json:"rathena_git_hash"`
	DataPath       string `json:"data_path"`
	CircularArea   bool   `json:"circular_area"`
}

// Elements is elements.json.
type Elements struct {
	Header
	Levels []struct {
		Level int `json:"level"`
		// Table is indexed by attack element, then defense element, in percent.
		Table [][]int `json:"table"`
	} `json:"levels"`
}

// Root returns the reference/rathena folder.
// It searches upward from the working directory for go.mod.
func Root() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "reference", "rathena"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("reference: go.mod not found above the working directory")
		}
		dir = parent
	}
}

// LoadFile decodes the JSON file at a path relative to Root into v.
func LoadFile(rel string, v any) error {
	root, err := Root()
	if err != nil {
		return err
	}
	// os.Root keeps reads inside the reference folder.
	dir, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	data, err := dir.ReadFile(filepath.FromSlash(rel))
	if cerr := dir.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("reference: %s: %w", rel, err)
	}
	return nil
}

// Load decodes the formula file for a mode and system into v.
func Load(mode Mode, system string, v any) error {
	return LoadFile(string(mode)+"/"+system+".json", v)
}
