package convert

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/castle"
)

// Castles (proposal 0004, section 1): rAthena's castle_db.yml, read from
// the castles.json extract, becomes core's data/castles.yml. A record's
// name is rAthena's castle name, such as Kriemhild; id, map, npc,
// edition, client_id and teleport keep rAthena's values.

// CastleData is the castles.json extract.
type CastleData struct {
	Mode    string        `json:"mode"`
	Castles []castleEntry `json:"castles"`
}

type castleEntry struct {
	ID       uint32          `json:"id"`
	Map      string          `json:"map"`
	Name     string          `json:"name"`
	NPC      string          `json:"npc"`
	Edition  string          `json:"edition"`
	ClientID uint32          `json:"client_id"`
	Teleport *CastleTeleport `json:"teleport"`
}

// CastleTeleport is a record's teleport cell and costs.
type CastleTeleport struct {
	X         int   `json:"x" yaml:"x"`
	Y         int   `json:"y" yaml:"y"`
	Cost      int64 `json:"cost" yaml:"cost"`
	SiegeCost int64 `json:"siege_cost" yaml:"siege_cost"`
}

// CastleRecord is a record of core's data/castles.yml.
type CastleRecord struct {
	Name     string          `yaml:"name"`
	ID       uint32          `yaml:"id"`
	Map      string          `yaml:"map"`
	Edition  string          `yaml:"edition"`
	NPC      string          `yaml:"npc"`
	ClientID uint32          `yaml:"client_id,omitempty"`
	Teleport *CastleTeleport `yaml:"teleport,omitempty,flow"`
}

// CastleResult is the converted castles of a mode.
type CastleResult struct {
	Mode    string
	Records []CastleRecord
}

// ConvertCastles converts a castles.json extract. Two castles with one
// name are an error: names are record names.
func ConvertCastles(r io.Reader) (*CastleResult, error) {
	var d CastleData
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return nil, fmt.Errorf("castles.json: %w", err)
	}
	out := &CastleResult{Mode: d.Mode}
	seen := map[string]bool{}
	for _, c := range d.Castles {
		if seen[c.Name] {
			return nil, fmt.Errorf("castles.json: two castles are named %s", c.Name)
		}
		seen[c.Name] = true
		out.Records = append(out.Records, CastleRecord{Name: c.Name, ID: c.ID, Map: c.Map, Edition: c.Edition, NPC: c.NPC,
			ClientID: c.ClientID, Teleport: c.Teleport})
	}
	return out, nil
}

// Files returns core's data/castles.yml.
func (r *CastleResult) Files() map[string]string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Core's castles, converted from rAthena's castle_db.yml (%s) by tools/convert.\n", r.Mode)
	b.WriteString("# A record's name is the castle's rAthena name.\n")
	if len(r.Records) > 0 {
		data, err := yaml.Marshal(r.Records)
		if err != nil {
			panic(fmt.Errorf("convert: castles: %w", err))
		}
		b.Write(data)
	}
	return map[string]string{castle.File: b.String()}
}
