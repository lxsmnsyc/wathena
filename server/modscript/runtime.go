// Package modscript runs mod scripts in a zone: one Lua state per zone
// (strategy section 4.1), an environment per mod, the hidden context of
// proposal 0002, and the first API modules: wa.context, wa.events,
// wa.timers and wa.items. mods/README.md describes the rules for mod
// authors.
package modscript

import (
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/variables"
)

//go:embed prelude.lua
var prelude string

// Host is what scripts reach in the zone. The zone implements it. Player
// IDs are the zone's own player IDs.
type Host interface {
	// Map is the zone's map name, such as prontera.
	Map() string
	// PlayerField returns a field of a player handle, such as name.
	PlayerField(player int64, field string) (script.Value, error)
	// MonsterField returns a field of a monster handle, such as name.
	MonsterField(monster int64, field string) (script.Value, error)
	// SendMessage shows a line of text to one player.
	SendMessage(player int64, text string) error
	// GiveItem gives a player items, as getitem. rentSeconds above 0
	// makes them rentals. It returns false when they do not fit.
	GiveItem(player int64, item uint32, amount int, rentSeconds int64) (bool, error)
	// GiveGroup gives a player the items of one use of an item group,
	// as getgroupitem.
	GiveGroup(player int64, group int64) error
	// CountItem counts the items of an ID a player has.
	CountItem(player int64, item uint32) (int, error)
	// Heal heals a player as itemheal, or as percentheal when percent is
	// set. HP and SP are [min, max], drawn as rand when they differ.
	Heal(player int64, percent bool, hp, sp [2]int) error
	// Warp moves a player after the script's event: to a map and cell
	// (where "map"), to a random cell of its map ("random"), or to its
	// save point ("save_point"). It returns false when the warp cannot
	// happen.
	Warp(player int64, where, mapName string, x, y int) (bool, error)
	// ShowEffect plays an effect on a player for everyone near, or a
	// headgear effect when headgear is set.
	ShowEffect(player int64, effect int64, headgear bool) error
	// SkillLevel is the level of a skill the player has, or 0.
	SkillLevel(player int64, skill int64) (int, error)
	// Equipped returns the copy a player has in an equipment slot, such
	// as right_hand, as an equipped handle ID. ok is false for an empty
	// slot.
	Equipped(player int64, slot string) (id int64, ok bool, err error)
	// EquippedField reads a field of an equipped copy, such as refine.
	EquippedField(id int64, field string) (script.Value, error)
	// RandomN returns a number in [0, n) from the zone's secret stream.
	RandomN(n int64) int64
	// ItemID finds an item by name: an AegisName or a mod's full name.
	ItemID(name string) (uint32, bool)
	// ItemGroupID finds an item group by its item_group_db name.
	ItemGroupID(name string) (int64, bool)
	// MonsterID finds a monster by its AegisName, such as PORING.
	MonsterID(name string) (int64, bool)
	// Now is the zone clock, in milliseconds. Timers start from it.
	Now() int64
}

// Jobs answers the job tests of wa.jobs from the job records and
// rAthena's map job IDs.
type Jobs interface {
	// MapID is pc_jobid2mapid. A mod job has the map job ID of its
	// class.
	MapID(job int) (uint64, bool)
	// JobOf is pc_mapid2jobid.
	JobOf(mapID uint64, male bool) (int, bool)
	// JobID finds a job by a name of wa.jobs or a mod job's full name.
	JobID(name string) (int, bool)
	// BaseJobOf is a job record's base_job. ok is false without one.
	BaseJobOf(job int) (int, bool)
}

// Options configure a zone's Runtime.
type Options struct {
	Host   Host
	Log    *slog.Logger
	Limits script.Limits
	// Mods are every loaded mod's ID, so require can tell a mod that is
	// not a dependency from one that does not exist.
	Mods []string
	// Constants are rAthena's script constants, for the constant modules
	// and skills.id. Without them, those modules are empty.
	Constants *constants.Data
	// Jobs answers the job tests. Without it, they raise an error.
	Jobs Jobs
	// Server holds the server variables that every zone shares. Without
	// it, server variables raise an error.
	Server *variables.Server
	// Testing adds wa.testing, for wactl mod test. A server never sets
	// it.
	Testing bool
	// DeferStart leaves the mods' entry scripts for StartMods to run.
	DeferStart bool
	// Clock is the server clock, for wa.schedules. Without it, schedules
	// raise an error.
	Clock func() time.Time
	// Location is the server's time zone. Schedules match times there.
	// It is UTC when nil.
	Location *time.Location
}

