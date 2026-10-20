package modscript

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// The player calls of review decision 58, area B7 of proposal 0002b:
// rAthena's getexp and getexp2, heal and Hp, setpcblock and getpcblock,
// disable_items and enable_items, nude, pushpc, transform, readbook,
// getsavepoint, getinventorylist, setlook, checkoption, jobchange with
// Upper, eaclass and roclass, and skill with SKILL_PERM_GRANT.

// PlayerLook is what player:set_look changes. A nil field stays as it
// is. The fields are rAthena's LOOK_ types.
type PlayerLook struct {
	HairStyle, HairColor, ClothColor, HeadTop, HeadMiddle, HeadBottom *int64
}

// InventoryEntry is one slot of player:get_inventory, as
// getinventorylist's arrays.
type InventoryEntry struct {
	Slot, Item, Amount, Equip, Refine int64
	Identified, Broken                bool
	Cards                             []int64
	// Expires is a rental's end in Unix seconds, or 0.
	Expires int64
	// Bound is "", account, guild, party or character.
	Bound string
	// Tradable is pc_can_trade_item.
	Tradable bool
	// Options are the random options: name, value and param.
	Options []RandomOptionEntry
}

// RandomOptionEntry is one random option of an InventoryEntry.
type RandomOptionEntry struct {
	Name         string
	Value, Param int64
}

// PlayerActionHost is the host of the calls of area B7. A host without
// it raises an error for them.
type PlayerActionHost interface {
	// GainExp is getexp (raw false) and getexp2 (raw true). getexp's
	// amounts are 0 or more and the host applies quest_exp_rate.
	// getexp2's go without rates, and a negative amount takes exp, as
	// pc_lostexp.
	GainExp(player, base, job int64, raw bool) error
	// HealFixed is heal: status_heal with fixed amounts. A negative
	// amount takes HP or SP, and may kill.
	HealFixed(player, hp, sp int64) error
	// SetHP is Hp = n: pc_setparam's SP_HP, from 1 to the maximum.
	SetHP(player, hp int64) error
	// SetBlocked is setpcblock: it adds or clears PCBLOCK_ flags.
	SetBlocked(player int64, mask uint32, blocked bool) error
	// Blocked is getpcblock.
	Blocked(player int64) (uint32, error)
	// SetItemsLocked is disable_items (locked) and enable_items for the
	// player of a conversation.
	SetItemsLocked(conv *Conversation, locked bool) error
	// UnequipAll is nude.
	UnequipAll(player int64) error
	// Push is pushpc: unit_blown by cells in a direction, 0 north to 7
	// north-east, as rAthena's DIR_.
	Push(player int64, direction, cells int) error
	// Transform is transform: the player looks like a monster for ms.
	// It returns false when the player cannot transform.
	Transform(player, monster, ms int64) (bool, error)
	// OpenBook is readbook.
	OpenBook(player, book, page int64) error
	// SavePoint is getsavepoint: the map's name and the cell.
	SavePoint(player int64) (string, int, int, error)
	// Inventory is getinventorylist.
	Inventory(player int64) ([]InventoryEntry, error)
	// SetPlayerLook is setlook.
	SetPlayerLook(player int64, look PlayerLook) error
	// HasOption is checkoption: whether any bit of mask is set in the
	// player's options.
	HasOption(player int64, mask int) (bool, error)
}

func (r *Runtime) playerActionHost(what string) (PlayerActionHost, error) {
	h, ok := r.host.(PlayerActionHost)
	if !ok {
		return nil, fmt.Errorf("%s: this zone has no player actions", what)
	}
	return h, nil
}

// BlockActions are the actions of player:set_blocked, rAthena's
// PCBLOCK_ flags by name (script.hpp 2221). npc and all are rAthena's
// PCBLOCK_NPC and PCBLOCK_ALL.
var BlockActions = map[string]uint32{
	"move": 0x1, "attack": 0x2, "skill": 0x4, "use_item": 0x8, "chat": 0x10, "immune": 0x20,
	"sit_stand": 0x40, "commands": 0x80, "npc_click": 0x100, "emotion": 0x200, "equip": 0x400,
	"npc": 0x58D, "all": 0x7FF,
}

