package zone

import (
	"errors"
	"maps"
	"math"
	"slices"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/skill"
)

// Saving: the zone hands a copy of a player's character to the player's
// Saver, which writes it on its own goroutines, so the zone never waits
// on the database (strategy section 4.1, item 5). It saves:
//   - every SaveInterval, when the character changed since the last save,
//   - when the player arrives from another map, at the new cell,
//   - when the player leaves, and
//   - for every player when the zone stops.

// DefaultSaveInterval is how often changed characters are saved: the
// data loss budget of strategy section 3.8.
const DefaultSaveInterval = 30 * time.Second

// ErrStopped is returned for a join after the zone stopped.
var ErrStopped = errors.New("zone: stopped")

// Saver saves a player's character. Save must not block. It owns c
// afterwards and must not change it. final is set for the last save of a
// session, when the player leaves or the zone stops.
type Saver interface {
	Save(c *character.Character, final bool)
}

// Stopped closes when Run has returned, after the zone handed every
// player's last save to its saver.
func (z *Zone) Stopped() <-chan struct{} { return z.stopped }

// StaleNotifier is a Saver that can report that its session lost the
// character to a newer session, such as a login elsewhere. The zone then
// disconnects the player, since nothing it does would be saved. f may be
// called from any goroutine, and may block.
type StaleNotifier interface {
	OnStale(f func())
}

// loggedInElsewhere disconnects a player whose character a newer session
// owns: the reason goes out, then the player leaves. Its last save is
// ignored by the saver.
func (z *Zone) loggedInElsewhere(p *Player) {
	if p.gone() || p.entity == nil || !z.entities.has(p.entity) {
		return
	}
	z.queue(p, loggedInElsewhereMessage())
	z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
}

func (z *Zone) saveInterval() time.Duration {
	if z.SaveInterval > 0 {
		return z.SaveInterval
	}
	return DefaultSaveInterval
}

// NewCharacter is a new character: the zone's Start character at base
// and job level 1, with full HP and SP and the start items, at the start
// cell. It only reads settings that are set before Run, so it may be
// called from any goroutine.
func (z *Zone) NewCharacter(name string, look Look) character.Character {
	c := character.Character{Info: character.Info{
		Name: name, Job: z.Start.Job, Male: look.Sex == 1,
		Hair: int(look.Hair), HairColor: int(look.HairColor),
		BaseLevel: 1, JobLevel: 1, Stats: z.Start.Stats,
		Map: z.Map, InventorySize: z.limits().InventorySlots, CreatedAt: z.wallClock(),
	}}
	c.X, c.Y = z.startCell()
	if z.Progress != nil {
		c.StatusPoints = z.Progress.NewCharacter(c.Job).StatusPoints
	}
	if z.StartBaseLevel > 1 || z.StartJobLevel > 1 {
		z.startLevels(&c)
	}
	// Without job tables, there is no weight limit to check start items
	// against.
	st, maxWeight := z.PlayerStatus, math.MaxInt
	if z.Status != nil {
		sc := z.Start
		sc.Job, sc.Male, sc.BaseLevel, sc.JobLevel = c.Job, c.Male, c.BaseLevel, c.JobLevel
		if c.Skills != nil {
			sc.Skills = c.Skills
		}
		res, err := z.Status.Calc(sc)
		if err == nil {
			st, maxWeight = res.Battle, res.MaxWeight
		} else {
			maxWeight = 0
		}
	}
	c.HP, c.SP = int64(st.MaxHP), int64(st.MaxSP)
	inv := item.NewInventoryOf(c.InventorySize, z.limits().MaxOf("inventory_slots"))
	for _, s := range z.StartItems {
		// Items the inventory cannot take are left out.
		if i, r := inv.Add(z.Items.Get(s.ID), s.Amount, maxWeight); r == item.AddSuccess && inv.Slots[i].Serial.IsZero() {
			inv.Slots[i].Serial = item.NewSerial()
		}
	}
	c.Items = savedItems(inv)
	return c
}

// fixRolls checks the rolls and random options of every copy a player
// brought in against the item data (strategy section 5.4).
func (z *Zone) fixRolls(p *Player) {
	if z.Items == nil {
		return
	}
	for i := range p.inventory.Slots {
		s := &p.inventory.Slots[i]
		if s.ID == 0 {
			continue
		}
		d := z.Items.Get(s.ID)
		if len(d.Rolls) == 0 && len(s.Rolls) == 0 && len(s.CardRolls) == 0 && len(s.Options) == 0 {
			continue
		}
		z.fixExtra(p.Name, i, d, &s.Extra, s.Cards)
	}
}