// Limits of what one context can hold, so a script cannot fill the
// zone's memory from Go.
const (
	// MaxItemScripts is the most item entry scripts a zone keeps loaded.
	// Loading one more drops the one loaded first. Contexts that run keep
	// what they use, so this only frees scripts nothing uses.
	MaxItemScripts = 256
	// MaxRegistrations is the most handlers and timers one context may
	// have at once.
	MaxRegistrations = 10_000
	// MaxFailures handler failures within FailureWindow zone milliseconds
	// end a mod's context in that zone (strategy section 6.6).
	MaxFailures   = 10
	FailureWindow = 60_000
)

// Runtime is one zone's mod scripts. Only the zone's goroutine uses it.
type Runtime struct {
	opts Options
	s    *script.State
	host Host
	log  *slog.Logger
	fns  struct {
		loadEntry, invoke, dropMod, dropFile, resetLoading, isProtected, converse *script.Ref
		loadTests, runCase, describe, runHooks, condition                         *script.Ref
		statusBonuses                                                             *script.Ref
	}
	nextID int64

	scripts  map[string]*mod.Scripts
	order    []string            // mods with scripts, in load order
	contexts map[string]*Context // the mod context of each mod, by mod ID

	handlers map[string][]*handler
	byID     map[int64]*handler
	timers   map[int64]*timer

	// current is the context whose code runs now. loading is the mod
	// whose file loads now, outside any context.
	current *Context
	loading string
	// computing is set while a computed bonus runs: calls that change
	// the game are refused (strategy section 5.3, rule 3).
	computing bool
	// group is the with_bonuses timer whose function runs now, or nil.
	group *timer
	// setups are the setup functions of item entry scripts, by
	// <mod>:<path>, loaded on first use. setupOrder is their load order,
	// for dropping the oldest past MaxItemScripts.
	setups     map[string]*script.Ref
	setupOrder []string
	// bonuses are the bonuses that have not ended, by handle ID.
	bonuses map[int64]*Bonus
	// constants are the constant modules, made once.
	constants script.Value
	// npcs are the NPC instances, in the order they started.
	npcs []*NPC
	// conversations are the conversations that have not ended, by ID.
	conversations map[int64]*Conversation
	// closed is the error a closed conversation raises.
	closed string
	// variables are the declared variables, by handle ID. declared maps
	// "<mod> <scope> <name>" to the type, so a name keeps one type.
	variables []*variable
	declared  map[string]string
	// hooks are the hooks by name, in the order they run. chain is the
	// chain that runs now.
	hooks    map[string][]*hook
	hookByID map[int64]*hook
	chain    []*hook
	// schedules are the schedules of wa.schedules, by handle ID.
	schedules map[int64]*scheduled
	// events are the event contexts that run, by context ID.
	events map[int64]*Context
	// saved are the states that hook chains the prelude runs replaced,
	// and computeStack the computing flags that computeBegin replaced.
	saved        []savedChain
	computeStack []bool
	// skillCtxs are the contexts of skills with scripts, by skill ID, and
	// skillEntries their entry scripts. passives are the contexts of
	// passive skills, by owner and skill.
	skillCtxs    map[int64]*Context
	skillEntries map[int64]string
	passives     map[passiveKey]*passive
	// monsterCtxs are the contexts of monsters' AI scripts, by monster
	// handle.
	monsterCtxs map[int64]*Context
}

// Context owns what a setup function and its handlers create
// (proposal 0002, "The hidden context").
type Context struct {
	ID   int64
	Kind string
	Mod  string
	// Instance is what context:get_instance() returns.
	Instance script.Value

	grants   mod.Grants
	handlers []*handler
	timers   map[int64]*timer
	ended    bool
	failures []int64
	// sink gets the bonuses of an equip context. bonuses are those it
	// owns.
	sink    BonusSink
	bonuses []*Bonus
	// player is the player of an item context: the user or the wearer.
	player int64
	// npc is the NPC handle of an NPC context.
	npc int64
	// conversations are those that run in an NPC context. retiring is
	// set when the context ended while some still run: its handlers and
	// timers are gone, and it ends with the last conversation.
	conversations map[int64]*Conversation
	retiring      bool
	// hooks and schedules are those the context added. spawned is set
	// once it spawned monsters.
	hooks     []*hook
	schedules map[int64]*scheduled
	spawned   bool
}

