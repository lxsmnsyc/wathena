package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
//
// A Core from the cache (LoadCoreCached) has Result, Files, Constants,
// Groups and Monsters. NPCs, MonsterSkills, RandomOptions and Quests
// are nil. waserver does not read them.
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
	// RandomOptions are the converted random options and groups.
	RandomOptions *RandomOptionResult
	// Quests are the converted quests.
	Quests *QuestResult
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
	in, err := readCoreInputs(mode, referenceReader(mode))
	if err != nil {
		return nil, err
	}
	return convertCore(in)
}

// referenceReader reads a mode's extract by name, without .json.
func referenceReader(mode reference.Mode) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		return reference.ReadFile("extracts/" + string(mode) + "/" + name + ".json")
	}
}

// extractNames are the extracts that LoadCore reads, in the order of
// coreInputs.files. npcs.json names more files, the NPC parts.
var extractNames = []string{"item_scripts", "script_constants", "item_groups", "client_tables", "npcs", "monsters", "skills", "random_options", "shops", "quests", "castles"}

// coreInputs are the extracts that LoadCore reads.
type coreInputs struct {
	// files are extractNames' contents, in their order.
	files [11][]byte
	// partNames and parts are the NPC part files that npcs.json lists.
	partNames []string
	parts     [][]byte
}

func readCoreInputs(mode reference.Mode, read func(string) ([]byte, error)) (*coreInputs, error) {
	in := &coreInputs{}
	for i, name := range extractNames {
		data, err := read(name)
		if err != nil {
			return nil, err
		}
		in.files[i] = data
	}
	names, err := npcPartNames(in.files[4])
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		data, err := read(name)
		if err != nil {
			return nil, err
		}
		in.partNames = append(in.partNames, name)
		in.parts = append(in.parts, data)
	}
	return in, nil
}

// convertCore converts the extracts.
func convertCore(in *coreInputs) (*Core, error) {
	res, err := ConvertItems(Input{
		Scripts: bytes.NewReader(in.files[0]), Constants: bytes.NewReader(in.files[1]),
		Groups: bytes.NewReader(in.files[2]), ClientTables: bytes.NewReader(in.files[3]),
	})
	if err != nil {
		return nil, err
	}
	quests, err := ConvertQuests(bytes.NewReader(in.files[9]))
	if err != nil {
		return nil, err
	}
	castles, err := ConvertCastles(bytes.NewReader(in.files[10]))
	if err != nil {
		return nil, err
	}
	parts := make([]io.Reader, len(in.parts))
	for i, p := range in.parts {
		parts[i] = bytes.NewReader(p)
	}
	npcs, err := ConvertNPCs(NPCInput{NPCs: bytes.NewReader(in.files[4]), Parts: parts, Constants: bytes.NewReader(in.files[1]), ClientTables: bytes.NewReader(in.files[3]), Shops: bytes.NewReader(in.files[8]), Quests: quests,
		Groups: bytes.NewReader(in.files[2])})
	if err != nil {
		return nil, err
	}
	mobs, err := ConvertMonsters(MonsterInput{Monsters: bytes.NewReader(in.files[5]), Constants: bytes.NewReader(in.files[1]), Skills: bytes.NewReader(in.files[6])})
	if err != nil {
		return nil, err
	}
	opts, err := res.ConvertRandomOptions(bytes.NewReader(in.files[7]))
	if err != nil {
		return nil, err
	}
	c, err := coreTables(in)
	if err != nil {
		return nil, err
	}
	files := fstest.MapFS{}
	for _, part := range []map[string]string{res.Files(), npcs.Files(), mobs.Files(), opts.Files(), quests.Files(), castles.Files()} {
		for path, content := range part {
			files[path] = &fstest.MapFile{Data: []byte(content), Mode: 0o444}
		}
	}
	c.Result, c.NPCs, c.Files, c.MonsterSkills, c.RandomOptions, c.Quests = res, npcs, files, mobs, opts, quests
	return c, nil
}

// coreTables reads the tables of a Core that are not converted: the
// constants, the item groups and the monster names.
func coreTables(in *coreInputs) (*Core, error) {
	consts, err := constants.Read(bytes.NewReader(in.files[1]))
	if err != nil {
		return nil, err
	}
	var groups item.GroupData
	if err := json.Unmarshal(in.files[2], &groups); err != nil {
		return nil, fmt.Errorf("item_groups.json: %w", err)
	}
	var tables struct {
		Monsters []struct {
			ID     int64  `json:"id"`
			Sprite string `json:"sprite"`
		} `json:"monsters"`
	}
	if err := json.Unmarshal(in.files[3], &tables); err != nil {
		return nil, fmt.Errorf("client_tables.json: %w", err)
	}
	monsters := map[string]int64{}
	for _, m := range tables.Monsters {
		monsters[strings.ToUpper(m.Sprite)] = m.ID
	}
	return &Core{Constants: consts, Groups: &groups, Monsters: monsters}, nil
}

// npcPartNames reads the part files that npcs.json lists: the script
// NPCs of the quests, warps and events folders, which the extract keeps
// apart so that each file stays small. It returns their names without
// .json.
func npcPartNames(npcs []byte) ([]string, error) {
	var head struct {
		Parts []string `json:"parts"`
	}
	if err := json.Unmarshal(npcs, &head); err != nil {
		return nil, fmt.Errorf("npcs.json: %w", err)
	}
	out := make([]string, 0, len(head.Parts))
	for _, name := range head.Parts {
		base, ok := strings.CutSuffix(name, ".json")
		if !ok || !strings.HasPrefix(base, "npcs-") || strings.ContainsAny(base, "/\\") {
			return nil, fmt.Errorf("npcs.json: %q is not a part file", name)
		}
		out = append(out, base)
	}
	return out, nil
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