// restoreInventory fills an inventory from a saved character. Its size is
// the saved one, within the inventory_slots limit's base and max (review
// decision 32). It holds more slots when the character holds items in
// later slots: lowering a limit deletes nothing. Slots past the engine's
// ceiling are left out.
func restoreInventory(t *item.Table, c *character.Character, l rules.Limits) *item.Inventory {
	r, _ := l.RangeOf("inventory_slots")
	inv := item.NewInventoryOf(r.Clamp(c.InventorySize), l.MaxOf("inventory_slots"))
	for _, it := range c.Items {
		if it.Slot >= 0 && it.Slot < item.MaxInventory {
			inv.Grow(it.Slot + 1)
		}
	}
	for _, it := range c.Items {
		if it.Slot < 0 || it.Slot >= len(inv.Slots) || it.ID == 0 || it.Amount <= 0 {
			continue
		}
		inv.Slots[it.Slot] = item.Slot{ID: it.ID, Amount: it.Amount, Serial: it.Serial, Equip: it.Equip, Refine: it.Refine,
			Cards: slices.Clone(it.Cards), Extra: it.Clone()}
		inv.Weight += t.Get(it.ID).Weight * it.Amount
	}
	return inv
}

func savedItems(inv *item.Inventory) []character.Item {
	var items []character.Item
	for i, s := range inv.Slots {
		if s.ID != 0 && s.Amount > 0 {
			items = append(items, character.Item{Slot: i, ID: s.ID, Amount: s.Amount, Serial: s.Serial,
				Equip: s.Equip, Refine: s.Refine, Cards: slices.Clone(s.Cards), Extra: s.Clone()})
		}
	}
	return items
}

// snapshot copies a player's character.
func (z *Zone) snapshot(p *Player) *character.Character {
	c := &character.Character{Info: p.record}
	c.Name = p.Name
	if p.gmHidden {
		// pc_makesavestatus saves OPTION_INVISIBLE, so @hide lasts
		// across logins.
		c.Options |= companion.OptionInvisible
	}
	c.Job, c.Male, c.Stats = p.char.Job, p.char.Male, p.char.Stats
	if z.Progress != nil {
		c.BaseLevel, c.JobLevel = p.progress.BaseLevel, p.progress.JobLevel
		c.BaseExp, c.JobExp = p.progress.BaseExp, p.progress.JobExp
		c.StatusPoints, c.SkillPoints = p.progress.StatusPoints, p.progress.SkillPoints
	}
	e := p.entity
	c.HP, c.SP = max(e.hp, 0), max(p.sp, 0)
	c.Map, c.X, c.Y = z.Map, e.x, e.y
	if e.dead {
		// pc_makesavestatus: a dead character is saved at its save
		// point, where it comes back.
		if to, x, y := z.savePoint(p); to != nil {
			c.Map, c.X, c.Y = to.Map, x, y
		}
	}
	c.InventorySize = p.inventory.Size
	c.Items = savedItems(p.inventory)
	if len(p.vars) > 0 {
		c.Variables = maps.Clone(p.vars)
	}
	c.Statuses = z.savedStatuses(p)
	c.TimedBonuses = z.savedTimedBonuses(p)
	c.Skills = learnedSkills(p)
	c.Blocks = slices.Clone(p.blocks)
	c.Sanctions = slices.Clone(p.sanctions)
	c.Currencies = savedCurrencies(p)
	return c
}

// save hands a player's character to its saver. A save that is not final
// is skipped when nothing changed since the last one. After the final
// save, the session's Leave returns.
func (z *Zone) save(p *Player, final bool) {
	s := p.sess
	if s == nil {
		return
	}
	// The player's transfers are committed before its later saves.
	if p.transferred {
		z.commitTransfers()
	}
	if s.saver != nil {
		c := z.snapshot(p)
		if final || p.saved == nil || !c.Equal(p.saved) {
			p.saved = c
			s.saver.Save(c, final)
		}
	}
	if final {
		s.done()
	}
}

// autosave saves the players whose save interval passed. Each player's
// interval starts when it enters, so saves spread over the interval.
func (z *Zone) autosave() {
	for _, p := range z.players {
		if z.now >= p.nextSave {
			p.nextSave = z.now + z.saveInterval().Milliseconds()
			z.save(p, false)
		}
	}
}

// startLevels gives a new character the development start levels, with
// the points those levels give, as if it had gained them.
func (z *Zone) startLevels(c *character.Character) {
	if z.StartBaseLevel > 1 {
		if z.Progress != nil {
			for l := c.BaseLevel; l < z.StartBaseLevel; l++ {
				c.StatusPoints += z.Progress.StatusPointGain(l)
			}
		}
		c.BaseLevel = z.StartBaseLevel
	}
	if z.StartJobLevel > 1 {
		c.SkillPoints += z.StartJobLevel - c.JobLevel
		c.JobLevel = z.StartJobLevel
	}
	if c.Job != 0 {
		// A first class job needs Basic Skill at level 9.
		c.Skills = map[int]int{skill.IDBasic: 9}
	}
}
