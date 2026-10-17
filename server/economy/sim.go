package economy

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"sync"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Monster is what the simulator reads of a monster.
type Monster struct {
	// Name is the AegisName or a mod monster's full name.
	Name string
	// Status is the monster's battle status, and Reward its level, exp
	// and drops after the drop rate settings.
	Status *battle.Status
	Reward *progression.Mob
	// Skills are the monster's skills, from mob_skill_db.
	Skills []mob.Skill
}

// Skills the model player avoids in a monster.
const (
	skillTeleport = 26 // AL_TELEPORT
	skillHeal     = 28 // AL_HEAL
)

// evasive reports whether a monster teleports away or heals itself in a
// fight: AL_TELEPORT, or AL_HEAL on itself, in a state with a target.
// The battle formulas leave skills out, so the simulator cannot tell how
// long such a fight lasts, and the model player leaves the monster
// alone, as a player walks past a monster it cannot finish.
func evasive(mon Monster) bool {
	for _, s := range mon.Skills {
		if s.Rate <= 0 {
			continue
		}
		switch s.State {
		case mob.StateBerserk, mob.StateAngry, mob.StateRush, mob.StateAnyTarget, mob.StateAny:
		default:
			continue
		}
		if s.ID == skillTeleport || s.ID == skillHeal && s.Target == mob.TargetSelf {
			return true
		}
	}
	return false
}

// LeftAlone reports whether the model player of a level leaves a
// monster alone: one that teleports away or heals itself in a fight,
// one it cannot kill, and one whose fights last longer than
// hunt.max_fight_ms on average.
func (w *World) LeftAlone(level int, mon Monster) (bool, error) {
	if evasive(mon) {
		return true, nil
	}
	fg, err := w.Fight(level, mon)
	if err != nil {
		return false, err
	}
	return fg.Unkillable || fg.FightMs > int64(w.Config.Hunt.MaxFightMs), nil
}

// World is the game data the simulator reads: the economy model, the
// items, the job and exp tables, the ruleset's flags and the monsters.
type World struct {
	Config *Config
	Items  *item.Table
	Jobs   *status.Tables
	Tables *progression.Tables
	// Rules are the ruleset's flags, or nil for renewal's.
	Rules *rules.Rules
	// Monster finds a monster by name.
	Monster func(name string) (Monster, bool)

	mu     sync.Mutex
	models map[int]*Model
	fights map[fightKey]Fight
}

// Tick is the zone's tick, which rounds attack delays up.
const Tick = 50

// Model returns the model player of a level with its battle status.
func (w *World) Model(level int) (*Model, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if m := w.models[level]; m != nil {
		return m, nil
	}
	p, err := w.Config.PlayerAt(level, w.Items, w.Jobs, w.Tables)
	if err != nil {
		return nil, err
	}
	st, err := p.wear(w.Items, w.Jobs)
	if err != nil {
		return nil, err
	}
	m := &Model{Player: p, Status: st, Heal: p.avgHeal(st.Vit)}
	if w.models == nil {
		w.models = map[int]*Model{}
	}
	w.models[level] = m
	return m, nil
}

// Fight is how the model player of a level fares against a monster, on
// average over the config's fights.
type Fight struct {
	// AttacksPermille is the player's attacks per kill, in thousandths.
	AttacksPermille int64
	// FightMs is the time from the first attack to the kill.
	FightMs int64
	// DamageTaken is the damage the monster deals the player per kill.
	DamageTaken int64
	// Unkillable is set when a fight did not end in 5,000 attacks, such
	// as a monster the player always misses.
	Unkillable bool
}

type fightKey struct {
	level int
	name  string
}

// maxAttacks ends a fight the player cannot win.
const maxAttacks = 5000

// roundTick rounds a delay up to the zone's tick, at least 100 ms, as
// the zone's attack timer runs.
func roundTick(ms int) int64 {
	ms = max(ms, 100)
	return int64((ms + Tick - 1) / Tick * Tick)
}

