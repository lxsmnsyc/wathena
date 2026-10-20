package zone

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/internal/script"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// NPCs across zones (review decision 58, decision 0062):
//   - The world keeps a directory of every NPC: its zone, its handle in
//     that zone, its names and place, a copy of its view state, and the
//     event labels its handlers listen to. Zones read it under its lock.
//   - A script reaches an NPC of another zone through a handle the zone
//     makes for it, from remoteBase on. Its fields come from the
//     directory. Its changes and events go to the NPC's zone as messages.
//   - Each zone has an inbox of NPC messages. The zone runs them after
//     the current step, with its script events: an event that a script
//     of the same zone sent runs once that script's call returned, as
//     rAthena's donpcevent queues it. Messages from one zone keep their
//     order.
//   - A message starts the NPC scripts of a zone that has not started
//     them, so the NPCs of a map nobody visited still get their events.
//   - An NPC without a map (floating) runs in the start map's zone, as a
//     world event without a map (decision 16). It has no place: no view,
//     no click and no touch.

// remoteBase is the first handle of an NPC of another zone. Handles of
// the zone's own NPCs are smaller, hint NPCs' too (hintBase).
const remoteBase = int64(1) << 41

// Limits of the NPC inbox: the most messages one inbox holds, and the
// most a zone runs before it lets other work in.
const (
	maxNPCInbox    = 1 << 16
	npcMessageStep = 4096
)

// NPCDirectory is the world's list of NPCs. Its methods are safe for
// concurrent use.
type NPCDirectory struct {
	mu      sync.Mutex
	byName  map[string]*npcEntry
	byZone  map[*Zone][]*npcEntry
	started map[*Zone]bool
}

// NewNPCDirectory makes an empty directory.
func NewNPCDirectory() *NPCDirectory {
	return &NPCDirectory{byName: map[string]*npcEntry{}, byZone: map[*Zone][]*npcEntry{}, started: map[*Zone]bool{}}
}

// npcEntry is one NPC in the directory. The fields after the lock note
// change while the world runs: only the NPC's zone writes them, under
// the directory's lock.
type npcEntry struct {
	zone     *Zone
	handle   int64
	name     string
	mod      string
	scripted bool
	floating bool

	// Under the directory's lock. display, x and y change with
	// npc:set_display_name and the NPC's moves (npcpresent.go).
	display  string
	x, y     int
	state    string
	cloaked  bool
	cloakFor map[int64]bool
	removed  bool
	labels   map[string]int
}

// NPCWorld is the part of a Directory that has the world's NPCs. A
// Directory without it reaches only the zone's own NPCs.
type NPCWorld interface {
	NPCs() *NPCDirectory
}

// AddZone lists a zone's NPCs. Call it before the zone runs. A record
// name that another zone has already, such as an NPC placed at every
// place of a tag, keeps the first.
func (d *NPCDirectory) AddZone(z *Zone) {
	d.mu.Lock()
	defer d.mu.Unlock()
	handles := make([]int64, 0, len(z.npcHandles))
	for h := range z.npcHandles {
		handles = append(handles, h)
	}
	slices.Sort(handles)
	for _, h := range handles {
		e := z.npcHandles[h]
		n := e.npc
		if n.hint != 0 {
			continue
		}
		m, _, _ := strings.Cut(n.Name, ":")
		en := &npcEntry{zone: z, handle: h, name: n.Name, mod: m, display: n.DisplayName, x: e.x, y: e.y,
			scripted: n.Script != "", floating: n.floating, state: n.state(), cloaked: n.cloak}
		n.entry = en
		d.byZone[z] = append(d.byZone[z], en)
		if d.byName[n.Name] == nil {
			d.byName[n.Name] = en
		}
	}
}

// RemoveZone takes a zone's NPCs out, when its map leaves the world.
func (d *NPCDirectory) RemoveZone(z *Zone) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, en := range d.byZone[z] {
		if d.byName[en.name] == en {
			delete(d.byName, en.name)
		}
	}
	delete(d.byZone, z)
	delete(d.started, z)
}

// find finds an NPC by name: a full record name, or a plain name, which
// is mod's record, then core's.
func (d *NPCDirectory) find(name, modID string) *npcEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, full := range fullNames(name, modID) {
		if en := d.byName[full]; en != nil && !en.removed {
			return en
		}
	}
	return nil
}

