package zone

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/internal/script"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/shop"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Headless runs a zone without its goroutine, a ticker or a network, for
// the tests of wactl mod test (mods/README.md, "Testing a mod"):
//   - Only the goroutine that made it uses it.
//   - Its clock starts at 0, and moves only with Advance, 50 ms per
//     tick, and with waits in a conversation and the item use interval.
//     Other calls run a tick at the current time.
//   - Its map is set by Bind, on the first call that needs one. The Lua
//     state exists before, so a test file can load first.
//   - The mods' entry scripts run at Bind, so their context knows the
//     map.
//   - Its server clock is Start plus the zone clock. Schedules and world
//     events follow it. AfterStep runs after every tick, for the world
//     event scheduler.
//
// Players have sessions without a connection. Their messages are read
// after every tick.
type Headless struct {
	z     *Zone
	bound bool
	// events are each player's events since its last talk, by player ID.
	events map[int64][]*wathenav1.Event
	// nextID gives test characters their database IDs.
	nextID int64
	// Start is the server time when the zone clock is 0. Set it before
	// Bind.
	Start time.Time
	// AfterStep runs after every tick.
	AfterStep func()
	// announcements are the lines announced to the world or the map.
	announcements []string
	// heard are the chat lines each player got, by player ID.
	heard map[int64][]Heard
}

// Heard is a chat line a test player got: a local line, a whisper, a
// channel's line, or a system line.
type Heard struct {
	// Channel is "public", "whisper", "system" or a channel's name with
	// its #.
	Channel string
	From    string
	Text    string
}

// NewHeadless makes a zone that tests drive. mods must have Testing set,
// so the runtime has wa.testing and starts the mods' entry scripts at
// Bind. seed fixes the zone's random stream.
func NewHeadless(mods *Mods, seed [32]byte) (*Headless, error) {
	if mods == nil || !mods.Testing {
		return nil, errors.New("zone: a headless zone needs Mods with Testing set")
	}
	z := New("", nil, rng.FromKey(seed).Stream(), nil)
	z.Mods = mods
	h := &Headless{z: z, events: map[int64][]*wathenav1.Event{}, Start: DefaultTestTime}
	z.Clock = func() time.Time { return h.Start.Add(time.Duration(z.now) * time.Millisecond) }
	if z.itemScripts() == nil {
		return nil, errors.New("zone: the mods' Lua state could not be made; see the log")
	}
	return h, nil
}

// DefaultTestTime is the server time a headless zone starts at:
// Monday 5 January 2026, 12:00 UTC.
var DefaultTestTime = time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

// Zone returns the zone, to set its tables before Bind.
func (h *Headless) Zone() *Zone { return h.z }

// Runtime returns the zone's Lua state.
func (h *Headless) Runtime() *modscript.Runtime { return h.z.scripts }

// Bound reports whether Bind has run, and the map's asset name.
func (h *Headless) Bound() (string, bool) { return h.z.Map, h.bound }

// Bind sets the zone's map, cells, NPCs and warps, and starts the mods'
// entry scripts. It runs once.
func (h *Headless) Bind(mapName string, grid *path.Grid, npcs []NPC, warps []Warp) error {
	if h.bound {
		return fmt.Errorf("zone: the world already runs %s", h.z.Map)
	}
	z := h.z
	z.Map, z.grid, z.SaveMap = mapName, grid, mapName
	z.Metrics.Name = mapName
	z.SetWarps(warps)
	z.SetNPCs(npcs)
	h.bound = true
	return z.scripts.StartMods()
}

// Close ends the zone's scripts.
func (h *Headless) Close() { h.z.stopScripts() }

// step runs one tick at a zone time, or at the current time when at is
// not later, and reads every player's messages. Only Advance, waits in
// a conversation, and the item use interval move the clock.
func (h *Headless) step(at int64) {
	z := h.z
	z.drainChat()
	z.drainSocial()
	z.runScripts()
	z.stepAt(max(at, z.now))
	z.runScripts()
	if h.AfterStep != nil {
		h.AfterStep()
		z.runScripts()
	}
	z.commitTransfers()
	h.drain()
}

