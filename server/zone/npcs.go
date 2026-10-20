package zone

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/internal/script"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/shop"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// NPC conversations, as rAthena's npc_click and npc_scriptcont:
//   - A click reaches an NPC within npcClickRange cells on both axes
//     (npc_checknear). A dead player, or one already talking, clicks
//     nothing.
//   - An NPC with a click handler starts a conversation. An NPC without
//     one opens its shop, when it has one.
//   - While a player talks, it cannot walk, attack, pick up, use or drop
//     items (pc_cant_act). A walk during a progress bar cancels it and
//     ends the conversation, as clif_progressbar_abort.
//   - Each step that waits gets a new dialog ID. An answer counts only
//     when it quotes that ID, fits the step, and the NPC is still in
//     range. Other answers are dropped, as npc_scriptcont drops them.
//   - Steps time out as rAthena's SECURE_NPCTIMEOUT: 60 seconds for
//     next, a menu and close, and 180 seconds for input.
//   - A conversation closes when the player leaves the zone, dies, or
//     is warped.

// npcClickRange is how far a player reaches an NPC: AREA_SIZE + 1
// (npc_checknear).
const npcClickRange = ViewRange + 1

// Conversation timeouts, in milliseconds (conf/secure.hpp).
const (
	timeoutNext  = 60_000
	timeoutMenu  = 60_000
	timeoutInput = 180_000
)

// talk is a player's conversation with an NPC.
type talk struct {
	conv *modscript.Conversation
	npc  *entity
	// dialog is the ID of the step that waits for an answer, and step
	// what it waits for. waiting is false while no step waits.
	dialog  uint32
	step    modscript.Step
	waiting bool
	// deadline is when the waiting step times out, and wake when a wait
	// or progress bar ends, in zone milliseconds. 0 is none.
	deadline, wake int64
	// shown is set while the dialog window shows lines.
	shown bool
	// ended is set when the script finished and the window shows its
	// Close button. The player still talks until it presses it.
	ended bool
	// teleport is set for a talk with the teleport service, which the
	// server runs without a script, and conv is nil then (worldmap.go).
	teleport *teleportTalk
	// items is whether the player may use and equip items during the
	// talk: itemsDefault, itemsLocked or itemsUnlocked, as disable_items
	// and enable_items set (playeractions.go).
	items int
}

type npcEmotion struct {
	e       *entity
	emotion uint32
}

// npcByHandle finds an NPC by its script handle.
func (z *Zone) npcByHandle(id int64) *entity {
	return z.npcHandles[id]
}

// startNPCs runs the scripts of the zone's NPCs, the first time a player
// enters. A zone nobody visits runs none.
func (z *Zone) startNPCs() {
	if z.npcsStarted {
		return
	}
	z.npcsStarted = true
	// The world counts the zone's labels once its NPCs have registered
	// them: until then, events for every NPC still come to it.
	if d := z.npcDir(); d != nil {
		defer d.markStarted(z)
	}
	var npcs []*entity
	for _, e := range z.npcsInOrder() {
		if e.npc.Script != "" && !e.npc.removed {
			npcs = append(npcs, e)
		}
	}
	if len(npcs) == 0 {
		return
	}
	r := z.itemScripts()
	if r == nil {
		return
	}
	for _, e := range npcs {
		n, err := r.StartNPC(e.npc.handle, e.npc.Script)
		if err != nil {
			if z.Mods != nil && z.Mods.Log != nil {
				z.Mods.Log.Warn("NPC script failed to start", "map", z.Map, "npc", e.npc.Name, "script", e.npc.Script, "err", err)
			}
			continue
		}
		e.npc.script = n
		e.npc.setScriptState()
	}
}

// npcLabel keeps the world's directory of an NPC's event labels.
func (z *Zone) npcLabel(handle int64, label string, add bool) {
	e := z.npcHandles[handle]
	d := z.npcDir()
	if e == nil || d == nil || e.npc.entry == nil {
		return
	}
	d.label(e.npc.entry, label, add)
}

// near is npc_checknear.
func near(p *Player, e *entity) bool {
	me := p.entity
	return abs(e.x-me.x) <= npcClickRange && abs(e.y-me.y) <= npcClickRange
}

// rkEnchantBlade is RK_ENCHANTBLADE, the first third class skill.
const rkEnchantBlade = 2001