// fullNames are the record names a name may mean.
func fullNames(name, modID string) []string {
	if strings.Contains(name, ":") {
		return []string{name}
	}
	out := []string{modID + ":" + name}
	if modID != mod.Core {
		out = append(out, mod.Core+":"+name)
	}
	return out
}

func (d *NPCDirectory) markStarted(z *Zone) {
	d.mu.Lock()
	d.started[z] = true
	d.mu.Unlock()
}

// hasLabel reports whether an NPC listens to a label, as far as the
// world knows: true for an NPC whose zone has not started its scripts.
func (d *NPCDirectory) hasLabel(en *npcEntry, label string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if en.removed || !en.scripted {
		return false
	}
	return !d.started[en.zone] || en.labels[label] > 0
}

func (d *NPCDirectory) label(en *npcEntry, label string, add bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if en.labels == nil {
		en.labels = map[string]int{}
	}
	if add {
		en.labels[label]++
	} else if en.labels[label]--; en.labels[label] <= 0 {
		delete(en.labels, label)
	}
}

// mirror copies an NPC's view state into its entry.
func (d *NPCDirectory) mirror(n *npc) {
	en := n.entry
	if en == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	en.state, en.cloaked, en.removed = n.state(), n.cloak, n.removed
	en.cloakFor = cloneSet(n.cloakFor)
	if n.removed && d.byName[en.name] == en {
		delete(d.byName, en.name)
	}
}