// Ended reports whether the context has ended.
func (c *Context) Ended() bool { return c.ended }

type handler struct {
	id      int64
	event   string
	ctx     *Context
	fn      *script.Ref
	removed bool
	// death holds the options of a death handler, or nil, status the
	// options of a status event handler, and skill those of a skill_used
	// handler.
	death  *deathFilter
	status *statusFilter
	skill  *skillFilter
}

type timer struct {
	id       int64
	ctx      *Context
	due      int64
	every    int64
	fn       *script.Ref
	canceled bool
	// bonuses are the bonuses of a with_bonuses timer, which end when it
	// is due or canceled. fn is nil for such a timer.
	bonuses []*Bonus
}

// New makes a zone's Lua state and loads every mod's entry scripts in
// load order. A mod whose scripts fail is logged and left off in this
// zone. Other mods still load.
func New(opts Options, mods []*mod.Scripts) (*Runtime, error) {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	r := &Runtime{
		opts: opts, host: opts.Host, log: opts.Log,
		scripts: map[string]*mod.Scripts{}, contexts: map[string]*Context{},
		handlers: map[string][]*handler{}, byID: map[int64]*handler{}, timers: map[int64]*timer{},
		setups: map[string]*script.Ref{}, bonuses: map[int64]*Bonus{},
		conversations: map[int64]*Conversation{}, declared: map[string]string{},
		hooks: map[string][]*hook{}, hookByID: map[int64]*hook{}, schedules: map[int64]*scheduled{}, events: map[int64]*Context{},
		skillCtxs: map[int64]*Context{}, skillEntries: map[int64]string{}, passives: map[passiveKey]*passive{},
		monsterCtxs: map[int64]*Context{},
	}
	s, err := script.New(script.Options{Limits: opts.Limits})
	if err != nil {
		return nil, err
	}
	r.s = s
	if err := r.init(); err != nil {
		s.Close()
		return nil, err
	}
	for _, sc := range mods {
		r.scripts[sc.Mod] = sc
		r.order = append(r.order, sc.Mod)
	}
	if !opts.DeferStart {
		_ = r.StartMods()
	}
	return r, nil
}

// StartMods loads every mod's entry scripts in load order, and runs each
// setup function in the mod's context. A mod whose scripts fail is
// logged and left off in this zone. Other mods still load. New runs it,
// unless Options.DeferStart is set. It returns the failures.
func (r *Runtime) StartMods() error {
	var errs []error
	for _, id := range r.order {
		sc := r.scripts[id]
		if err := r.start(sc); err != nil {
			r.log.Error("mod scripts failed to load; the mod is off in this zone", "map", r.host.Map(), "mod", sc.Mod, "err", err)
			errs = append(errs, fmt.Errorf("%s: %w", sc.Mod, err))
		}
	}
	return errors.Join(errs...)
}

