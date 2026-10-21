package zone

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// Players across zones (review decision 59, decision 0066):
//   - The world keeps a directory of every online player: its zone, its
//     ID in that zone, its names and IDs, and a copy of what other zones
//     read of it, which its zone writes at the end of each step.
//   - A script reaches a player of another zone through a handle the
//     zone makes for it, from remotePlayerBase on. The handle names the
//     character, so it follows the player from map to map. Its fields
//     come from the directory. Its actions go to the player's zone as
//     messages, which run after the current step in that zone. Its reads
//     raise an error there.
//   - The player events of one player (login, logout, map_enter, the
//     level ups and a player's death) reach the NPC handlers of every
//     map, as rAthena's OnPC*Event labels run for every NPC with the
//     label. Other zones get them as messages. A logout waits until the
//     other zones ran their handlers, so their actions on the player run
//     before its last save, as npc_script_event runs before map_quit.

// remotePlayerBase is the first handle of a player of another zone.
// Handles of the zone's own players are zone player IDs, which are 32
// bits. NPC handles of other zones start at remoteBase.
const remotePlayerBase = int64(1) << 42

// logoutHoldMs is the longest a logout waits for the handlers of other
// zones.
const logoutHoldMs = 2000

// errRemotePlayer is the error of a read of a player of another zone,
// and errOffline that of a handle whose player went offline.
var (
	errRemotePlayer = errors.New("the player is on another map: read it on its own map")
	errOffline      = errors.New("the player is offline")
)

// PlayerDirectory is the world's list of online players. Its methods are
// safe for concurrent use.
type PlayerDirectory struct {
	mu     sync.Mutex
	byName map[string]*playerEntry
}

// NewPlayerDirectory makes an empty directory.
func NewPlayerDirectory() *PlayerDirectory {
	return &PlayerDirectory{byName: map[string]*playerEntry{}}
}

// PlayerWorld is the part of a Directory that has the world's players.
type PlayerWorld interface {
	Players() *PlayerDirectory
}

// playerEntry is one online player. Only its zone writes snap, under the
// directory's lock.
type playerEntry struct {
	zone          *Zone
	id            uint32
	name          string
	account, char int64
	male          bool
	snap          playerSnap
	removed       bool
}

// playerSnap is what other zones read of a player: its state at the end
// of its zone's last step.
type playerSnap struct {
	x, y                          int
	baseLevel, jobLevel, job      int
	zeny, hp, maxHP, sp, maxSP    int64
	weight, maxWeight, skillPoint int
	party, guild                  int64
}

func (z *Zone) snapOf(p *Player) playerSnap {
	e := p.entity
	s := playerSnap{baseLevel: p.progress.BaseLevel, jobLevel: p.progress.JobLevel, job: p.char.Job, zeny: p.record.Zeny,
		sp: p.sp, weight: p.inventory.Weight, maxWeight: p.maxWeight, skillPoint: p.progress.SkillPoints,
		party: p.partyID(), guild: p.guildID()}
	if e != nil {
		s.x, s.y, s.hp = e.x, e.y, e.hp
		if e.status != nil {
			s.maxHP, s.maxSP = int64(e.status.MaxHP), int64(e.status.MaxSP)
		}
	}
	return s
}

// playerDir is the world's player directory, or nil for a zone without
// one.
func (z *Zone) playerDir() *PlayerDirectory {
	if w, ok := z.World.(PlayerWorld); ok {
		return w.Players()
	}
	return nil
}

// listPlayer adds a player that entered the zone to the directory.
func (z *Zone) listPlayer(p *Player) {
	d := z.playerDir()
	if d == nil {
		return
	}
	en := &playerEntry{zone: z, id: p.charID, name: p.Name, account: p.record.AccountID, char: p.record.ID, male: p.char.Male,
		snap: z.snapOf(p)}
	d.mu.Lock()
	if old := d.byName[p.Name]; old != nil {
		old.removed = true
	}
	d.byName[p.Name] = en
	d.mu.Unlock()
	p.listed = en
}

