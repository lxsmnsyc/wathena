package zone

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/internal/script"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/currency"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/quest"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/shop"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/variables"
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
	// Shops are the shops of mods' data, for player:open_shop and
	// world:get_shop. Placed shop NPCs carry their own, which is the
	// registry's too.
	Shops *shop.Registry
	// Server holds the server variables. Without it, server variables
	// raise an error.
	Server *variables.Server
	// Temporary holds the temporary server variables and NPC variables
	// that every zone shares (review decision 58). Without it, each zone
	// keeps its own.
	Temporary *variables.Temporary
	// Testing gives the Lua state wa.testing, and starts the mods' entry
	// scripts when a Headless zone gets its map. Only wactl mod test
	// sets it.
	Testing bool
	// Monsters are every monster's spawn of one, by monster ID, with
	// its stats, for spawning.monsters.
	Monsters map[uint32]Spawn
	// OwnLookJobs are the jobs that a look shows by their own ID: mods'
	// jobs, and jobs whose records name body sprites of art. The client
	// finds their art and the job they look like in the client data
	// (review decision 40).
	OwnLookJobs map[int]bool
	// Location is the server's time zone, where schedules match their
	// times. It is UTC when nil.
	Location *time.Location
	// RandomOptions are the random options and their groups, for drops
	// and equipment (strategy section 5.4).
	RandomOptions *mod.RandomOptions
	// ItemRolls is the item_rolls ruleset flag. Off, every roll of a new
	// copy is its default, as rAthena. Random options roll either way.
	ItemRolls bool
	// Rules are the world's ruleset flags (decision 0019). Without them
	// the zone follows the core ruleset of its base.
	Rules *rules.Rules
	// Companions are the companions and mounts. Without them the zone
	// uses core's (companion.Core).
	Companions *companion.Set
	// Storages are the extra storages of data/storages.yml, by ID.
	Storages []*storage.Record
	// Currencies are the currencies of data/currencies.yml. Without them
	// the zone uses core's (currency.Core).
	Currencies *currency.Table
	// Chat is the chat channels and filter, for a zone that makes its own
	// chat hub, such as a headless zone. Without it the zone uses core's
	// (chat.Core).
	Chat *chat.Data
	// Guilds are the guild tables. Without them the zone uses core's
	// tables of its base (social.CoreGuildTables).
	Guilds *social.GuildTables
	// Places are the named places of the world's generated maps, for
	// wa.places (review decision 42). A classic world has none.
	Places *places.Index
	// Atlas is the world layout (decision 0041): display names, towns,
	// the map graph, and the world map. A classic world has none.
	Atlas *layout.Atlas
	// Teleport is the teleport service's prices.
	Teleport TeleportPrices
	// DropRollLevels makes rolled drops better in higher bands (strategy
	// section 8.11, "Rolled drops", decision 0046): on a map of the
	// atlas, each full DropRollLevels levels below the band's lowest
	// level adds one draw to each random roll of a monster's drop, and
	// the best draw stays. 0 turns it off.
	DropRollLevels int
	// Quests are the quests of data/quests.yml (review decision 45).
	// Without them a zone has no quests.
	Quests *quest.Table
	// World are a procedural world's quests for core's quest boards and
	// job guilds (decision 0044), by ID. A classic world has none.
	World []modscript.CoreQuest
}

// NPC is an NPC placed on the map by a mod's data. It shows to players.
// A click runs its script's click handlers, or opens its shop.
type NPC struct {
	// Name is the full record name, such as example.town:Healer.
	Name string
	// DisplayName is the name players see.
	DisplayName string
	// Hidden is the part after "#" of the NPC's name, as rAthena's
	// strnpcinfo(2): the record's, or the display_name field's (mod.NPC).
	Hidden string
	X, Y   int
	// Direction is 0 to 7, starting south and turning clockwise.
	Direction int
	// Sprite is the NPC sprite ID.
	Sprite uint32
	// Script is the NPC's entry script, as <mod>:<path>, or "".
	Script string
	// Shop is the NPC's shop, or nil.
	Shop *shop.Shop
	// Service is a service the server runs for the NPC, such as
	// ServiceTeleport, or "". Mods' records cannot set it.
	Service string
	// Touch is the NPC's touch area, or nil (touch.go).
	Touch *TouchArea
	// State is how the NPC starts: "" or shown, disabled, hidden or
	// cloaked, as rAthena's script(DISABLED), (HIDDEN) and (CLOAKED)
	// lines (review decision 58).
	State string
	// NoSprite is an NPC without a view, as rAthena's view -1: players
	// cannot see or click it, and its events and touch area work.
	NoSprite bool
	// Floating is an NPC without a map, which runs only through events,
	// in the start map's zone. It has no place.
	Floating bool
}

