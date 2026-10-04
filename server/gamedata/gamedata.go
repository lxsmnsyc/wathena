// Package gamedata loads the game data that waserver and the mod tools
// share: the reference extracts of rAthena's databases, the job tables,
// the monsters, and the mods merged over core.
package gamedata

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mob"
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
// extracted from rAthena's item_db: items.json, with the equipment data
// of equipment.json, the names of client_tables.json and the prices of
// item_prices.json. Mods' item overlays apply over it.
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
	var trade struct {
		Items []struct {
			ID           uint32   `json:"id"`
			Restrictions []string `json:"restrictions"`
		} `json:"items"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/item_trade.json", &trade); err != nil {
		return nil, err
	}
	// shops.json lists the items of rAthena's BuyingStore flag (decision
	// 0024).
	var shops struct {
		BuyingStore []struct {
			ID uint32 `json:"id"`
		} `json:"buying_store"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/shops.json", &shops); err != nil {
		return nil, err
	}
	var equip item.EquipData
	if err := reference.LoadFile("extracts/"+string(mode)+"/equipment.json", &equip); err != nil {
		return nil, err
	}
	d.MergeEquipment(&equip)
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
	for _, b := range shops.BuyingStore {
		if def := byID[b.ID]; def != nil {
			def.BuyingStore = true
		}
	}
	// item_trade's flags become restrictions. trade_partner and
	// no_auction have no use without marriage and auctions.
	known := item.RestrictionNames()
	for _, t := range trade.Items {
		def := byID[t.ID]
		if def == nil {
			continue
		}
		for _, r := range t.Restrictions {
			if slices.Contains(known, r) {
				def.Restrictions = append(def.Restrictions, r)
			}
		}
	}
	for i := range d.Items {
		d.Items[i].SyncRestrictions(false)
	}
	return &d, nil
}

// Monster is one monster's data: its base stats, AI and skills, its
// battle status computed from them, and, with the exp tables, its level,
// exp and drops.
type Monster struct {
	// Def is the monster's mob_db data. Overlays change it, and Status
	// is computed from it.
	Def    *mob.Def
	Status *battle.Status
	// Reward is nil without the exp tables.
	Reward *progression.Mob
	// AegisName is the monster's database name, such as PORING, or a mod
	// monster's full name. Name is the name players see.
	AegisName, Name string
	// Sprite is the monster ID whose sprite players see, or 0 for its
	// own.
	Sprite uint32
	// Skills are the monster's skills (mob_skill_db), from core's and the
	// mods' data/monsters.yml.
	Skills []mob.Skill
	// Script is the AI entry script, as mod:path, or "".
	Script string
	// AI is the AI type number of mob_db's Ai, and modesOn and modesOff
	// the Modes that mob_db sets and clears over the AI type's modes.
	// renewal picks the stat formulas.
	modesOn, modesOff []uint32
	renewal           bool
	// mvpDrops are the drops before the drop rate settings.
	drops, mvpDrops []progression.Drop
}

// LoadMonsters reads every monster of monsters.json, the mob_db that
// reference/rathena extracted, by monster ID, and computes its battle
// status as rAthena's status_calc_mob. With tables, monsters also get
// their exp and drops.
func LoadMonsters(mode reference.Mode, tables *progression.Tables, data *progression.Data) (map[uint32]Monster, error) {
	defs, err := mob.Load(mode)
	if err != nil {
		return nil, err
	}
	var client struct {
		Monsters []struct {
			ID   uint32 `json:"id"`
			Name string `json:"name"`
		} `json:"monsters"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/client_tables.json", &client); err != nil {
		return nil, err
	}
	var groups struct {
		Monsters []struct {
			ID     uint32 `json:"id"`
			Groups []int  `json:"groups"`
		} `json:"monsters"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/monster_groups.json", &groups); err != nil {
		return nil, err
	}
	race2 := make(map[uint32][]int, len(groups.Monsters))
	for _, m := range groups.Monsters {
		race2[m.ID] = m.Groups
	}
	names := make(map[uint32]string, len(client.Monsters))
	for _, m := range client.Monsters {
		names[m.ID] = m.Name
	}
	extra := map[uint32]*progression.MobData{}
	if data != nil {
		for i := range data.Mobs {
			extra[data.Mobs[i].ID] = &data.Mobs[i]
		}
		if err := setOptionGroups(mode, extra); err != nil {
			return nil, err
		}
	}
	renewal := mode == reference.Renewal
	out := make(map[uint32]Monster, len(defs.Monsters))
	for i := range defs.Monsters {
		d := defs.Monsters[i]
		d.Skills = nil
		ai, ok := mob.AIModes(d.AI)
		if !ok {
			return nil, fmt.Errorf("monster %d: unknown AI %q", d.ID, d.AI)
		}
		if got := mob.Modes(ai, d.ModesOn, d.ModesOff, d.Class); got != d.Mode {
			return nil, fmt.Errorf("monster %d: modes %#x, want %#x", d.ID, got, d.Mode)
		}
		m := Monster{Def: &d, AegisName: d.AegisName, Name: cmp.Or(names[d.ID], d.Name),
			modesOn: d.ModesOn, modesOff: d.ModesOff, renewal: renewal}
		if x := extra[d.ID]; x != nil {
			m.drops, m.mvpDrops = x.Drops, x.MvpDrops
		}
		st := mob.Calc(&d, renewal)
		st.Race2 = race2[d.ID]
		m.Status = &st
		if tables != nil && extra[d.ID] != nil {
			m.Reward = &progression.Mob{}
			m.fillReward(tables)
		}
		out[d.ID] = m
	}
	return out, nil
}

