package modscript

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// The host functions behind the API modules. prelude.lua wraps each one,
// so errors point at the script's line.

// needContext returns the running context, or an error that says the
// call needs one.
func (r *Runtime) needContext(what string) (*Context, error) {
	if r.current == nil || r.current.ended {
		return nil, fmt.Errorf("%s needs a context: call it inside a setup function or a handler, not while the file loads", what)
	}
	return r.current, nil
}

// actor is the mod whose code runs now: the context's mod, or the mod
// whose file loads.
func (r *Runtime) actor() string {
	if r.current != nil {
		return r.current.Mod
	}
	return r.loading
}

// need checks that the running mod holds a permission.
func (r *Runtime) need(what, permission string) error {
	id := r.actor()
	sc := r.scripts[id]
	if sc == nil || !sc.Grants.Has(permission) {
		return fmt.Errorf("%s needs the permission %s, which %s does not have; request it in mod.yaml, and the server owner approves it", what, permission, id)
	}
	return nil
}

// resolve finds the file a module name names, for require. A name is
// <mod ID>.<path>: example.halloween.lib.rewards is scripts/lib/rewards.lua
// of example.halloween. The longest mod ID that matches wins. A mod may
// only require its own files and those of the mods in its depends.
func (r *Runtime) resolve(c *script.Call) error {
	from, name := c.String(0), c.String(1)
	sc := r.scripts[from]
	if sc == nil {
		return fmt.Errorf("require: %s has no scripts", from)
	}
	owner := longestPrefix(name, sc.Visible)
	if owner == "" {
		if other := longestPrefix(name, r.opts.Mods); other != "" {
			return fmt.Errorf("require: %s cannot load %s: %s is not in its depends", from, name, other)
		}
		return fmt.Errorf("require: no mod has the module %s; mod files are named <mod ID>.<path>, such as %s.lib.rewards for scripts/lib/rewards.lua", name, from)
	}
	path, ok := mod.ModulePath(strings.TrimPrefix(name, owner+"."))
	if !ok {
		return fmt.Errorf("require: %s is not a module name: after the mod ID come names of letters, digits, _ and -, separated by dots", name)
	}
	osc := r.scripts[owner]
	if osc == nil {
		return fmt.Errorf("require: module %s not found: %s has no scripts", name, owner)
	}
	src, ok := osc.Files[path]
	if !ok {
		return fmt.Errorf("require: module %s not found: %s has no %s", name, owner, path)
	}
	c.Return(script.String(owner), script.String(path), script.String(src))
	return nil
}

func longestPrefix(name string, ids []string) string {
	best := ""
	for _, id := range ids {
		if strings.HasPrefix(name, id+".") && len(id) > len(best) {
			best = id
		}
	}
	return best
}

func (r *Runtime) contextInstance(c *script.Call) error {
	ctx, err := r.needContext("context:get_instance")
	if err != nil {
		return err
	}
	c.Return(ctx.Instance)
	return nil
}

func (r *Runtime) contextKind(c *script.Call) error {
	ctx, err := r.needContext("context:get_kind")
	if err != nil {
		return err
	}
	c.Return(script.String(ctx.Kind))
	return nil
}

func (r *Runtime) contextMod(c *script.Call) error {
	ctx, err := r.needContext("context:get_mod")
	if err != nil {
		return err
	}
	c.Return(script.String(ctx.Mod))
	return nil
}

func (r *Runtime) eventsOn(c *script.Call) error {
	ctx, err := r.needContext("events.on")
	if err != nil {
		return err
	}
	if err := r.mutating("events.on"); err != nil {
		return err
	}
	if err := r.need("events.on", "event.listen"); err != nil {
		return err
	}
	name := c.String(0)
	if err := checkEvent(name, ctx.Kind); err != nil {
		return err
	}
	var filter *deathFilter
	var sfilter *statusFilter
	var kfilter *skillFilter
	var cfilter *chatFilter
	var tfilter *tradeFilter
	if opts := c.Arg(2); !opts.IsNil() {
		var err error
		switch {
		case name == "death":
			filter, err = readDeathFilter(opts)
		case slices.Contains(statusEvents, name):
			sfilter, err = readStatusFilter(name, opts)
		case name == "skill_used":
			kfilter, err = readSkillFilter(opts)
		case name == "chat":
			cfilter, err = readChatFilter(opts)
		case name == "trade":
			tfilter, err = readTradeFilter(opts)
		default:
			return fmt.Errorf("events.on(%q): only the death, status, skill_used, chat and trade events take options", name)
		}
		if err != nil {
			return err
		}
	}
	if err := r.room(ctx); err != nil {
		return err
	}
	ref, err := c.State().Keep(c.Arg(1))
	if err != nil {
		return err
	}
	h := &handler{id: r.newID(), event: name, ctx: ctx, fn: ref, death: filter, status: sfilter, skill: kfilter, chat: cfilter, trade: tfilter}
	ctx.handlers = append(ctx.handlers, h)
	r.handlers[name] = append(r.handlers[name], h)
	r.byID[h.id] = h
	c.Return(script.Int(h.id))
	return nil
}

