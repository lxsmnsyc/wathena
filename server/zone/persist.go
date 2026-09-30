package zone

import (
	"errors"
	"math"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
)

// Saving: the zone hands a copy of a player's character to the player's
// Saver, which writes it on its own goroutines, so the zone never waits
// on the database (strategy section 4.1, item 5). It saves:
//   - every SaveInterval, when the character changed since the last save,
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

// staleSave hands a player whose saves went stale to the zone goroutine.
// It blocks until the zone takes it or stops, so it must not run on the
// zone goroutine or a save worker.
func (z *Zone) staleSave(p *Player) {
	select {
	case z.stale <- p:
	case <-z.stopped:
	}
}

// loggedInElsewhere disconnects a player whose character a newer session
// owns: the reason goes out, then the player leaves. Its last save is
// ignored by the saver.
func (z *Zone) loggedInElsewhere(p *Player) {
	if p.dropped || p.entity == nil || !z.entities[p.entity] {
		return
	}
	p.queue(wathenav1.ServerMessage_builder{Disconnect: wathenav1.Disconnect_builder{
		Reason:  wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_IN_ELSEWHERE,
		Message: "Your character logged in somewhere else.",
	}.Build()}.Build())
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
		Map: z.Map, InventorySize: item.BaseSlots,
	}}
	c.X, c.Y = z.startCell()
	if z.Progress != nil {
		c.StatusPoints = z.Progress.NewCharacter(c.Job).StatusPoints
	}
	// Without job tables, there is no weight limit to check start items
	// against.
	st, maxWeight := z.PlayerStatus, math.MaxInt
	if z.Status != nil {
		sc := z.Start
		sc.Job, sc.Male, sc.BaseLevel, sc.JobLevel = c.Job, c.Male, 1, 1
		res, err := z.Status.Calc(sc)
		if err == nil {
			st, maxWeight = res.Battle, res.MaxWeight
		} else {
			maxWeight = 0
		}
	}
	c.HP, c.SP = int64(st.MaxHP), int64(st.MaxSP)
	inv := item.NewInventory()
	for _, s := range z.StartItems {
		// Items the inventory cannot take are left out.
		if i, r := inv.Add(z.Items.Get(s.ID), s.Amount, maxWeight); r == item.AddSuccess && inv.Slots[i].Serial.IsZero() {
			inv.Slots[i].Serial = item.NewSerial()
		}
	}
	c.Items = savedItems(inv)
	return c
}

// restoreInventory fills an inventory from a saved character. Slots
// outside the inventory are left out.
func restoreInventory(t *item.Table, c *character.Character) *item.Inventory {
	inv := item.NewInventory()
	if c.InventorySize > 0 && c.InventorySize <= item.MaxInventory {
		inv.Size = c.InventorySize
	}
	for _, it := range c.Items {
		if it.Slot < 0 || it.Slot >= item.MaxInventory || it.ID == 0 || it.Amount <= 0 {
			continue
		}
		inv.Slots[it.Slot] = item.Slot{ID: it.ID, Amount: it.Amount, Serial: it.Serial}
		inv.Weight += t.Get(it.ID).Weight * it.Amount
	}
	return inv
}

func savedItems(inv *item.Inventory) []character.Item {
	var items []character.Item
	for i, s := range inv.Slots {
		if s.ID != 0 && s.Amount > 0 {
			items = append(items, character.Item{Slot: i, ID: s.ID, Amount: s.Amount, Serial: s.Serial})
		}
	}
	return items
}

// snapshot copies a player's character.
func (z *Zone) snapshot(p *Player) *character.Character {
	c := &character.Character{Info: p.record}
	c.Name = p.Name
	c.Job, c.Male, c.Stats = p.char.Job, p.char.Male, p.char.Stats
	if z.Progress != nil {
		c.BaseLevel, c.JobLevel = p.progress.BaseLevel, p.progress.JobLevel
		c.BaseExp, c.JobExp = p.progress.BaseExp, p.progress.JobExp
		c.StatusPoints, c.SkillPoints = p.progress.StatusPoints, p.progress.SkillPoints
	}
	e := p.entity
	c.HP, c.SP = max(e.hp, 0), max(p.sp, 0)
	c.Map, c.X, c.Y = z.Map, e.x, e.y
	c.InventorySize = p.inventory.Size
	c.Items = savedItems(p.inventory)
	return c
}

// save hands a player's character to its saver. A save that is not final
// is skipped when nothing changed since the last one.
func (z *Zone) save(p *Player, final bool) {
	if p.saver != nil {
		c := z.snapshot(p)
		if final || p.saved == nil || !c.Equal(p.saved) {
			p.saved = c
			p.saver.Save(c, final)
		}
	}
	if final && p.left != nil && !p.gone {
		p.gone = true
		close(p.left)
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