// unlistPlayer takes a player that left the zone out of the directory.
func (z *Zone) unlistPlayer(p *Player) {
	en := p.listed
	d := z.playerDir()
	if en == nil || d == nil {
		return
	}
	p.listed = nil
	d.mu.Lock()
	en.removed = true
	if d.byName[en.name] == en {
		delete(d.byName, en.name)
	}
	d.mu.Unlock()
}

// publishPlayers copies what other zones read of the zone's players into
// the directory, at the end of a step.
func (z *Zone) publishPlayers() {
	d := z.playerDir()
	if d == nil || len(z.players) == 0 {
		return
	}
	d.mu.Lock()
	for _, p := range z.players {
		if en := p.listed; en != nil {
			en.snap = z.snapOf(p)
		}
	}
	d.mu.Unlock()
}

// find finds an online player by name.
func (d *PlayerDirectory) find(name string) *playerEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.byName[name]
}

// findByID finds an online player by account ID, or else by character
// ID, as convertpcinfo's map_id2sd and map_charid2sd. The first of the
// sorted names wins a tie, so the answer does not depend on map order.
func (d *PlayerDirectory) findByID(id int64) *playerEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	var byAccount, byChar *playerEntry
	for _, en := range d.byName {
		if en.account == id && id != 0 && (byAccount == nil || en.name < byAccount.name) {
			byAccount = en
		}
		if en.char == id && id != 0 && (byChar == nil || en.name < byChar.name) {
			byChar = en
		}
	}
	if byAccount != nil {
		return byAccount
	}
	return byChar
}

// read returns a copy of an entry's zone, ID and state, and whether it
// is still online.
func (d *PlayerDirectory) read(en *playerEntry) (z *Zone, id uint32, s playerSnap, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return en.zone, en.id, en.snap, !en.removed
}

// remotePlayerHandle is this zone's handle for a player by name.
func (z *Zone) remotePlayerHandle(name string) int64 {
	if id, ok := z.remotePlayerIDs[name]; ok {
		return id
	}
	if z.remotePlayers == nil {
		z.remotePlayers, z.remotePlayerIDs = map[int64]string{}, map[string]int64{}
	}
	id := remotePlayerBase + int64(len(z.remotePlayers))
	z.remotePlayers[id], z.remotePlayerIDs[name] = name, id
	return id
}

// handleOf is the handle of a player of the directory: its zone ID in
// this zone, or a remote handle for another zone.
func (z *Zone) handleOf(en *playerEntry) int64 {
	if en.zone == z {
		if p := z.players[en.id]; p != nil {
			return int64(p.charID)
		}
	}
	return z.remotePlayerHandle(en.name)
}

// remotePlayer finds the player of a remote handle: in the directory,
// with the player itself when it is in this zone now.
func (h scriptHost) remotePlayer(id int64) (*playerEntry, *Player, error) {
	z := h.z
	name, ok := z.remotePlayers[id]
	d := z.playerDir()
	if !ok || d == nil {
		return nil, nil, errors.New("not a player handle")
	}
	en := d.find(name)
	if en == nil {
		return nil, nil, errOffline
	}
	if en.zone == z {
		if p := z.players[en.id]; p != nil && !p.gone() {
			return en, p, nil
		}
		return nil, nil, errors.New("the player has left this map")
	}
	return en, nil, nil
}

// IsRemotePlayer reports whether a handle is that of a player of another
// zone.
func IsRemotePlayer(id int64) bool { return id >= remotePlayerBase && id < remotePlayerBase<<1 }