type npc struct {
	NPC
	// handle is the NPC's ID for scripts, and script its running
	// instance, once the zone started it.
	handle int64
	script *modscript.NPC
	// hint is the typed run of a hint NPC (autoevents.go), or 0.
	hint int64
	// The view state (npcworld.go): invisible after disablenpc, hide
	// and cloak as OPTION_HIDE and OPTION_CLOAK, and cloakFor the
	// characters for whom the cloak is the opposite. removed is set by
	// unloadnpc, and floating for an NPC without a map. entry is the
	// NPC's entry in the world's directory, or nil.
	invisible, hide, cloak bool
	cloakFor               map[int64]bool
	removed, floating      bool
	// guild is the guild whose badge the NPC shows (npc:set_guild,
	// flagemblem), 0 for none, and badge the badge viewers see.
	guild int64
	badge *wathenav1.GuildBadge
	entry *npcEntry
	// The presentation of review decision 58, area B5 (npcpresent.go):
	// the title of setunittitle, the look of a job view (setunitdata's
	// UNPC_ looks), the end and color of progressbar_npc's bar in zone
	// milliseconds, and the level and stat of npcskill.
	title      string
	look       npcLook
	barEnd     int64
	barColor   uint32
	skillLevel int
	skillStat  int
	// walkDone is the event label of npc:walk_to's label option, as
	// unitwalk's ud->walk_done_event, and walkX and walkY the cell of
	// the last walk, as ud->to_x and to_y (npcpresent.go).
	walkDone     *monsterLabel
	walkX, walkY int
}

// SetNPCs places NPCs on the map. It replaces the NPCs set before. Call
// it before Run.
func (z *Zone) SetNPCs(npcs []NPC) {
	for e := range z.entities.all() {
		if e.npc != nil {
			z.entities.remove(e)
		}
	}
	z.npcHandles = map[int64]*entity{}
	z.touchNPCs = nil
	for i, n := range npcs {
		if n.Hidden == "" {
			// A record without display_name's # part keeps its name's.
			_, n.Hidden, _ = strings.Cut(mod.Local(n.Name), "#")
		}
		e := &entity{npc: &npc{NPC: n, handle: int64(i + 1), floating: n.Floating, skillLevel: 1, skillStat: 1}, x: n.X, y: n.Y, dir: n.Direction,
			speed: defaultNPCSpeed}
		switch n.State {
		case "disabled":
			e.npc.invisible, e.npc.hide = true, true
		case "hidden":
			e.npc.hide = true
		case "cloaked":
			e.npc.cloak = true
		}
		z.npcHandles[e.npc.handle] = e
		if n.Floating {
			// An NPC without a map has no place on this one.
			continue
		}
		z.entities.add(e)
		if n.Touch != nil && n.Script != "" {
			z.touchNPCs = append(z.touchNPCs, e)
		}
	}
}

func (z *Zone) npcAppear(id uint32, e *entity) *wathenav1.Event {
	n := e.npc
	return wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{
		Id:     id,
		Kind:   wathenav1.EntityKind_ENTITY_KIND_NPC,
		X:      uint32(e.x),   // #nosec G115 -- map cells are at most 1024.
		Y:      uint32(e.y),   // #nosec G115 -- as above.
		Dir:    uint32(e.dir), // #nosec G115 -- 0 to 7.
		Look:   z.npcLookMessage(n),
		Name:   n.DisplayName,
		Title:  n.title,
		WalkMs: uint32(max(e.speed, 0)), // #nosec G115 -- 20 to 1000.
		Guild:  n.badge,
	}.Build()}.Build()
}