// drain reads the messages of every player, and the announcements.
func (h *Headless) drain() {
	for {
		select {
		case a := <-h.z.announces:
			h.z.announce(a)
			h.announcements = append(h.announcements, a.text)
			continue
		case n := <-h.z.notices:
			h.z.worldNotice(n)
			continue
		default:
		}
		break
	}
	for id, p := range h.z.players {
		events := readEvents(p.Out)
		h.events[int64(id)] = append(h.events[int64(id)], events...)
		for _, e := range events {
			c := e.GetChat()
			if c == nil {
				continue
			}
			heard := Heard{From: c.GetFromName(), Text: c.GetText()}
			switch c.GetChannel() {
			case wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL:
				heard.Channel = "public"
			case wathenav1.ChatChannel_CHAT_CHANNEL_WHISPER:
				heard.Channel = "whisper"
			case wathenav1.ChatChannel_CHAT_CHANNEL_NAMED:
				heard.Channel = c.GetChannelName()
			default:
				heard.Channel = "system"
			}
			if h.heard == nil {
				h.heard = map[int64][]Heard{}
			}
			h.heard[int64(id)] = append(h.heard[int64(id)], heard)
		}
	}
}

// Say makes a player say a line, as a client's Chat: in local chat
// ("public"), as a whisper to a name, or in a channel by name. The line
// passes the mute, the filter and the hooks, and the zone runs a tick.
func (h *Headless) Say(player int64, channel, text, to string) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	if t, ok := cleanLine(text); !ok {
		return errors.New("the text must be 1 to 255 characters of UTF-8")
	} else {
		text = t
	}
	cmd := &chatCommand{kind: chatLocal, text: text}
	switch channel {
	case "public":
	case "whisper":
		if to == "" {
			return errors.New("a whisper needs to, the target's name")
		}
		cmd.kind, cmd.to = chatWhisper, to
	default:
		cmd.kind, cmd.to = chatChannel, channel
	}
	h.z.say(p, cmd)
	h.step(0)
	h.step(0)
	return nil
}

// Heard returns the chat lines a player got so far, oldest first.
func (h *Headless) Heard(player int64) []Heard { return slices.Clone(h.heard[player]) }

// readEvents reads the messages waiting on a channel and returns their
// events.
func readEvents(out <-chan *wathenav1.ServerMessage) []*wathenav1.Event {
	var list []*wathenav1.Event
	for {
		select {
		case m, ok := <-out:
			if !ok {
				return list
			}
			list = append(list, m.GetFrame().GetEvents()...)
		default:
			return list
		}
	}
}

// Advance moves the zone clock forward, one tick at a time. Timers and
// waiting conversations run as they come due.
func (h *Headless) Advance(ms int64) {
	end := h.z.now + ms
	for h.z.now+Tick.Milliseconds() <= end {
		h.step(h.z.now + Tick.Milliseconds())
	}
	if h.z.now < end {
		h.step(end)
	}
}

// TestPlayer is a character that a test makes.
type TestPlayer struct {
	Name                     string
	X, Y                     int
	Job, BaseLevel, JobLevel int
	Zeny                     int64
	Items                    []StartItem
	// Female makes a female character. Skills are learned skills by ID,
	// Stats the raised stats (nil for the start character's), and
	// SkillPoints and StatusPoints the points left to spend (nil for the
	// level's).
	Female                    bool
	Skills                    map[int]int
	Stats                     *status.Stats
	SkillPoints, StatusPoints *int
}

// AddPlayer adds a character at a cell, with full HP and SP, and returns
// its player ID, which player handles hold.
func (h *Headless) AddPlayer(tp TestPlayer) (int64, error) {
	z := h.z
	if !h.bound {
		return 0, errors.New("zone: the world has no map yet")
	}
	if tp.X < 0 || tp.Y < 0 || tp.X >= z.grid.Width || tp.Y >= z.grid.Height || z.grid.NoPass(tp.X, tp.Y) {
		return 0, fmt.Errorf("%d,%d is not a walkable cell of %s", tp.X, tp.Y, strings.TrimPrefix(z.Map, "map/"))
	}
	if tp.BaseLevel < 1 || tp.JobLevel < 1 || tp.BaseLevel > 999 || tp.JobLevel > 999 {
		return 0, errors.New("base_level and job_level are 1 to 999")
	}
	if tp.Zeny < 0 || tp.Zeny > character.MaxZeny {
		return 0, fmt.Errorf("zeny is 0 to %d", character.MaxZeny)
	}
	sex := uint32(1)
	if tp.Female {
		sex = 0
	}
	c := z.NewCharacter(tp.Name, Look{Sex: sex})
	c.Job, c.BaseLevel, c.JobLevel, c.Zeny = tp.Job, tp.BaseLevel, tp.JobLevel, tp.Zeny
	if tp.Stats != nil {
		c.Stats = *tp.Stats
	}
	if len(tp.Skills) > 0 {
		c.Skills = maps.Clone(tp.Skills)
	}
	if tp.SkillPoints != nil {
		c.SkillPoints = *tp.SkillPoints
	}
	if tp.StatusPoints != nil {
		c.StatusPoints = *tp.StatusPoints
	}
	c.X, c.Y = tp.X, tp.Y
	if z.Status != nil {
		sc := z.Start
		sc.Job, sc.Male, sc.BaseLevel, sc.JobLevel = c.Job, c.Male, c.BaseLevel, c.JobLevel
		if _, err := z.Status.Calc(sc); err != nil {
			return 0, fmt.Errorf("job %d: %w", tp.Job, err)
		}
	}
	// HP 0 enters with full HP. SP is capped at the maximum.
	c.HP, c.SP = 0, math.MaxInt64
	inv := item.NewInventory()
	for _, s := range tp.Items {
		d, ok := z.Items.Lookup(s.ID)
		if !ok {
			return 0, fmt.Errorf("there is no item %d", s.ID)
		}
		i, r := inv.Add(d, s.Amount, math.MaxInt)
		if r != item.AddSuccess {
			return 0, fmt.Errorf("%d of item %s do not fit in the inventory", s.Amount, d.AegisName)
		}
		if inv.Slots[i].Serial.IsZero() {
			inv.Slots[i].Serial = item.NewSerial()
		}
	}
	c.Items = savedItems(inv)
	h.nextID++
	c.ID, c.AccountID = h.nextID, h.nextID
	// A large buffer, since the messages are read after each tick only.
	out := make(chan *wathenav1.ServerMessage, 1<<14)
	s := &Session{Out: out, out: out, left: make(chan struct{})}
	p := newPlayer(s, &c)
	s.zone, s.player = z, p
	z.join(p)
	h.step(0)
	return int64(p.charID), nil
}