// room checks that a context can own one more registration.
func (r *Runtime) room(ctx *Context) error {
	if ctx.retiring {
		return errors.New("the NPC's script was reloaded: a conversation of the old version cannot add handlers or timers")
	}
	if n := len(ctx.handlers) + len(ctx.timers) + len(ctx.hooks) + len(ctx.schedules); n >= MaxRegistrations {
		return fmt.Errorf("the context has %d handlers, timers, hooks and schedules, the most it may have", n)
	}
	return nil
}

func (r *Runtime) eventsRemove(c *script.Call) error {
	h := r.byID[c.Int(0)]
	if h == nil {
		// Removing twice, or after the context ended, does nothing.
		return nil
	}
	r.removeHandler(h)
	h.ctx.handlers = removeFrom(h.ctx.handlers, h)
	return nil
}

func removeFrom(list []*handler, h *handler) []*handler {
	for i, x := range list {
		if x == h {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}

func (r *Runtime) timersStart(c *script.Call) error {
	repeat := c.Bool(2)
	what := "timers.after"
	if repeat {
		what = "timers.every"
	}
	ctx, err := r.needContext(what)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	ms := c.Int(0)
	switch {
	case repeat && ms <= 0:
		return fmt.Errorf("%s: milliseconds must be above 0", what)
	case ms < 0:
		return fmt.Errorf("%s: milliseconds cannot be negative", what)
	case ms > 1<<40:
		return fmt.Errorf("%s: milliseconds is too large", what)
	}
	if err := r.room(ctx); err != nil {
		return err
	}
	ref, err := c.State().Keep(c.Arg(1))
	if err != nil {
		return err
	}
	t := &timer{id: r.newID(), ctx: ctx, due: r.host.Now() + ms, fn: ref}
	if repeat {
		t.every = ms
	}
	ctx.timers[t.id] = t
	r.timers[t.id] = t
	c.Return(script.Int(t.id))
	return nil
}

func (r *Runtime) timersCancel(c *script.Call) error {
	t := r.timers[c.Int(0)]
	if t == nil {
		return nil
	}
	if err := r.mutating("timer:cancel"); err != nil {
		return err
	}
	r.dropTimer(t)
	r.endTimerBonuses(t)
	if t.fn != nil {
		t.fn.Release()
	}
	r.flushStats()
	return nil
}

func (r *Runtime) itemsID(c *script.Call) error {
	if err := r.need("items.id", "game.read"); err != nil {
		return err
	}
	name := c.String(0)
	id, ok := r.host.ItemID(name)
	if !ok {
		return fmt.Errorf("items.id: there is no item %q", name)
	}
	c.Return(script.Int(int64(id)))
	return nil
}

// playerID reads a player handle's ID.
func playerID(c *script.Call) (int64, error) {
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return 0, errors.New("not a player: call the method with a colon, such as player:send_message(text)")
	}
	return id, nil
}

func (r *Runtime) playerField(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.need("reading a player", "game.read"); err != nil {
		return err
	}
	v, err := r.host.PlayerField(id, c.String(1))
	if err != nil {
		return err
	}
	c.Return(v)
	return nil
}

func (r *Runtime) monsterField(c *script.Call) error {
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return errors.New("not a monster: read fields of a monster handle, such as monster.name")
	}
	if err := r.need("reading a monster", "game.read"); err != nil {
		return err
	}
	v, err := r.host.MonsterField(id, c.String(1))
	if err != nil {
		return err
	}
	c.Return(v)
	return nil
}

