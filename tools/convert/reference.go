package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
	"testing/fstest"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/item"
)

// Core is what the server needs for core's converted scripts in one
// mode: the converted files, and the tables their names refer to.
type Core struct {
	Result *Result
	// NPCs are the converted NPCs and shops.
	NPCs *NPCResult
	// Files are the converted files, as core's files for the mod loader.
	Files fs.FS
	// Constants are the script constants.
	Constants *constants.Data
	// Groups are the item groups.
	Groups *item.GroupData
	// Monsters maps monster AegisNames, in upper case, to IDs.
	Monsters map[string]int64
	// MonsterSkills are the converted monster skills.
	MonsterSkills *MonsterResult
}

// Reference converts the item scripts of a mode's extracts.
func Reference(mode reference.Mode) (*Result, error) {
	c, err := LoadCore(mode)
	if err != nil {
		return nil, err
	}
	return c.Result, nil
}

// LoadCore converts the item scripts and the NPCs of a mode's extracts
// in reference/rathena, read through internal/reference, so waserver can
// use the extracts built into it.
func LoadCore(mode reference.Mode) (*Core, error) {
	read := func(name string) ([]byte, error) {
		return reference.ReadFile("extracts/" + string(mode) + "/" + name + ".json")
	}
	var in [7][]byte
	for i, name := range []string{"item_scripts", "script_constants", "item_groups", "client_tables", "npcs", "monsters", "skills"} {
		data, err := read(name)
		if err != nil {
			return nil, err
		}
		in[i] = data
	}
	res, err := ConvertItems(Input{
		Scripts: bytes.NewReader(in[0]), Constants: bytes.NewReader(in[1]),
		Groups: bytes.NewReader(in[2]), ClientTables: bytes.NewReader(in[3]),
	})
	if err != nil {
		return nil, err
	}
	npcs, err := ConvertNPCs(NPCInput{NPCs: bytes.NewReader(in[4]), Constants: bytes.NewReader(in[1]), ClientTables: bytes.NewReader(in[3])})
	if err != nil {
		return nil, err
	}
	mobs, err := ConvertMonsters(MonsterInput{Monsters: bytes.NewReader(in[5]), Constants: bytes.NewReader(in[1]), Skills: bytes.NewReader(in[6])})
	if err != nil {
		return nil, err
	}
	consts, err := constants.Read(bytes.NewReader(in[1]))
	if err != nil {
		return nil, err
	}
	var groups item.GroupData
	if err := json.Unmarshal(in[2], &groups); err != nil {
		return nil, fmt.Errorf("item_groups.json: %w", err)
	}
	var tables struct {
		Monsters []struct {
			ID     int64  `json:"id"`
			Sprite string `json:"sprite"`
		} `json:"monsters"`
	}
	if err := json.Unmarshal(in[3], &tables); err != nil {
		return nil, fmt.Errorf("client_tables.json: %w", err)
	}
	monsters := map[string]int64{}
	for _, m := range tables.Monsters {
		monsters[strings.ToUpper(m.Sprite)] = m.ID
	}
	files := fstest.MapFS{}
	for _, part := range []map[string]string{res.Files(), npcs.Files(), mobs.Files()} {
		for path, content := range part {
			files[path] = &fstest.MapFile{Data: []byte(content), Mode: 0o444}
		}
	}
	return &Core{Result: res, NPCs: npcs, Files: files, Constants: consts, Groups: &groups, Monsters: monsters, MonsterSkills: mobs}, nil
}

// FS returns the generated files as a read-only file system.
func (r *Result) FS() fs.FS {
	files := fstest.MapFS{}
	for path, content := range r.Files() {
		files[path] = &fstest.MapFile{Data: []byte(content), Mode: 0o444}
	}
	return files
}

// Names are the names that item data refers to, for checking static
// bonuses. items maps AegisNames to item IDs.
func (c *Core) Names(items map[string]uint32) *bonus.Tables {
	t := &bonus.Tables{Constants: map[string]map[string]int64{}, Skills: c.Constants.Skills,
		Items: map[string]int64{}, Groups: map[string]int64{}, Monsters: map[string]int64{}}
	for name, m := range c.Constants.Modules() {
		t.Constants[name] = m.Values
	}
	for name, id := range items {
		t.Items[name] = int64(id)
	}
	for _, g := range c.Groups.Groups {
		t.Groups[g.Name] = g.ID
	}
	for name, id := range c.Monsters {
		t.Monsters[name] = id
	}
	return t
}