// init runs the prelude with the host functions.
func (r *Runtime) init() error {
	host := []struct {
		name string
		fn   script.HostFunc
	}{
		{"resolve", r.resolve},
		{"context_instance", r.contextInstance},
		{"context_kind", r.contextKind},
		{"context_mod", r.contextMod},
		{"events_on", r.eventsOn},
		{"events_remove", r.eventsRemove},
		{"timers_start", r.timersStart},
		{"timers_cancel", r.timersCancel},
		{"items_id", r.itemsID},
		{"player_field", r.playerField},
		{"monster_field", r.monsterField},
		{"player_send_message", r.playerSendMessage},
		{"player_give_item", r.playerGiveItem},
		{"player_count_item", r.playerCountItem},
		{"player_give_group", r.playerGiveGroup},
		{"player_heal", r.playerHeal},
		{"player_warp", r.playerWarp},
		{"player_show_effect", r.playerShowEffect},
		{"player_skill_level", r.playerSkillLevel},
		{"player_equipped", r.playerEquipped},
		{"entity_status", r.entityStatus},
		{"statuses_define", r.statusesDefine},
		{"hook_chain_begin", r.hookChainBegin},
		{"hook_chain_end", r.hookChainEnd},
		{"compute_begin", r.computeBegin},
		{"compute_end", r.computeEnd},
		{"player_add_timed_bonuses", r.playerAddTimedBonuses},
		{"equipped_field", r.equippedField},
		{"equipped_set_roll", r.equippedSetRoll},
		{"equipped_reroll", r.equippedReroll},
		{"items_group", r.itemsGroup},
		{"skills_id", r.skillsID},
		{"monsters_id", r.monstersID},
		{"random_integer", r.randomInteger},
		{"random_chance", r.randomChance},
		{"bonuses_add", r.bonusesAdd},
		{"bonuses_remove", r.bonusesRemove},
		{"timers_bonus_start", r.timersBonusStart},
		{"timers_bonus_done", r.timersBonusDone},
		{"jobs_test", r.jobsTest},
		{"jobs_id", r.jobsID},
		{"jobs_next", r.jobsNext},
		{"player_change_job", r.playerChangeJob},
		{"player_rebirth", r.playerRebirth},
		{"player_reset", r.playerReset},
		{"player_reset_skill", r.playerResetSkill},
		{"player_reset_stat", r.playerResetStat},
		{"constants", r.constantsTable},
		{"npc_field", r.npcField},
		{"npc_emotion", r.npcEmotion},
		{"npc_say", r.npcSay},
		{"conversation_say", r.conversationSay},
		{"conversation_next", r.conversationNext},
		{"conversation_choose", r.conversationChoose},
		{"conversation_ask_number", r.conversationAskNumber},
		{"conversation_ask_text", r.conversationAskText},
		{"conversation_close", r.conversationClose},
		{"conversation_wait", r.conversationWait},
		{"conversation_image", r.conversationImage},
		{"conversation_progress", r.conversationProgress},
		{"player_pay_zeny", r.playerPayZeny},
		{"player_give_zeny", r.playerGiveZeny},
		{"player_take_item", r.playerTakeItem},
		{"player_can_carry", r.playerCanCarry},
		{"player_set_save_point", r.playerSetSavePoint},
		{"player_mark_minimap", r.playerMarkMinimap},
		{"player_open_shop", r.playerOpenShop},
		{"items_name", r.itemsName},
		{"text", r.text},
		{"text_item_link", r.textItemLink},
		{"announce", r.announce},
		{"variables_declare", r.variablesDeclare},
		{"variable_get", r.variableGet},
		{"variable_set", r.variableSet},
		{"variable_add", r.variableAdd},
		{"hooks_on", r.hooksOn},
		{"hooks_remove", r.hooksRemove},
		{"hook_enter", r.hookEnter},
		{"hook_failed", r.hookFailed},
		{"schedules_add", r.schedulesAdd},
		{"schedules_when", r.schedulesWhen},
		{"schedules_cancel", r.schedulesCancel},
		{"spawning_monsters", r.spawningMonsters},
		{"maps_find_entities", r.mapsFindEntities},
		{"entity_exists", r.entityExists},
		{"entity_show_effect", r.entityShowEffect},
		{"cast_deal_damage", r.castDealDamage},
		{"cast_heal", r.castHeal},
		{"cast_run_default", r.castRunDefault},
		{"monster_set_target", r.monsterSetTarget},
		{"monster_clear_target", r.monsterClearTarget},
		{"monster_walk_to", r.monsterWalkTo},
		{"monster_use_skill", r.monsterUseSkill},
		{"monster_talk", r.monsterTalk},
		{"monster_stop", r.monsterStop},
	}
	if r.opts.Testing {
		host = append(host, struct {
			name string
			fn   script.HostFunc
		}{"testing_world", testingWorld})
	}
	var fields []script.Field
	for _, h := range host {
		f, err := r.s.Func(h.name, h.fn)
		if err != nil {
			return err
		}
		fields = append(fields, script.KV(h.name, f))
	}
	res, err := r.s.Load("=modscript", "return function(...) "+prelude+"\nend")
	if err != nil {
		return fmt.Errorf("modscript prelude: %w", err)
	}
	t, err := r.s.Call(res[0], script.NewTable(fields...))
	if err != nil {
		return fmt.Errorf("modscript prelude: %w", err)
	}
	tab, ok := t[0].AsTable()
	if !ok {
		return errors.New("modscript prelude returned no table")
	}
	for _, f := range []struct {
		name string
		to   **script.Ref
	}{
		{"load_entry", &r.fns.loadEntry},
		{"invoke", &r.fns.invoke},
		{"drop_mod", &r.fns.dropMod},
		{"drop_file", &r.fns.dropFile},
		{"reset_loading", &r.fns.resetLoading},
		{"is_protected", &r.fns.isProtected},
		{"converse", &r.fns.converse},
		{"load_tests", &r.fns.loadTests},
		{"run_case", &r.fns.runCase},
		{"describe", &r.fns.describe},
		{"run_hooks", &r.fns.runHooks},
		{"check_condition", &r.fns.condition},
		{"status_bonuses", &r.fns.statusBonuses},
	} {
		ref, err := r.s.Keep(tab.Get(f.name))
		if err != nil {
			return fmt.Errorf("modscript prelude: %s: %w", f.name, err)
		}
		*f.to = ref
	}
	msg, err := r.s.Call1(tab.Get("closed_message"))
	if err != nil {
		return fmt.Errorf("modscript prelude: %w", err)
	}
	r.closed, _ = msg.AsString()
	return nil
}