// scriptEvent is an event for mod scripts, held until the zone reaches a
// point where scripts may run: no entity list is being walked.
type scriptEvent struct {
	name string
	args []modscript.Arg
	// death is set for the death event, which runs with FireDeath, and
	// status for a status event, which runs with FireStatus.
	death  *modscript.Death
	status *modscript.StatusEvent
	// release are monster handles that end after the event runs.
	release []int64
	// monster is set to run the setup of a monster's AI script.
	monster *entity
	// chat is set for the chat event, which runs with FireChat.
	chat *modscript.ChatMessage
	// party is set for a party event, which runs with FireParty.
	party *modscript.PartyEvent
	// guild is set for a guild event, which runs with FireGuild.
	guild *modscript.GuildEvent
	// trade is set for the trade event, which runs with FireTrade.
	trade *modscript.TradeEvent
	// sale is set for the shop_buy, shop_sell, vend, buying_store_sale
	// and mail events, which run with FireSale.
	sale *modscript.SaleEvent
	// quest is set for the quest events, which run with FireQuest.
	quest *modscript.QuestEvent
	// touch is set for an NPC's touch and untouch events, which run
	// with Touch.
	touch *touchEvent
	// monsterTouch is set for an NPC's monster_touch event.
	monsterTouch *monsterTouch
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
	for _, sc := range z.modScripts() {
		if len(sc.Entries) > 0 {
			z.itemScripts()
			break
		}
	}
	// NPCs without a map start with the zone: they listen for the
	// events of the world from the start, as rAthena loads every NPC.
	for _, e := range z.npcHandles {
		if e.npc.floating && e.npc.Script != "" {
			z.startNPCs()
			return
		}
	}
}

// modScripts are the mods' scripts: Mods.Scripts, with the ones that a
// reload read in their place.
func (z *Zone) modScripts() []*mod.Scripts {
	scripts := z.Mods.Scripts
	if len(z.reloaded) == 0 {
		return scripts
	}
	scripts = slices.Clone(scripts)
	for i, sc := range scripts {
		if n := z.reloaded[sc.Mod]; n != nil {
			scripts[i] = n
		}
	}
	for _, id := range slices.Sorted(maps.Keys(z.reloaded)) {
		if !slices.ContainsFunc(scripts, func(sc *mod.Scripts) bool { return sc.Mod == id }) {
			scripts = append(scripts, z.reloaded[id])
		}
	}
	return scripts
}