func (h *Headless) player(id int64) (*Player, error) {
	return scriptHost{h.z}.player(id)
}

// findNPC finds an NPC by its full record name, its name in the mod
// modID, or the name players see when one NPC of the map has it.
func (h *Headless) findNPC(name, modID string) (*entity, error) {
	var byDisplay []*entity
	for e := range h.z.entities.all() {
		n := e.npc
		if n == nil {
			continue
		}
		if n.Name == name || n.Name == modID+":"+name {
			return e, nil
		}
		if n.DisplayName == name {
			byDisplay = append(byDisplay, e)
		}
	}
	switch len(byDisplay) {
	case 1:
		return byDisplay[0], nil
	case 0:
		return nil, fmt.Errorf("there is no NPC %q on %s", name, strings.TrimPrefix(h.z.Map, "map/"))
	}
	var names []string
	for _, e := range byDisplay {
		names = append(names, e.npc.Name)
	}
	slices.Sort(names)
	return nil, fmt.Errorf("%d NPCs on %s show the name %q; use a full name: %s", len(names), strings.TrimPrefix(h.z.Map, "map/"), name, strings.Join(names, ", "))
}

// Talk clicks an NPC and answers each step that waits for the player,
// in order: an integer picks a menu option, an integer fills number
// input, and a string fills text input. Next and close are pressed
// without an answer, and waits and progress bars run on the clock. It
// returns the NPC's lines, one per line. It fails when the conversation
// asks for more answers than given, when an answer does not fit its
// step, or when answers are left over.
func (h *Headless) Talk(player int64, npcName, modID string, answers []script.Value) (string, error) {
	z := h.z
	p, err := h.player(player)
	if err != nil {
		return "", err
	}
	e, err := h.findNPC(npcName, modID)
	if err != nil {
		return "", err
	}
	if p.talk != nil {
		return "", errors.New("the player is already talking to an NPC")
	}
	if !near(p, e) {
		return "", fmt.Errorf("%s at %d,%d is too far from the player at %d,%d: players reach NPCs within %d cells",
			e.npc.Name, e.x, e.y, p.entity.x, p.entity.y, npcClickRange)
	}
	h.events[player] = nil
	netID := p.netID(e)
	z.handle(command{player: p, npcClick: &netID})
	h.step(0)
	used := 0
	stop := func(format string, args ...any) (string, error) {
		if p.talk != nil {
			z.closeTalk(p)
			h.step(0)
		}
		return "", fmt.Errorf(format, args...)
	}
	for range 10_000 {
		t := p.talk
		if t == nil {
			break
		}
		r := wathenav1.NpcReply_builder{Dialog: t.dialog}
		switch {
		case t.ended:
			r.Close = new(true)
		case t.wake > 0:
			h.step(t.wake)
			continue
		case !t.waiting:
			h.step(z.now + Tick.Milliseconds())
			continue
		default:
			switch t.step.Kind {
			case modscript.StepNext:
				r.Next = new(true)
			case modscript.StepClose:
				r.Close = new(true)
			case modscript.StepMenu, modscript.StepNumber, modscript.StepText:
				what := describeStep(t.step)
				if used >= len(answers) {
					return stop("the conversation asks for %s after %d answers; give more answers", what, used)
				}
				a := answers[used]
				used++
				switch t.step.Kind {
				case modscript.StepMenu:
					n, ok := a.AsInt()
					if !ok || n < 1 || int(n) > len(t.step.Options) || t.step.Options[n-1] == "" {
						return stop("answer %d is %s, and the conversation asks for %s", used, a, what)
					}
					r.Menu = new(uint32(n)) // #nosec G115 -- n is checked to be 1 to len(t.step.Options) above.
				case modscript.StepNumber:
					n, ok := a.AsInt()
					if !ok || n < t.step.Min || n > t.step.Max {
						return stop("answer %d is %s, and the conversation asks for %s", used, a, what)
					}
					r.InputNumber = new(n)
				case modscript.StepText:
					s, ok := a.AsString()
					if !ok || s == "" {
						return stop("answer %d is %s, and the conversation asks for %s", used, a, what)
					}
					r.InputText = new(s)
				}
			default:
				h.step(z.now + Tick.Milliseconds())
				continue
			}
		}
		dialog := t.dialog
		z.handle(command{player: p, npcReply: r.Build()})
		h.step(0)
		if p.talk == t && t.waiting && t.dialog == dialog {
			return stop("the NPC did not take answer %d", used)
		}
	}
	if p.talk != nil {
		return stop("the conversation did not end")
	}
	var lines []string
	for _, ev := range h.events[player] {
		if d := ev.GetNpcDialog(); d != nil {
			lines = append(lines, d.GetLines()...)
		}
	}
	h.events[player] = nil
	if used < len(answers) {
		return "", fmt.Errorf("the conversation ended after %d of %d answers", used, len(answers))
	}
	return strings.Join(lines, "\n"), nil
}

