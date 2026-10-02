// Package gamedata loads the game data that waserver and the mod tools
// share: the reference extracts of rAthena's databases, the job tables,
// the monsters, and the mods merged over core.
package gamedata

import (
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// LoadProgression reads the exp tables, level penalty, monster drops and
// item types that reference/rathena extracted from rAthena's database.
func LoadProgression(mode reference.Mode) (*progression.Tables, *progression.Data, error) {
	var d progression.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/progression.json", &d); err != nil {
		return nil, nil, err
	}
	var client struct {
		Items []struct {
			ID   uint32 `json:"id"`
			Type string `json:"type"`
		} `json:"items"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/client_tables.json", &client); err != nil {
		return nil, nil, err
	}
	items := make(map[uint32]string, len(client.Items))
	for _, it := range client.Items {
		items[it.ID] = it.Type
	}
	return progression.New(&d, items, progression.DefaultConfig), &d, nil
}

// LoadItemData reads the item database that reference/rathena
// extracted from rAthena's item_db: items.json, with the names of
// client_tables.json and the prices of item_prices.json. Mods' item
// overlays apply over it.
func LoadItemData(mode reference.Mode) (*item.Data, error) {
	var d item.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/items.json", &d); err != nil {
		return nil, err
	}
	var names struct {
		Items []struct {
			ID   uint32 `json:"id"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/client_tables.json", &names); err != nil {
		return nil, err
	}
	var prices struct {
		Items []struct {
			ID     uint32 `json:"id"`
			Buy    int    `json:"buy"`
			Sell   int    `json:"sell"`
			NoSell bool   `json:"no_sell"`
		} `json:"items"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/item_prices.json", &prices); err != nil {
		return nil, err
	}
	byID := make(map[uint32]*item.Def, len(d.Items))
	for i := range d.Items {
		byID[d.Items[i].ID] = &d.Items[i]
	}
	for _, n := range names.Items {
		if def := byID[n.ID]; def != nil {
			def.Name = n.Name
		}
	}
	for _, p := range prices.Items {
		if def := byID[p.ID]; def != nil {
			def.Buy, def.Sell, def.NoSell = p.Buy, p.Sell, p.NoSell
		}
	}
	return &d, nil
}

// Monster is one monster's battle status and, with the exp tables, its
// level, exp and drops.
type Monster struct {
	Status *battle.Status
	// Reward is nil without the exp tables.
	Reward *progression.Mob
}

// LoadMonsters reads every monster's stats that reference/rathena
// recorded from rAthena, by monster ID. With tables, monsters also get
// their DamageTaken, exp and drops.
func LoadMonsters(mode reference.Mode, tables *progression.Tables, data *progression.Data) (map[uint32]Monster, error) {
	var mobs struct {
		Mobs []struct {
			ID      uint32        `json:"id"`
			Level   int           `json:"level"`
			BaseExp int64         `json:"base_exp"`
			JobExp  int64         `json:"job_exp"`
			Status  battle.Status `json:"status"`
		} `json:"mobs"`
	}
	if err := reference.Load(mode, "mob_status", &mobs); err != nil {
		return nil, err
	}
	extra := map[uint32]*progression.MobData{}
	if data != nil {
		for i := range data.Mobs {
			extra[data.Mobs[i].ID] = &data.Mobs[i]
		}
	}
	out := make(map[uint32]Monster, len(mobs.Mobs))
	for _, m := range mobs.Mobs {
		st := m.Status
		st.Level = m.Level
		x := extra[m.ID]
		if tables == nil || x == nil {
			out[m.ID] = Monster{Status: &st}
			continue
		}
		st.DamageTaken = x.DamageTaken
		// mob_status.json recorded the exp at the default rates of 100%.
		r := &progression.Mob{
			ID: m.ID, Level: m.Level, MaxHP: st.MaxHP,
			BaseExp: progression.MobExp(m.BaseExp, tables.BaseExpRate),
			JobExp:  progression.MobExp(m.JobExp, tables.JobExpRate),
			Mode:    st.Mode, Class: st.Class,
		}
		r.Drops = tables.AdjustDrops(r.Boss(), x.Drops)
		out[m.ID] = Monster{Status: &st, Reward: r}
	}
	return out, nil
}

// Spawn is a spawn of one monster with its stats: Amount is 1, and the
// area is the one cell.
func (m Monster) Spawn(id uint32, name string, x, y int) zone.Spawn {
	return zone.Spawn{
		Mob: id, Name: name, Amount: 1, X: x, Y: y,
		// Mode bit 1 is MD_CANMOVE.
		Speed: m.Status.Speed, CanMove: m.Status.Mode&1 != 0,
		Status: m.Status, Reward: m.Reward,
	}
}

// LoadSpawns reads every map's monsters, by map name such as prontera,
// from the spawns and monster stats that reference/rathena recorded from
// rAthena, until the core mod has its own. With tables, monsters also get
// their DamageTaken, exp and drops.
func LoadSpawns(mode reference.Mode, tables *progression.Tables, data *progression.Data) (map[string][]zone.Spawn, error) {
	if mode == "none" {
		return nil, nil
	}
	var spawns struct {
		Maps []struct {
			Map    string `json:"map"`
			Spawns []struct {
				Mob       uint32 `json:"mob"`
				Name      string `json:"name"`
				Amount    int    `json:"amount"`
				Area      [4]int `json:"area"`
				RespawnMs int    `json:"respawn_ms"`
			} `json:"spawns"`
		} `json:"maps"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/spawns.json", &spawns); err != nil {
		return nil, err
	}
	monsters, err := LoadMonsters(mode, tables, data)
	if err != nil {
		return nil, err
	}
	out := map[string][]zone.Spawn{}
	for _, m := range spawns.Maps {
		for _, s := range m.Spawns {
			mob, ok := monsters[s.Mob]
			if !ok {
				continue
			}
			sp := mob.Spawn(s.Mob, s.Name, s.Area[0], s.Area[1])
			// rAthena searches up to xs - 1 cells from the center.
			sp.Amount, sp.W, sp.H, sp.RespawnMs = s.Amount, max(s.Area[2]-1, 0), max(s.Area[3]-1, 0), s.RespawnMs
			out[m.Map] = append(out[m.Map], sp)
		}
	}
	return out, nil
}

