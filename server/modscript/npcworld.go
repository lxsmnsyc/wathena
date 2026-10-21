package modscript

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// The NPC API of review decision 58, areas B1 to B3 of proposal 0002b:
//   - B1: world.get_npc finds an NPC of any zone by record name, and
//     maps.find_player and maps.find_player_by_id find a player of the
//     zone.
//   - B2: an NPC handle shows, hides, cloaks and removes its NPC, as
//     rAthena's enablenpc and its aliases (script.cpp 12339,
//     npc_enable_target in npc.cpp 1036).
//   - B3: npc_event handlers run an NPC's named events, which
//     npc:fire_event and world.fire_npc_event send, as donpcevent and
//     doevent. An event goes to the target NPC's zone as a message and
//     runs after the current step.
//
// A handle may stand for an NPC of another zone: the host queues its
// changes to that zone. Reads of its fields come from the world's
// directory of NPCs.

// NPCWorldHost is an NPCHost that reaches NPCs across the world. A host
// without it finds only nothing, and the change calls raise an error.
type NPCWorldHost interface {
	// FindNPC finds an NPC by record name: a full name, such as
	// core:Guard#1, or a plain name, which is the mod's record, then
	// core's. It returns the NPC's handle in this zone. ok is false when
	// no NPC has the name, or it was removed.
	FindNPC(name, mod string) (id int64, ok bool)
	// NPCMod is the mod whose record placed an NPC.
	NPCMod(npc int64) (string, error)
	// NPCKey is an NPC's full record name, which keys its NPC variables.
	NPCKey(npc int64) (string, error)
	// ControlNPC changes an NPC's view: show, hide, set_hidden,
	// set_cloaked or remove. A change of an NPC of another zone runs in
	// its zone after the current step.
	ControlNPC(npc int64, c NPCControl) error
	// NPCCloaked reports whether an NPC is cloaked for a player, or for
	// everyone when player is 0.
	NPCCloaked(npc, player int64) (bool, error)
	// FireNPCEvent sends an event to an NPC: its npc_event handlers of
	// the label run after the current step, with the player when it is
	// not 0. It reports whether the NPC has a handler of the label, as
	// far as the world knows (decision 0062).
	FireNPCEvent(npc int64, label string, player int64) (bool, error)
	// BroadcastNPCEvent sends an event to every NPC with a handler of
	// the label, of the mods listed, or of every mod when mods is nil.
	// It returns how many NPCs the world knows to have one.
	BroadcastNPCEvent(label string, mods []string) (int, error)
	// FindPlayer and FindPlayerByID find an online player of this zone,
	// by name, or by account ID and else character ID.
	FindPlayer(name string) (int64, bool)
	FindPlayerByID(id int64) (int64, bool)
}

// NPCControl is a change of an NPC's view.
type NPCControl struct {
	// Op is show, hide, set_hidden, set_cloaked or remove.
	Op string
	// On is the value of set_hidden and set_cloaked.
	On bool
	// Player is the player of set_cloaked for one player, or 0 for
	// everyone.
	Player int64
}