// describeStep says what a waiting step asks for, in errors.
func describeStep(s modscript.Step) string {
	switch s.Kind {
	case modscript.StepMenu:
		var opts []string
		for i, o := range s.Options {
			if o != "" {
				opts = append(opts, fmt.Sprintf("%d %q", i+1, o))
			}
		}
		return "a menu choice (" + strings.Join(opts, ", ") + ")"
	case modscript.StepNumber:
		return fmt.Sprintf("a number from %d to %d", s.Min, s.Max)
	case modscript.StepText:
		return fmt.Sprintf("text of 1 to %d characters", s.MaxLength)
	}
	return "an answer"
}

// UseItem uses one of a player's items, as a click on it. It waits for
// the item use interval first, as a player who clicks again. It fails
// when the player has no such item, or the zone refuses the use.
func (h *Headless) UseItem(player int64, itemID uint32) error {
	z := h.z
	p, err := h.player(player)
	if err != nil {
		return err
	}
	slot := -1
	for i, s := range p.inventory.Slots {
		if s.ID == itemID && s.Amount > 0 {
			slot = i
			break
		}
	}
	name := z.Items.Get(itemID).AegisName
	if slot < 0 {
		return fmt.Errorf("the player has no %s (%d)", name, itemID)
	}
	if p.canUseItem > z.now {
		h.step(p.canUseItem)
	}
	h.events[player] = nil
	z.handle(command{player: p, use: &slot})
	h.step(0)
	for _, ev := range h.events[player] {
		if f := ev.GetUseItemFailed(); f != nil {
			why := strings.ToLower(strings.TrimPrefix(f.GetReason().String(), "USE_ITEM_FAILURE_"))
			return fmt.Errorf("the zone refused to use %s: %s", name, strings.ReplaceAll(why, "_", " "))
		}
	}
	return nil
}

// SpawnMonster places one monster of a spawn on its cell, and returns
// its handle, which monster handles hold. It does not come back after
// it dies.
func (h *Headless) SpawnMonster(s Spawn) (int64, error) {
	z := h.z
	if !h.bound {
		return 0, errors.New("zone: the world has no map yet")
	}
	if s.X < 0 || s.Y < 0 || s.X >= z.grid.Width || s.Y >= z.grid.Height || z.grid.NoPass(s.X, s.Y) {
		return 0, fmt.Errorf("%d,%d is not a walkable cell of %s", s.X, s.Y, strings.TrimPrefix(z.Map, "map/"))
	}
	s.Amount, s.W, s.H, s.noRespawn = 1, 0, 0, true
	e := z.spawnOne(&s)
	if e == nil {
		return 0, errors.New("the monster found no cell")
	}
	id := z.monsterHandle(e)
	h.step(0)
	return id, nil
}