// setOptionGroups sets the random option group of each drop that has
// one in mob_db, from random_options.json. The drops are copied first, so
// the progression data is not changed.
func setOptionGroups(mode reference.Mode, mobs map[uint32]*progression.MobData) error {
	var d struct {
		Drops []struct {
			Monster uint32 `json:"monster"`
			MVP     bool   `json:"mvp"`
			Index   int    `json:"index"`
			Item    uint32 `json:"item"`
			Group   string `json:"group"`
		} `json:"drops"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/random_options.json", &d); err != nil {
		return err
	}
	copied := map[uint32]bool{}
	for _, x := range d.Drops {
		m := mobs[x.Monster]
		if m == nil {
			continue
		}
		if !copied[x.Monster] {
			c := *m
			c.Drops, c.MvpDrops = slices.Clone(m.Drops), slices.Clone(m.MvpDrops)
			mobs[x.Monster], m = &c, &c
			copied[x.Monster] = true
		}
		list := m.Drops
		if x.MVP {
			list = m.MvpDrops
		}
		if x.Index < 0 || x.Index >= len(list) || list[x.Index].Item != x.Item {
			return fmt.Errorf("random_options.json: monster %d has no drop of item %d at %d", x.Monster, x.Item, x.Index)
		}
		list[x.Index].RandomOptionGroup = x.Group
	}
	return nil
}

// fillReward sets the reward from the monster's data, with the exp rates
// and drop settings of tables. mob_db's exp is at the default rates of
// 100%.
func (m *Monster) fillReward(tables *progression.Tables) {
	d, r := m.Def, m.Reward
	r.ID, r.Level, r.MaxHP, r.Mode, r.Class = d.ID, d.Level, m.Status.MaxHP, m.Status.Mode, d.Class
	r.BaseExp = progression.MobExp(d.BaseExp, tables.BaseExpRate)
	r.JobExp = progression.MobExp(d.JobExp, tables.JobExpRate)
	r.MvpExp = progression.MobExp(d.MvpExp, tables.MvpExpRate)
	r.Drops = tables.AdjustDrops(r.Boss(), m.drops)
	r.MvpDrops = slices.Clone(m.mvpDrops)
}

// Spawn is a spawn of one monster with its stats: Amount is 1, and the
// area is the one cell.
func (m Monster) Spawn(id uint32, name string, x, y int) zone.Spawn {
	s := zone.Spawn{
		Mob: id, Name: name, Amount: 1, X: x, Y: y,
		Speed: m.Status.Speed, CanMove: m.Status.Mode&mob.ModeCanMove != 0,
		Status: m.Status, Reward: m.Reward, Sprite: m.Sprite,
		Skills: m.Skills, Script: m.Script,
	}
	if m.Def != nil {
		s.ViewRange, s.ChaseRange = m.Def.SkillRange, m.Def.ChaseRange
	}
	return s
}

// LoadSpawns reads every map's monsters, by map name such as prontera,
// from the spawns and monster stats that reference/rathena recorded from
// rAthena, until the core mod has its own. With tables, monsters also get
// their DamageTaken, exp and drops.
func LoadSpawns(mode reference.Mode, tables *progression.Tables, data *progression.Data) (map[string][]zone.Spawn, error) {
	if mode == "none" {
		return nil, nil
	}
	monsters, err := LoadMonsters(mode, tables, data)
	if err != nil {
		return nil, err
	}
	return SpawnsOf(mode, monsters)
}

// SpawnsOf reads every map's monsters, by map name, from the spawns
// that reference/rathena recorded from rAthena, with the stats of
// monsters, such as the monsters after the mods' overlays.
func SpawnsOf(mode reference.Mode, monsters map[uint32]Monster) (map[string][]zone.Spawn, error) {
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

// StartCharacter loads the mode's job and refine tables and makes the
// character every new character starts as: a novice with no equipment
// and no skills, and the raised stats of a build. The sex comes from the
// session's look. The default, all_1, is a new character as rAthena
// makes one for recent clients: every stat at 1 and the level 1 status
// points to spend. The other builds keep the same status points, for
// testing.
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
	var refine status.RefineData
	if err := reference.LoadFile("extracts/"+string(mode)+"/refine_db.json", &refine); err != nil {
		return nil, status.Character{}, err
	}
	if err := tables.SetRefine(&refine); err != nil {
		return nil, status.Character{}, err
	}
	c := status.Character{Job: 0, Male: true, BaseLevel: 1, JobLevel: 1, Stats: stats}
	// Check that the tables have the job.
	if _, err := tables.Calc(c); err != nil {
		return nil, status.Character{}, err
	}
	return tables, c, nil
}