// Close ends every context and frees the Lua state.
func (r *Runtime) Close() {
	for _, conv := range r.conversations {
		conv.Done = true
	}
	for _, n := range r.npcs {
		if n.ctx != nil {
			n.ctx.conversations = nil
			r.end(n.ctx)
		}
	}
	for _, id := range r.order {
		if c := r.contexts[id]; c != nil {
			r.end(c)
		}
	}
	for _, c := range r.events {
		r.end(c)
	}
	r.endSkills()
	for id, c := range r.monsterCtxs {
		delete(r.monsterCtxs, id)
		r.end(c)
	}
	for k, s := range r.setups {
		s.Release()
		delete(r.setups, k)
	}
	r.s.Close()
}

// State returns the zone's Lua state, for tests.
func (r *Runtime) State() *script.State { return r.s }

// Context returns a mod's context in this zone, or nil when the mod has
// none, such as after its scripts failed.
func (r *Runtime) Context(modID string) *Context { return r.contexts[modID] }

// start loads a mod's entry scripts and runs each setup function in the
// mod's context.
func (r *Runtime) start(sc *mod.Scripts) error {
	if len(sc.Entries) == 0 {
		return nil
	}
	c := &Context{ID: r.newID(), Kind: KindMod, Mod: sc.Mod, grants: sc.Grants, timers: map[int64]*timer{}}
	c.Instance = script.NewTable(
		script.KV("id", script.String(sc.Mod)),
		script.KV("map", script.String(r.host.Map())),
	)
	var setups []*script.Ref
	defer func() {
		for _, s := range setups {
			s.Release()
		}
	}()
	r.loading = sc.Mod
	for _, e := range sc.Entries {
		fn, err := r.s.Call1(r.fns.loadEntry.Value(), script.String(sc.Mod), script.String(e), script.String(sc.Files[e]))
		if err != nil {
			r.loading = ""
			r.afterError()
			return err
		}
		ref, err := r.s.Keep(fn)
		if err != nil {
			r.loading = ""
			return err
		}
		setups = append(setups, ref)
	}
	r.loading = ""
	r.contexts[sc.Mod] = c
	for i, s := range setups {
		if err := r.run(c, s.Value()); err != nil {
			r.end(c)
			return fmt.Errorf("%s/%s: setup: %w", sc.Mod, sc.Entries[i], err)
		}
	}
	return nil
}

// Reload loads a mod's scripts again from a new snapshot: its context in
// this zone ends, which removes its handlers and timers, and the new
// entry scripts run in a new context. Other mods that required the
// mod's helper modules keep the versions they loaded. A reload that
// fails leaves the mod off in this zone until a reload succeeds.
func (r *Runtime) Reload(sc *mod.Scripts) error {
	if old := r.contexts[sc.Mod]; old != nil {
		r.end(old)
	}
	if _, err := r.s.Call(r.fns.dropMod.Value(), script.String(sc.Mod)); err != nil {
		return err
	}
	for k, s := range r.setups {
		if strings.HasPrefix(k, sc.Mod+":") {
			s.Release()
			delete(r.setups, k)
		}
	}
	r.setupOrder = slices.DeleteFunc(r.setupOrder, func(k string) bool { return strings.HasPrefix(k, sc.Mod+":") })
	if _, ok := r.scripts[sc.Mod]; !ok {
		r.order = append(r.order, sc.Mod)
	}
	r.scripts[sc.Mod] = sc
	err := r.start(sc)
	r.reloadMonsters(sc.Mod)
	return errors.Join(err, r.reloadNPCs(sc.Mod), r.reloadSkills(sc.Mod))
}