// busyText is the text of MSI_BUSY.
const busyText = "You cannot do that now: you are busy with another action."

// busy is clif_msg with MSI_BUSY, which only renewal sends: the
// busy_restrictions flag.
func (z *Zone) busy(p *Player) {
	if z.rules().BusyRestrictions {
		p.pending = append(p.pending, systemMessage(busyText))
	}
}

// click is npc_click, after clif_parse_NpcClicked's checks.
func (z *Zone) click(p *Player, netID uint32) {
	// npc_click refuses PCBLOCK_NPCCLICK (npc.cpp 2232).
	if p.entity.dead || p.isBlocked(blockNPCClick) {
		return
	}
	if p.talk != nil {
		z.busy(p)
		return
	}
	// With busy_restrictions on, a player who casts a skill below the
	// third class skills cannot click an NPC.
	if c := p.entity.cast; c != nil && z.rules().BusyRestrictions && c.def.ID < rkEnchantBlade {
		z.busy(p)
		return
	}
	var e *entity
	for o, id := range p.netIDs.all() {
		if id == netID {
			e = o
			break
		}
	}
	if e == nil || e.npc == nil || !z.entities.has(e) || !near(p, e) || !e.npc.clickable() {
		return
	}
	n := e.npc
	if n.hint != 0 {
		z.hintTalk(p, e)
		return
	}
	if n.Service == ServiceTeleport && z.atlas() != nil {
		z.closeShop(p, false)
		p.entity.target, p.entity.pickItem = nil, nil
		z.startTeleport(p, e)
		return
	}
	if n.script != nil && n.script.Clickable() {
		z.closeShop(p, false)
		p.entity.target, p.entity.pickItem = nil, nil
		conv, err := z.scripts.Click(n.script, int64(p.charID))
		if err != nil {
			z.Mods.Log.Warn("NPC click failed", "map", z.Map, "npc", n.Name, "err", err)
			return
		}
		if conv != nil && !conv.Done && p.talk == nil {
			p.talk = &talk{conv: conv, npc: e}
		}
		z.afterTalk(p)
		return
	}
	if n.Shop != nil {
		z.openShop(p, n.Shop, e)
	}
}

// talkOf returns the conversation of a player, made when the script
// shows its first step.
func (z *Zone) talkOf(conv *modscript.Conversation) (*Player, *talk) {
	if conv.Player <= 0 || conv.Player > int64(^uint32(0)) {
		return nil, nil
	}
	p := z.players[uint32(conv.Player)]
	if p == nil {
		return nil, nil
	}
	if p.talk == nil {
		p.talk = &talk{conv: conv, npc: z.npcByHandle(conv.NPC)}
	}
	if p.talk.conv != conv {
		return nil, nil
	}
	return p, p.talk
}

// npcNetID is the player's ID for the NPC of a conversation.
func (p *Player) npcNetID(t *talk) uint32 {
	if t.npc == nil {
		return 0
	}
	id, _ := p.netIDs.get(t.npc)
	return id
}

func (z *Zone) dialog(p *Player, t *talk, build func(b *wathenav1.NpcDialog_builder)) {
	b := wathenav1.NpcDialog_builder{Npc: p.npcNetID(t), Dialog: t.dialog}
	build(&b)
	p.pending = append(p.pending, wathenav1.Event_builder{NpcDialog: b.Build()}.Build())
}