func (r *Runtime) playerSendMessage(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating("player:send_message"); err != nil {
		return err
	}
	text := c.String(1)
	if text == "" || len(text) > MaxMessage {
		return fmt.Errorf("player:send_message: the text must be 1 to %d bytes", MaxMessage)
	}
	return r.host.SendMessage(id, text)
}

// MaxMessage is the longest text send_message accepts, in bytes. It is
// the chat limit of the web client.
const MaxMessage = 255

func (r *Runtime) playerGiveItem(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating("player:give_item"); err != nil {
		return err
	}
	if err := r.need("player:give_item", "player.give_items"); err != nil {
		return err
	}
	itemID, amount := c.Int(1), int64(1)
	if c.NArgs() > 2 && !c.Arg(2).IsNil() {
		amount = c.Int(2)
	}
	if itemID <= 0 || itemID > 1<<32-1 || amount < 1 || amount > 30000 {
		return errors.New("player:give_item: want an item ID from items.id and an amount from 1 to 30000")
	}
	rent := c.Int(3)
	if rent < 0 || rent > 1<<31 {
		return errors.New("player:give_item: rent_seconds must be from 0 to 2^31")
	}
	roll, err := r.giveRoll(c.Arg(4))
	if err != nil {
		return err
	}
	where, bound, err := giveOptions("player:give_item", c.Arg(5), c.Arg(6))
	if err != nil {
		return err
	}
	var ok bool
	if where != "" || bound != item.BoundNone {
		h, err := r.storageHost("player:give_item")
		if err != nil {
			return err
		}
		ok, err = h.StorageGive(id, where, r.actor(), uint32(itemID), int(amount), roll, bound) // #nosec G115 -- checked above.
		if err != nil {
			return err
		}
		c.Return(script.Bool(ok))
		return nil
	}
	if h, isRoll := r.host.(RollHost); isRoll {
		ok, err = h.GiveRolledItem(id, uint32(itemID), int(amount), rent, roll) // #nosec G115 -- checked above.
	} else if roll.Mode == RollDefault {
		ok, err = r.host.GiveItem(id, uint32(itemID), int(amount), rent) // #nosec G115 -- checked above.
	} else {
		err = errors.New("player:give_item: rolled values do not exist here")
	}
	if err != nil {
		return err
	}
	c.Return(script.Bool(ok))
	return nil
}

func (r *Runtime) playerGiveGroup(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating("player:give_item_from_group"); err != nil {
		return err
	}
	if err := r.need("player:give_item_from_group", "player.give_items"); err != nil {
		return err
	}
	group, ok := c.Arg(1).AsInt()
	if !ok {
		// A group may be named directly.
		name, isName := c.Arg(1).AsString()
		if !isName {
			return errors.New("player:give_item_from_group: want a group from items.group")
		}
		if group, ok = r.host.ItemGroupID(name); !ok {
			return fmt.Errorf("player:give_item_from_group: there is no item group %q", name)
		}
	}
	return r.host.GiveGroup(id, group)
}

func (r *Runtime) playerHeal(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	percent := c.Bool(1)
	what := "player:item_heal"
	if percent {
		what = "player:heal_percent"
	}
	// Heals need no approved permission: a healing item or a healer is
	// a common part of a mod.
	if err := r.mutating(what); err != nil {
		return err
	}
	var v [4]int
	for i := range v {
		n := c.Int(2 + i)
		if n < -1<<31 || n > 1<<31-1 {
			return fmt.Errorf("%s: amounts must fit in 32 bits", what)
		}
		v[i] = int(n)
	}
	return r.host.Heal(id, percent, [2]int{v[0], v[1]}, [2]int{v[2], v[3]})
}

