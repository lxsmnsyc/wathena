package modscript

import (
	"errors"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Core's procedural NPCs (decision 0044): the quest boards, the job
// guilds, the refiner and the inn of procedural worlds are core's own
// scripts. They reach what the API has no call for through wa.internal,
// which only core may require. It is not API: it may change without
// notice.

// CoreQuest is a world quest as core's scripts read it: a quest board's
// quest or a step of a job guild's test, from the world's content.
type CoreQuest struct {
	ID int64
	// Town is the map ID of the town that offers it. Purpose is board
	// or guild. Guild, Stage and Step name a guild test's step.
	Town, Purpose, Guild, Stage string
	Step                        int
	// Kind is hunt, collect or delivery.
	Kind, Title string
	// Monster and Item are IDs, 0 for none. Count is the kills or items
	// it asks for. To is a delivery's town.
	Monster, Item int64
	Count         int
	To            string
	// BaseExp, JobExp and Zeny are the reward.
	BaseExp, JobExp, Zeny int64
}

// RefineInfo is what the refiner reads of an equipped item, as rAthena's
// getequiprefinecost and getequippercentrefinery for REFINE_COST_NORMAL.
type RefineInfo struct {
	// Item is the item ID and Refine its refine level.
	Item   int64
	Refine int
	// Refineable is false for an item rAthena never refines.
	Refineable bool
	// Max is set at the refiner's highest level.
	Max bool
	// Chance is the success chance in percent, Price the zeny and
	// Material the ore's item ID.
	Chance   int
	Price    int64
	Material int64
}

// CoreWorldHost is what core's procedural NPCs use through wa.internal.
type CoreWorldHost interface {
	// WorldQuests lists the world's quests, by ID.
	WorldQuests() []CoreQuest
	// GiveExp gives a player base and job exp, as a quest reward.
	GiveExp(player, base, job int64) error
	// SkillPoints is the player's unused skill points, rAthena's
	// SkillPoint.
	SkillPoints(player int64) (int, error)
	// Weight is the weight the player carries, rAthena's Weight.
	Weight(player int64) (int, error)
	// RefineInfo reads the item in an equipment slot for the refiner.
	// ok is false for an empty slot.
	RefineInfo(player int64, slot string) (RefineInfo, bool, error)
	// Refine pays the price and the ore and refines the item in a slot,
	// as rAthena's refinemain: it returns "success", "failure" (the
	// item breaks), or why nothing happened: "empty", "not_refineable",
	// "max", "no_material" or "no_zeny".
	Refine(player int64, slot string) (string, error)
}

// coreWorldHost checks that the core mod runs, and returns its host.
func (r *Runtime) coreWorldHost(what string) (CoreWorldHost, error) {
	if r.actor() != "core" {
		return nil, errors.New(what + " is for core's scripts only")
	}
	h, ok := r.host.(CoreWorldHost)
	if !ok {
		return nil, errors.New(what + ": this zone has no procedural content")
	}
	return h, nil
}

func (r *Runtime) coreWorldQuests(c *script.Call) error {
	h, err := r.coreWorldHost("world_quests")
	if err != nil {
		return err
	}
	list := h.WorldQuests()
	out := make([]script.Value, len(list))
	for i, q := range list {
		fields := []script.Field{
			script.KV("id", script.Int(q.ID)), script.KV("town", script.String(q.Town)),
			script.KV("purpose", script.String(q.Purpose)), script.KV("kind", script.String(q.Kind)),
			script.KV("title", script.String(q.Title)), script.KV("count", script.Int(int64(q.Count))),
			script.KV("base_exp", script.Int(q.BaseExp)), script.KV("job_exp", script.Int(q.JobExp)),
			script.KV("zeny", script.Int(q.Zeny)),
		}
		if q.Guild != "" {
			fields = append(fields, script.KV("guild", script.String(q.Guild)), script.KV("stage", script.String(q.Stage)),
				script.KV("step", script.Int(int64(q.Step))))
		}
		if q.Monster != 0 {
			fields = append(fields, script.KV("monster", script.Int(q.Monster)))
		}
		if q.Item != 0 {
			fields = append(fields, script.KV("item", script.Int(q.Item)))
		}
		if q.To != "" {
			fields = append(fields, script.KV("to", script.String(q.To)))
		}
		out[i] = script.NewTable(fields...)
	}
	c.Return(script.List(out...))
	return nil
}

func (r *Runtime) coreGiveExp(c *script.Call) error {
	h, err := r.coreWorldHost("give_exp")
	if err != nil {
		return err
	}
	if err := r.mutating("give_exp"); err != nil {
		return err
	}
	base, job := c.Int(1), c.Int(2)
	if base < 0 || job < 0 {
		return errors.New("give_exp: the exp must be 0 or more")
	}
	return h.GiveExp(c.Int(0), base, job)
}

func (r *Runtime) coreSkillPoints(c *script.Call) error {
	h, err := r.coreWorldHost("skill_points")
	if err != nil {
		return err
	}
	n, err := h.SkillPoints(c.Int(0))
	if err != nil {
		return err
	}
	c.Return(script.Int(int64(n)))
	return nil
}

func (r *Runtime) coreRefineInfo(c *script.Call) error {
	h, err := r.coreWorldHost("refine_info")
	if err != nil {
		return err
	}
	info, ok, err := h.RefineInfo(c.Int(0), c.String(1))
	if err != nil {
		return err
	}
	if !ok {
		c.Return(script.Nil)
		return nil
	}
	c.Return(script.NewTable(
		script.KV("item", script.Int(info.Item)), script.KV("refine", script.Int(int64(info.Refine))),
		script.KV("refineable", script.Bool(info.Refineable)), script.KV("max", script.Bool(info.Max)),
		script.KV("chance", script.Int(int64(info.Chance))), script.KV("price", script.Int(info.Price)),
		script.KV("material", script.Int(info.Material)),
	))
	return nil
}

func (r *Runtime) coreRefine(c *script.Call) error {
	h, err := r.coreWorldHost("refine")
	if err != nil {
		return err
	}
	if err := r.mutating("refine"); err != nil {
		return err
	}
	res, err := h.Refine(c.Int(0), c.String(1))
	if err != nil {
		return err
	}
	c.Return(script.String(res))
	return nil
}

func (r *Runtime) coreWeight(c *script.Call) error {
	h, err := r.coreWorldHost("weight")
	if err != nil {
		return err
	}
	n, err := h.Weight(c.Int(0))
	if err != nil {
		return err
	}
	c.Return(script.Int(int64(n)))
	return nil
}