// converse shows a conversation step to its player.
func (z *Zone) converse(conv *modscript.Conversation, s modscript.Step) error {
	p, t := z.talkOf(conv)
	if p == nil {
		return errors.New("the player has left this map")
	}
	switch s.Kind {
	case modscript.StepSay:
		t.shown = true
		z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) { b.Lines = s.Lines })
		return nil
	case modscript.StepImage:
		pos := map[string]wathenav1.ImagePosition{
			"left": wathenav1.ImagePosition_IMAGE_POSITION_LEFT, "center": wathenav1.ImagePosition_IMAGE_POSITION_CENTER,
			"right": wathenav1.ImagePosition_IMAGE_POSITION_RIGHT,
		}[s.Position]
		p.pending = append(p.pending, wathenav1.Event_builder{NpcImage: wathenav1.NpcImage_builder{Name: s.Image, Position: pos}.Build()}.Build())
		return nil
	}
	p.dialogSeq++
	t.dialog, t.step, t.waiting = p.dialogSeq, s, true
	t.deadline, t.wake = 0, 0
	switch s.Kind {
	case modscript.StepNext:
		t.deadline = z.now + timeoutNext
		z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) { b.Next = new(true) })
	case modscript.StepMenu:
		t.deadline = z.now + timeoutMenu
		z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) {
			b.Menu = wathenav1.NpcMenu_builder{Options: s.Options}.Build()
		})
	case modscript.StepNumber:
		t.deadline = z.now + timeoutInput
		z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) {
			b.InputNumber, b.NumberMin, b.NumberMax = new(true), s.Min, s.Max
		})
	case modscript.StepText:
		t.deadline = z.now + timeoutInput
		z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) {
			b.InputText, b.TextMax = new(true), uint32(s.MaxLength) // #nosec G115 -- at most MaxTextInput.
		})
	case modscript.StepClose:
		t.deadline = z.now + timeoutNext
		z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) { b.Close = new(true) })
	case modscript.StepWait:
		t.wake = z.now + s.Ms
	case modscript.StepProgress:
		t.wake = z.now + s.Ms
		p.pending = append(p.pending, wathenav1.Event_builder{NpcProgress: wathenav1.NpcProgress_builder{
			DurationMs: uint32(s.Ms), Color: s.Color, // #nosec G115 -- at most MaxWait.
		}.Build()}.Build())
	}
	return nil
}

// conversationEnded cleans up after a conversation's thread finished.
// A script that ended with lines on show leaves the Close button, which
// the player presses to leave the conversation. Otherwise the window
// closes at once.
func (z *Zone) conversationEnded(conv *modscript.Conversation) {
	if conv.Player <= 0 || conv.Player > int64(^uint32(0)) {
		return
	}
	p := z.players[uint32(conv.Player)]
	if p == nil || p.talk == nil || p.talk.conv != conv {
		return
	}
	t := p.talk
	t.waiting, t.wake = false, 0
	if t.shown && !conv.Cancelled {
		p.dialogSeq++
		t.dialog, t.ended = p.dialogSeq, true
		t.deadline = z.now + timeoutNext
		z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) { b.Close = new(true) })
		return
	}
	z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) { b.End = new(true) })
	p.talk = nil
}

// closeTalk ends a player's conversation without its script, such as
// when the player leaves or a step times out.
func (z *Zone) closeTalk(p *Player) {
	t := p.talk
	if t == nil {
		return
	}
	if t.ended || t.conv == nil || t.conv.Done {
		z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) { b.End = new(true) })
		p.talk = nil
		return
	}
	z.scripts.CloseConversation(t.conv)
	if p.talk == t {
		p.talk = nil
	}
}

// reply is npc_scriptcont: the player's answer to the waiting step.
func (z *Zone) reply(p *Player, r *wathenav1.NpcReply) {
	t := p.talk
	if t == nil || r.GetDialog() != t.dialog {
		return
	}
	if t.npc != nil && (!z.entities.has(t.npc) || !near(p, t.npc)) {
		return
	}
	if t.ended {
		if r.HasClose() {
			p.talk = nil
		}
		return
	}
	if !t.waiting {
		return
	}
	var answer script.Value
	switch t.step.Kind {
	case modscript.StepNext:
		switch {
		case r.HasNext():
		case r.HasClose():
			z.closeTalk(p)
			return
		default:
			return
		}
	case modscript.StepMenu:
		switch {
		case r.HasMenu():
			n := r.GetMenu()
			if n < 1 || int(n) > len(t.step.Options) || t.step.Options[n-1] == "" {
				return
			}
			answer = script.Int(int64(n))
		case r.HasClose():
			// A cancelled menu ends the script, as select's 255.
			z.closeTalk(p)
			return
		default:
			return
		}
	case modscript.StepNumber:
		switch {
		case r.HasInputNumber():
			n := r.GetInputNumber()
			if n < t.step.Min || n > t.step.Max {
				return
			}
			answer = script.Int(n)
		case r.HasClose():
			z.closeTalk(p)
			return
		default:
			return
		}
	case modscript.StepText:
		switch {
		case r.HasInputText():
			s := r.GetInputText()
			if !utf8.ValidString(s) || utf8.RuneCountInString(s) > t.step.MaxLength {
				return
			}
			answer = script.String(s)
		case r.HasClose():
			z.closeTalk(p)
			return
		default:
			return
		}
	case modscript.StepClose:
		if !r.HasClose() {
			return
		}
		// close2: the window closes, and the script goes on.
		t.shown = false
	default:
		// Waits and progress bars take no answer.
		return
	}
	t.waiting, t.deadline = false, 0
	if t.teleport != nil {
		n, _ := answer.AsInt()
		z.teleportReply(p, t, int(n))
		return
	}
	z.scripts.ResumeConversation(t.conv, answer)
	z.afterTalk(p)
}