// end removes everything a context owns. An NPC context with
// conversations that still run retires instead: it ends with the last
// of them.
func (r *Runtime) end(c *Context) {
	if c.ended {
		return
	}
	if len(c.conversations) > 0 {
		c.retiring = true
	} else {
		c.ended = true
	}
	for _, h := range c.handlers {
		r.removeHandler(h)
	}
	c.handlers = nil
	for _, t := range c.timers {
		t.canceled = true
		r.endTimerBonuses(t)
		if t.fn != nil {
			t.fn.Release()
		}
		delete(r.timers, t.id)
	}
	c.timers = nil
	for _, b := range c.bonuses {
		r.dropBonus(b)
	}
	c.bonuses = nil
	for _, h := range slices.Clone(c.hooks) {
		r.removeHook(h)
	}
	for _, sc := range c.schedules {
		r.dropSchedule(sc)
	}
	if c.spawned {
		c.spawned = false
		if h, ok := r.host.(SpawnHost); ok {
			h.Despawn(c.ID)
		}
	}
	if r.contexts[c.Mod] == c {
		delete(r.contexts, c.Mod)
	}
}

func (r *Runtime) removeHandler(h *handler) {
	if h.removed {
		return
	}
	h.removed = true
	h.fn.Release()
	delete(r.byID, h.id)
	list := r.handlers[h.event]
	if i := slices.Index(list, h); i >= 0 {
		r.handlers[h.event] = slices.Delete(list, i, i+1)
	}
}

func (r *Runtime) newID() int64 {
	r.nextID++
	return r.nextID
}

// run calls fn in a context. Errors are returned, and count toward the
// context's failures when it is a handler or timer.
func (r *Runtime) run(c *Context, fn script.Value, args ...script.Value) error {
	prev := r.current
	r.current = c
	_, err := r.s.Call(fn, args...)
	r.current = prev
	if t := r.group; t != nil {
		// A limit stopped a with_bonuses function, which pcall cannot
		// catch: its bonuses end.
		r.group = nil
		r.dropTimer(t)
		r.endTimerBonuses(t)
	}
	if err != nil {
		r.afterError()
	}
	return err
}

// afterError clears files a stopped call left half loaded. It does
// nothing after a nested call, since the outer call still runs. A limit
// that stops a nested call stops the outer call too, and the outer
// call's error clears them.
func (r *Runtime) afterError() {
	if r.s.InHost() {
		return
	}
	if _, err := r.s.Call(r.fns.resetLoading.Value()); err != nil {
		r.log.Error("modscript: reset after an error failed", "err", err)
	}
}

// failed records a handler failure, and ends the context when it fails
// too often.
func (r *Runtime) failed(c *Context, what string, err error) {
	r.log.Warn("mod handler failed", "map", r.host.Map(), "mod", c.Mod, "kind", c.Kind, "in", what, "err", err)
	now := r.host.Now()
	c.failures = append(c.failures, now)
	for len(c.failures) > 0 && c.failures[0] <= now-FailureWindow {
		c.failures = c.failures[1:]
	}
	if len(c.failures) >= MaxFailures && !c.ended {
		r.log.Error("mod failed too often and is off in this zone until it is reloaded", "map", r.host.Map(), "mod", c.Mod,
			"failures", len(c.failures), "window_ms", FailureWindow)
		r.end(c)
	}
}

// Arg is one argument of an event.
type Arg struct {
	kind string
	v    script.Value
}

// Player is an argument that becomes a player handle.
func Player(id int64) Arg { return Arg{"player", script.Int(id)} }

// Value is an argument passed as it is.
func Value(v script.Value) Arg { return Arg{"value", v} }

// Monster is an argument that becomes a monster handle. id is the
// zone's handle ID for the monster.
func Monster(id int64) Arg { return Arg{"monster", script.Int(id)} }

// JobChange is the object of the job_change event: { player, from, to }
// (review decision 24). player is the character's ID.
func JobChange(player, from, to int64) Arg {
	return Arg{"job_change", script.NewTable(
		script.KV("player", script.Int(player)), script.KV("from", script.Int(from)), script.KV("to", script.Int(to)))}
}