func cloneSet(m map[int64]bool) map[int64]bool {
	if len(m) == 0 {
		return nil
	}
	out := make(map[int64]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// mirrorPlace copies an NPC's name and cell into its entry.
func (d *NPCDirectory) mirrorPlace(e *entity) {
	en := e.npc.entry
	if en == nil {
		return
	}
	d.mu.Lock()
	en.display, en.x, en.y = e.npc.DisplayName, e.x, e.y
	d.mu.Unlock()
}

// place returns an entry's name and cell.
func (d *NPCDirectory) place(en *npcEntry) (display string, x, y int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return en.display, en.x, en.y
}

// read returns an entry's view state.
func (d *NPCDirectory) read(en *npcEntry) (state string, cloaked bool, cloakFor map[int64]bool, removed bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return en.state, en.cloaked, en.cloakFor, en.removed
}

// broadcastTargets are the zones other than from that an event for
// every NPC with a label goes to, and how many of their NPCs the world
// knows to listen. mods limits the NPCs to those mods, or nil for every
// mod.
func (d *NPCDirectory) broadcastTargets(from *Zone, label string, mods []string) (zones []*Zone, count int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for z, list := range d.byZone {
		if z == from {
			continue
		}
		send := false
		for _, en := range list {
			if en.removed || !en.scripted || mods != nil && !slices.Contains(mods, en.mod) {
				continue
			}
			if !d.started[z] {
				send = true
				continue
			}
			if en.labels[label] > 0 {
				send = true
				count++
			}
		}
		if send {
			zones = append(zones, z)
		}
	}
	sort.Slice(zones, func(i, j int) bool { return zones[i].Map < zones[j].Map })
	return zones, count
}

// npcMessage is work for a zone's NPCs.
type npcMessage struct {
	kind npcMessageKind
	// handle is the NPC's handle in the zone that gets the message.
	handle int64
	label  string
	// player is the zone's player ID for an event with a player, which
	// only the zone itself sends. charID is the character of a cloak for
	// one player, from any zone.
	player, charID int64
	control        modscript.NPCControl
	mods           []string
	// run is the work of a presentation message on the NPC, and
	// zoneRun the work of a message for a zone's players
	// (npcpresent.go).
	run     func(z *Zone, e *entity)
	zoneRun func(z *Zone)
}

type npcMessageKind int

const (
	npcEventMessage npcMessageKind = iota
	npcBroadcastMessage
	npcControlMessage
	npcEndMessage
	// npcPresentMessage and npcZoneMessage need no NPC scripts: they show
	// something to the zone's players (npcpresent.go).
	npcPresentMessage
	npcZoneMessage
)

// npcInbox is a zone's NPC messages. Other zones post to it.
type npcInbox struct {
	mu   sync.Mutex
	msgs []npcMessage
	wake chan struct{}
}

// post adds a message and wakes the zone. It returns false when the
// inbox is full, and drops the message.
func (in *npcInbox) post(m npcMessage) bool {
	in.mu.Lock()
	if len(in.msgs) >= maxNPCInbox {
		in.mu.Unlock()
		return false
	}
	in.msgs = append(in.msgs, m)
	in.mu.Unlock()
	in.signal()
	return true
}

func (in *npcInbox) signal() {
	select {
	case in.wake <- struct{}{}:
	default:
	}
}

// take removes up to n messages, oldest first.
func (in *npcInbox) take(n int) []npcMessage {
	in.mu.Lock()
	defer in.mu.Unlock()
	n = min(n, len(in.msgs))
	if n == 0 {
		return nil
	}
	out := slices.Clone(in.msgs[:n])
	clear(in.msgs[:n])
	in.msgs = in.msgs[n:]
	if len(in.msgs) == 0 {
		in.msgs = nil
	}
	return out
}

func (in *npcInbox) pending() bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	return len(in.msgs) > 0
}

// npcDir is the world's NPC directory, or nil for a zone without one.
func (z *Zone) npcDir() *NPCDirectory {
	if w, ok := z.World.(NPCWorld); ok {
		return w.NPCs()
	}
	return nil
}

// postNPC sends a message to a zone's NPCs. A full inbox drops it, with
// a warning.
func (z *Zone) postNPC(to *Zone, m npcMessage) {
	if !to.npcIn.post(m) && z.Mods != nil && z.Mods.Log != nil {
		z.Mods.Log.Warn("NPC message dropped: the zone's inbox is full", "map", z.Map, "to", to.Map, "label", m.label)
	}
}

// npcMessages runs up to npcMessageStep waiting messages. It returns how
// many ran.
func (z *Zone) npcMessages() int {
	msgs := z.npcIn.take(npcMessageStep)
	if len(msgs) == 0 {
		return 0
	}
	for _, m := range msgs {
		if m.kind != npcPresentMessage && m.kind != npcZoneMessage {
			z.startNPCs()
			break
		}
	}
	for _, m := range msgs {
		z.runNPCMessage(m)
	}
	return len(msgs)
}

func (z *Zone) runNPCMessage(m npcMessage) {
	switch m.kind {
	case npcZoneMessage:
		m.zoneRun(z)
		return
	case npcPresentMessage:
		if e := z.npcHandles[m.handle]; e != nil && !e.npc.removed {
			m.run(z, e)
		}
		return
	}
	// A view change needs no scripts: a shop NPC shows and hides too.
	r := z.scripts
	if r == nil && m.kind != npcControlMessage {
		return
	}
	switch m.kind {
	case npcBroadcastMessage:
		// npc_event_doall_sub (npc.cpp 1255): every NPC with the label,
		// duplicates too, with no player.
		for _, e := range z.npcsInOrder() {
			n := e.npc
			if n.script == nil || n.removed || m.mods != nil && !slices.Contains(m.mods, n.mod()) {
				continue
			}
			_, _, _ = r.FireNPCEvent(n.script, m.label, 0)
		}
		return
	}
	e := z.npcHandles[m.handle]
	if e == nil || e.npc.removed && m.kind != npcEndMessage {
		return
	}
	n := e.npc
	switch m.kind {
	case npcEventMessage:
		if n.script == nil {
			return
		}
		if m.player == 0 {
			_, _, _ = r.FireNPCEvent(n.script, m.label, 0)
			return
		}
		p := z.players[uint32(m.player)] // #nosec G115 -- a zone player ID.
		if p == nil || p.gone() {
			return
		}
		z.runNPCEvent(&touchEvent{npc: e, player: p, label: m.label})
	case npcControlMessage:
		z.controlNPC(e, m.control, m.charID)
	case npcEndMessage:
		if n.script != nil && r != nil {
			r.EndNPC(n.script)
			n.script = nil
		}
	}
}

// npcsInOrder are the zone's NPCs with a script handle, in handle order.
func (z *Zone) npcsInOrder() []*entity {
	out := make([]*entity, 0, len(z.npcHandles))
	for _, e := range z.npcHandles {
		if e.npc.hint == 0 {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b *entity) int { return int(a.npc.handle - b.npc.handle) })
	return out
}

// runNPCEvent runs an event with a player, as doevent's npc_event_sub:
// it waits in the player's event queue while the player talks, as a
// touch does (touch.go), and runs in a conversation with the player.
func (z *Zone) runNPCEvent(t *touchEvent) {
	p, n := t.player, t.npc.npc
	if n.script == nil || z.scripts == nil || n.removed {
		return
	}
	if p.talk != nil {
		if len(p.touchQueue) < maxTouchQueue {
			p.touchQueue = append(p.touchQueue, t)
			p.touchAt = z.now + touchQueueDelay
		}
		return
	}
	conv, _, err := z.scripts.FireNPCEvent(n.script, t.label, int64(p.charID))
	if err != nil {
		if z.Mods != nil && z.Mods.Log != nil {
			z.Mods.Log.Warn("NPC event failed", "map", z.Map, "npc", n.Name, "label", t.label, "err", err)
		}
		return
	}
	if conv != nil && !conv.Done && p.talk == nil {
		p.talk = &talk{conv: conv, npc: t.npc}
	}
	z.afterTalk(p)
}

// The view state of an NPC, as rAthena's npc_data: is_invisible for a
// disabled NPC, and OPTION_HIDE and OPTION_CLOAK in sc.option. cloakFor
// holds the characters for whom the cloak is the opposite, as the
// player's cloaked_npc list (npc_is_cloaked, npc.cpp 1004).

func (n *npc) state() string {
	switch {
	case n.invisible:
		return "disabled"
	case n.hide:
		return "hidden"
	}
	return "shown"
}

// mod is the mod of the NPC's record.
func (n *npc) mod() string {
	m, _, _ := strings.Cut(n.Name, ":")
	return m
}

// cloakedFor reports whether the NPC is cloaked for a character, or for
// everyone when charID is 0.
func (n *npc) cloakedFor(charID int64) bool {
	if charID != 0 && n.cloakFor[charID] {
		return !n.cloak
	}
	return n.cloak
}

// visibleTo reports whether a player sees the NPC: placed, with a
// sprite, not disabled, hidden or cloaked for it.
func (n *npc) visibleTo(p *Player) bool {
	if n.floating || n.removed || n.NoSprite || n.invisible || n.hide {
		return false
	}
	return p == nil || !n.cloakedFor(cloakKey(p))
}

// clickable is npc_click's check (npc.cpp 2225): a view, and not
// hidden. A disabled NPC is hidden too.
func (n *npc) clickable() bool {
	return !n.floating && !n.removed && !n.NoSprite && !n.invisible && !n.hide
}

// touchableBy is npc_touch_areanpc's check (npc.cpp 1897): not disabled,
// and not cloaked for the player. A hidden NPC still has its touch area.
func (n *npc) touchableBy(p *Player) bool {
	if n.floating || n.removed || n.invisible {
		return false
	}
	return !n.cloakedFor(cloakKey(p))
}

// setScriptState tells the NPC's script whether it is disabled.
func (n *npc) setScriptState() {
	if n.script != nil {
		n.script.Disabled = n.invisible
	}
}

// controlNPC changes an NPC's view, as npc_enable_target (npc.cpp 1036).
// charID is the character of a cloak for one player, or 0.
func (z *Zone) controlNPC(e *entity, c modscript.NPCControl, charID int64) {
	n := e.npc
	if n.removed {
		return
	}
	visible := false
	var only *Player
	switch c.Op {
	case "show":
		// NPCVIEW_ENABLE: shows, and clears OPTION_HIDE.
		n.invisible, n.hide, visible = false, false, true
		z.clearCloaksOnMap(n)
	case "hide":
		// NPCVIEW_DISABLE: OPTION_HIDE and is_invisible.
		n.invisible, n.hide = true, true
		z.clearCloaksOnMap(n)
	case "set_hidden":
		n.hide, visible = c.On, !c.On
		z.clearCloaksOnMap(n)
	case "set_cloaked":
		if charID != 0 {
			// The character's cloak differs from the NPC's while it is in
			// the list.
			if c.On != n.cloak {
				if n.cloakFor == nil {
					n.cloakFor = map[int64]bool{}
				}
				n.cloakFor[charID] = true
			} else {
				delete(n.cloakFor, charID)
			}
			visible = !c.On
			only = z.playerByChar(charID)
			if only == nil {
				visible = false
			}
		} else {
			n.cloak, visible = c.On, !c.On
			z.clearCloaksOnMap(n)
		}
	case "remove":
		z.removeNPC(e)
		return
	}
	n.setScriptState()
	if d := z.npcDir(); d != nil {
		d.mirror(n)
	}
	if visible {
		// NPCVIEW_VISIBLE: the players standing in the touch area touch
		// it (npc_enable_sub, npc.cpp 1100).
		z.touchShown(e, only)
	}
}

// clearCloaksOnMap is npc_cloaked_sub: a change for every player ends
// the single players' cloaks of the players on the NPC's map.
func (z *Zone) clearCloaksOnMap(n *npc) {
	if len(n.cloakFor) == 0 {
		return
	}
	for _, p := range z.players {
		delete(n.cloakFor, cloakKey(p))
	}
}

func (z *Zone) playerByChar(charID int64) *Player {
	for _, p := range z.players {
		if cloakKey(p) == charID && !p.gone() {
			return p
		}
	}
	return nil
}

// touchShown fires the touch event of the players who stand in an NPC's
// touch area once it can be touched again, or of one player.
func (z *Zone) touchShown(e *entity, only *Player) {
	n := e.npc
	if n.Touch == nil || n.script == nil || !z.entities.has(e) {
		return
	}
	for _, p := range z.players {
		if only != nil && p != only || p.gone() || !n.touchableBy(p) || slices.Contains(p.touching, e) {
			continue
		}
		if n.touches(e, p.entity.x, p.entity.y) {
			p.touching = append(p.touching, e)
			z.queueTouch(e, p, false)
		}
	}
}

// removeNPC is unloadnpc (npc_unload): the NPC leaves the map and the
// directory, its NPC variables go, and its context ends after the
// current step, so a script that removes its own NPC finishes its call.
func (z *Zone) removeNPC(e *entity) {
	n := e.npc
	n.removed = true
	if z.entities.has(e) {
		z.entities.remove(e)
		z.forget(e)
		z.removed = append(z.removed, removal{e, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT})
	}
	z.touchNPCs = slices.DeleteFunc(z.touchNPCs, func(o *entity) bool { return o == e })
	for _, p := range z.players {
		if i := slices.Index(p.touching, e); i >= 0 {
			p.touching = slices.Delete(p.touching, i, i+1)
			z.queueTouch(e, p, true)
		}
	}
	if d := z.npcDir(); d != nil {
		d.mirror(n)
	}
	if z.scripts != nil {
		z.scripts.DropNPCVariables(n.Name)
	}
	z.npcIn.post(npcMessage{kind: npcEndMessage, handle: n.handle})
}

// The NPCWorldHost side of scriptHost.

// npcTarget is what an NPC handle names: an NPC of this zone, or the
// entry of an NPC of another zone.
type npcTarget struct {
	local *entity
	entry *npcEntry
}

func (h scriptHost) npcTarget(id int64) (npcTarget, error) {
	z := h.z
	if id >= remoteBase {
		en := z.remoteNPCs[id]
		if en == nil {
			return npcTarget{}, errors.New("not an NPC handle")
		}
		if _, _, _, removed := z.npcDir().read(en); removed {
			return npcTarget{}, errors.New("the NPC was removed")
		}
		return npcTarget{entry: en}, nil
	}
	e := z.npcByHandle(id)
	if e == nil {
		return npcTarget{}, errors.New("the NPC is gone from this map")
	}
	if e.npc.removed {
		return npcTarget{}, errors.New("the NPC was removed")
	}
	return npcTarget{local: e}, nil
}

// remoteHandle is this zone's handle for an NPC of another zone.
func (z *Zone) remoteHandle(en *npcEntry) int64 {
	if id, ok := z.remoteIDs[en]; ok {
		return id
	}
	if z.remoteNPCs == nil {
		z.remoteNPCs, z.remoteIDs = map[int64]*npcEntry{}, map[*npcEntry]int64{}
	}
	id := remoteBase + int64(len(z.remoteNPCs))
	z.remoteNPCs[id], z.remoteIDs[en] = en, id
	return id
}

// FindNPC is world.get_npc.
func (h scriptHost) FindNPC(name, modID string) (int64, bool) {
	z := h.z
	if d := z.npcDir(); d != nil {
		en := d.find(name, modID)
		if en == nil {
			return 0, false
		}
		if en.zone == z {
			return en.handle, true
		}
		return z.remoteHandle(en), true
	}
	// A zone without a world finds its own NPCs.
	for _, full := range fullNames(name, modID) {
		for _, e := range z.npcsInOrder() {
			if e.npc.Name == full && !e.npc.removed {
				return e.npc.handle, true
			}
		}
	}
	return 0, false
}

// NPCMod is the mod of an NPC's record.
func (h scriptHost) NPCMod(id int64) (string, error) {
	t, err := h.npcTarget(id)
	if err != nil {
		return "", err
	}
	if t.entry != nil {
		return t.entry.mod, nil
	}
	return t.local.npc.mod(), nil
}

// NPCKey is an NPC's full record name.
func (h scriptHost) NPCKey(id int64) (string, error) {
	t, err := h.npcTarget(id)
	if err != nil {
		return "", err
	}
	if t.entry != nil {
		return t.entry.name, nil
	}
	return t.local.npc.Name, nil
}

// charOf is the character ID of a script's player handle.
func (h scriptHost) charOf(player int64) (int64, error) {
	p, err := h.player(player)
	if err != nil {
		return 0, err
	}
	return cloakKey(p), nil
}

// cloakKey is the key of a player's cloaks: its character ID, or for a
// character that only lives in memory, which has none, its zone ID made
// negative.
func cloakKey(p *Player) int64 {
	if p.record.ID != 0 {
		return p.record.ID
	}
	return -int64(p.charID)
}

// ControlNPC changes an NPC's view: at once for an NPC of this zone, and
// in its zone after the current step for another's. remove always waits
// for the end of the step to end the NPC's context.
func (h scriptHost) ControlNPC(id int64, c modscript.NPCControl) error {
	t, err := h.npcTarget(id)
	if err != nil {
		return err
	}
	var charID int64
	if c.Player != 0 {
		if charID, err = h.charOf(c.Player); err != nil {
			return err
		}
	}
	if t.local != nil {
		if t.local.npc.hint != 0 {
			return errors.New("a world event's hint NPC cannot change")
		}
		h.z.controlNPC(t.local, c, charID)
		return nil
	}
	h.z.postNPC(t.entry.zone, npcMessage{kind: npcControlMessage, handle: t.entry.handle, control: c, charID: charID})
	return nil
}

// NPCCloaked is isnpccloaked.
func (h scriptHost) NPCCloaked(id, player int64) (bool, error) {
	t, err := h.npcTarget(id)
	if err != nil {
		return false, err
	}
	var charID int64
	if player != 0 {
		if charID, err = h.charOf(player); err != nil {
			return false, err
		}
	}
	if t.local != nil {
		return t.local.npc.cloakedFor(charID), nil
	}
	_, cloaked, cloakFor, _ := h.z.npcDir().read(t.entry)
	if charID != 0 && cloakFor[charID] {
		return !cloaked, nil
	}
	return cloaked, nil
}

// FireNPCEvent sends an event to an NPC, as donpcevent, or as doevent
// with a player. The event runs after the current step: in this zone
// once the script's call returned, or in the NPC's zone.
func (h scriptHost) FireNPCEvent(id int64, label string, player int64) (bool, error) {
	z := h.z
	t, err := h.npcTarget(id)
	if err != nil {
		return false, err
	}
	if t.entry != nil {
		if player != 0 {
			return false, errors.New("the player is on another map than the NPC: an event with a player runs on the NPC's map")
		}
		z.postNPC(t.entry.zone, npcMessage{kind: npcEventMessage, handle: t.entry.handle, label: label})
		return z.npcDir().hasLabel(t.entry, label), nil
	}
	n := t.local.npc
	if player != 0 {
		if _, err := h.player(player); err != nil {
			return false, err
		}
		if n.invisible {
			// npc_event_sub: a disabled NPC runs no event with a player.
			return false, nil
		}
	}
	if n.Script == "" || z.scripts == nil {
		return false, nil
	}
	// The NPC may be in its own setup, which has not returned yet: the
	// event runs after it.
	z.postNPC(z, npcMessage{kind: npcEventMessage, handle: n.handle, label: label, player: player})
	return z.scripts.HasNPCLabel(n.handle, label), nil
}

// BroadcastNPCEvent sends an event to every NPC with the label. The
// zone counts its own NPCs from its scripts, and the others' from the
// world's directory.
func (h scriptHost) BroadcastNPCEvent(label string, mods []string) (int, error) {
	z := h.z
	count, scripted := 0, false
	for _, e := range z.npcsInOrder() {
		n := e.npc
		if n.Script == "" || n.removed || mods != nil && !slices.Contains(mods, n.mod()) {
			continue
		}
		scripted = true
		if z.scripts != nil && z.scripts.HasNPCLabel(n.handle, label) {
			count++
		}
	}
	if scripted {
		z.postNPC(z, npcMessage{kind: npcBroadcastMessage, label: label, mods: mods})
	}
	if d := z.npcDir(); d != nil {
		zones, n := d.broadcastTargets(z, label, mods)
		for _, to := range zones {
			z.postNPC(to, npcMessage{kind: npcBroadcastMessage, label: label, mods: mods})
		}
		count += n
	}
	return count, nil
}

// FindPlayer finds a player of the zone by name.
func (h scriptHost) FindPlayer(name string) (int64, bool) {
	for _, p := range h.z.playersInOrder() {
		if p.record.Name == name {
			return int64(p.charID), true
		}
	}
	return 0, false
}

// FindPlayerByID finds a player of the zone by account ID, or else by
// character ID, as convertpcinfo's map_id2sd then map_charid2sd.
func (h scriptHost) FindPlayerByID(id int64) (int64, bool) {
	players := h.z.playersInOrder()
	for _, p := range players {
		if p.record.AccountID == id {
			return int64(p.charID), true
		}
	}
	for _, p := range players {
		if p.record.ID == id {
			return int64(p.charID), true
		}
	}
	return 0, false
}

// playersInOrder are the zone's players that have not left, by ID.
func (z *Zone) playersInOrder() []*Player {
	out := make([]*Player, 0, len(z.players))
	for _, p := range z.players {
		if !p.gone() {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b *Player) int { return int(a.charID) - int(b.charID) })
	return out
}

// npcNames are an NPC's names as strnpcinfo reads them: the record's
// name without the mod (3), the part after # (2), and the name with its
// # part (0).
func npcNames(full, display string) (unique, hidden, withHidden string) {
	unique = mod.Local(full)
	_, hidden, has := strings.Cut(unique, "#")
	withHidden = display
	if has {
		withHidden = display + "#" + hidden
	}
	return unique, hidden, withHidden
}

// remoteField reads a field of an NPC of another zone.
func (h scriptHost) remoteField(en *npcEntry, field string) (script.Value, error) {
	display, x, y := h.z.npcDir().place(en)
	unique, hidden, full := npcNames(en.name, display)
	switch field {
	case "kind":
		return script.String("npc"), nil
	case "name":
		return script.String(display), nil
	case "full_name":
		return script.String(full), nil
	case "hidden_name":
		return script.String(hidden), nil
	case "unique_name":
		return script.String(unique), nil
	case "map":
		if en.floating {
			return script.String(""), nil
		}
		return script.String(strings.TrimPrefix(en.zone.Map, "map/")), nil
	case "x":
		return intValue(x), nil
	case "y":
		return intValue(y), nil
	case "state":
		state, _, _, _ := h.z.npcDir().read(en)
		return script.String(state), nil
	}
	return script.Nil, fmt.Errorf("npc has no field %q", field)
}

var _ modscript.NPCWorldHost = scriptHost{}

// monsterTouch is a monster that walked into an NPC's touch area.
type monsterTouch struct {
	npc, monster *entity
}

// monsterTouched is npc_touch_areanpc2 (npc.cpp 1997) after a monster's
// step: the first NPC whose touch area holds the cell, not disabled or
// cloaked, runs its monster_touch once, until the monster leaves the
// areas or walks into another NPC's (unit.cpp 693).
func (z *Zone) monsterTouched(e *entity) {
	if len(z.touchNPCs) == 0 || z.scripts == nil {
		return
	}
	m := e.monster
	var at *entity
	for _, n := range z.touchNPCs {
		if n.npc.invisible || n.npc.cloak || n.npc.removed || !n.npc.touches(n, e.x, e.y) {
			continue
		}
		at = n
		break
	}
	if at == nil {
		m.touchNPC = nil
		return
	}
	if at == m.touchNPC || at.npc.script == nil || !z.scripts.NPCListens(at.npc.script, "monster_touch") {
		return
	}
	m.touchNPC = at
	z.scriptEvents = append(z.scriptEvents, scriptEvent{monsterTouch: &monsterTouch{npc: at, monster: e}})
}

// runMonsterTouch runs a monster_touch, when the monster still lives on
// the map.
func (z *Zone) runMonsterTouch(t *monsterTouch) {
	n := t.npc.npc
	if n.script == nil || n.removed || t.monster.dead || !z.entities.has(t.monster) {
		return
	}
	z.scripts.FireMonsterTouch(n.script, z.monsterHandle(t.monster))
}