// afterTalk runs what a conversation step asked for once its thread
// paused or ended: a warp ends the conversation and moves the player.
func (z *Zone) afterTalk(p *Player) {
	if p.warpTo == nil {
		return
	}
	if p.talk != nil {
		z.closeTalk(p)
	}
	z.finishWarp(p)
}

// talkTimers resumes waits and progress bars that are over, and closes
// steps that timed out.
func (z *Zone) talkTimers() {
	for _, p := range z.players {
		t := p.talk
		if p.endTalk {
			p.endTalk = false
			z.closeTalk(p)
			continue
		}
		if t == nil {
			continue
		}
		switch {
		case t.wake > 0 && z.now >= t.wake && t.conv != nil:
			t.wake, t.waiting = 0, false
			z.scripts.ResumeConversation(t.conv, script.Nil)
			z.afterTalk(p)
		case t.deadline > 0 && z.now >= t.deadline:
			z.closeTalk(p)
		}
	}
}

// walkDuringTalk is a walk command while the player talks: it is
// ignored, except during a progress bar, which it cancels.
func (z *Zone) walkDuringTalk(p *Player) {
	if t := p.talk; t != nil && t.waiting && t.step.Kind == modscript.StepProgress {
		z.closeTalk(p)
	}
}

// emotionEvents are the emotions a player sees this tick.
func (z *Zone) emotionEvents(p *Player) []*wathenav1.Event {
	var events []*wathenav1.Event
	for _, em := range z.emotions {
		if id, ok := p.netIDs.get(em.e); ok {
			events = append(events, wathenav1.Event_builder{Emotion: wathenav1.Emotion_builder{Entity: id, Emotion: em.emotion}.Build()}.Build())
		}
	}
	return events
}

// The NPCHost side of scriptHost.

func (h scriptHost) npc(id int64) (*entity, error) {
	if id >= remoteBase {
		return nil, errors.New("the NPC is on another map: only its fields, its view and its events reach it")
	}
	e := h.z.npcByHandle(id)
	if e == nil || e.npc.removed || !e.npc.floating && !h.z.entities.has(e) {
		return nil, errors.New("the NPC is gone from this map")
	}
	return e, nil
}

// NPCField reads an NPC handle: name, map, x or y.
func (h scriptHost) NPCField(id int64, field string) (script.Value, error) {
	if id >= remoteBase {
		t, err := h.npcTarget(id)
		if err != nil {
			return script.Nil, err
		}
		return h.remoteField(t.entry, field)
	}
	e, err := h.npc(id)
	if err != nil {
		return script.Nil, err
	}
	read, ok := npcFields[field]
	if !ok {
		return script.Nil, fmt.Errorf("npc has no field %q", field)
	}
	return read(h, e), nil
}

// npcFields are the fields of NPC handles. server/modscript/luaapi
// defines the same names, and a test checks both.
var npcFields = map[string]func(h scriptHost, e *entity) script.Value{
	"kind": func(scriptHost, *entity) script.Value { return script.String("npc") },
	"name": func(_ scriptHost, e *entity) script.Value { return script.String(e.npc.DisplayName) },
	"full_name": func(_ scriptHost, e *entity) script.Value {
		_, _, full := npcNames(e.npc.Name, e.npc.DisplayName, e.npc.Hidden)
		return script.String(full)
	},
	"hidden_name": func(_ scriptHost, e *entity) script.Value {
		_, hidden, _ := npcNames(e.npc.Name, e.npc.DisplayName, e.npc.Hidden)
		return script.String(hidden)
	},
	"unique_name": func(_ scriptHost, e *entity) script.Value {
		unique, _, _ := npcNames(e.npc.Name, e.npc.DisplayName, e.npc.Hidden)
		return script.String(unique)
	},
	"map": func(h scriptHost, e *entity) script.Value {
		if e.npc.floating {
			// An NPC without a map, as strnpcinfo(4) reads it (nd->m < 0).
			return script.String("")
		}
		return script.String(h.Map())
	},
	"x":     func(_ scriptHost, e *entity) script.Value { return intValue(e.x) },
	"y":     func(_ scriptHost, e *entity) script.Value { return intValue(e.y) },
	"state": func(_ scriptHost, e *entity) script.Value { return script.String(e.npc.state()) },
}

