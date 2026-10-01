package zone

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// Mods is what mods bring to a zone: their scripts and the names of
// their items. Set Zone.Mods before Run.
type Mods struct {
	// Scripts are the mods with Lua files, in load order.
	Scripts []*mod.Scripts
	// IDs are every loaded mod's ID.
	IDs []string
	// ItemIDs maps item names to IDs, for items.id.
	ItemIDs map[string]uint32
	// Limits are the Lua state's limits.
	Limits script.Limits
	// Log gets script errors.
	Log *slog.Logger
	// Constants are rAthena's script constants: the constant modules and
	// skills.id.
	Constants *constants.Data
	// Groups are the item groups that boxes open.
	Groups *item.Groups
	// MonsterIDs maps monster AegisNames, such as PORING, to IDs.
	MonsterIDs map[string]int64
}

// NPC is an NPC placed on the map by a mod's data. It shows to players.
// Clicking it does nothing yet (phase 3 step 4).
type NPC struct {
	// Name is the full record name, such as example.town:Healer.
	Name string
	// DisplayName is the name players see.
	DisplayName string
	X, Y        int
	// Direction is 0 to 7, starting south and turning clockwise.
	Direction int
	// Sprite is the NPC sprite ID.
	Sprite uint32
}

type npc struct {
	NPC
}

// SetNPCs places NPCs on the map. It replaces the NPCs set before. Call
// it before Run.
func (z *Zone) SetNPCs(npcs []NPC) {
	for e := range z.entities.all() {
		if e.npc != nil {
			z.entities.remove(e)
		}
	}
	for _, n := range npcs {
		z.entities.add(&entity{npc: &npc{n}, x: n.X, y: n.Y, dir: n.Direction})
	}
}

func (z *Zone) npcAppear(id uint32, e *entity) *wathenav1.Event {
	return wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{
		Id:   id,
		Kind: wathenav1.EntityKind_ENTITY_KIND_NPC,
		X:    uint32(e.x),   // #nosec G115 -- map cells are at most 1024.
		Y:    uint32(e.y),   // #nosec G115 -- as above.
		Dir:  uint32(e.dir), // #nosec G115 -- 0 to 7.
		Look: wathenav1.Look_builder{Body: e.npc.Sprite}.Build(),
		Name: e.npc.DisplayName,
	}.Build()}.Build()
}

// scriptEvent is an event for mod scripts, held until the zone reaches a
// point where scripts may run: no entity list is being walked.
type scriptEvent struct {
	name string
	args []modscript.Arg
	// release are monster handles that end after the event runs.
	release []int64
}

// startScripts makes the zone's Lua state and runs the mods' entry
// scripts, when a mod has any. A zone whose mods only have item scripts
// makes its state when a player first uses an item, so the hundreds of
// zones nobody visits hold no Lua state. It runs on the zone's
// goroutine.
func (z *Zone) startScripts() {
	if z.Mods == nil {
		return
	}
	for _, sc := range z.Mods.Scripts {
		if len(sc.Entries) > 0 {
			z.itemScripts()
			return
		}
	}
}

// itemScripts returns the zone's Lua state, made on first use, or nil
// when the zone runs no scripts.
func (z *Zone) itemScripts() *modscript.Runtime {
	if z.scripts != nil || z.scriptsFailed || z.Mods == nil || len(z.Mods.Scripts) == 0 {
		return z.scripts
	}
	log := z.Mods.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	opts := modscript.Options{Host: scriptHost{z}, Log: log, Limits: z.Mods.Limits, Mods: z.Mods.IDs, Constants: z.Mods.Constants}
	if z.Status != nil {
		opts.Jobs = z.Status
	}
	r, err := modscript.New(opts, z.Mods.Scripts)
	if err != nil {
		log.Error("mod scripts cannot run in this zone", "map", z.Map, "err", err)
		z.scriptsFailed = true
		return nil
	}
	z.scripts = r
	return r
}