// Kill has a player kill a monster at once: the monster dies as if the
// player's last hit took its HP, and the player gets its exp and drops.
// death handlers run. monster is any monster handle.
func (h *Headless) Kill(player, monster int64) error {
	z := h.z
	p, err := h.player(player)
	if err != nil {
		return err
	}
	e := z.monsterHandles[monster]
	if e == nil || e.dead || !z.entities.has(e) {
		return errors.New("the monster is dead or gone")
	}
	z.logDamage(p.entity, e, max(e.hp, 1))
	e.hp = 0
	z.die(e, p.entity)
	h.step(0)
	return nil
}

// Monsters returns the handles of the living monsters on the map, of
// one monster ID, or of every monster when id is 0, in the order they
// spawned.
func (h *Headless) Monsters(id uint32) []int64 {
	var out []int64
	for e := range h.z.entities.all() {
		if e.monster != nil && !e.dead && (id == 0 || e.monster.spawn.Mob == id) {
			out = append(out, h.z.monsterHandle(e))
		}
	}
	return out
}

// Announcements returns the lines announced to the world or the map so
// far.
func (h *Headless) Announcements() []string { return slices.Clone(h.announcements) }

// FloorItems counts the items of an ID lying on the map, and those
// about to fall.
func (h *Headless) FloorItems(id uint32) int {
	n := 0
	for e := range h.z.entities.all() {
		if e.item != nil && e.item.id == id {
			n += e.item.amount
		}
	}
	for _, f := range h.z.falling {
		for _, it := range f.items {
			if it.id == id {
				n += max(it.amount, 1)
			}
		}
	}
	return n
}

// TakeFloorItems puts every item lying on the map in a player's
// inventory, without pick-up rights or distance, for Go tests of mods.
// Items that do not fit stay.
func (h *Headless) TakeFloorItems(player int64) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	z := h.z
	// Drops wait for their drop delay first.
	for i := 0; i < 10 && len(z.falling) > 0; i++ {
		h.step(z.now + 1000)
	}
	var items []*entity
	for e := range z.entities.all() {
		if e.item != nil {
			items = append(items, e)
		}
	}
	for _, e := range items {
		g := e.item
		if z.addCopy(p, g.id, g.amount, g.refine, g.cards, g.extra, g.pickUpOrigin()) == item.AddSuccess {
			z.entities.remove(e)
		}
	}
	h.step(0)
	return nil
}

// EquipItem puts on a player's copy in a slot, for Go tests of mods.
func (h *Headless) EquipItem(player int64, slot int) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	h.z.handle(command{player: p, equip: &equipCommand{slot: slot}})
	h.step(0)
	if p.inventory.Slots[slot].Equip == 0 {
		return fmt.Errorf("slot %d did not go on", slot)
	}
	return nil
}

// Equip gives a player one copy of an item, with the roll option of
// player:give_item, and puts it on, as a click on it. It returns the
// handle of the equipped copy, as player:get_equipped. It fails when the
// copy does not fit or does not go on. A copy that does not go on stays
// in the inventory.
func (h *Headless) Equip(player int64, itemID uint32, roll modscript.GiveRoll) (int64, error) {
	z := h.z
	p, err := h.player(player)
	if err != nil {
		return 0, err
	}
	d, ok := z.Items.Lookup(itemID)
	if !ok {
		return 0, fmt.Errorf("there is no item %d", itemID)
	}
	had := map[int]bool{}
	for i, s := range p.inventory.Slots {
		if s.ID == itemID {
			had[i] = true
		}
	}
	fits, err := z.giveRolled(p, itemID, 1, roll, item.BoundNone)
	if err != nil {
		return 0, err
	}
	slot := -1
	for i, s := range p.inventory.Slots {
		if s.ID == itemID && !had[i] {
			slot = i
			break
		}
	}
	if !fits || slot < 0 {
		return 0, fmt.Errorf("%s (%d) does not fit in the inventory", d.AegisName, itemID)
	}
	h.events[player] = nil
	z.handle(command{player: p, equip: &equipCommand{slot: slot}})
	h.step(0)
	for _, ev := range h.events[player] {
		if f := ev.GetEquipFailed(); f != nil {
			why := strings.ToLower(strings.TrimPrefix(f.GetReason().String(), "EQUIP_FAILURE_"))
			return 0, fmt.Errorf("the zone refused to equip %s: %s", d.AegisName, strings.ReplaceAll(why, "_", " "))
		}
	}
	w := p.worn[slot]
	if p.inventory.Slots[slot].Equip == 0 || w == nil {
		return 0, fmt.Errorf("%s (%d) did not go on", d.AegisName, itemID)
	}
	return w.handle, nil
}