// NPCEmotion shows an emotion over an NPC to everyone who sees it, as
// clif_emotion. An NPC of another map shows it in its zone after the
// current step (review decision 58).
func (h scriptHost) NPCEmotion(id int64, emotion int64) error {
	return h.onNPC(id, false, func(z *Zone, e *entity) {
		if e.npc.floating {
			// An NPC without a map has no place to show it (nd->m < 0).
			return
		}
		z.emotions = append(z.emotions, npcEmotion{e, uint32(emotion)}) // #nosec G115 -- checked by the runtime.
	})
}

// NPCSay is npctalk: a line from the NPC to every player who sees it, as
// AREA (npcpresent.go).
func (h scriptHost) NPCSay(id int64, text string) error {
	return h.SayAloud(modscript.Entity{Kind: "npc", ID: id}, text, modscript.SendTarget{To: "area"}, -1)
}

// Converse shows a conversation step.
func (h scriptHost) Converse(conv *modscript.Conversation, s modscript.Step) error {
	return h.z.converse(conv, s)
}

// ConversationEnded cleans up after a conversation.
func (h scriptHost) ConversationEnded(conv *modscript.Conversation) { h.z.conversationEnded(conv) }

// PayZeny is pc_payzeny.
func (h scriptHost) PayZeny(id, amount int64) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	if p.record.Zeny < amount {
		return false, nil
	}
	return h.z.payZeny(p, amount, transfer.ActionTake, transfer.End(transfer.KindScript)), nil
}

// GiveZeny is pc_getzeny: zeny over MAX_ZENY is lost.
func (h scriptHost) GiveZeny(id, amount int64) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	h.z.earnZeny(p, min(amount, character.MaxZeny), transfer.ActionGive, transfer.End(transfer.KindScript))
	return nil
}

// TakeItem is delitem: the amount leaves the first slots that hold the
// item, in slot order. A player with too few loses nothing.
func (h scriptHost) TakeItem(id int64, itemID uint32, amount int) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	if p.inventory.Count(itemID) < amount {
		return false, nil
	}
	for slot := range p.inventory.Slots {
		if amount == 0 {
			break
		}
		s := p.inventory.Slots[slot]
		if s.ID != itemID || s.Amount <= 0 {
			continue
		}
		n := min(s.Amount, amount)
		h.z.deleteItem(p, slot, n, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_TAKEN)
		amount -= n
	}
	return true, nil
}

// CanCarry is checkweight for one item.
func (h scriptHost) CanCarry(id int64, itemID uint32, amount int) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	d := h.z.Items.Get(itemID)
	if p.inventory.Weight+d.Weight*amount > p.maxWeight {
		return false, nil
	}
	switch checkAddItem(p.inventory, d, amount) {
	case addNew:
		need := 1
		if !d.Stackable {
			need = amount
		}
		return p.inventory.Blank() >= need, nil
	case addOverAmount:
		return false, nil
	}
	return true, nil
}

// SetSavePoint is savepoint: the map must run in the world, and the cell
// be walkable.
func (h scriptHost) SetSavePoint(id int64, mapName string, x, y int) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	asset := "map/" + strings.TrimPrefix(mapName, "map/")
	to := h.z.zoneOf(asset)
	if to == nil || to.grid.NoPass(x, y) {
		return false, nil
	}
	p.record.SaveMap, p.record.SaveX, p.record.SaveY = asset, x, y
	return true, nil
}

// MarkMinimap is viewpoint.
func (h scriptHost) MarkMinimap(id int64, mark int64, x, y int, color uint32, remove bool) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	p.pending = append(p.pending, wathenav1.Event_builder{MinimapMark: wathenav1.MinimapMark_builder{
		Id: uint32(mark), X: uint32(x), Y: uint32(y), Color: color, Remove: remove, // #nosec G115 -- checked by the runtime.
	}.Build()}.Build())
	return nil
}