func (z *Zone) stopScripts() {
	if z.scripts != nil {
		z.scripts.Close()
		z.scripts = nil
	}
}

// scriptEventFor queues an event for mod scripts.
func (z *Zone) scriptEventFor(name string, args ...modscript.Arg) {
	if z.scripts == nil {
		return
	}
	z.scriptEvents = append(z.scriptEvents, scriptEvent{name: name, args: args})
}

// monsterKilled queues monster_killed. The handle to the dead monster
// stays readable until the event has run.
func (z *Zone) monsterKilled(killer *Player, e *entity) {
	if z.scripts == nil {
		return
	}
	if z.monsterHandles == nil {
		z.monsterHandles = map[int64]*entity{}
	}
	z.nextMonsterHandle++
	h := z.nextMonsterHandle
	z.monsterHandles[h] = e
	z.scriptEvents = append(z.scriptEvents, scriptEvent{
		name:    "monster_killed",
		args:    []modscript.Arg{modscript.Player(int64(killer.charID)), modscript.Monster(h)},
		release: []int64{h},
	})
}

// runScripts runs the queued events' handlers. Handlers may queue more
// events, which run in the same call.
func (z *Zone) runScripts() {
	for len(z.scriptEvents) > 0 {
		ev := z.scriptEvents[0]
		z.scriptEvents = z.scriptEvents[1:]
		if z.scripts != nil {
			z.scripts.Fire(ev.name, ev.args...)
		}
		for _, h := range ev.release {
			delete(z.monsterHandles, h)
		}
	}
	z.scriptEvents = z.scriptEvents[:0]
}

// ErrNoScripts is ReloadMod's error in a zone that runs no mod scripts.
var ErrNoScripts = errors.New("zone: the zone runs no mod scripts")