// Fire runs every handler of an event, in registration order. A handler
// that fails is logged and does not stop the others. Handlers added
// while the event runs wait for the next one.
func (r *Runtime) Fire(event string, args ...Arg) {
	list := r.handlers[event]
	if len(list) == 0 {
		return
	}
	list = slices.Clone(list)
	vals := make([]script.Value, 0, 1+2*len(args))
	for _, h := range list {
		if h.removed || h.ctx.ended {
			continue
		}
		vals = append(vals[:0], h.fn.Value())
		for _, a := range args {
			vals = append(vals, script.String(a.kind), a.v)
		}
		if err := r.run(h.ctx, r.fns.invoke.Value(), vals...); err != nil {
			r.failed(h.ctx, "event "+event, err)
		}
	}
}

// Advance runs the timers that are due at now, in zone milliseconds, in
// order of due time.
func (r *Runtime) Advance(now int64) {
	var due []*timer
	for _, t := range r.timers {
		if t.due <= now {
			due = append(due, t)
		}
	}
	if len(due) == 0 {
		return
	}
	sort.Slice(due, func(i, j int) bool {
		if due[i].due != due[j].due {
			return due[i].due < due[j].due
		}
		return due[i].id < due[j].id
	})
	for _, t := range due {
		if t.canceled || t.ctx.ended {
			continue
		}
		if t.fn == nil {
			// A with_bonuses timer: its bonuses end.
			r.dropTimer(t)
			r.endTimerBonuses(t)
			continue
		}
		if t.every > 0 {
			// A repeating timer keeps its rhythm, and skips runs it
			// missed rather than running them all at once.
			t.due += t.every
			if t.due <= now {
				t.due = now + t.every
			}
		} else {
			r.dropTimer(t)
		}
		if err := r.run(t.ctx, t.fn.Value()); err != nil {
			r.failed(t.ctx, "timer", err)
		}
		if t.every == 0 {
			t.fn.Release()
		}
	}
}

// Shift moves every timer later, for time the zone spent asleep. Timers
// follow the zone clock, which stops while a zone has no players.
func (r *Runtime) Shift(ms int64) {
	for _, t := range r.timers {
		t.due += ms
	}
}

func (r *Runtime) dropTimer(t *timer) {
	t.canceled = true
	delete(r.timers, t.id)
	delete(t.ctx.timers, t.id)
}

// Kinds of context (proposal 0002). Mod, NPC, item, skill and event
// contexts exist.
const (
	KindMod     = "mod"
	KindNPC     = "npc"
	KindItem    = "item"
	KindSkill   = "skill"
	KindMonster = "monster"
	KindEvent   = "event"
)

// Events lists every event name and the context kinds that can listen
// to it. touch is a name the next steps fire, listed so a typo is told
// apart from an event of another kind.
var Events = map[string][]string{
	"login":         {KindMod, KindEvent},
	"logout":        {KindMod, KindEvent},
	"map_enter":     {KindMod, KindEvent},
	"death":         {KindMod, KindNPC, KindEvent},
	"base_level_up": {KindMod, KindEvent},
	"job_level_up":  {KindMod, KindEvent},
	// job_change follows a job change (review decision 24).
	"job_change": {KindMod, KindEvent},
	"click":      {KindNPC},
	"touch":      {KindNPC},
	"use":        {KindItem},
	"damage":     {KindItem},
	// skill_used is an item event, and an event of mod and event
	// contexts (review decision 18).
	"skill_used": {KindItem, KindMod, KindEvent},
	// cast runs in a skill's context (review decision 20).
	"cast": {KindSkill},
	// Converted UnEquipScripts run on unequip.
	"unequip": {KindItem},
	// Status effects (review decision 19).
	"status_start": {KindMod, KindEvent},
	"status_end":   {KindMod, KindEvent},
	"status_tick":  {KindMod, KindEvent},
}

// checkEvent checks an event name for a context kind.
func checkEvent(name, kind string) error {
	kinds, ok := Events[name]
	if !ok {
		var names []string
		for n, ks := range Events {
			if slices.Contains(ks, kind) {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		return fmt.Errorf("events.on: there is no event %q; a %s context can listen to %s", name, kind, strings.Join(names, ", "))
	}
	if !slices.Contains(kinds, kind) {
		return fmt.Errorf("events.on: %q is an event of %s contexts, and this is a %s context", name, strings.Join(kinds, " and "), kind)
	}
	return nil
}