// OpenShop is callshop: a shop by its record name. A shop that a script
// opens is not tied to an NPC's place.
func (h scriptHost) OpenShop(id int64, name string) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	var s *shop.Shop
	if h.z.Mods != nil {
		s = h.z.Mods.Shops.Get(name)
	}
	if s == nil {
		return fmt.Errorf("player:open_shop: there is no shop %q", name)
	}
	h.z.openShop(p, s, nil)
	return nil
}

// maxVariables is the most variables of each kind one player may hold.
const maxVariables = 4096

// varMap returns the map of a player's variables that holds a scope.
func (p *Player) varMap(scope string) map[character.Variable]string {
	if scope == "character_temporary" {
		if p.temp == nil {
			p.temp = map[character.Variable]string{}
		}
		return p.temp
	}
	if p.vars == nil {
		p.vars = map[character.Variable]string{}
	}
	return p.vars
}

// Variable reads a player's variable.
func (h scriptHost) Variable(id int64, v modscript.VariableKey) (string, bool, error) {
	p, err := h.player(id)
	if err != nil {
		return "", false, err
	}
	s, ok := p.varMap(v.Scope)[character.Variable{Mod: v.Mod, Name: v.Name, Account: v.Scope == "account"}]
	return s, ok, nil
}

// SetVariable changes a player's variable. Saved ones are written with
// the character's next save.
func (h scriptHost) SetVariable(id int64, v modscript.VariableKey, value string) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	m := p.varMap(v.Scope)
	k := character.Variable{Mod: v.Mod, Name: v.Name, Account: v.Scope == "account"}
	if value == "" {
		delete(m, k)
		return nil
	}
	old, ok := m[k]
	if !ok && len(m) >= maxVariables {
		return fmt.Errorf("the player holds %d variables, the most it may", maxVariables)
	}
	if len(value) > len(old) {
		// A mod's share of the player's variables is limited too, so one
		// mod cannot fill the player's record (review decision 52).
		used := len(value)
		for k2, v2 := range m {
			if k2.Mod == v.Mod && k2 != k {
				used += len(v2)
			}
		}
		if used > modscript.MaxModPlayerBytes {
			return fmt.Errorf("%s's variables on the player would take %d bytes, more than the %d a mod may", v.Mod, used, modscript.MaxModPlayerBytes)
		}
	}
	m[k] = value
	return nil
}

// ItemName is the item's name from the item database.
func (h scriptHost) ItemName(itemID uint32) (string, bool) {
	d, ok := h.z.Items.Lookup(itemID)
	if !ok {
		return "", false
	}
	if d.Name == "" {
		return d.AegisName, true
	}
	return d.Name, true
}

// Announce is announce and mapannounce. Players see mod with a mod's
// line.
func (h scriptHost) Announce(mapName, mod, text string) error {
	if mapName == "" {
		if h.z.World == nil {
			h.z.AnnounceFrom(text, 0, mod)
			return nil
		}
		h.z.World.AnnounceFrom(text, 0, mod)
		return nil
	}
	to := h.z.zoneOf("map/" + strings.TrimPrefix(mapName, "map/"))
	if to == nil {
		return fmt.Errorf("maps.announce: the map %s does not run in this world", mapName)
	}
	to.AnnounceFrom(text, 0, mod)
	return nil
}

// addCheck is pc_checkadditem's result.
type addCheck int

const (
	addExists addCheck = iota
	addNew
	addOverAmount
)

// checkAddItem is pc_checkadditem.
func checkAddItem(inv *item.Inventory, d *item.Def, amount int) addCheck {
	if amount > item.MaxAmount {
		return addOverAmount
	}
	if !d.Stackable {
		return addNew
	}
	if d.StackLimit > 0 && amount > d.StackLimit {
		return addOverAmount
	}
	if d.UniqueID {
		return addNew
	}
	for _, s := range inv.Slots {
		if s.ID == d.ID {
			if amount > item.MaxAmount-s.Amount || d.StackLimit > 0 && amount > d.StackLimit-s.Amount {
				return addOverAmount
			}
			return addExists
		}
	}
	return addNew
}

var _ modscript.NPCHost = scriptHost{}