// itemScripts returns the zone's Lua state, made on first use, or nil
// when the zone runs no scripts.
func (z *Zone) itemScripts() *modscript.Runtime {
	if z.scripts != nil || z.scriptsFailed || z.Mods == nil || len(z.Mods.Scripts) == 0 && len(z.reloaded) == 0 {
		return z.scripts
	}
	log := z.Mods.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	opts := modscript.Options{Host: scriptHost{z}, Log: log, Limits: z.Mods.Limits, Mods: z.Mods.IDs, Constants: z.Mods.Constants, Server: z.Mods.Server,
		Testing: z.Mods.Testing, DeferStart: z.Mods.Testing, Clock: z.wallClock, Location: z.Mods.Location, Rules: z.worldRules(),
		Off: z.offList(), Errors: z.modErrors, Places: z.Mods.Places, Atlas: z.Mods.Atlas, Quests: z.Mods.Quests,
		Castles: z.Castles, Temporary: z.Mods.Temporary, NPCLabels: z.npcLabel, NPCEvents: z.npcEventListener}
	// The job tables are read at each call: a headless zone gets them
	// after its Lua state is made.
	opts.Jobs = zoneJobs{z}
	scripts := z.modScripts()
	r, err := modscript.New(opts, scripts)
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

// runScripts runs the queued events' handlers. Handlers may queue more
// events, which run in the same call. Then the NPC messages run, those
// that this zone's handlers sent too, as rAthena's donpcevent runs its
// label after the current one (review decision 58). A zone runs at most
// npcRounds batches of them here, and the rest at its next pass.
func (z *Zone) runScripts() {
	z.flushDeferred()
	z.castleNotices()
	for round := 0; ; round++ {
		for len(z.scriptEvents) > 0 {
			ev := z.scriptEvents[0]
			z.scriptEvents = z.scriptEvents[1:]
			switch {
			case z.scripts == nil:
			case ev.monster != nil:
				z.runMonsterScript(ev.monster)
			case ev.death != nil:
				z.scripts.FireDeath(*ev.death)
			case ev.status != nil:
				z.scripts.FireStatus(ev.name, *ev.status)
			case ev.chat != nil:
				z.scripts.FireChat(*ev.chat)
			case ev.trade != nil:
				z.scripts.FireTrade(*ev.trade)
			case ev.sale != nil:
				z.scripts.FireSale(ev.name, *ev.sale)
			case ev.party != nil:
				z.scripts.FireParty(*ev.party)
			case ev.guild != nil:
				z.scripts.FireGuild(*ev.guild)
			case ev.quest != nil:
				z.scripts.FireQuest(*ev.quest)
			case ev.touch != nil:
				z.runTouch(ev.touch)
			case ev.monsterTouch != nil:
				z.runMonsterTouch(ev.monsterTouch)
			default:
				z.scripts.Fire(ev.name, ev.args...)
			}
			for _, h := range ev.release {
				if e := z.monsterHandles[h]; e != nil && e.monster != nil {
					e.monster.handle = 0
				}
				delete(z.monsterHandles, h)
			}
		}
		if round == npcRounds || z.npcMessages() == 0 {
			break
		}
		z.flushDeferred()
	}
	z.scriptEvents = z.scriptEvents[:0]
	if z.npcIn.pending() {
		z.npcIn.signal()
	}
}

// npcRounds is the most batches of NPC messages one runScripts runs.
const npcRounds = 4

// ReloadMod loads a mod's scripts again in the zone, from a new
// snapshot. The mod's old context ends first, which removes its handlers
// and timers. It waits until the zone has done it.
func (z *Zone) ReloadMod(ctx context.Context, sc *mod.Scripts) error {
	return z.do(ctx, func() error { return z.reloadScripts(sc) })
}

// reloadScripts is ReloadMod on the zone's goroutine. A zone that has no
// Lua state yet makes it from the new files when it needs one.
func (z *Zone) reloadScripts(sc *mod.Scripts) error {
	if z.reloaded == nil {
		z.reloaded = map[string]*mod.Scripts{}
	}
	z.reloaded[sc.Mod] = sc
	if z.scripts == nil {
		return nil
	}
	return z.scripts.Reload(sc)
}

// scriptHost is the zone as mod scripts see it.
type scriptHost struct{ z *Zone }

// zoneJobs answers the job tests of scripts from the zone's job tables.
type zoneJobs struct{ z *Zone }

func (j zoneJobs) MapID(job int) (uint64, bool) {
	if j.z.Status == nil {
		return 0, false
	}
	return j.z.Status.MapID(job)
}

func (j zoneJobs) JobOf(mapID uint64, male bool) (int, bool) {
	if j.z.Status == nil {
		return 0, false
	}
	return j.z.Status.JobOf(mapID, male)
}

func (j zoneJobs) JobID(name string) (int, bool) {
	if j.z.Status == nil {
		return 0, false
	}
	return j.z.Status.JobID(name)
}

func (j zoneJobs) BaseJobOf(job int) (int, bool) {
	if j.z.Status == nil {
		return 0, false
	}
	return j.z.Status.BaseJobOf(job)
}

func (h scriptHost) Map() string { return strings.TrimPrefix(h.z.Map, "map/") }

func (h scriptHost) player(id int64) (*Player, error) {
	if id > 0 && id <= int64(^uint32(0)) {
		// A player whose logout waits for other zones still takes their
		// calls (playerworld.go).
		if p := h.z.players[uint32(id)]; p != nil && (!p.gone() || p.leaving != nil) {
			return p, nil
		}
	}
	if IsRemotePlayer(id) {
		// A handle of a player of another zone acts here once the player
		// came to this zone.
		_, p, err := h.remotePlayer(id)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, errors.New("the player is on another map")
		}
		return p, nil
	}
	return nil, errors.New("the player has left this map")
}

// PlayerField reads a field of a player handle (proposal 0002, "Players
// and other entities").
func (h scriptHost) PlayerField(id int64, field string) (script.Value, error) {
	if IsRemotePlayer(id) {
		en, p, err := h.remotePlayer(id)
		if err != nil {
			return script.Nil, err
		}
		if p == nil {
			return h.remotePlayerField(en, field)
		}
	}
	p, err := h.player(id)
	if err != nil {
		return script.Nil, err
	}
	read, ok := playerFields[field]
	if !ok {
		return script.Nil, fmt.Errorf("player has no field %q", field)
	}
	return read(h, p), nil
}