// labelName is an event label's name: rAthena's labels are at most 23
// characters (NAME_LENGTH), and the event name with the NPC's at most 50
// (EVENT_NAME_LENGTH).
var labelName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,49}$`)

func (r *Runtime) npcWorld(what string) (NPCWorldHost, error) {
	h, ok := r.host.(NPCWorldHost)
	if !ok {
		return nil, fmt.Errorf("%s: NPCs of the world are not reachable here", what)
	}
	return h, nil
}

// controls checks that the running mod may change an NPC: its own mod's
// NPCs need no permission, and another mod's need npcs.control.
func (r *Runtime) controls(what string, h NPCWorldHost, npc int64) error {
	owner, err := h.NPCMod(npc)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if owner == r.actor() {
		return nil
	}
	return r.need(what+" on another mod's NPC", "npcs.control")
}

func (r *Runtime) worldGetNPC(c *script.Call) error {
	const what = "world.get_npc"
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	name, ok := c.Arg(0).AsString()
	if !ok || name == "" || len(name) > 128 {
		return errors.New(what + ": want an NPC's record name of data/npcs.yml")
	}
	h, ok := r.host.(NPCWorldHost)
	if !ok {
		return nil
	}
	if id, found := h.FindNPC(name, r.actor()); found {
		c.Return(script.Int(id))
	}
	return nil
}

func checkLabel(what string, v script.Value) (string, error) {
	s, ok := v.AsString()
	if !ok || !labelName.MatchString(s) {
		return "", fmt.Errorf("%s: want an event label, such as OnEnable: 1 to 50 letters, digits and _", what)
	}
	return strings.ToLower(s), nil
}

func (r *Runtime) worldFireNPCEvent(c *script.Call) error {
	const what = "world.fire_npc_event"
	if err := r.mutating(what); err != nil {
		return err
	}
	label, err := checkLabel(what, c.Arg(0))
	if err != nil {
		return err
	}
	h, err := r.npcWorld(what)
	if err != nil {
		return err
	}
	// Without npcs.control, the event reaches the mod's own NPCs.
	var mods []string
	if id := r.actor(); r.scripts[id] == nil || !r.scripts[id].Grants.Has("npcs.control") {
		mods = []string{id}
	}
	n, err := h.BroadcastNPCEvent(label, mods)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	c.Return(script.Int(int64(n)))
	return nil
}

// npcControl is the change methods of an NPC handle: Arg(0) is the
// handle, Arg(1) the method, Arg(2) the value and Arg(3) the player.
func (r *Runtime) npcControl(c *script.Call) error {
	id, err := npcID(c)
	if err != nil {
		return err
	}
	op := c.String(1)
	what := "npc:" + op
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.npcWorld(what)
	if err != nil {
		return err
	}
	if err := r.controls(what, h, id); err != nil {
		return err
	}
	ctl := NPCControl{Op: op}
	switch op {
	case "show", "hide", "remove":
	case "set_hidden", "set_cloaked":
		on, ok := c.Arg(2).AsBool()
		if !ok {
			return fmt.Errorf("%s: want true or false", what)
		}
		ctl.On = on
		if p := c.Arg(3); !p.IsNil() {
			if op != "set_cloaked" {
				return fmt.Errorf("%s takes no player", what)
			}
			if ctl.Player, ok = p.AsInt(); !ok {
				return fmt.Errorf("%s: want a player handle", what)
			}
		}
	default:
		return fmt.Errorf("npc has no method %q", op)
	}
	if err := h.ControlNPC(id, ctl); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

func (r *Runtime) npcIsCloaked(c *script.Call) error {
	const what = "npc:is_cloaked"
	id, err := npcID(c)
	if err != nil {
		return err
	}
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	h, err := r.npcWorld(what)
	if err != nil {
		return err
	}
	var player int64
	if p := c.Arg(1); !p.IsNil() {
		var ok bool
		if player, ok = p.AsInt(); !ok {
			return errors.New(what + ": want a player handle")
		}
	}
	cloaked, err := h.NPCCloaked(id, player)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	c.Return(script.Bool(cloaked))
	return nil
}

func (r *Runtime) npcFireEvent(c *script.Call) error {
	const what = "npc:fire_event"
	id, err := npcID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	label, err := checkLabel(what, c.Arg(1))
	if err != nil {
		return err
	}
	var player int64
	if p := c.Arg(2); !p.IsNil() {
		var ok bool
		if player, ok = p.AsInt(); !ok {
			return errors.New(what + ": the player option must be a player handle")
		}
	}
	h, err := r.npcWorld(what)
	if err != nil {
		return err
	}
	if err := r.controls(what, h, id); err != nil {
		return err
	}
	ran, err := h.FireNPCEvent(id, label, player)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	c.Return(script.Bool(ran))
	return nil
}

func (r *Runtime) mapsFindPlayer(c *script.Call) error {
	const what = "maps.find_player"
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	name, ok := c.Arg(0).AsString()
	if !ok {
		return errors.New(what + ": want a character name")
	}
	h, ok := r.host.(NPCWorldHost)
	if !ok {
		return nil
	}
	if id, found := h.FindPlayer(name); found {
		c.Return(script.Int(id))
	}
	return nil
}

func (r *Runtime) mapsFindPlayerByID(c *script.Call) error {
	const what = "maps.find_player_by_id"
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return errors.New(what + ": want a character or account ID")
	}
	h, ok := r.host.(NPCWorldHost)
	if !ok {
		return nil
	}
	if p, found := h.FindPlayerByID(id); found {
		c.Return(script.Int(p))
	}
	return nil
}

// npcEventHandlers are the npc_event handlers of an NPC's context for a
// label, in the order they were added.
func (r *Runtime) npcEventHandlers(c *Context, label string) []*handler {
	var out []*handler
	for _, h := range r.handlers["npc_event"] {
		if h.ctx == c && !h.removed && h.label == label {
			out = append(out, h)
		}
	}
	return out
}

// FireNPCEvent runs an NPC's npc_event handlers of a label, in lower
// case. Without a player, each handler runs on its own, as donpcevent's
// run_script with no player. With one, they run in a conversation with
// the player, as doevent's npc_event_sub, which the zone queues while
// the player talks: FireNPCEvent returns it, or nil when it ended. A
// disabled NPC runs no event with a player (npc.cpp 1767), and runs one
// without (npc_event_do_sub, npc.cpp 1284, does not check). ran reports
// whether a handler ran.
func (r *Runtime) FireNPCEvent(n *NPC, label string, player int64) (conv *Conversation, ran bool, err error) {
	c := n.Context()
	if c == nil {
		return nil, false, nil
	}
	label = strings.ToLower(label)
	list := r.npcEventHandlers(c, label)
	if len(list) == 0 {
		return nil, false, nil
	}
	if player > 0 {
		if n.Disabled {
			return nil, false, nil
		}
		vals := make([]script.Value, len(list))
		for i, h := range list {
			vals[i] = h.fn.Value()
		}
		conv, err := r.converse(n, c, player, "npc_event "+label, r.fns.npcEvent, []script.Value{script.String(label)}, vals)
		return conv, true, err
	}
	for _, h := range list {
		if h.removed || c.ended {
			continue
		}
		if err := r.run(c, r.fns.npcEvent.Value(), script.String(label), script.Nil, script.Nil, h.fn.Value()); err != nil {
			r.failed(c, "event npc_event "+label, err)
		}
	}
	return nil, true, nil
}

// FireNPCEventAway runs an NPC's npc_event handlers of a label with a
// player of another zone, as npc_event attaches a player of any map: the
// event has the player's handle and no conversation, since dialogs show
// on the player's own map. A disabled NPC runs no event with a player
// (npc.cpp 1767). It reports whether a handler ran.
func (r *Runtime) FireNPCEventAway(n *NPC, label string, player int64) bool {
	c := n.Context()
	if c == nil || n.Disabled {
		return false
	}
	label = strings.ToLower(label)
	list := r.npcEventHandlers(c, label)
	for _, h := range list {
		if h.removed || c.ended {
			continue
		}
		if err := r.run(c, r.fns.npcEvent.Value(), script.String(label), script.Int(player), script.Nil, h.fn.Value()); err != nil {
			r.failed(c, "event npc_event "+label, err)
		}
	}
	return len(list) > 0
}

// NPCEventLabels are the labels of an NPC's npc_event handlers, in lower
// case, sorted.
func (r *Runtime) NPCEventLabels(n *NPC) []string {
	c := n.Context()
	if c == nil {
		return nil
	}
	var out []string
	for _, h := range r.handlers["npc_event"] {
		if h.ctx == c && !h.removed && !slices.Contains(out, h.label) {
			out = append(out, h.label)
		}
	}
	slices.Sort(out)
	return out
}

// NPCListens reports whether an NPC's context has a handler of an
// event, such as monster_touch.
func (r *Runtime) NPCListens(n *NPC, event string) bool {
	c := n.Context()
	return c != nil && r.Listens(c, event)
}

// FireMonsterTouch runs an NPC's monster_touch handlers with a monster
// that walked into its touch area, as rAthena's OnTouchNPC
// (npc_touch_areanpc2, npc.cpp 1997). monster is the zone's handle ID.
func (r *Runtime) FireMonsterTouch(n *NPC, monster int64) {
	c := n.Context()
	if c == nil {
		return
	}
	for _, h := range slices.Clone(r.handlers["monster_touch"]) {
		if h.ctx != c || h.removed || c.ended {
			continue
		}
		if err := r.runHandler(c, r.fns.invoke.Value(), h.fn.Value(), script.String("monster"), script.Int(monster)); err != nil {
			r.failed(c, "event monster_touch", err)
		}
	}
}

// playerEvents are the events of one player that rAthena runs for every
// NPC with the label, but not for a disabled one (npc_script_event and
// npc_event_sub, npc.cpp 5824 and 1767). death is OnPCDieEvent when the
// victim is a player.
var playerEvents = map[string]bool{"login": true, "logout": true, "map_enter": true, "base_level_up": true, "job_level_up": true,
	"death": true}

// skipsDisabled reports whether a handler belongs to a disabled NPC that
// must not hear an event.
func skipsDisabled(h *handler, event string) bool {
	return h.ctx.Kind == KindNPC && h.ctx.npcRef != nil && h.ctx.npcRef.Disabled && playerEvents[event]
}

// labelChanged tells the host that an NPC gained or lost a handler of an
// event label.
func (r *Runtime) labelChanged(h *handler, add bool) {
	if h.event != "npc_event" || r.opts.NPCLabels == nil {
		return
	}
	r.opts.NPCLabels(h.ctx.npc, h.label, add)
}

// coreMod is core's mod ID, which keeps no variable limit.
const coreMod = mod.Core

// EndNPC ends an NPC's context for good, as rAthena's unloadnpc: its
// handlers, timers and schedules go. Conversations that run finish
// first.
func (r *Runtime) EndNPC(n *NPC) {
	if n.ctx != nil {
		r.end(n.ctx)
	}
	r.npcs = slices.DeleteFunc(r.npcs, func(x *NPC) bool { return x == n })
}

// DropNPCVariables forgets the NPC variables of an NPC, by its full
// record name, when it is removed.
func (r *Runtime) DropNPCVariables(npc string) { r.temp.DropNPC(npc) }

// HasNPCLabel reports whether the NPC of a handle has an npc_event
// handler of a label, in lower case. It works while the NPC's setup
// runs, before StartNPC returns.
func (r *Runtime) HasNPCLabel(npc int64, label string) bool {
	for _, h := range r.handlers["npc_event"] {
		if h.ctx.npc == npc && h.ctx.Kind == KindNPC && !h.removed && !h.ctx.ended && h.label == label {
			return true
		}
	}
	return false
}