// ReloadMod loads a mod's scripts again in the zone, from a new
// snapshot. The mod's old context ends first, which removes its handlers
// and timers. It waits until the zone has done it.
func (z *Zone) ReloadMod(ctx context.Context, sc *mod.Scripts) error {
	done := make(chan error, 1)
	f := func() {
		if z.scripts == nil {
			done <- ErrNoScripts
			return
		}
		done <- z.scripts.Reload(sc)
	}
	select {
	case z.control <- f:
	case <-ctx.Done():
		return ctx.Err()
	case <-z.stopped:
		return ErrStopped
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// scriptHost is the zone as mod scripts see it.
type scriptHost struct{ z *Zone }

func (h scriptHost) Map() string { return strings.TrimPrefix(h.z.Map, "map/") }

func (h scriptHost) player(id int64) (*Player, error) {
	if id > 0 && id <= int64(^uint32(0)) {
		if p := h.z.players[uint32(id)]; p != nil && !p.gone() {
			return p, nil
		}
	}
	return nil, errors.New("the player has left this map")
}

// PlayerField reads a field of a player handle (proposal 0002, "Players
// and other entities").
func (h scriptHost) PlayerField(id int64, field string) (script.Value, error) {
	p, err := h.player(id)
	if err != nil {
		return script.Nil, err
	}
	e := p.entity
	switch field {
	case "name":
		return script.String(p.Name), nil
	case "base_level":
		return script.Int(int64(p.progress.BaseLevel)), nil
	case "job_level":
		return script.Int(int64(p.progress.JobLevel)), nil
	case "job":
		return script.Int(int64(p.char.Job)), nil
	case "zeny":
		return script.Int(p.record.Zeny), nil
	case "hp":
		return script.Int(e.hp), nil
	case "max_hp":
		if e.status == nil {
			return script.Int(0), nil
		}
		return script.Int(int64(e.status.MaxHP)), nil
	case "sp":
		return script.Int(p.sp), nil
	case "max_sp":
		if e.status == nil {
			return script.Int(0), nil
		}
		return script.Int(int64(e.status.MaxSP)), nil
	case "map":
		return script.String(h.Map()), nil
	case "x":
		return script.Int(int64(e.x)), nil
	case "y":
		return script.Int(int64(e.y)), nil
	case "character_id":
		return script.Int(p.record.ID), nil
	case "account_id":
		return script.Int(p.record.AccountID), nil
	case "party":
		// Parties do not exist yet.
		return script.Nil, nil
	case "base_str":
		return script.Int(int64(p.char.Stats.Str)), nil
	case "base_agi":
		return script.Int(int64(p.char.Stats.Agi)), nil
	case "base_vit":
		return script.Int(int64(p.char.Stats.Vit)), nil
	case "base_int":
		return script.Int(int64(p.char.Stats.Int)), nil
	case "base_dex":
		return script.Int(int64(p.char.Stats.Dex)), nil
	case "base_luk":
		return script.Int(int64(p.char.Stats.Luk)), nil
	case "base_pow", "base_sta", "base_wis", "base_spl", "base_con", "base_crt":
		// Trait stats cannot be raised yet.
		return script.Int(0), nil
	case "job_class":
		if h.z.Status == nil {
			return script.Int(0), nil
		}
		m, _ := h.z.Status.MapID(p.char.Job)
		return script.Int(int64(m)), nil // #nosec G115 -- map job IDs fit in 32 bits.
	}
	return script.Nil, fmt.Errorf("player has no field %q", field)
}

// MonsterField reads a field of a monster handle. A killed monster is
// readable until its monster_killed event has run, with is_alive false.
func (h scriptHost) MonsterField(id int64, field string) (script.Value, error) {
	e := h.z.monsterHandles[id]
	if e == nil {
		return script.Nil, errors.New("the monster is gone from this map")
	}
	m := e.monster.spawn
	switch field {
	case "id":
		return script.Int(int64(m.Mob)), nil
	case "name":
		return script.String(m.Name), nil
	case "level":
		if m.Status == nil {
			return script.Int(0), nil
		}
		return script.Int(int64(m.Status.Level)), nil
	case "hp":
		return script.Int(max(e.hp, 0)), nil
	case "max_hp":
		if e.status == nil {
			return script.Int(0), nil
		}
		return script.Int(int64(e.status.MaxHP)), nil
	case "map":
		return script.String(h.Map()), nil
	case "x":
		return script.Int(int64(e.x)), nil
	case "y":
		return script.Int(int64(e.y)), nil
	case "is_alive":
		return script.Bool(!e.dead && h.z.entities.has(e)), nil
	}
	return script.Nil, fmt.Errorf("monster has no field %q", field)
}

// SendMessage shows a line to one player in the chat log, as server
// text (CHAT_CHANNEL_SYSTEM).
func (h scriptHost) SendMessage(id int64, text string) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	p.pending = append(p.pending, wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
		Channel: wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM,
		Text:    text,
	}.Build()}.Build())
	return nil
}

// GiveItem gives a player items, as getitem: a stackable item in one
// add, anything else one at a time. It stops at the first that does not
// fit, as getitem does, and the player sees why. Rentals are given as
// normal items: items have no expiry yet.
func (h scriptHost) GiveItem(id int64, itemID uint32, amount int, rentSeconds int64) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	return h.z.giveItem(p, itemID, amount, h.z.Items.Get(itemID).Stackable), nil
}

// CountItem is countitem.
func (h scriptHost) CountItem(id int64, itemID uint32) (int, error) {
	p, err := h.player(id)
	if err != nil {
		return 0, err
	}
	return p.inventory.Count(itemID), nil
}

// Now is the zone clock.
func (h scriptHost) Now() int64 { return h.z.now }

// ItemID finds an item by name.
func (h scriptHost) ItemID(name string) (uint32, bool) {
	if h.z.Mods == nil {
		return 0, false
	}
	id, ok := h.z.Mods.ItemIDs[name]
	return id, ok
}