// Copies returns the slots of a player's copies of an item, by slot, for
// Go tests of mods.
func (h *Headless) Copies(player int64, itemID uint32) (map[int]item.Slot, error) {
	p, err := h.player(player)
	if err != nil {
		return nil, err
	}
	out := map[int]item.Slot{}
	for i, s := range p.inventory.Slots {
		if s.ID == itemID {
			out[i] = s
		}
	}
	return out, nil
}

// Now returns the server time.
func (h *Headless) Now() time.Time { return h.z.wallClock() }

// SetClock sets the server time when the zone clock is 0, and the
// server's time zone. Call it before Bind.
func (h *Headless) SetClock(start time.Time, loc *time.Location) error {
	if h.bound {
		return errors.New("the world already has a map")
	}
	h.Start = start
	mods := *h.z.Mods
	mods.Location = loc
	h.z.Mods = &mods
	h.z.scripts.SetLocation(loc)
	return nil
}

// Location returns the server's time zone.
func (h *Headless) Location() *time.Location {
	if h.z.Mods.Location == nil {
		return time.UTC
	}
	return h.z.Mods.Location
}

// StartEvent starts a run of a world event in the zone, on the calling
// goroutine.
func (h *Headless) StartEvent(run EventRun) error {
	err := h.z.startEvent(run)
	h.z.runScripts()
	return err
}

// EndEvent ends a run of a world event.
func (h *Headless) EndEvent(id int64) {
	h.z.endEvent(id)
	h.z.runScripts()
}

// EventCondition runs the condition of a world event.
func (h *Headless) EventCondition(mod, module, fn string) (bool, error) {
	return h.z.eventCondition(mod, module, fn)
}

// UseSkill has a player use a skill on a monster, as the client's skill
// command. It is for Go tests of mods: wa.testing has no call for it.
func (h *Headless) UseSkill(player int64, skillID, level int, monster int64) error {
	z := h.z
	p, err := h.player(player)
	if err != nil {
		return err
	}
	e := z.monsterHandles[monster]
	if e == nil || e.dead || !z.entities.has(e) {
		return errors.New("the monster is dead or gone")
	}
	z.handle(command{player: p, useSkill: &useSkillCommand{skill: skillID, level: level,
		target: SkillTarget{Entity: p.netID(e), HasEntity: true}}})
	h.step(0)
	return nil
}

// MonsterHP returns a monster's HP. It is for Go tests of mods.
func (h *Headless) MonsterHP(monster int64) (int64, error) {
	e := h.z.monsterHandles[monster]
	if e == nil || !h.z.entities.has(e) {
		return 0, errors.New("the monster is gone")
	}
	return e.hp, nil
}

// SkillList returns the skills of a player's skill list and whether a
// point raises each, as the client's skill window shows them. It is for
// Go tests of mods.
func (h *Headless) SkillList(player int64) (map[int]bool, error) {
	z := h.z
	p, err := h.player(player)
	if err != nil {
		return nil, err
	}
	if !z.skillsReady() {
		return nil, errors.New("the zone has no skill data")
	}
	entries, err := z.SkillTrees.List(z.Skills, z.Status, z.learner(p), z.SkillSettings)
	if err != nil {
		return nil, err
	}
	out := map[int]bool{}
	for _, e := range entries {
		out[e.Skill] = e.Upgradable
	}
	return out, nil
}

// TradeOffer is one side of a test world's trade: items by ID with their
// amounts, and zeny.
type TradeOffer struct {
	Items map[uint32]int
	Zeny  int64
}