// remotePlayerField reads a field of a player of another zone, from the
// directory.
func (h scriptHost) remotePlayerField(en *playerEntry, field string) (script.Value, error) {
	z, _, s, ok := h.z.playerDir().read(en)
	if !ok {
		return script.Nil, errOffline
	}
	switch field {
	case "kind":
		return script.String("player"), nil
	case "name":
		return script.String(en.name), nil
	case "map":
		return script.String(strings.TrimPrefix(z.Map, "map/")), nil
	case "x":
		return intValue(s.x), nil
	case "y":
		return intValue(s.y), nil
	case "character_id":
		return script.Int(en.char), nil
	case "account_id":
		return script.Int(en.account), nil
	case "sex":
		if en.male {
			return script.String("male"), nil
		}
		return script.String("female"), nil
	case "base_level":
		return intValue(s.baseLevel), nil
	case "job_level":
		return intValue(s.jobLevel), nil
	case "job":
		return intValue(s.job), nil
	case "zeny":
		return script.Int(s.zeny), nil
	case "hp":
		return script.Int(s.hp), nil
	case "max_hp":
		return script.Int(s.maxHP), nil
	case "sp":
		return script.Int(s.sp), nil
	case "max_sp":
		return script.Int(s.maxSP), nil
	case "weight":
		return intValue(s.weight), nil
	case "max_weight":
		return intValue(s.maxWeight), nil
	case "skill_points":
		return intValue(s.skillPoint), nil
	case "party":
		if s.party == 0 {
			return script.Nil, nil
		}
		return script.Int(s.party), nil
	case "guild":
		if s.guild == 0 {
			return script.Nil, nil
		}
		return script.Int(s.guild), nil
	case "job_class":
		if h.z.Status == nil {
			return script.Int(0), nil
		}
		m, _ := h.z.Status.MapID(s.job)
		return script.Int(int64(m)), nil // #nosec G115 -- map job IDs fit in 32 bits.
	}
	if _, known := playerFields[field]; !known {
		return script.Nil, fmt.Errorf("player has no field %q", field)
	}
	return script.Nil, fmt.Errorf("player.%s: %w", field, errRemotePlayer)
}

// RemotePlayerCall sends a player method to the zone of a player of
// another map, as a message that runs after the current step. local is
// true when the player is in this zone now, so the caller runs the
// method itself. A player that went offline gets nothing, as rAthena's
// script commands do nothing for a player that is not found.
func (h scriptHost) RemotePlayerCall(id int64, modID, method string, args []script.Value) (local bool, err error) {
	if !IsRemotePlayer(id) {
		return true, nil
	}
	en, p, err := h.remotePlayer(id)
	if errors.Is(err, errOffline) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if p != nil {
		return true, nil
	}
	to, _, _, ok := h.z.playerDir().read(en)
	if !ok || method == "" {
		return false, nil
	}
	name := en.name
	h.z.postNPC(to, npcMessage{kind: npcZoneMessage, zoneRun: func(t *Zone) { t.runPlayerCall(name, modID, method, args) }})
	return false, nil
}

// RemotePlayerExists is player:exists of a remote handle: the player is
// online on any map.
func (h scriptHost) RemotePlayerExists(id int64) bool {
	_, _, err := h.remotePlayer(id)
	return err == nil
}

// playerByName is an online player of the zone by name. A player whose
// logout waits for other zones counts.
func (z *Zone) playerByName(name string) *Player {
	for _, p := range z.players {
		if p.Name == name && (!p.gone() || p.leaving != nil) {
			return p
		}
	}
	return nil
}

// runPlayerCall runs a method that another zone sent for one of this
// zone's players.
func (z *Zone) runPlayerCall(name, modID, method string, args []script.Value) {
	p := z.playerByName(name)
	if p == nil {
		return
	}
	r := z.itemScripts()
	if r == nil {
		return
	}
	if err := r.RunPlayerCall(modID, int64(p.charID), method, args); err != nil && z.Mods != nil && z.Mods.Log != nil {
		z.Mods.Log.Warn("a player call from another map failed", "map", z.Map, "player", name, "method", method, "err", err)
	}
	z.later(func() { z.afterTalk(p) })
}

// playerEventMessage is a player event for the NPCs of another zone.
type playerEventMessage struct {
	event string
	// name is the player's character name, from is its zone, and hold
	// is set for a logout that waits for this zone.
	name string
	from *Zone
	hold bool
	// death is set for the death event: the map, the cell and the skill,
	// and the killer's name when a player killed the player.
	death  *modscript.Death
	killer string
}