// Fight plays the model player of a level against a monster with the
// server's battle formulas: the player attacks at its attack delay until
// the monster's HP is gone, and a monster that can attack hits back at
// its own delay. The draws come from a stream seeded by the monster and
// the level, so the result is the same each time.
func (w *World) Fight(level int, mon Monster) (Fight, error) {
	k := fightKey{level, mon.Name}
	w.mu.Lock()
	if f, ok := w.fights[k]; ok {
		w.mu.Unlock()
		return f, nil
	}
	w.mu.Unlock()
	m, err := w.Model(level)
	if err != nil {
		return Fight{}, err
	}
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%s:%d", mon.Name, level)
	r := rand.New(rand.NewPCG(h.Sum64(), 0x9e3779b97f4a7c15)) // #nosec G404 -- a seeded simulation, not a secret.
	interval := roundTick(m.Status.ADelay)
	mInterval := roundTick(mon.Status.ADelay)
	canAttack := mon.Status.Mode&mob.ModeCanAttack != 0
	n := w.Config.Hunt.Fights
	var attacks, fightMs, taken int64
	var out Fight
	for range n {
		hp := int64(mon.Status.MaxHP)
		a := int64(0)
		for hp > 0 && a < maxAttacks {
			target := *mon.Status
			target.HP = hp
			attacker := m.Status
			res := battle.AttackRules(&attacker, &target, w.Rules, r)
			hp -= max(res.Damage, 0)
			a++
		}
		if hp > 0 {
			out.Unkillable = true
		}
		ms := (a - 1) * interval
		attacks += a
		fightMs += ms
		if canAttack {
			// The monster hits back once the fight starts, then at its
			// delay.
			for range ms/mInterval + 1 {
				target := m.Status
				attacker := *mon.Status
				res := battle.AttackRules(&attacker, &target, w.Rules, r)
				taken += max(res.Damage, 0)
			}
		}
	}
	out.AttacksPermille = attacks * 1000 / int64(n)
	out.FightMs = fightMs / int64(n)
	out.DamageTaken = taken / int64(n)
	w.mu.Lock()
	if w.fights == nil {
		w.fights = map[fightKey]Fight{}
	}
	w.fights[k] = out
	w.mu.Unlock()
	return out, nil
}

// Line is one spawn line a player hunts.
type Line struct {
	Monster   string
	Count     int
	RespawnMs int
}

// Field is a map's hunt: its walkable cells and the spawn lines a player
// hunts, without bosses.
type Field struct {
	Walkable int
	Lines    []Line
}

// Rates are a field's results per hour for a model player.
type Rates struct {
	// Kills is the kills per hour.
	Kills int64
	// BaseExp and JobExp are the exp per hour.
	BaseExp, JobExp int64
	// Zeny is the zeny per hour from selling the drops to NPCs.
	Zeny int64
	// Supplies is the zeny per hour of the potions that heal the damage
	// taken, at a dealer's price. Natural regen is left out.
	Supplies int64
	// Lines are each line's kills per hour, in thousandths, in the
	// field's order.
	Lines []int64
}

// lineCost is what one kill of a line gives and takes.
type lineCost struct {
	ms               int64
	base, job        int64
	zeny             int64 // in 1/10,000 zeny
	taken            int64
	count, respawnMs int64
	absent           bool
}

// Simulate estimates a field's rates for the model player of a level
// (strategy section 8.13):
//
//   - The player meets monsters as their counts share the field, so each
//     line's share of the kills is its share of the monsters.
//   - Each kill takes the walk to the next monster, the fight, the
//     overhead and the loot (hunt in economy.yml).
//   - A line cannot give more kills than its monsters come back: count
//     times an hour over the respawn. The time a capped line leaves goes
//     to the other lines.
//   - Exp is the kill's exp after the level penalty, and zeny each drop's
//     rate after the level penalty times its NPC sell price.
//
// Monsters the player leaves alone (LeftAlone), and names the world does
// not know, are left out: the player walks past them.
func (w *World) Simulate(level int, f Field) (Rates, error) {
	m, err := w.Model(level)
	if err != nil {
		return Rates{}, err
	}
	h := w.Config.Hunt
	costs := make([]lineCost, len(f.Lines))
	fights := make([]Fight, len(f.Lines))
	var monsters int64
	for i, l := range f.Lines {
		mon, ok := w.Monster(l.Monster)
		if !ok || mon.Status == nil || mon.Reward == nil || l.Count < 1 {
			costs[i].absent = true
			continue
		}
		alone, err := w.LeftAlone(level, mon)
		if err != nil {
			return Rates{}, err
		}
		if alone {
			// The player leaves the monster alone, and walks past it.
			costs[i].absent = true
			continue
		}
		fights[i], _ = w.Fight(level, mon)
		monsters += int64(l.Count)
	}
	if monsters == 0 {
		return Rates{Lines: make([]int64, len(f.Lines))}, nil
	}
	// The walk: the side of the square of one monster, in thousandths
	// of a cell, less the melee reach.
	side := isqrt(int64(f.Walkable) * 1_000_000 / monsters)
	walk := max(side*int64(h.ApproachPermille)/1000-1000, 0) * int64(max(m.Status.Speed, 1)) / 1000
	for i, l := range f.Lines {
		c := &costs[i]
		if c.absent {
			continue
		}
		mon, _ := w.Monster(l.Monster)
		fg := fights[i]
		c.count, c.respawnMs = int64(l.Count), int64(max(l.RespawnMs, 1))
		c.base, c.job = w.exp(m, mon.Reward)
		items, zeny, _ := w.drops(m, mon.Reward)
		c.zeny = zeny
		c.taken = fg.DamageTaken
		c.ms = walk + fg.FightMs + int64(h.OverheadMs) + int64(h.LootMs)*items/10000
	}
	// Kills per hour in thousandths: first every line by its share,
	// then the lines over their respawn get their cap and the rest share
	// the time left.
	const hour = int64(3_600_000)
	kills := make([]int64, len(f.Lines))
	capped := make([]bool, len(f.Lines))
	for range len(f.Lines) + 1 {
		left := hour * 1000
		var weight int64
		for i, c := range costs {
			if c.absent {
				continue
			}
			if capped[i] {
				left -= kills[i] * c.ms
				continue
			}
			weight += c.count * c.ms
		}
		if weight == 0 || left <= 0 {
			break
		}
		changed := false
		for i, c := range costs {
			if c.absent || capped[i] {
				continue
			}
			kills[i] = left * c.count / weight
			if limit := c.count * hour * 1000 / c.respawnMs; kills[i] > limit {
				kills[i], capped[i], changed = limit, true, true
			}
		}
		if !changed {
			break
		}
	}
	out := Rates{Lines: kills}
	var taken int64
	for i, c := range costs {
		k := kills[i]
		out.Kills += k
		out.BaseExp += k * c.base
		out.JobExp += k * c.job
		out.Zeny += k * c.zeny
		taken += k * c.taken
	}
	out.Kills /= 1000
	out.BaseExp /= 1000
	out.JobExp /= 1000
	out.Zeny /= 1000 * 10000
	out.Supplies = taken / 1000 / int64(m.Heal) * int64(m.PotionPrice)
	return out, nil
}