// Trade runs a whole trade between two players for wa.testing's
// world:trade: b stands next to a, a asks, b accepts, both offer, lock
// and confirm. It returns true when the trade completed, and otherwise
// the outcome's name, or the trade hook's message.
func (h *Headless) Trade(a, b int64, offerA, offerB TradeOffer) (bool, string, error) {
	pa, err := h.player(a)
	if err != nil {
		return false, "", err
	}
	pb, err := h.player(b)
	if err != nil {
		return false, "", err
	}
	if pa == pb {
		return false, "", errors.New("a player cannot trade with itself")
	}
	z := h.z
	pb.entity.x, pb.entity.y = pa.entity.x, pa.entity.y
	h.step(0)
	last := ""
	run := func(p *Player, t TradeAction) {
		before := len(p.pending)
		z.tradeCommand(p, &t)
		for _, e := range p.pending[min(before, len(p.pending)):] {
			switch {
			case e.HasTradeResult() && e.GetTradeResult().GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT &&
				e.GetTradeResult().GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_WAITING:
				last = tradeOutcomeName(e.GetTradeResult().GetOutcome())
				if m := e.GetTradeResult().GetMessage(); m != "" {
					last = m
				}
			}
		}
	}
	run(pa, TradeAction{Request: pa.netID(pb.entity)})
	run(pb, TradeAction{Respond: true, Accept: true})
	if pa.trade == nil {
		h.step(0)
		return false, last, nil
	}
	for _, side := range []struct {
		p *Player
		o TradeOffer
	}{{pa, offerA}, {pb, offerB}} {
		for id, n := range side.o.Items {
			for i := range side.p.inventory.Slots {
				s := &side.p.inventory.Slots[i]
				if n <= 0 {
					break
				}
				if s.ID == id && s.Equip == 0 {
					k := min(n, s.Amount)
					run(side.p, TradeAction{AddSlot: i, AddAmount: k})
					n -= k
				}
			}
			if last != "" {
				// A refused offer ends the trade.
				z.tradeCancel(side.p, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED)
				h.step(0)
				return false, last, nil
			}
			if n > 0 {
				z.tradeCancel(side.p, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED)
				h.step(0)
				return false, "", fmt.Errorf("the player has too few of item %d", id)
			}
		}
		if side.o.Zeny > 0 {
			run(side.p, TradeAction{Zeny: side.o.Zeny, HasZeny: true})
		}
	}
	if pa.trade == nil || pb.trade == nil {
		h.step(0)
		return false, last, nil
	}
	run(pa, TradeAction{Lock: true})
	run(pb, TradeAction{Lock: true})
	run(pa, TradeAction{Confirm: true})
	done := pb.trade != nil
	run(pb, TradeAction{Confirm: true})
	ok := done && pb.trade == nil && last == ""
	if !ok {
		z.tradeCancel(pa, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED)
	}
	h.step(0)
	h.step(0)
	return ok, last, nil
}

// tradeOutcomeName is an outcome's name in lower case, such as
// "restricted".
func tradeOutcomeName(o wathenav1.TradeOutcome) string {
	return strings.ToLower(strings.TrimPrefix(o.String(), "TRADE_OUTCOME_"))
}

// CloseStorage closes a player's storage window, for wa.testing's
// world:close_storage.
func (h *Headless) CloseStorage(player int64) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	if p.storage != nil {
		h.z.closeStorage(p)
	} else {
		h.z.closeGuildStorage(p, true)
	}
	h.step(0)
	return nil
}

// Buy buys from a shop of data/npcs.yml for wa.testing's world:buy, as
// player:open_shop then the window's Buy. It returns true when the
// purchase went through, and otherwise the outcome's name in lower
// case, such as "out_of_stock".
func (h *Headless) Buy(player int64, name string, items map[uint32]int) (bool, string, error) {
	p, err := h.player(player)
	if err != nil {
		return false, "", err
	}
	z := h.z
	var s *shop.Shop
	if z.Mods != nil {
		s = z.Mods.Shops.Get(name)
	}
	if s == nil {
		return false, "", fmt.Errorf("there is no shop %q", name)
	}
	z.openShop(p, s, nil)
	var lines []buyLine
	for _, id := range slices.Sorted(maps.Keys(items)) {
		lines = append(lines, buyLine{id: id, amount: items[id]})
	}
	before := len(p.pending)
	z.buy(p, 0, lines)
	out := ""
	for _, e := range p.pending[min(before, len(p.pending)):] {
		if r := e.GetShopResult(); r != nil {
			out = strings.ToLower(strings.TrimPrefix(r.GetOutcome().String(), "SHOP_OUTCOME_"))
		}
	}
	h.step(0)
	return out == "bought", out, nil
}

// TestMail is a mail as wa.testing's world:get_mails shows it.
type TestMail struct {
	From, Title    string
	Zeny           int64
	Items          map[uint32]int
	Read, Returned bool
}

// Mails lists a player's mails, oldest first, for world:get_mails.
func (h *Headless) Mails(player int64) ([]TestMail, error) {
	p, err := h.player(player)
	if err != nil {
		return nil, err
	}
	h.step(0)
	b := h.z.mailboxOf(p)
	if b == nil {
		return nil, nil
	}
	var out []TestMail
	for _, m := range b.Mails() {
		m.Lock()
		if !m.Deleted() {
			t := TestMail{From: m.FromName, Title: m.Title, Zeny: m.Zeny, Items: map[uint32]int{}, Read: m.Read, Returned: m.Returned}
			for _, s := range m.Box.Slots {
				if s.ID != 0 {
					t.Items[s.ID] += s.Amount
				}
			}
			out = append(out, t)
		}
		m.Unlock()
	}
	return out, nil
}