func (r *Runtime) playerWarp(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	where := c.String(1)
	what := map[string]string{"map": "player:warp", "random": "player:warp_to_random_cell", "save_point": "player:warp_to_save_point"}[where]
	if what == "" {
		return fmt.Errorf("player warp: unknown target %q", where)
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.need(what, "player.warp"); err != nil {
		return err
	}
	var mapName string
	var x, y int64
	if where == "map" {
		mapName, x, y = c.String(2), c.Int(3), c.Int(4)
		if mapName == "" || x < 0 || y < 0 || x > 1<<16 || y > 1<<16 {
			return errors.New("player:warp: want a map name and a cell")
		}
	}
	ok, err := r.host.Warp(id, where, mapName, int(x), int(y))
	if err != nil {
		return err
	}
	c.Return(script.Bool(ok))
	return nil
}

func (r *Runtime) playerShowEffect(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	headgear := c.Bool(2)
	what := "player:show_effect"
	if headgear {
		what = "player:show_headgear_effect"
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	effect, ok := c.Arg(1).AsInt()
	if !ok || effect < 0 || effect > 1<<20 {
		return fmt.Errorf("%s: want an effect from wa.effects", what)
	}
	return r.host.ShowEffect(id, effect, headgear)
}

func (r *Runtime) playerSkillLevel(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.need("player:get_skill_level", "game.read"); err != nil {
		return err
	}
	skill, ok := c.Arg(1).AsInt()
	if !ok || skill <= 0 {
		return errors.New("player:get_skill_level: want a skill from skills.id")
	}
	lv, err := r.host.SkillLevel(id, skill)
	if err != nil {
		return err
	}
	c.Return(script.Int(int64(lv)))
	return nil
}

// Equipment slot names of player:get_equipped.
var equipSlots = map[string]bool{
	"head_top": true, "head_middle": true, "head_low": true, "armor": true, "left_hand": true, "right_hand": true,
	"garment": true, "shoes": true, "left_accessory": true, "right_accessory": true, "ammo": true,
	"costume_head_top": true, "costume_head_middle": true, "costume_head_low": true, "costume_garment": true,
	"shadow_armor": true, "shadow_weapon": true, "shadow_shield": true, "shadow_shoes": true,
	"shadow_right_accessory": true, "shadow_left_accessory": true,
}

func (r *Runtime) playerEquipped(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.need("player:get_equipped", "game.read"); err != nil {
		return err
	}
	slot := c.String(1)
	if !equipSlots[slot] {
		return fmt.Errorf("player:get_equipped: there is no slot %q", slot)
	}
	e, ok, err := r.host.Equipped(id, slot)
	if err != nil {
		return err
	}
	if ok {
		c.Return(script.Int(e))
	} else {
		c.Return(script.Nil)
	}
	return nil
}

func (r *Runtime) equippedField(c *script.Call) error {
	if err := r.need("reading an equipped item", "game.read"); err != nil {
		return err
	}
	v, err := r.host.EquippedField(c.Int(0), c.String(1))
	if err != nil {
		return err
	}
	c.Return(v)
	return nil
}

func (r *Runtime) itemsGroup(c *script.Call) error {
	if err := r.need("items.group", "game.read"); err != nil {
		return err
	}
	name := c.String(0)
	id, ok := r.host.ItemGroupID(name)
	if !ok {
		return fmt.Errorf("items.group: there is no item group %q", name)
	}
	c.Return(script.Int(id))
	return nil
}

func (r *Runtime) skillsID(c *script.Call) error {
	if err := r.need("skills.id", "game.read"); err != nil {
		return err
	}
	name := c.String(0)
	if r.opts.Constants != nil {
		if id, ok := r.opts.Constants.Skill(name); ok {
			c.Return(script.Int(id))
			return nil
		}
	}
	// Mods' skills, which the zone's skill database has.
	if h, ok := r.host.(SkillHost); ok {
		if id, ok := h.SkillID(name); ok {
			c.Return(script.Int(id))
			return nil
		}
	}
	return fmt.Errorf("skills.id: there is no skill %q", name)
}

func (r *Runtime) monstersID(c *script.Call) error {
	if err := r.need("monsters.id", "game.read"); err != nil {
		return err
	}
	name := c.String(0)
	id, ok := r.host.MonsterID(name)
	if !ok {
		return fmt.Errorf("monsters.id: there is no monster %q", name)
	}
	c.Return(script.Int(id))
	return nil
}

func (r *Runtime) randomInteger(c *script.Call) error {
	if err := r.mutating("random.integer"); err != nil {
		return err
	}
	lo, hi := c.Int(0), c.Int(1)
	if hi-lo < 0 || hi-lo >= 1<<62 {
		return errors.New("random.integer: the range is too large")
	}
	c.Return(script.Int(lo + r.host.RandomN(hi-lo+1)))
	return nil
}

func (r *Runtime) randomChance(c *script.Call) error {
	if err := r.mutating("random.chance"); err != nil {
		return err
	}
	num, den := c.Int(0), c.Int(1)
	if den <= 0 || den >= 1<<62 {
		return errors.New("random.chance: the denominator must be from 1 to 2^62")
	}
	// rnd() % den < num, as rAthena's rate checks.
	c.Return(script.Bool(r.host.RandomN(den) < num))
	return nil
}

// Job flags of rAthena's map job IDs.
const (
	mapidUpper     = 0x100000
	mapidBaby      = 0x200000
	mapidThird     = 0x1000
	mapidFourth    = 0x10000
	mapidBaseMask  = 0xff
	mapidUpperMask = 0xfff
)

// jobsTest runs a job test of wa.jobs.
func (r *Runtime) jobsTest(c *script.Call) error {
	test, job := c.String(0), c.Int(1)
	if r.opts.Jobs == nil {
		return fmt.Errorf("jobs.%s: the job tables are not loaded", test)
	}
	if job < 0 || job > 1<<20 {
		return fmt.Errorf("jobs.%s: %d is not a job", test, job)
	}
	m, ok := r.opts.Jobs.MapID(int(job))
	if !ok {
		return fmt.Errorf("jobs.%s: %d is not a job", test, job)
	}
	switch test {
	case "is_transcendent":
		c.Return(script.Bool(m&mapidUpper != 0))
	case "is_baby":
		c.Return(script.Bool(m&mapidBaby != 0))
	case "is_third":
		c.Return(script.Bool(m&mapidThird != 0))
	case "is_fourth":
		c.Return(script.Bool(m&mapidFourth != 0))
	case "base_job", "first_job":
		if test == "base_job" {
			if b, ok := r.opts.Jobs.BaseJobOf(int(job)); ok {
				c.Return(script.Int(int64(b)))
				return nil
			}
		}
		mask := uint64(mapidUpperMask)
		if test == "first_job" {
			mask = mapidBaseMask
		}
		// Jobs that differ by sex share a map job ID: keep the sex of
		// the job asked about.
		male := true
		if j, ok := r.opts.Jobs.JobOf(m, true); ok && j != int(job) {
			male = false
		}
		out, ok := r.opts.Jobs.JobOf(m&mask, male)
		if !ok {
			return fmt.Errorf("jobs.%s: %d has no base job", test, job)
		}
		c.Return(script.Int(int64(out)))
	default:
		return fmt.Errorf("jobs: no test %q", test)
	}
	return nil
}

// jobsID is jobs.id: a job by a name of wa.jobs or a mod job's full
// name.
func (r *Runtime) jobsID(c *script.Call) error {
	if err := r.need("jobs.id", "game.read"); err != nil {
		return err
	}
	name := c.String(0)
	if r.opts.Jobs != nil {
		if id, ok := r.opts.Jobs.JobID(name); ok {
			c.Return(script.Int(int64(id)))
			return nil
		}
	}
	return fmt.Errorf("jobs.id: there is no job %q", name)
}

// constantsTable returns the constant modules, as { races = { ... } }.
func (r *Runtime) constantsTable(c *script.Call) error {
	if !r.constants.IsNil() {
		c.Return(r.constants)
		return nil
	}
	var mods []script.Field
	if r.opts.Constants != nil {
		for name, m := range r.opts.Constants.Modules() {
			fields := make([]script.Field, 0, len(m.Values))
			for k, v := range m.Values {
				fields = append(fields, script.KV(k, script.Int(v)))
			}
			mods = append(mods, script.KV(name, script.NewTable(fields...)))
		}
	}
	r.constants = script.NewTable(mods...)
	c.Return(r.constants)
	return nil
}

func (r *Runtime) playerCountItem(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if !c.Arg(2).IsNil() {
		return r.playerStorageCount(c)
	}
	if err := r.need("player:count_item", "game.read"); err != nil {
		return err
	}
	item := c.Int(1)
	if item <= 0 || item > 1<<32-1 {
		return errors.New("player:count_item: want an item ID from items.id")
	}
	n, err := r.host.CountItem(id, uint32(item)) // #nosec G115 -- checked above.
	if err != nil {
		return err
	}
	c.Return(script.Int(int64(n)))
	return nil
}