// exp is the base and job exp of one solo kill, after the level penalty.
func (w *World) exp(m *Model, r *progression.Mob) (int64, int64) {
	var log progression.DamageLog
	log.Add(1, int64(max(r.MaxHP, 1)), 0)
	k := w.Tables.Kill(r, &log, 1, func(uint32) progression.Attacker {
		return progression.Attacker{Online: true, BaseLevel: m.Level}
	})
	if len(k.Rewards) == 0 {
		return 0, 0
	}
	return k.Rewards[0].BaseExp, k.Rewards[0].JobExp
}

// drops returns a kill's expected items, in 1/10,000 items, its
// expected NPC sale value, in 1/10,000 zeny, and the variance of that
// value, in zeny squared: each drop's rate after the level penalty and
// the player's LUK, times the item's sell price, each drop rolled on its
// own. Items NPCs do not buy count as items, not zeny.
func (w *World) drops(m *Model, r *progression.Mob) (int64, int64, int64) {
	mod := w.Tables.Penalty(progression.PenaltyDrop, m.Level, r)
	killer := &progression.Killer{Player: true, Luk: m.Status.Luk}
	var items, zeny, variance int64
	for _, d := range r.Drops {
		if _, ok := w.Tables.ItemType(d.Item); !ok || d.Item == 0 {
			continue
		}
		rate := int64(w.Tables.DropRate(d.Rate, mod, killer))
		items += rate
		def, ok := w.Items.Lookup(d.Item)
		if !ok || def.NoSell {
			continue
		}
		v := int64(def.Sell)
		zeny += rate * v
		variance += rate * (10000 - rate) * v * v / 100_000_000
	}
	return items, zeny, variance
}

// Kill is what one kill of a monster gives the model player of a level
// on average.
type Kill struct {
	BaseExp, JobExp int64
	// Zeny is the expected NPC sale value of the drops, in 1/10,000
	// zeny, and ZenyVariance its variance, in zeny squared.
	Zeny, ZenyVariance int64
}

// KillOf returns what one kill of a monster gives the model player of a
// level: exp after the level penalty and the drops' value.
func (w *World) KillOf(level int, mon Monster) (Kill, error) {
	m, err := w.Model(level)
	if err != nil {
		return Kill{}, err
	}
	var k Kill
	k.BaseExp, k.JobExp = w.exp(m, mon.Reward)
	_, k.Zeny, k.ZenyVariance = w.drops(m, mon.Reward)
	return k, nil
}

// isqrt is the integer square root.
func isqrt(n int64) int64 {
	if n <= 0 {
		return 0
	}
	x := n
	y := (x + 1) / 2
	for y < x {
		x = y
		y = (x + n/x) / 2
	}
	return x
}