// playerEventZones are the zones other than z whose NPCs listen to a
// player event: those that registered a handler, and those that have
// not started their NPC scripts and whose NPC scripts name the event.
func (z *Zone) playerEventZones(event string) []*Zone {
	d := z.npcDir()
	if d == nil {
		return nil
	}
	return d.eventTargets(z, event)
}

// sendPlayerEvent sends a player event to the NPCs of the other zones
// that listen to it. It returns how many zones got it.
func (z *Zone) sendPlayerEvent(event string, p *Player, death *modscript.Death, killer *Player, hold bool) int {
	if z.playerDir() == nil || p.listed == nil {
		return 0
	}
	zones := z.playerEventZones(event)
	m := playerEventMessage{event: event, name: p.Name, from: z, hold: hold, death: death}
	if killer != nil {
		m.killer = killer.Name
	}
	n := 0
	for _, to := range zones {
		msg := m
		if to.npcIn.post(npcMessage{kind: npcPlayerEventMessage, playerEvent: &msg}) {
			n++
		} else if z.Mods != nil && z.Mods.Log != nil {
			z.Mods.Log.Warn("player event dropped: the zone's inbox is full", "map", z.Map, "to", to.Map, "event", event)
		}
	}
	return n
}

// playerEvent queues a player event for the zone's handlers, and sends
// it to the NPCs of the other zones.
func (z *Zone) playerEvent(event string, p *Player) {
	z.scriptEventFor(event, modscript.Player(int64(p.charID)))
	z.sendPlayerEvent(event, p, nil, nil, false)
}

// runPlayerEvent runs a player event of another zone for this zone's NPC
// handlers, with a handle of the player, then tells a logout that waits
// that this zone is done.
func (z *Zone) runPlayerEvent(m *playerEventMessage) {
	defer func() {
		if m.hold {
			from, name := m.from, m.name
			z.postNPC(from, npcMessage{kind: npcZoneMessage, zoneRun: func(t *Zone) { t.logoutAck(name) }})
		}
	}()
	r := z.scripts
	d := z.playerDir()
	if r == nil || d == nil {
		return
	}
	en := d.find(m.name)
	if en == nil || en.zone != m.from {
		// The player went offline or moved on: the event has no player.
		return
	}
	id := z.handleOf(en)
	if m.death != nil {
		dd := *m.death
		dd.Victim, dd.VictimKind, dd.NPCOnly = id, "player", true
		dd.Killer, dd.KillerKind = 0, ""
		if m.killer != "" {
			if k := d.find(m.killer); k != nil {
				dd.Killer, dd.KillerKind = z.handleOf(k), "player"
			}
		}
		r.FireDeath(dd)
		return
	}
	r.FireNPCs(m.event, modscript.Player(id))
}

// logoutHold is a logout that waits for the NPC handlers of other zones.
type logoutHold struct {
	pending int
	until   int64
}

// logoutAck is one zone's end of a logout that waits.
func (z *Zone) logoutAck(name string) {
	for _, p := range z.players {
		if p.Name != name || p.leaving == nil {
			continue
		}
		if p.leaving.pending--; p.leaving.pending <= 0 {
			z.finishLogout(p)
		}
		return
	}
}

// heldLogouts ends the logouts whose wait is over.
func (z *Zone) heldLogouts() {
	var done []*Player
	for _, p := range z.players {
		if p.leaving != nil && z.now >= p.leaving.until {
			done = append(done, p)
		}
	}
	slices.SortFunc(done, func(a, b *Player) int { return int(a.charID) - int(b.charID) })
	for _, p := range done {
		z.finishLogout(p)
	}
}