// VendLine is one line of world:vend: an item of the seller, the amount
// to put on sale, its price, and how many the buyer buys.
type VendLine struct {
	Item          uint32
	Amount, Price int
	Buy           int
}

// Vend runs a whole vending sale for wa.testing's world:vend: the seller
// gets a cart and the Vending skill, puts the lines' items in the cart,
// opens its shop, and the buyer, next to it, buys. It returns true when
// the sale went through, and otherwise the outcome's name, or the vend
// hook's message. The shop closes after it.
func (h *Headless) Vend(seller, buyer int64, lines []VendLine) (bool, string, error) {
	ps, err := h.player(seller)
	if err != nil {
		return false, "", err
	}
	pb, err := h.player(buyer)
	if err != nil {
		return false, "", err
	}
	if ps == pb {
		return false, "", errors.New("a player cannot buy from itself")
	}
	z := h.z
	pb.entity.x, pb.entity.y = ps.entity.x, ps.entity.y
	if ps.char.Skills == nil {
		ps.char.Skills = map[int]int{}
	}
	ps.char.Skills[skillPushCart] = max(ps.char.Skills[skillPushCart], 1)
	ps.char.Skills[skill.IDVending] = max(ps.char.Skills[skill.IDVending], 10)
	if !z.hasCart(ps) {
		z.setCart(ps, 1)
	}
	h.step(0)
	var open []*wathenav1.VendingLine
	var buy []*wathenav1.VendingBuyLine
	for i, l := range lines {
		slot := -1
		for j := range ps.inventory.Slots {
			if s := &ps.inventory.Slots[j]; s.ID == l.Item && s.Equip == 0 && s.Amount >= l.Amount {
				slot = j
				break
			}
		}
		if slot < 0 {
			return false, "", fmt.Errorf("the seller has too few of item %d", l.Item)
		}
		z.storageMove(ps, &storageMove{kind: MoveCartPut, slot: slot, amount: l.Amount})
		cart := z.cartOf(ps)
		cartSlot := -1
		cart.Lock()
		for j, s := range cart.Box().Slots {
			if s.ID == l.Item {
				cartSlot = j
			}
		}
		cart.Unlock()
		if cartSlot < 0 {
			return false, "", fmt.Errorf("item %d did not go into the cart", l.Item)
		}
		open = append(open, wathenav1.VendingLine_builder{CartSlot: uint32(cartSlot), Amount: uint32(l.Amount), Price: uint32(l.Price)}.Build()) // #nosec G115 -- small test values.
		if l.Buy > 0 {
			buy = append(buy, wathenav1.VendingBuyLine_builder{Index: uint32(i), Amount: uint32(l.Buy)}.Build()) // #nosec G115 -- as above.
		}
	}
	ps.prevend = min(len(open)+2, maxVending)
	z.openVending(ps, wathenav1.VendingOpen_builder{Title: "Test shop", Items: open}.Build())
	if ps.vend == nil {
		h.step(0)
		return false, "failed", nil
	}
	before := len(pb.pending)
	z.vendingBuy(pb, wathenav1.VendingBuy_builder{Vendor: pb.netID(ps.entity), Shop: ps.vend.id, Items: buy}.Build())
	out := ""
	for _, e := range pb.pending[min(before, len(pb.pending)):] {
		if r := e.GetVendingResult(); r != nil {
			out = strings.ToLower(strings.TrimPrefix(r.GetOutcome().String(), "VENDING_OUTCOME_"))
			if m := r.GetMessage(); m != "" {
				out = m
			}
		}
	}
	z.closeVending(ps)
	h.step(0)
	h.step(0)
	return out == "bought", out, nil
}

// SpawnEvent places a typed run's monsters, on the calling goroutine.
func (h *Headless) SpawnEvent(s worldevent.Spawn) ([]worldevent.Placed, error) {
	if !h.bound || s.Map != h.z.Map {
		return nil, fmt.Errorf("the map %s does not run in this world", worldevent.MapID(s.Map))
	}
	return h.z.spawnEvent(s)
}

// EventAlive counts a run's monsters that live.
func (h *Headless) EventAlive(run int64) int { return h.z.eventAlive(run) }

// EventHints places a treasure hunt's hint NPCs.
func (h *Headless) EventHints(run int64, hints []worldevent.Hint) { h.z.eventHints(run, hints) }

// EventReward gives a run's rewards for a unit of progress.
func (h *Headless) EventReward(rr worldevent.RewardRun) { h.z.eventReward(rr) }

// ClearEvent removes a run's monsters and NPCs.
func (h *Headless) ClearEvent(run int64) { h.z.clearEvent(run) }