// blockNames are BlockActions' names, sorted, for errors.
func blockNames() string {
	names := make([]string, 0, len(BlockActions))
	for k := range BlockActions {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// PlayerOptions are the options of player:has_option, rAthena's
// OPTION_ bits (status.hpp). cart and dragon are every cart and dragon
// look.
var PlayerOptions = map[string]int{
	"sight": 0x1, "hide": 0x2, "cloak": 0x4, "cart": 0x8 | 0x80 | 0x100 | 0x200 | 0x400, "falcon": 0x10, "riding": 0x20,
	"invisible": 0x40, "orcish": 0x800, "wedding": 0x1000, "ruwach": 0x2000, "chasewalk": 0x4000, "flying": 0x8000,
	"xmas": 0x10000, "transform": 0x20000, "summer": 0x40000,
	"dragon": 0x80000 | 0x800000 | 0x1000000 | 0x2000000 | 0x4000000,
	"warg":   0x100000, "warg_rider": 0x200000, "madogear": 0x400000, "hanbok": 0x8000000, "oktoberfest": 0x10000000,
	"summer2": 0x20000000,
}

// MaxPushCells bounds player:push's cells: rAthena's unit_blown takes a
// count of cells that path_blownpos walks one by one.
const MaxPushCells = 25

// MaxExpAmount is MAX_EXP for PACKETVER 20170830 and later: INT64_MAX.
// The API takes amounts up to 2^62, so a sum never overflows.
const MaxExpAmount = 1 << 62

// playerAction is the B7 methods of player handles: Arg(0) is the
// handle, Arg(1) the method, then its arguments.
func (r *Runtime) playerAction(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	op := c.String(1)
	what := "player:" + op
	arg := func(i int) script.Value { return c.Arg(2 + i) }
	if op == "is_blocked" || op == "has_option" || op == "get_save_point" || op == "get_inventory" {
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
	} else if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.playerActionHost(what)
	if err != nil {
		return err
	}
	switch op {
	case "give_exp":
		if err := r.need(what, "player.give_exp"); err != nil {
			return err
		}
		base, ok1 := arg(0).AsInt()
		job, ok2 := arg(1).AsInt()
		if !ok1 || !ok2 || base < -MaxExpAmount || base > MaxExpAmount || job < -MaxExpAmount || job > MaxExpAmount {
			return fmt.Errorf("%s: want base and job exp, integers from -2^62 to 2^62", what)
		}
		raw := arg(2).Truthy()
		switch {
		case base == 0 && job == 0:
			// buildin_getexp and buildin_getexp2 refuse 0 and 0.
			return errors.New(what + ": base and job exp are both 0")
		case !raw && (base < 0 || job < 0):
			return errors.New(what + ": exp must be 0 or more; { raw = true } takes exp with a negative amount, as getexp2")
		}
		return wrap(what, h.GainExp(id, base, job, raw))
	case "heal":
		hp, ok1 := arg(0).AsInt()
		sp, ok2 := arg(1).AsInt()
		if !ok1 || !ok2 || hp < -(1<<31) || hp > 1<<31-1 || sp < -(1<<31) || sp > 1<<31-1 {
			return errors.New(what + ": want HP and SP amounts that fit in 32 bits")
		}
		return wrap(what, h.HealFixed(id, hp, sp))
	case "set_hp":
		hp, ok := arg(0).AsInt()
		if !ok || hp < -(1<<31) || hp > 1<<31-1 {
			return errors.New(what + ": want an amount that fits in 32 bits")
		}
		return wrap(what, h.SetHP(id, hp))
	case "set_blocked", "is_blocked":
		name, _ := arg(0).AsString()
		mask, ok := BlockActions[name]
		if !ok {
			return fmt.Errorf("%s: the action is one of %s", what, blockNames())
		}
		if op == "is_blocked" {
			cur, err := h.Blocked(id)
			if err != nil {
				return wrap(what, err)
			}
			c.Return(script.Bool(cur&mask == mask))
			return nil
		}
		b := arg(1)
		if _, isBool := b.AsBool(); !isBool {
			return errors.New(what + ": want an action and true or false")
		}
		return wrap(what, h.SetBlocked(id, mask, b.Truthy()))
	case "unequip_all":
		return wrap(what, h.UnequipAll(id))
	case "push":
		dir, ok1 := arg(0).AsInt()
		cells, ok2 := arg(1).AsInt()
		if !ok1 || !ok2 || dir < 0 || dir > 7 || cells < -MaxPushCells || cells > MaxPushCells {
			return fmt.Errorf("%s: want a direction from 0 (north) to 7 (north-east) and -%d to %d cells", what, MaxPushCells, MaxPushCells)
		}
		return wrap(what, h.Push(id, int(dir), int(cells)))
	case "transform":
		m, ok1 := arg(0).AsInt()
		ms, ok2 := arg(1).AsInt()
		if !ok1 || m <= 0 || m > 1<<31-1 {
			return errors.New(what + ": want a monster from monsters.id")
		}
		if !ok2 || ms < 0 || ms > 1<<40 {
			return errors.New(what + ": milliseconds must be 0 or more")
		}
		ok, err := h.Transform(id, m, ms)
		if err != nil {
			return wrap(what, err)
		}
		c.Return(script.Bool(ok))
		return nil
	case "open_book":
		book, ok1 := arg(0).AsInt()
		page, ok2 := arg(1).AsInt()
		if !ok1 || !ok2 || book <= 0 || book > 1<<32-1 || page < 0 || page > 1<<31-1 {
			return errors.New(what + ": want a book, its item ID, and a page from 0")
		}
		return wrap(what, h.OpenBook(id, book, page))
	case "get_save_point":
		m, x, y, err := h.SavePoint(id)
		if err != nil {
			return wrap(what, err)
		}
		c.Return(script.String(m), script.Int(int64(x)), script.Int(int64(y)))
		return nil
	case "get_inventory":
		list, err := h.Inventory(id)
		if err != nil {
			return wrap(what, err)
		}
		c.Return(inventoryList(list))
		return nil
	case "set_look":
		look, err := readPlayerLook(what, arg(0))
		if err != nil {
			return err
		}
		return wrap(what, h.SetPlayerLook(id, look))
	case "has_option":
		name, _ := arg(0).AsString()
		mask, ok := PlayerOptions[name]
		if !ok {
			names := make([]string, 0, len(PlayerOptions))
			for k := range PlayerOptions {
				names = append(names, k)
			}
			slices.Sort(names)
			return fmt.Errorf("%s: the option is one of %s", what, strings.Join(names, ", "))
		}
		on, err := h.HasOption(id, mask)
		if err != nil {
			return wrap(what, err)
		}
		c.Return(script.Bool(on))
		return nil
	}
	return fmt.Errorf("player has no method %q", op)
}

// playerLookFields are player:set_look's fields.
var playerLookFields = []string{"hair_style", "hair_color", "cloth_color", "head_top", "head_middle", "head_bottom"}

func readPlayerLook(what string, v script.Value) (PlayerLook, error) {
	t, ok := v.AsTable()
	if !ok {
		return PlayerLook{}, fmt.Errorf("%s: want a table of %q", what, playerLookFields)
	}
	var look PlayerLook
	for _, f := range t.Fields {
		key, _ := f.Key.AsString()
		n, ok := f.Value.AsInt()
		if !ok || n < 0 || n > 1<<31-1 {
			return PlayerLook{}, fmt.Errorf("%s: %s must be an integer of 0 or more", what, key)
		}
		switch key {
		case "hair_style":
			look.HairStyle = &n
		case "hair_color":
			look.HairColor = &n
		case "cloth_color":
			look.ClothColor = &n
		case "head_top":
			look.HeadTop = &n
		case "head_middle":
			look.HeadMiddle = &n
		case "head_bottom":
			look.HeadBottom = &n
		default:
			return PlayerLook{}, fmt.Errorf("%s: there is no field %q; the fields are %q", what, key, playerLookFields)
		}
	}
	return look, nil
}

// inventoryList is player:get_inventory's list.
func inventoryList(list []InventoryEntry) script.Value {
	out := make([]script.Value, len(list))
	for i, e := range list {
		cards := make([]script.Value, len(e.Cards))
		for j, card := range e.Cards {
			cards[j] = script.Int(card)
		}
		opts := make([]script.Value, len(e.Options))
		for j, o := range e.Options {
			opts[j] = script.NewTable(script.KV("option", script.String(o.Name)), script.KV("value", script.Int(o.Value)),
				script.KV("param", script.Int(o.Param)))
		}
		fields := []script.Field{
			script.KV("slot", script.Int(e.Slot)), script.KV("item", script.Int(e.Item)), script.KV("amount", script.Int(e.Amount)),
			script.KV("equip", script.Int(e.Equip)), script.KV("refine", script.Int(e.Refine)),
			script.KV("identified", script.Bool(e.Identified)), script.KV("broken", script.Bool(e.Broken)),
			script.KV("cards", script.List(cards...)), script.KV("expires", script.Int(e.Expires)),
			script.KV("tradable", script.Bool(e.Tradable)), script.KV("random_options", script.List(opts...)),
		}
		if e.Bound != "" {
			fields = append(fields, script.KV("bound", script.String(e.Bound)))
		}
		out[i] = script.NewTable(fields...)
	}
	return script.List(out...)
}

// conversationItems is conversation:lock_items and
// conversation:unlock_items: Arg(0) is the conversation, Arg(1) true to
// lock.
func (r *Runtime) conversationItems(c *script.Call) error {
	locked := c.Bool(1)
	what := "conversation:unlock_items"
	if locked {
		what = "conversation:lock_items"
	}
	conv, _, err := r.conversation(c, what)
	if err != nil {
		return err
	}
	h, err := r.playerActionHost(what)
	if err != nil {
		return err
	}
	return wrap(what, h.SetItemsLocked(conv, locked))
}

// jobsClass is jobs.class_flags and jobs.from_class: eaclass and roclass
// (script.cpp 12835, 12853). Arg(0) is the call, Arg(1) the job or the
// flags, Arg(2) the sex for from_class.
func (r *Runtime) jobsClass(c *script.Call) error {
	op := c.String(0)
	what := "jobs." + op
	if r.opts.Jobs == nil {
		return fmt.Errorf("%s: the job tables are not loaded", what)
	}
	n, ok := c.Arg(1).AsInt()
	if !ok || n < 0 || n > 1<<32-1 {
		return fmt.Errorf("%s: want a job or class flags of 0 or more", what)
	}
	switch op {
	case "class_flags":
		m, ok := r.opts.Jobs.MapID(int(n))
		if !ok {
			// pc_jobid2mapid gives -1 for a job that does not exist.
			c.Return(script.Int(-1))
			return nil
		}
		c.Return(script.Int(int64(m))) // #nosec G115 -- map job IDs fit in 32 bits.
	case "from_class":
		male := true
		if v := c.Arg(2); !v.IsNil() {
			s, _ := v.AsString()
			if s != "male" && s != "female" {
				return errors.New(what + `: the sex is "male" or "female"`)
			}
			male = s == "male"
		}
		j, ok := r.opts.Jobs.JobOf(uint64(n), male)
		if !ok {
			c.Return(script.Int(-1))
			return nil
		}
		c.Return(script.Int(int64(j)))
	default:
		return fmt.Errorf("jobs has no function %q", op)
	}
	return nil
}

// keepLine is jobchange's Upper argument (pc_jobchange, pc.cpp 10861):
// the job in the player's line, transcendent or baby, as
// pc_jobid2mapid(job) | JOBL_UPPER or JOBL_BABY, then pc_mapid2jobid by
// the player's sex.
func (r *Runtime) keepLine(player int64, job int) (int, error) {
	if r.opts.Jobs == nil {
		return 0, errors.New("the job tables are not loaded")
	}
	cur, err := r.host.PlayerField(player, "job")
	if err != nil {
		return 0, err
	}
	sex, err := r.host.PlayerField(player, "sex")
	if err != nil {
		return 0, err
	}
	curJob, _ := cur.AsInt()
	curMap, ok := r.opts.Jobs.MapID(int(curJob))
	if !ok {
		return job, nil
	}
	m, ok := r.opts.Jobs.MapID(job)
	if !ok {
		return job, nil
	}
	m |= curMap & (mapidUpper | mapidBaby)
	male, _ := sex.AsString()
	out, ok := r.opts.Jobs.JobOf(m, male == "male")
	if !ok {
		return job, nil
	}
	return out, nil
}
