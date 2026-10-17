package economy

import (
	"fmt"
	"math"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Player is the model player of a base level: what a bot run makes
// and what the simulator fights with.
type Player struct {
	Level, Job, JobLevel int
	// Stats are the raised stats, from 1 each.
	Stats status.Stats
	// Gear is the item IDs worn, and Positions where each is worn (EQP_
	// bits), as the inventory's equip column keeps them. Refine is the
	// gear's refine level, and Refines each piece's: 0 for one that
	// cannot be refined.
	Gear      []uint32
	Positions []uint32
	Refine    int
	Refines   []int
	// Potion is the potion's item ID, PotionHP its itemheal range, and
	// PotionPrice what a dealer sells it for.
	Potion      uint32
	PotionHP    [2]int
	PotionPrice int
}

// Model is a model player with its battle status.
type Model struct {
	Player
	Status battle.Status
	// Heal is the HP one potion heals on average, with the VIT bonus of
	// pc_itemheal.
	Heal int
}

// PlayerAt makes the model player of a level from the config: the job
// and job level, the stats bought with the level's status points, and
// the gear and potion of the level.
func (c *Config) PlayerAt(level int, items *item.Table, jobs *status.Tables, prog *progression.Tables) (Player, error) {
	if level < 1 {
		return Player{}, fmt.Errorf("economy: level %d", level)
	}
	pc := c.Player
	js := pc.Jobs[stepAt(level, func(i int) int { return pc.Jobs[i].From }, len(pc.Jobs))]
	p := Player{Level: level, Job: js.Job, JobLevel: max(1, min(level-js.From+1, js.MaxJobLevel))}
	p.Stats = status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}
	ch := status.Character{Job: p.Job, Male: true, BaseLevel: level, JobLevel: p.JobLevel, Stats: p.Stats}
	points := prog.StatusPoints(level)
	type stat struct {
		st     status.Stat
		weight int
		value  *int
	}
	b := pc.Build
	list := []stat{
		{status.Str, b.Str, &ch.Stats.Str}, {status.Agi, b.Agi, &ch.Stats.Agi},
		{status.Vit, b.Vit, &ch.Stats.Vit}, {status.Dex, b.Dex, &ch.Stats.Dex},
	}
	for {
		// The stat with the lowest value per weight that the points
		// can still raise, the first in the list on a tie.
		var best *stat
		for i := range list {
			s := &list[i]
			if s.weight == 0 {
				continue
			}
			cost := jobs.NeedStatusPoint(&ch, s.st, 1)
			if cost == 0 || cost > points {
				continue
			}
			if best == nil || *s.value*best.weight < *best.value*s.weight {
				best = s
			}
		}
		if best == nil {
			break
		}
		points -= jobs.NeedStatusPoint(&ch, best.st, 1)
		*best.value++
	}
	p.Stats = ch.Stats
	gs := pc.Gear[stepAt(level, func(i int) int { return pc.Gear[i].From }, len(pc.Gear))]
	p.Refine = gs.Refine
	for _, name := range gs.Items {
		d, ok := items.ByName(name)
		if !ok {
			return Player{}, fmt.Errorf("economy: no item %s for level %d", name, level)
		}
		p.Gear = append(p.Gear, d.ID)
	}
	ps := pc.Potions[stepAt(level, func(i int) int { return pc.Potions[i].From }, len(pc.Potions))]
	d, ok := items.ByName(ps.Item)
	if !ok {
		return Player{}, fmt.Errorf("economy: no item %s", ps.Item)
	}
	p.Potion, p.PotionHP, p.PotionPrice = d.ID, ps.HP, d.Buy
	return p, nil
}

// wear puts the gear on and returns the battle status. Each item must
// go on: a job or a level that cannot wear one is an error of the
// config.
func (p *Player) wear(items *item.Table, jobs *status.Tables) (battle.Status, error) {
	inv := item.NewInventory()
	w := &item.Wearer{Job: p.Job, Male: true, BaseLevel: p.Level, Renewal: jobs.Renewal()}
	w.MapID, _ = jobs.MapID(p.Job)
	if m, ok := jobs.EquipMapID(p.Job); ok && m != w.MapID {
		w.EquipsAs, w.EquipMapID = true, m
	}
	p.Positions = make([]uint32, len(p.Gear))
	p.Refines = make([]int, len(p.Gear))
	for i, id := range p.Gear {
		d := items.Get(id)
		slot, r := inv.Add(d, 1, math.MaxInt)
		if r != item.AddSuccess {
			return battle.Status{}, fmt.Errorf("economy: %s does not fit", d.AegisName)
		}
		if _, res := inv.Equip(items, slot, uint32(d.Location), w); res != item.EquipOK {
			return battle.Status{}, fmt.Errorf("economy: job %d at level %d cannot wear %s", p.Job, p.Level, d.AegisName)
		}
		p.Positions[i] = inv.Slots[slot].Equip
		if d.Refineable {
			inv.Slots[slot].Refine = p.Refine
			p.Refines[i] = p.Refine
		}
	}
	src := func(_, _ int, d *item.Def) []status.Bonus { return status.StaticBonuses(d, nil) }
	eq := status.WearOptions(inv, items, src, nil)
	res, err := jobs.Calc(status.Character{Job: p.Job, Male: true, BaseLevel: p.Level, JobLevel: p.JobLevel,
		Stats: p.Stats, Equipment: eq})
	if err != nil {
		return battle.Status{}, err
	}
	return res.Battle, nil
}

// avgHeal is the potion's average heal for the player's VIT.
func (p *Player) avgHeal(vit int) int {
	hp, _ := item.ItemHeal((p.PotionHP[0]+p.PotionHP[1])/2, 0, vit, 1, nil)
	return max(hp, 1)
}