// Builds are the raised stats of the recorder's PC_SAMPLE_BUILDS in the
// rAthena fork's wathena_recorder.cpp: STR, AGI, VIT, INT, DEX and LUK
// before job bonuses.
var Builds = map[string]status.Stats{
	"all_1":    {Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1},
	"melee":    {Str: 90, Agi: 50, Vit: 30, Int: 1, Dex: 40, Luk: 1},
	"critical": {Str: 70, Agi: 90, Vit: 1, Int: 1, Dex: 40, Luk: 50},
	"magic":    {Str: 1, Agi: 1, Vit: 30, Int: 99, Dex: 80, Luk: 1},
	"ranged":   {Str: 30, Agi: 40, Vit: 1, Int: 1, Dex: 99, Luk: 30},
	"tank":     {Str: 60, Agi: 1, Vit: 99, Int: 1, Dex: 30, Luk: 1},
}

// StartCharacter loads the mode's job tables and makes the character
// every new character starts as: a novice with no equipment and no
// skills, and the raised stats of a build. The sex comes from the
// session's look. The
// default, all_1, is a new character as rAthena makes one for recent
// clients: every stat at 1 and the level 1 status points to spend. The
// other builds keep the same status points, for testing.
func StartCharacter(mode reference.Mode, build string) (*status.Tables, status.Character, error) {
	stats, ok := Builds[build]
	if !ok {
		return nil, status.Character{}, fmt.Errorf("unknown build %q", build)
	}
	var d status.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &d); err != nil {
		return nil, status.Character{}, err
	}
	tables, err := status.New(&d)
	if err != nil {
		return nil, status.Character{}, err
	}
	c := status.Character{Job: 0, Male: true, BaseLevel: 1, JobLevel: 1, Stats: stats}
	// Check that the tables have the job.
	if _, err := tables.Calc(c); err != nil {
		return nil, status.Character{}, err
	}
	return tables, c, nil
}