// eventTargets are the zones other than from whose NPCs listen to a
// player event.
func (d *NPCDirectory) eventTargets(from *Zone, event string) []*Zone {
	d.mu.Lock()
	var out, unknown []*Zone
	for z, list := range d.byZone {
		if z == from {
			continue
		}
		if d.started[z] {
			if d.events[z][event] > 0 {
				out = append(out, z)
			}
			continue
		}
		scripted := false
		for _, en := range list {
			if en.scripted && !en.removed {
				scripted = true
				break
			}
		}
		if scripted {
			unknown = append(unknown, z)
		}
	}
	d.mu.Unlock()
	for _, z := range unknown {
		if d.mentions(z, event) {
			out = append(out, z)
		}
	}
	slices.SortFunc(out, func(a, b *Zone) int { return strings.Compare(a.Map, b.Map) })
	return out
}

// playerEventNames are the events that reach the NPCs of every map.
var playerEventNames = []string{"login", "logout", "map_enter", "base_level_up", "job_level_up", "death"}

// mentions reports whether the NPC scripts of a zone that has not
// started them name a player event in their source, as
// events.on("login", ...) does. It reads them once.
func (d *NPCDirectory) mentions(z *Zone, event string) bool {
	d.mu.Lock()
	m, ok := d.named[z]
	d.mu.Unlock()
	if !ok {
		m = map[string]bool{}
		d.mu.Lock()
		var paths []string
		for _, en := range d.byZone[z] {
			if en.scripted && en.script != "" {
				paths = append(paths, en.script)
			}
		}
		d.mu.Unlock()
		for _, entry := range paths {
			src := z.npcSource(entry)
			for _, ev := range playerEventNames {
				if strings.Contains(src, `"`+ev+`"`) || strings.Contains(src, `'`+ev+`'`) {
					m[ev] = true
				}
			}
		}
		d.mu.Lock()
		if d.named == nil {
			d.named = map[*Zone]map[string]bool{}
		}
		d.named[z] = m
		d.mu.Unlock()
	}
	return m[event]
}

// npcSource is the source of an NPC's script entry, "<mod>:<path>", from
// the mods' scripts as they loaded. Any goroutine may call it.
func (z *Zone) npcSource(entry string) string {
	if z.Mods == nil {
		return ""
	}
	modID, file, ok := strings.Cut(entry, ":")
	if !ok {
		return ""
	}
	for _, sc := range z.Mods.Scripts {
		if sc.Mod == modID {
			return sc.Files[file]
		}
	}
	return ""
}

// npcEventListener keeps the world's count of the zone's NPC handlers of
// a player event.
func (z *Zone) npcEventListener(event string, add bool) {
	d := z.npcDir()
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.events == nil {
		d.events = map[*Zone]map[string]int{}
	}
	m := d.events[z]
	if m == nil {
		m = map[string]int{}
		d.events[z] = m
	}
	if add {
		m[event]++
	} else if m[event]--; m[event] <= 0 {
		delete(m, event)
	}
}

// FindPlayer finds an online player by name, on any map: a player of
// this zone, or a handle of a player of another zone (review decision
// 59), as map_nick2sd.
func (h scriptHost) FindPlayer(name string) (int64, bool) {
	z := h.z
	for _, p := range z.playersInOrder() {
		if p.record.Name == name {
			return int64(p.charID), true
		}
	}
	if d := z.playerDir(); d != nil {
		if en := d.find(name); en != nil && en.zone != z {
			return z.handleOf(en), true
		}
	}
	return 0, false
}

// FindPlayerByID finds an online player by account ID, or else by
// character ID, on any map, as convertpcinfo's map_id2sd then
// map_charid2sd.
func (h scriptHost) FindPlayerByID(id int64) (int64, bool) {
	z := h.z
	players := z.playersInOrder()
	for _, p := range players {
		if p.record.AccountID == id {
			return int64(p.charID), true
		}
	}
	d := z.playerDir()
	if d != nil {
		if en := d.findByID(id); en != nil && en.account == id && en.zone != z {
			return z.handleOf(en), true
		}
	}
	for _, p := range players {
		if p.record.ID == id {
			return int64(p.charID), true
		}
	}
	if d != nil {
		if en := d.findByID(id); en != nil && en.zone != z {
			return z.handleOf(en), true
		}
	}
	return 0, false
}

var _ modscript.RemotePlayerHost = scriptHost{}