func intValue[T int | int64 | uint32](v T) script.Value { return script.Int(int64(v)) }

// playerFields are the fields of player handles. server/modscript/luaapi
// defines the same names, and a test checks both.
var playerFields = map[string]func(h scriptHost, p *Player) script.Value{
	"kind":         func(scriptHost, *Player) script.Value { return script.String("player") },
	"name":         func(_ scriptHost, p *Player) script.Value { return script.String(p.Name) },
	"base_level":   func(_ scriptHost, p *Player) script.Value { return intValue(p.progress.BaseLevel) },
	"job_level":    func(_ scriptHost, p *Player) script.Value { return intValue(p.progress.JobLevel) },
	"job":          func(_ scriptHost, p *Player) script.Value { return intValue(p.char.Job) },
	"zeny":         func(_ scriptHost, p *Player) script.Value { return script.Int(p.record.Zeny) },
	"hp":           func(_ scriptHost, p *Player) script.Value { return script.Int(p.entity.hp) },
	"max_hp":       func(_ scriptHost, p *Player) script.Value { return maxOf(p.entity, true) },
	"sp":           func(_ scriptHost, p *Player) script.Value { return script.Int(p.sp) },
	"max_sp":       func(_ scriptHost, p *Player) script.Value { return maxOf(p.entity, false) },
	"map":          func(h scriptHost, _ *Player) script.Value { return script.String(h.Map()) },
	"x":            func(_ scriptHost, p *Player) script.Value { return intValue(p.entity.x) },
	"y":            func(_ scriptHost, p *Player) script.Value { return intValue(p.entity.y) },
	"character_id": func(_ scriptHost, p *Player) script.Value { return script.Int(p.record.ID) },
	"account_id":   func(_ scriptHost, p *Player) script.Value { return script.Int(p.record.AccountID) },
	"party": func(h scriptHost, p *Player) script.Value {
		if p.soc == nil {
			return script.Nil
		}
		if id := h.z.socialHub().PartyOf(p.soc); id != 0 {
			return script.Int(id)
		}
		return script.Nil
	},
	"guild": func(h scriptHost, p *Player) script.Value {
		if p.soc == nil {
			return script.Nil
		}
		if id := h.z.socialHub().GuildIDOf(p.soc); id != 0 {
			return script.Int(id)
		}
		return script.Nil
	},
	"sex": func(_ scriptHost, p *Player) script.Value {
		if p.char.Male {
			return script.String("male")
		}
		return script.String("female")
	},
	"base_str": func(_ scriptHost, p *Player) script.Value { return intValue(p.char.Stats.Str) },
	"base_agi": func(_ scriptHost, p *Player) script.Value { return intValue(p.char.Stats.Agi) },
	"base_vit": func(_ scriptHost, p *Player) script.Value { return intValue(p.char.Stats.Vit) },
	"base_int": func(_ scriptHost, p *Player) script.Value { return intValue(p.char.Stats.Int) },
	"base_dex": func(_ scriptHost, p *Player) script.Value { return intValue(p.char.Stats.Dex) },
	"base_luk": func(_ scriptHost, p *Player) script.Value { return intValue(p.char.Stats.Luk) },
	// Trait stats cannot be raised yet.
	"base_pow": zeroField, "base_sta": zeroField, "base_wis": zeroField,
	"base_spl": zeroField, "base_con": zeroField, "base_crt": zeroField,
	"companion": func(_ scriptHost, p *Player) script.Value { return companionField(p.companion) },
	"mount":     func(_ scriptHost, p *Player) script.Value { return companionField(p.mount) },
	"cart":      func(_ scriptHost, p *Player) script.Value { return cartField(p) },
	// Area B7 of review decision 58: Weight, MaxWeight and SkillPoint,
	// getlook and getgmlevel.
	"weight":       func(_ scriptHost, p *Player) script.Value { return intValue(p.inventory.Weight) },
	"max_weight":   func(_ scriptHost, p *Player) script.Value { return intValue(p.maxWeight) },
	"skill_points": func(_ scriptHost, p *Player) script.Value { return intValue(p.progress.SkillPoints) },
	"staff_level":  func(h scriptHost, p *Player) script.Value { return intValue(h.z.staffLevel(p)) },
	"look": func(_ scriptHost, p *Player) script.Value {
		l := p.Look
		return script.NewTable(
			script.KV("hair_style", intValue(l.Hair)), script.KV("hair_color", intValue(l.HairColor)),
			script.KV("cloth_color", intValue(l.ClothColor)), script.KV("head_top", intValue(l.HeadTop)),
			script.KV("head_middle", intValue(l.HeadMid)), script.KV("head_bottom", intValue(l.HeadBottom)),
			script.KV("weapon", intValue(l.Weapon)), script.KV("shield", intValue(l.Shield)))
	},
	"job_class": func(h scriptHost, p *Player) script.Value {
		if h.z.Status == nil {
			return script.Int(0)
		}
		m, _ := h.z.Status.MapID(p.char.Job)
		return script.Int(int64(m)) // #nosec G115 -- map job IDs fit in 32 bits.
	},
}

func zeroField(scriptHost, *Player) script.Value { return script.Int(0) }

// maxOf is an entity's maximum HP or SP, or 0 without a status.
func maxOf(e *entity, hp bool) script.Value {
	if e.status == nil {
		return script.Int(0)
	}
	if hp {
		return intValue(e.status.MaxHP)
	}
	return intValue(e.status.MaxSP)
}

// MonsterField reads a field of a monster handle. A killed monster is
// readable until its death event has run, with is_alive false.
func (h scriptHost) MonsterField(id int64, field string) (script.Value, error) {
	if isRemoteMonster(id) {
		return h.remoteMonsterField(id, field)
	}
	e := h.z.monsterHandles[id]
	if e == nil {
		return script.Nil, errors.New("the monster is gone from this map")
	}
	read, ok := monsterFields[field]
	if !ok {
		return script.Nil, fmt.Errorf("monster has no field %q", field)
	}
	return read(h, e), nil
}

// monsterFields are the fields of monster handles. server/modscript/luaapi
// defines the same names, and a test checks both.
var monsterFields = map[string]func(h scriptHost, e *entity) script.Value{
	"kind": func(scriptHost, *entity) script.Value { return script.String("monster") },
	"id":   func(_ scriptHost, e *entity) script.Value { return intValue(e.monster.spawn.Mob) },
	"name": func(_ scriptHost, e *entity) script.Value { return script.String(e.monster.spawn.Name) },
	"level": func(_ scriptHost, e *entity) script.Value {
		if e.monster.spawn.Status == nil {
			return script.Int(0)
		}
		return intValue(e.monster.spawn.Status.Level)
	},
	"hp":       func(_ scriptHost, e *entity) script.Value { return script.Int(max(e.hp, 0)) },
	"max_hp":   func(_ scriptHost, e *entity) script.Value { return maxOf(e, true) },
	"map":      func(h scriptHost, _ *entity) script.Value { return script.String(h.Map()) },
	"x":        func(_ scriptHost, e *entity) script.Value { return intValue(e.x) },
	"y":        func(_ scriptHost, e *entity) script.Value { return intValue(e.y) },
	"is_alive": func(h scriptHost, e *entity) script.Value { return script.Bool(!e.dead && h.z.entities.has(e)) },
	// game_id is rAthena's GID of the monster (review decision 59).
	"game_id": func(h scriptHost, e *entity) script.Value { return script.Int(h.z.gameID(e)) },
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
// fit, as getitem does, and the player sees why. The runtime gives
// rentals through GiveItemSpec.
func (h scriptHost) GiveItem(id int64, itemID uint32, amount int, rentSeconds int64) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	return h.z.giveItem(p, itemID, amount, h.z.Items.Get(itemID).Stackable), nil
}

// CountItem is countitem: rentals do not count, as script_countitem_sub
// counts copies whose expire_time is 0 (script.cpp 7152).
func (h scriptHost) CountItem(id int64, itemID uint32) (int, error) {
	p, err := h.player(id)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range p.inventory.Slots {
		if s := &p.inventory.Slots[i]; s.ID == itemID && s.Amount > 0 && s.Expires == 0 {
			n += s.Amount
		}
	}
	return n, nil
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
