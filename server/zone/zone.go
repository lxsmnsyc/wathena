// Package zone runs one map: the players on it, their walks, and what each
// player sees. One goroutine owns all of a zone's state and advances it in
// 50 ms ticks (strategy section 4.1), so game logic needs no locks.
package zone

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/currency"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/market"
	"github.com/lxsmnsyc/wathena/server/metrics"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/shop"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/transfer"
	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

const (
	// Tick is the time between zone updates.
	Tick = 50 * time.Millisecond
	// ViewRange is how far a player sees, in cells, as rAthena's
	// area_size.
	ViewRange = 14
	// WalkSpeed is RO's default walk speed, in milliseconds per cell.
	WalkSpeed = 150
	// outBuffer is how many messages may wait for a slow connection
	// before the zone drops it.
	outBuffer = 64
)

// Look is a player's appearance, as IDs from the client data tables.
type Look struct {
	Job, Sex, Hair, HairColor uint32
	// Weapon is the weapon type, Shield the shield's view, and the rest
	// the views of headgear and garment (rAthena's view_data).
	Weapon, Shield, HeadTop, HeadMid, HeadBottom, Robe uint32
	// WeaponView is the view of the right hand's weapon when its record
	// names a view_sprite, or 0 (decision 0036).
	WeaponView uint32
	// Companion and Mount are the record IDs of the player's companion
	// and mount, or 0 (decision 0036).
	Companion, Mount uint32
	// ClothColor is the body's palette, as rAthena's LOOK_CLOTHES_COLOR.
	ClothColor uint32
	// Transform is the monster a transformed player looks like, or 0
	// (player:transform).
	Transform uint32
}

// message is the protocol's Look.
func (l Look) message() *wathenav1.Look {
	b := wathenav1.Look_builder{
		Body: l.Job, Sex: l.Sex, Hair: l.Hair, HairColor: l.HairColor, ClothColor: l.ClothColor,
		Weapon: l.Weapon, Shield: l.Shield, HeadTop: l.HeadTop, HeadMid: l.HeadMid, HeadBottom: l.HeadBottom, Robe: l.Robe,
		WeaponView: l.WeaponView, Companion: l.Companion, Mount: l.Mount,
	}
	if l.Transform != 0 {
		// A transformed player shows as the monster, as
		// SC_MONSTER_TRANSFORM's view (status.cpp 12494).
		b.Body, b.NpcView = l.Transform, wathenav1.NpcView_NPC_VIEW_MONSTER
	}
	return b.Build()
}

// Player is one connected player in one zone. Its Out channel is its
// session's, which carries messages to the connection.
type Player struct {
	Name string
	Look Look
	Out  <-chan *wathenav1.ServerMessage

	out    chan *wathenav1.ServerMessage
	entity *entity
	// queued counts the player's commands in the zone's queue, which
	// Session.send keeps below playerQueue.
	queued atomic.Int32
	// tainted is set when a panic may have left the player's state
	// half changed (recover.go). Its saves then write its last saved
	// copy, once.
	tainted, taintSaved bool
	// netIDs are this connection's IDs for the entities it sees.
	netIDs  netIDs
	nextNet uint32
	ackSeq  uint32
	// fresh is set until the player's first frame, which shows the player
	// to itself.
	fresh bool
	// dropped is set once the zone closed the player's messages, or saw
	// that the sender closed them. Read it through gone.
	dropped bool
	// ownClosed is the closing state of out for a player without a
	// session, as in tests.
	ownClosed outClosed

	// charID stands for rAthena's char_id in damage logs and pick-up
	// rights, until characters are stored. It is unique within the zone.
	charID uint32
	// progress is the player's levels and exp. It is only used when the
	// zone has progression tables.
	progress progression.Character
	// char is what the player's stats come from. Its levels are unused:
	// they come from progress.
	char status.Character
	// sp is the player's SP. HP is on its entity.
	sp int64
	// maxWeight and window are from the last stat calculation.
	maxWeight int
	window    status.Window
	// sent holds the stat values last sent to the player.
	sent map[wathenav1.StatId]int64
	// inventory is the player's items and their weight.
	inventory *item.Inventory
	// overweight is the level pc_updateweightstatus last set, from
	// item.WeightNormal to item.Weight90.
	overweight int
	// canUseItem is canuseitem_tick: when the player may use an item
	// again, in zone milliseconds.
	canUseItem int64
	// itemDelays and statusDelays are when item reuse delays end, by
	// item ID and by delay status, as item_delay and the Reuse_Limit
	// statuses.
	itemDelays   map[uint32]int64
	statusDelays map[string]int64
	// regen is what natural regen heals, from the last stat calculation.
	regen      status.Regen
	regenTicks regenState
	// pending are events for the player alone, sent with the next frame.
	pending []*wathenav1.Event
	// pages are events that each go in a frame of their own after the
	// next frame, such as the later pages of a long list (pages.go).
	pages []*wathenav1.Event
	// storage is the personal storage the player has open, or nil, and
	// ownStorages the storages of a player a test added (storage.go).
	storage     *openStorage
	ownStorages []*storage.Storage
	// trade is the player's trade, or nil, and tradeAsk the player who
	// asked it to trade and waits for an answer (trade.go).
	trade    *trade
	tradeAsk *Player
	// onGround counts the player's dropped items on the ground, and
	// dropAt is its drop rate's next free time, in zone milliseconds
	// (dropAllowed).
	onGround int
	dropAt   int64
	// nextTradeAsk is when the player may next ask someone to trade, in
	// zone milliseconds (tradeAskEvery).
	nextTradeAsk int64
	// currencies are the character's own currencies of
	// data/currencies.yml, by ID (currency.go).
	currencies map[uint32]int64
	// visited are the generated maps the character entered, and memos
	// its Warp Portal memo points (worldmap.go).
	visited []string
	memos   []character.Memo
	// quests are the character's quest log, and questIcons the icons
	// scripts showed it over NPCs of this map (quests.go). questInfo is
	// what npc:add_quest_icon showed it, as rAthena's qi_display, and
	// questInfoDirty asks for its check at the end of the step
	// (npcpresent.go).
	quests         []character.Quest
	questIcons     map[*entity]questIconShown
	questInfo      map[*entity]questIconShown
	questInfoDirty bool
	// questInfoSize is how many NPCs with quest icons the map had when
	// the player's icons were set up, as the size of rAthena's
	// qi_display, and questInfoReinit asks to set them up at the next
	// check, as pc_show_questinfo_reinit at map entry (npcpresent.go).
	questInfoSize   int
	questInfoReinit bool
	// prevend is the most lines of the vending setup the Vending skill
	// opened, or 0; vend is the player's open shop. buySetup is the
	// slots of the buying store setup, or 0; buying is the open store.
	// search is the store search window (vending.go).
	prevend  int
	vend     *vendShop
	buySetup int
	buying   *buyStore
	search   *storeSearch
	// remote is the store a store search opened from afar, which the
	// player may buy from or sell to without range checks.
	remote market.Key
	// autotrade is set while the player's store stays open without its
	// connection, and detached once the connection closed: its messages
	// go nowhere, and it stays in the zone.
	autotrade, detached bool
	// mail is the player's mail state (mail.go).
	mail mailState

	// load is the character the player enters as. When it is nil, the
	// player enters as a new character.
	load *character.Character
	// record holds the saved fields the zone does not change, such as
	// the database IDs and zeny.
	record character.Info
	// sess is the player's session, which holds its saver. It is nil for
	// players that tests add directly.
	sess *Session
	// saved is the last character handed to the saver. nextSave is when
	// the next autosave runs, in zone milliseconds.
	saved    *character.Character
	nextSave int64
	// transferred is set while the player is in the zone's txPlayers.
	transferred bool

	// companion and mount are the player's records of data/companions.yml
	// and data/mounts.yml, or nil (companions.go).
	companion, mount *companion.Record

	// spheres are the player's spirit spheres.
	spheres

	// arrival is set while a player that warped from another zone joins.
	arrival *handoff
	// mapChange is set until the first frame after a warp from another
	// zone, which starts with MapChange.
	mapChange bool
	// touchOnEnter is set until the zone checks the cell the player
	// entered on for warps and touch areas, as clif_parse_LoadEndAck
	// does.
	touchOnEnter bool
	// touching are the NPCs whose touch area holds the player, in the
	// order it came into them (touch.go).
	touching []*entity
	// touchQueue are touches that came while the player talked, and
	// touchAt is when the next may run (touch.go).
	touchQueue []*touchEvent
	touchAt    int64
	// rewarp is rAthena's count_rewarp: warps in a row onto warp areas.
	rewarp int
	// listed is the player's entry in the world's player directory, and
	// leaving is set while its logout waits for the NPC handlers of other
	// zones (playerworld.go).
	listed  *playerEntry
	leaving *logoutHold
	// warpTo is a warp a script asked for. It happens once the script's
	// event has run.
	warpTo *scriptWarp
	// talk is the player's conversation with an NPC, or nil, and
	// dialogSeq the last dialog step ID it was sent.
	talk      *talk
	dialogSeq uint32
	// shop is the shop window the player has open, or nil.
	shop *openShop
	// endTalk is set when the player's conversation must close at the
	// next point where scripts may run, such as after its death.
	endTalk bool
	// vars are the player's saved variables of mods, as JSON: its
	// character's and its account's. temp are the temporary ones, which
	// last until logout.
	vars map[character.Variable]string
	temp map[character.Variable]string
	// worn are the player's equipped slots, by inventory slot, with their
	// handles and equip contexts.
	worn map[int]*wornSlot
	// statsDirty is set when the player's bonuses changed since its last
	// stat calculation. The player is then in the zone's dirtyStats.
	statsDirty bool
	// base and bonuses are base_status and the bonuses outside the
	// battle status, from the last stat calculation.
	base    battle.Status
	bonuses status.Bonuses
	// timed are the player's timed bonuses, in the order they started.
	timed []*timedBonus
	// cooldowns are when each skill may be used again, by skill ID, as
	// rAthena's scd.
	cooldowns map[int]int64
	// passives are the contexts of the Lua scripts of learned passive
	// skills, by skill ID.
	passives map[int]*passiveSkill
	// autocast is sd->state.autocast: an autospell casts, so its skill
	// costs no SP. autoSpellLock holds the bAutoSpellOnSkill entries that
	// are casting, by index, so an entry does not trigger itself.
	autocast      bool
	autoSpellLock map[int]bool
	// keepAspd is set while the stats wait for a status's own
	// status_calc_bl, which does not compute ASPD.
	keepAspd bool
	// tempSkills are the levels player:give_skill gave for a time, by
	// skill ID, as pc_skill's ADDSKILL_TEMP and ADDSKILL_TEMP_ADDLEVEL.
	// They go at the next recalc, as pc_calc_skilltree clears them in
	// status_calc_pc, unless keepTempSkills is set: a status's own
	// status_calc_bl does not recalculate skills.
	tempSkills     map[int]int
	keepTempSkills bool
	// rogue is the state of the Rogue's skills: the skill Plagiarism
	// copied, and Gangster's Paradise.
	rogue rogueState
	// castOld is the cast that Cast Cancel stops, as sd->skill_id_old and
	// skill_lv_old.
	castOld *casting
	// perform is the player's last song or dance and Encore's mark
	// (skills_bard.go).
	perform performState

	// member is the player in the world's chat hub, and blocks its block
	// list (social.go). sanctions are its mutes and jails. seen are the
	// player lines it saw in the last 5 minutes, for reports.
	// jailPending is set while a jail waits to move the player.
	member      *chat.Member
	blocks      []string
	sanctions   []character.Sanction
	seen        []chat.Line
	jailPending bool
	// granted are the skills player:give_skill gave with
	// permanent_grant, in ID order, as rAthena's SKILL_FLAG_PERM_GRANTED
	// (playeractions.go).
	granted []int
	// blocked is the player's PCBLOCK_ flags of player:set_blocked
	// (playeractions.go).
	blocked uint32
	// transform is the monster the player looks like while
	// player:transform lasts, or 0 (playeractions.go).
	transform uint32
	// setLook are the headgear views setlook gave, top, middle and
	// bottom, as rAthena's setlook_head_*, or 0.
	setLook [3]uint32
	// gmHidden is set while the player is hidden with @hide: other
	// players do not see it and monsters ignore it, as rAthena's
	// OPTION_INVISIBLE. It lasts across map changes, and across logins
	// while the login may still use @hide (decision 0036).
	gmHidden bool

	// soc is the player in the world's social registry, and partyState
	// its party as this zone knows it (party.go).
	soc        *social.Player
	partyState partyState
	// guild is the player's guild as this zone knows it (guild.go).
	guild guildState
}

type entity struct {
	// idx and slot are the entity's place in the zone's entitySet.
	idx, slot int32
	// uid is the entity's ID: the order the zone added it in, which
	// entity queries sort ties by.
	uid int64
	// player is set for players. Monsters have monster instead, items
	// on the ground have item, warp portals have warp, and NPCs that
	// mods place have npc.
	player  *Player
	monster *monster
	item    *groundItem
	warp    *Warp
	npc     *npc
	x, y    int
	// dir is the facing, 0 south, clockwise.
	dir   int
	speed int
	// path holds the steps left. stepStart is when path[0] began, in zone
	// milliseconds.
	path      []path.Dir
	stepStart int64
	// walkChanged is set when a walk starts or changes, so viewers get a
	// move event. stopped is set when a walk ends early. lookChanged is
	// set when a player's look changed, so viewers get LookChange.
	walkChanged bool
	stopped     bool
	lookChanged bool
	// shopChanged is set when a player's store opened or closed, so
	// viewers get EntityShop.
	shopChanged bool
	// guildChanged is set when a player's guild badge changed, so viewers
	// get EntityGuild.
	guildChanged bool
	// optionChanged is set when the entity's status options changed, so
	// viewers get OptionChange.
	optionChanged bool
	// sc is the entity's status_change, or nil before its first status.
	sc *statuschange.List
	// packed is path in the protocol's steps, made for tick packedTick.
	// Every viewer's move event shares it.
	packed     []byte
	packedTick uint32
	fight
	pickUp
	unitSkill
	bladeStopState
	// unit is set for a cell of a ground skill.
	unit *skillUnit
}

type command struct {
	player *Player
	// queued is set for a command that Session.send counted in the
	// player's share of the queue (playerQueue).
	queued bool
	seq    uint32
	walkTo *[2]int
	attack *attackCommand
	pickUp *uint32
	raise  *raiseCommand
	chat   *chatCommand
	// channel, block and report are chat commands (social.go).
	channel *channelCommand
	block   *blockCommand
	report  *reportCommand
	// party, friend and booking are social commands (party.go), and
	// guild and storage guild commands (guild.go).
	guild    *guildCommand
	storage  *storageCommand
	trade    *TradeAction
	party    *partyCommand
	friend   *friendCommand
	booking  *bookingCommand
	use      *int
	drop     *dropCommand
	npcClick *uint32
	npcReply *wathenav1.NpcReply
	shopBuy  *shopBuyCommand
	shopSell *shopSellCommand
	// shopClose names the NPC of the shop window the player closed.
	shopClose *uint32
	// equip, unequip and insertCard are equipment commands, by slot.
	equip      *equipCommand
	unequip    *int
	insertCard *insertCardCommand
	// useSkill and learnSkill are skill commands.
	useSkill   *useSkillCommand
	learnSkill *int
	// staff runs work of a GM command on the zone the player is in now,
	// such as the second half of @jumpto (gmcommands.go).
	staff func(z *Zone, p *Player)
	// mail, vending, buying and search are the commands of review
	// decision 30 (mail.go, vending.go).
	mail    *wathenav1.MailCommand
	vending *wathenav1.VendingCommand
	buying  *wathenav1.BuyingStoreCommand
	search  *wathenav1.StoreSearchCommand
	// memo is the /memo command (worldmap.go).
	memo bool
	// questActive is the quest window's active toggle (quests.go).
	questActive *questActiveCommand
}

type useSkillCommand struct {
	skill, level int
	target       SkillTarget
}

type shopBuyCommand struct {
	npc   uint32
	lines []buyLine
}

type shopSellCommand struct {
	npc   uint32
	lines []sellLine
}

type raiseCommand struct {
	stat   status.Stat
	amount int
}

type attackCommand struct {
	target uint32
	repeat bool
}

// Zone is one running map.
type Zone struct {
	Map  string
	grid *path.Grid
	// ownGrid is set once the zone copied its grid to change cells, as
	// Ice Wall does. The map's grid may be shared.
	ownGrid bool
	// shared is what other zones read of the map, and cellChanges the
	// cells that scripts changed, by index, for players who enter
	// (maps.go).
	shared      mapShared
	cellChanges map[int]uint8
	// aiMonsters is set once a script spawned a monster with an AI and
	// no master, which monsters without an AI look for (mobai.go).
	aiMonsters bool

	start    time.Time
	now      int64
	tick     uint32
	entities entitySet
	// blocks finds the entities near a player during updates.
	blocks blockIndex
	// viewsForTest, when set, replaces viewEvents. A test checks
	// viewEvents against a scan of every entity with it.
	viewsForTest func(p *Player, events []*wathenav1.Event) []*wathenav1.Event
	rand         *rng.Rand
	combat

	// Status holds the job tables players' stats come from. Set it before
	// Run. Without it, every player has PlayerStatus.
	Status *status.Tables
	// Start is the character every new character starts as: its job,
	// raised stats and skills. Its levels and sex are unused, since new
	// characters start at level 1 with the sex of their look. Set it
	// before Run.
	Start status.Character
	// StartBaseLevel and StartJobLevel, when above 1, are the levels new
	// characters start at, for development. They start with the status
	// points of their base level and the skill points of their job level,
	// and a job other than Novice starts with Basic Skill learned. Set
	// them before Run.
	StartBaseLevel, StartJobLevel int
	// PlayerStatus is every player's battle status when Status is nil.
	PlayerStatus battle.Status
	// Progress holds the exp tables and drop settings. Without it, kills
	// give no exp and no items. Set it before Run.
	Progress *progression.Tables
	// Items is the item database. Without it, every item is an Etc item
	// with no weight. Set it before Run.
	Items *item.Table
	// StartItems are items every new character starts with, for
	// development. Set it before Run.
	StartItems []StartItem
	// SaveInterval is how often a player's changed character is saved.
	// It is DefaultSaveInterval when 0. Set it before Run.
	SaveInterval time.Duration
	// World finds the zones that warps lead to. Without it, only warps
	// within the map work. Set it before Run.
	World Directory
	// SaveMap is the asset name of the map where players' save point is,
	// such as map/prontera: characters have no save point of their own
	// yet, so it is the start map. Butterfly Wings go to its start cell.
	// Set it before Run.
	SaveMap string
	// Spawn is the cell where new characters start and where a save
	// point without a cell of its own puts players, when HasSpawn is set,
	// such as a home map's start place. Otherwise the start cell is the
	// walkable cell nearest the map's middle. Set it before Run.
	Spawn    [2]int
	HasSpawn bool
	// Moved are where the points of removed maps go, which every zone of
	// the world shares (decision 0045). Set it before Run.
	Moved *Relocations
	// Metrics measures the zone's ticks. New makes it.
	Metrics *metrics.Zone
	// Mods are the mods' scripts and item names. Without them, no
	// scripts run. Set it before Run.
	Mods *Mods
	// Clock is the server clock, which schedules and world events
	// follow. It is the system clock when nil. Set it before Run.
	Clock func() time.Time
	// WorldEvents is the world's event manager, which runs typed world
	// events (autoevents.go). Without it, hint NPCs say nothing and
	// world_events calls raise an error. Set it before Run.
	WorldEvents worldevent.Sink
	// MaxCardSlots is MAX_SLOTS, the most cards an item holds. It is
	// DefaultMaxCardSlots when 0. Set it before Run.
	MaxCardSlots int
	// GvG marks a GvG map, as MF_GVG. No map is one until sieges come;
	// tests set it.
	GvG bool
	// Committer saves the characters that transfers changed, with the
	// transfers' log lines (transfers.go). Without it, the lines go to
	// the log. Set it before Run.
	Committer Committer
	// Chat is the world's chat hub, which every zone of the world shares
	// (social.go). Without it, the zone makes its own on first use. Set
	// it before Run.
	Chat *chat.Hub
	// Jail is where jailed players go. DefaultJail when its map is empty.
	// Set it before Run.
	Jail Jail
	// chatIn gets the hub's deliveries for this zone's players.
	chatIn chan chatIn
	// Staff is the world's staff registry, which player:get_staff_role
	// reads (decision 0028). Without it, no player has a role. Set it
	// before Run.
	Staff *staff.Registry
	// Moderator gives the sanctions of @mute, @jail and @kick, with
	// their rows and audit rows (gmcommands.go). Without it, those
	// commands answer that moderation is not available.
	Moderator Moderator
	// Social is the world's social registry: parties, friends and party
	// booking (party.go). Without it, the zone makes its own on first
	// use. Set it before Run.
	Social *social.Registry
	// txStorageRefs are the guild storages the transfers since the last
	// commit changed, and txStorages the storages a guild that broke up
	// left (guild.go).
	txStorageRefs []*social.GuildStorage
	txStorages    []StorageCommit
	// Mail is the world's mail registry (mail.go), and Market its list
	// of vending shops and buying stores (market.go). Without them, the
	// zone makes its own on first use. Set them before Run.
	Mail   *mail.Registry
	Market *market.Registry
	// Storages is the world's storage registry: accounts' storages,
	// extra storages and carts (storage.go). Without it, the zone makes
	// its own on first use. Set it before Run.
	Storages *storage.Registry
	// coreCurrencies are core's currencies, for a zone without mods.
	coreCurrencies *currency.Table
	// txPersonal are the storages and carts the transfers since the last
	// commit changed, and storageRelease the storages of players who
	// logged out, let go after the next commit (storage.go).
	txPersonal []*storage.Storage
	// txShops are the shops whose stock changed since the last commit
	// (shops.go), txMails the mails, and txStores the store rows
	// (market.go). mailDeliver are sent mails that reach their mailbox
	// once the commit is queued.
	txShops     []*shop.Shop
	txMails     []*mail.Mail
	txStores    []market.Write
	mailDeliver []*mail.Mail
	// mailRelease are the mailboxes of players who logged out, let go
	// after the next commit.
	mailRelease []*mail.Mailbox
	// Autotraders are the world's autotraders, which the gateway ends
	// at a new login of their character (vending.go). Set it before Run.
	Autotraders    *Autotraders
	storageRelease []*storage.Storage
	// storageLog are guild storage log lines that wait to go to the
	// registry (guild.go).
	storageLog []storageLogBatch
	// socialIn gets the registry's notices for this zone's players.
	socialIn chan socialIn
	// channelHandles are the chat channels script handles point to, by
	// handle ID, and channelOwners the channels each context made.
	channelHandles map[int64]*chat.Channel
	nextChannel    int64
	channelOwners  map[int64][]*chat.Channel
	// nextSanctionCheck is when checkSanctions next runs.
	nextSanctionCheck int64
	// txLogs are the log lines of the transfers since the last commit,
	// and txPlayers the players they changed.
	txLogs    transfer.Logs
	txPlayers []*Player
	// txOffline are the offline characters a restore changed, txMailRows
	// the rows of the mails it changed, read under their locks, and
	// txAudit its audit rows (stafftools.go).
	txOffline  []CommitSave
	txMailRows []mail.Write
	txAudit    []staff.AuditRow

	// Statuses is the status database. Without it, no status starts. Set
	// it before Run.
	Statuses *statuschange.DB
	// Skills and SkillTrees are the skill database and the skill trees.
	// Without them, players have no skills to learn or cast. Set them
	// before Run.
	Skills     *skill.DB
	SkillTrees *skill.Trees
	// SkillSettings are the skill tree options of rAthena's player.conf.
	SkillSettings skill.Settings

	// statusChanges runs the statuses of the zone's entities, made on
	// first use.
	statusChanges *statuschange.Engine
	// quietStatuses is set while statuses that never ended come back,
	// such as after a map change: no status event fires for them.
	quietStatuses bool
	// inScriptCall is set while a script starts or ends a status, so the
	// status hooks and the bonuses of mod statuses do not run Lua inside
	// the script's host call: the prelude ran them first. hookOverride
	// holds what the script's own run of the status hooks gave.
	inScriptCall bool
	hookOverride *hookOverride
	// dirtyStats are the players whose stats wait for flushStats.
	// holdStats is above 0 while equipment changes or a player joins:
	// stats wait for the end of that change, which computes them once.
	dirtyStats []*Player
	holdStats  int

	// equippedRefs are the equipped slots that script handles point to,
	// by handle ID.
	equippedRefs map[int64]equippedRef
	nextEquipped int64

	// scripts runs the mods' Lua in the zone's own state, made by Run.
	// scriptEvents wait for a point where scripts may run.
	scripts      *modscript.Runtime
	scriptEvents []scriptEvent
	// reloaded are the mods' scripts that a reload read, by mod ID. They
	// replace Mods.Scripts when the zone makes its Lua state.
	reloaded map[string]*mod.Scripts
	// scriptsFailed is set when the Lua state could not be made, so it
	// is not tried on every item use.
	scriptsFailed bool
	// pool is the shared pool state of item groups in this zone.
	pool *item.Pool
	// monsterHandles are the monsters that script handles point to, by
	// handle ID. A killed monster stays here until its event has run.
	monsterHandles    map[int64]*entity
	nextMonsterHandle int64
	// owned are the spawns of spawning.monsters, by owning context ID.
	owned map[int64][]*Spawn
	// eventRuns are the runs of world events in the zone, by run ID.
	eventRuns map[int64]*modscript.Context
	// typedRuns are the zone's parts of typed runs (autoevents.go), by
	// run ID, and nextHint counts their hint NPCs. txEventUnits are the
	// units of progress the next commit saves.
	typedRuns    map[int64]*zoneRun
	nextHint     int64
	txEventUnits []EventUnit
	// notices get the starts and ends of world events.
	notices chan worldevent.Notice
	// restarts get planned and cancelled restarts (worldtools.go).
	restarts chan modscript.RestartPlanned
	// changes get rotations and region changes, and evac is where the
	// players of a removed map go (worldchange.go).
	changes chan modscript.WorldChange
	evac    *evacuation
	// modsOff are the mods that are off in the zone, and modErrors
	// counts the mods' script errors (decision 0031).
	modsOff   map[string]bool
	modErrors *modscript.ErrorLog
	// control gets work that must run on the zone's goroutine, such as a
	// mod reload. stallNext is a stall, in nanoseconds, between taking
	// the next control call and running it, for the duplication tests
	// (StallNextCall).
	control   chan func()
	stallNext atomic.Int64
	// announces get announcements from other zones and the world.
	announces chan announcement
	// npcHandles are the NPCs by script handle. npcsStarted is set once
	// their scripts run.
	npcHandles  map[int64]*entity
	npcsStarted bool
	// npcIn gets the NPC messages of this zone and others, and
	// remoteNPCs are the handles of NPCs of other zones, by handle, with
	// remoteIDs the reverse (npcworld.go).
	npcIn      npcInbox
	remoteNPCs map[int64]*npcEntry
	remoteIDs  map[*npcEntry]int64
	// remotePlayers are the handles of players of other zones, by
	// handle, with the characters' names, and remotePlayerIDs the
	// reverse (playerworld.go).
	remotePlayers   map[int64]string
	remotePlayerIDs map[string]int64
	// serial is the zone's serial for game IDs, mobSerials counts the
	// serials of its monsters, presetSerial is the serial that a spawn of
	// another zone reserved for the next monster, and remoteMonsters are
	// the handles of monsters of other zones, by handle, with their game
	// IDs, and remoteMonsterIDs the reverse (remotemonsters.go).
	serial                           int64
	mobSerials                       atomic.Int64
	presetSerial                     int64
	remoteMonsters, remoteMonsterIDs map[int64]int64
	// countView is the monster counts that a monster label's event reads
	// while it runs (maps.go).
	countView *labelCounts
	// touchNPCs are the NPCs with a touch area and a script, in the
	// order SetNPCs placed them (touch.go).
	touchNPCs []*entity
	// emotions are this tick's emotions over NPCs and monsters.
	emotions []npcEmotion
	// questInfoNPCs are the NPCs that npc:add_quest_icon gave icons, in
	// the order they got them, as rAthena's map qi_npc (npcpresent.go).
	questInfoNPCs []*entity
	// prevThink is when the monster AI last ran.
	prevThink int64

	// skillEvents are this tick's casts, cancels and skill effects.
	skillEvents []skillEvent
	// area is rAthena's skill_area_temp, the scratch values of the
	// splash in progress.
	area areaTemp
	// bowled is rAthena's bowling_db: the units one Bowling Bash already
	// hit.
	bowled map[*entity]bool
	// potion is rAthena's potion_flag and the potion values it fills,
	// for the thrown potions of the Alchemist skills.
	potion potionRun
	// groups are the ground skill groups, by group ID. nextGroup counts
	// them, and nextUnitTimer is when skill_unit_timer next runs.
	groups        map[int64]*unitGroup
	nextGroup     int64
	nextUnitTimer int64
	// timerSkills are the skill effects waiting for their time, as
	// skill_addtimerskill's timers.
	timerSkills []*timerSkill
	// partners are the partners skill_check_pc_partner found last, which
	// it keeps between the cast start and end.
	partners []*entity
	// deferred is work that waits for a script's call to return, such
	// as the death of a unit a cast's script killed. nextCast numbers the
	// casts that scripts see.
	deferred []func()
	nextCast int64

	// warps are the map's warps, in load order.
	warps []Warp
	// asleep is set while the zone has no players and its ticker is off.
	asleep bool
	// nextRentalCheck is when the zone checks rentals next, in zone
	// milliseconds (rentals.go).
	nextRentalCheck int64

	// players maps charID to the players in the zone.
	players  map[uint32]*Player
	nextChar uint32
	loot
	// chats are this tick's local chat lines.
	chats []chatLine
	// used are this tick's item uses.
	used []used
	// prevHeal and nextHeal are when natural regen last ran and next
	// runs, in zone milliseconds.
	prevHeal, nextHeal int64

	joins    chan joinRequest
	leaves   chan *Player
	commands chan command
	// arrivals get players that warped from other zones. incoming counts
	// the sends to it that have not finished.
	arrivals chan *handoff
	incoming sync.WaitGroup
	// stale gets players whose saver lost the character to a newer
	// session.
	stale chan *Player
	// stopped closes when Run returns, after the last saves.
	stopped chan struct{}
	// failed is set after a panic the zone cannot contain, which stops
	// it (recover.go).
	failed bool

	// outbox holds messages for the sender, which runs while Run runs.
	// See outbox.go.
	outbox     []outMsg
	sends      chan []outMsg
	spare      chan []outMsg
	senderDone chan struct{}
}

type joinRequest struct {
	player *Player
	done   chan joinResult
}

type joinResult struct {
	netID uint32
	tick  uint32
}

// New makes a zone for a map, given its asset name and cells. rand is the
// zone's random stream, which must be secret, since it decides where
// monsters go. spawns are the map's monsters.
func New(mapName string, grid *path.Grid, rand *rng.Rand, spawns []Spawn) *Zone {
	z := &Zone{
		Map:       mapName,
		grid:      grid,
		rand:      rand,
		players:   map[uint32]*Player{},
		joins:     make(chan joinRequest),
		leaves:    make(chan *Player, 16),
		commands:  make(chan command, 1024),
		arrivals:  make(chan *handoff),
		stale:     make(chan *Player),
		groups:    map[int64]*unitGroup{},
		control:   make(chan func()),
		announces: make(chan announcement, 256),
		notices:   make(chan worldevent.Notice, 64),
		restarts:  make(chan modscript.RestartPlanned, 8),
		changes:   make(chan modscript.WorldChange, 8),
		modErrors: &modscript.ErrorLog{},
		chatIn:    make(chan chatIn, 1024),
		socialIn:  make(chan socialIn, 1024),
		stopped:   make(chan struct{}),
		Metrics:   &metrics.Zone{Name: mapName},
	}
	z.serial = zoneSerials.Add(1)
	z.npcIn.wake = make(chan struct{}, 1)
	z.entities.shared = &z.shared
	z.shared.grid.Store(grid)
	z.spawn(spawns)
	return z
}

// Run advances the zone until ctx ends. It then hands every player's
// character to its saver and drops the players.
//
// A zone without players sleeps: its ticker stops, so its monsters stand
// still and it costs no CPU. It wakes when a player joins or arrives.
// rAthena similarly runs full monster AI only near players, and lets
// monsters no player has seen stand still (mob_ai_sub_lazy).
func (z *Zone) Run(ctx context.Context) {
	defer close(z.stopped)
	z.start = time.Now()
	z.startSender()
	z.startScripts()
	defer z.stopScripts()
	var t *time.Ticker
	var ticks <-chan time.Time
	// sched wakes a sleeping zone for its scripts' schedules, which
	// follow the server clock.
	var sched *time.Timer
	defer func() {
		if t != nil {
			t.Stop()
		}
		if sched != nil {
			sched.Stop()
		}
	}()
	z.asleep = true
	for {
		if z.failed {
			z.failStop()
			return
		}
		switch {
		case len(z.players) > 0 && ticks == nil:
			t = time.NewTicker(Tick)
			ticks = t.C
		case len(z.players) == 0 && ticks != nil:
			t.Stop()
			t, ticks = nil, nil
			z.asleep = true
			z.Metrics.Asleep()
			z.blocks.clear()
		}
		// Handlers of what the last event did run now, and what they
		// queued goes to the sender in one batch.
		z.safely("scripts", z.runScripts)
		if z.failed {
			continue
		}
		z.safely("commit", z.commitTransfers)
		if z.failed {
			continue
		}
		z.flush()
		var schedC <-chan time.Time
		if ticks == nil {
			schedC = z.scheduleWait(&sched)
		}
		select {
		case <-schedC:
			z.safely("schedules", z.runSchedules)
		case <-ctx.Done():
			for _, p := range z.players {
				z.save(p, true)
				z.chatOffline(p)
				z.socialOffline(p)
				z.drop(p)
			}
			z.stopSender()
			return
		case j := <-z.joins:
			began := time.Now()
			z.safely("join", func() {
				z.wake()
				j.done <- z.join(j.player)
			})
			z.Metrics.Handled(time.Since(began))
		case h := <-z.arrivals:
			began := time.Now()
			z.safely("arrival", func() {
				z.wake()
				z.arrive(h)
			})
			z.Metrics.Handled(time.Since(began))
		case p := <-z.leaves:
			began := time.Now()
			z.safely("leave", func() { z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT) })
			if z.failed {
				// The leave may have stopped before the last save, which
				// the session's Leave waits for.
				p.tainted = true
				z.save(p, true)
			}
			z.Metrics.Handled(time.Since(began))
		case p := <-z.stale:
			z.safely("stale", func() { z.loggedInElsewhere(p) })
		case f := <-z.control:
			if d := z.stallNext.Swap(0); d > 0 {
				time.Sleep(time.Duration(d))
			}
			z.safely("control", f)
		case a := <-z.announces:
			z.safely("announce", func() { z.announce(a) })
		case n := <-z.notices:
			z.safely("notice", func() { z.worldNotice(n) })
		case ev := <-z.restarts:
			z.safely("restart", func() { z.restartPlanned(ev) })
		case ev := <-z.changes:
			z.safely("world change", func() { z.worldChanged(ev) })
		case in := <-z.chatIn:
			z.safely("chat", func() { z.receiveChat(in) })
		case in := <-z.socialIn:
			z.safely("social", func() { z.receiveSocial(in) })
		case <-z.npcIn.wake:
			// NPC messages run with the script events at the top of the
			// loop (runScripts).
		case c := <-z.commands:
			began := time.Now()
			z.handleSafely(c)
			z.Metrics.Handled(time.Since(began))
		case <-ticks:
			began := time.Now()
			z.safely("tick", z.step)
			z.Metrics.Tick(time.Since(began), len(z.players))
		}
	}
}

// wake brings a sleeping zone's clock to now before a player enters.
// Monsters pick new wander times, and natural regen starts again, so the
// time asleep does not count.
func (z *Zone) wake() {
	if !z.asleep {
		return
	}
	z.asleep = false
	now := time.Since(z.start).Milliseconds()
	if z.scripts != nil {
		// Mod timers follow the zone clock, which stood still.
		z.scripts.Shift(now - z.now)
	}
	if z.statusChanges != nil {
		// So do status timers.
		z.statusChanges.Shift(now - z.now)
	}
	z.now = now
	z.prevHeal, z.nextHeal = z.now, z.now+naturalHealInterval
	for e := range z.entities.all() {
		if m := e.monster; m != nil && len(e.path) == 0 {
			if e.status == nil {
				m.nextWalk = -1
				continue
			}
			m.nextWalk = z.now + int64(z.rand.IntN(1000)+minRandomWalk)
			m.nextThink = z.now
		}
	}
}

// WaitIncoming waits until no player is on its way to the zone. Call it
// after every zone of the world has stopped: players on their way to a
// stopped zone get their last save then.
func (z *Zone) WaitIncoming() { z.incoming.Wait() }

// Join adds a new character that only lives in memory, at the walkable
// cell nearest the map's center.
func (z *Zone) Join(ctx context.Context, name string, look Look) (*Session, error) {
	c := z.NewCharacter(name, look)
	return z.JoinCharacter(ctx, &c, nil)
}

// JoinCharacter adds a player as a loaded character, and returns its
// session, which holds the player's own entity ID and the current tick.
// saver, when not nil, saves the character while the player is in the
// world and when it leaves. Zones never wait on saver.
func (z *Zone) JoinCharacter(ctx context.Context, c *character.Character, saver Saver) (*Session, error) {
	s := newSession(saver)
	s.staff = c.Staff
	p := newPlayer(s, c)
	s.zone, s.player = z, p
	if err := z.acquireStorages(ctx, s); err != nil {
		return nil, err
	}
	release := func() {
		for _, st := range s.storages {
			z.Storages.Release(st)
		}
		s.storages = nil
		if s.mailbox != nil {
			z.mailRegistry().Release(s.mailbox)
			s.mailbox = nil
		}
	}
	if err := z.acquireMailbox(ctx, s); err != nil {
		release()
		return nil, err
	}
	if n, ok := saver.(StaleNotifier); ok {
		n.OnStale(s.stale)
	}
	done := make(chan joinResult, 1)
	select {
	case z.joins <- joinRequest{p, done}:
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	case <-z.stopped:
		release()
		return nil, ErrStopped
	}
	var r joinResult
	select {
	case r = <-done:
	case <-z.stopped:
		// The join panicked, and the zone stopped (recover.go).
		release()
		return nil, ErrStopped
	}
	s.Map, s.NetID, s.Tick = z.Map, r.netID, r.tick
	return s, nil
}

func (z *Zone) join(p *Player) joinResult {
	c := p.load
	if c == nil {
		n := z.NewCharacter(p.Name, p.Look)
		c = &n
	}
	p.load = nil
	z.checkJob(c)
	// The saved @hide bit stays out of the options that looks send.
	savedHidden := c.Options&companion.OptionInvisible != 0
	c.Options &^= companion.OptionInvisible
	p.record = c.Info
	p.blocks, p.sanctions = slices.Clone(c.Blocks), slices.Clone(c.Sanctions)
	p.currencies = maps.Clone(c.Currencies)
	p.visited, p.memos = slices.Clone(c.Visited), slices.Clone(c.Memos)
	// Points on a map that was removed move to its town (decision 0045).
	z.relocate(p, z.Moved)
	p.quests, p.questIcons = slices.Clone(c.Quests), nil
	// pc_show_questinfo_reinit and pc_show_questinfo at map entry
	// (clif.cpp 11187).
	p.questInfo, p.questInfoDirty = nil, true
	z.restoreCompanions(p, c)
	p.Look = Look{Job: uint32(viewJob(z.lookJob(c.Job), bodyOptions(p))), Sex: c.Sex(), Hair: uint32(c.Hair), HairColor: uint32(c.HairColor),
		ClothColor: uint32(max(c.ClothColor, 0))} // #nosec G115 -- IDs from the data tables, which the database keeps at 0 or more.
	x, y := c.X, c.Y
	h := p.arrival
	p.arrival = nil
	switch {
	case h != nil:
		// A warp places the player as pc_setpos does.
		x, y = z.arrivalCell(x, y)
		p.ackSeq, p.rewarp, p.mapChange = h.ackSeq, h.rewarp, true
		p.gmHidden = h.gmHidden
	case c.Map != z.Map || z.grid.NoPass(x, y):
		// A character from another map, or one on a cell that is no
		// longer walkable, starts at the start cell. A character that
		// logged out dead was saved at its save point.
		x, y = z.startCell()
	}
	if h == nil {
		// pc_authok: a character saved hidden stays hidden while its
		// login may still use @hide.
		p.gmHidden = savedHidden && z.mayHide(p)
	}
	e := &entity{player: p, x: x, y: y, speed: WalkSpeed}
	p.entity = e
	z.entities.add(e)
	z.nextChar++
	p.charID = z.nextChar
	z.players[p.charID] = p
	z.shared.players.Store(int64(len(z.players)))
	// map_iwall_get: the cells that scripts changed.
	p.pending = append(p.pending, z.changedCells()...)
	p.inventory = restoreInventory(z.Items, c, z.limits())
	z.fixRolls(p)
	// pc_authok starts the regen waits at login.
	p.regenTicks = regenState{hp: z.now, sp: z.now}
	p.itemDelays = map[uint32]int64{}
	p.statusDelays = map[string]int64{}
	if h != nil {
		// Item waits go on across the map change.
		p.canUseItem = z.now + h.canUseItem
		for id, left := range h.itemDelays {
			p.itemDelays[id] = z.now + left
		}
		for st, left := range h.statusDelays {
			p.statusDelays[st] = z.now + left
		}
		for id, left := range h.cooldowns {
			if p.cooldowns == nil {
				p.cooldowns = map[int]int64{}
			}
			p.cooldowns[id] = z.now + left
		}
		// Spirit spheres go on too.
		for _, left := range h.spiritBalls {
			p.spiritTimers = append(p.spiritTimers, z.now+left)
		}
	}
	p.sent = map[wathenav1.StatId]int64{}
	p.char = z.Start
	p.char.Job, p.char.Male, p.char.Stats = c.Job, c.Male, c.Stats
	// The character's own skills, or for a character without any the
	// start character's, in a map of its own.
	if c.Skills != nil {
		p.char.Skills = maps.Clone(c.Skills)
	} else {
		p.char.Skills = maps.Clone(z.Start.Skills)
	}
	p.granted = slices.Clone(c.Granted)
	// Equipped items start their scripts before the first stat
	// calculation, as status_calc_pc with SCO_FIRST. Changes to stats
	// wait until the saved statuses and timed bonuses are back too.
	z.holdStats++
	if z.Items != nil {
		z.wearAll(p)
	}
	p.Look = z.lookOf(p)
	if z.Progress != nil {
		p.progress = progression.Character{
			Job: c.Job, BaseLevel: c.BaseLevel, JobLevel: c.JobLevel,
			BaseExp: c.BaseExp, JobExp: c.JobExp,
			StatusPoints: c.StatusPoints, SkillPoints: c.SkillPoints,
		}
		// The first frame carries the levels and exp.
		p.pending = append(p.pending, z.progressStats(p, nil))
	}
	if z.Status != nil {
		p.pending = append(p.pending, z.jobNameEvent(p.char.Job))
	}
	z.recalc(p)
	// HP and SP are the saved ones, up to their maximum. A character that
	// logged out dead comes back with full HP, as a revived player does.
	// hpChanged sends the player its HP in the first frame.
	e.hp, p.sp = min(c.HP, int64(e.status.MaxHP)), min(c.SP, int64(e.status.MaxSP))
	if c.HP <= 0 {
		e.hp = int64(e.status.MaxHP)
	}
	if h != nil && h.dead {
		// A dead player that a script warped arrives dead, as pc_setpos
		// keeps pc_isdead, and returns to its save point after the rest
		// of its wait.
		e.hp, e.dead, e.diedAt = 0, true, z.now-h.deadFor
	}
	e.hpChanged = e.status.MaxHP > 0
	if e.status.Speed > 0 {
		e.speed = e.status.Speed
	}
	z.updateWeight(p)
	if h == nil {
		z.sendWorldEvents(p)
	}
	p.sendPages(z.inventoryPages(p))
	// pc_scdata_received runs pc_inventory_rentals at login: rentals
	// that ended while the player was away go (pc.cpp 14807).
	z.expirePlayerRentals(p, z.wallSeconds())
	z.sendCart(p)
	z.sendPlayerLimits(p)
	// The map joins the character's visited maps, and the world map
	// comes at login and with each new map.
	z.visit(p, h == nil)
	// CurrencyUpdate comes at login, as its message says, when the
	// player has a currency: the client starts with none.
	z.sendCurrenciesHeld(p)
	// The quest log comes at login and after each map change.
	z.sendQuestList(p)
	// clif_parse_LoadEndAck sets up the quest icons of the map's NPCs
	// and shows them (clif.cpp 11187).
	p.questInfoReinit, p.questInfoDirty = true, true
	// Saved statuses and timed bonuses come back, as chrif_load_scdata
	// and the bonus_script load. A login fires status_start; a map change
	// does not, since they never ended.
	z.restoreStatuses(p, c.Statuses, h == nil)
	z.restoreTimedBonuses(p, c.TimedBonuses)
	z.holdStats--
	if p.statsDirty {
		z.recalc(p)
	}
	if s := z.battleStats(p); s != nil {
		p.pending = append(p.pending, s)
	}
	z.loadPlagiarism(p, c.Variables)
	z.sendSkillList(p)
	z.startPassives(p)
	// The first save comes one interval after the player enters. A
	// warp saves at once, as the map change checkpoint of strategy
	// section 3.8.
	if s := p.sess; s != nil && s.saver != nil {
		p.saved = z.snapshot(p)
		if h != nil {
			s.saver.Save(p.saved, false)
		}
	}
	p.nextSave = z.now + z.saveInterval().Milliseconds()
	if len(c.Variables) > 0 {
		p.vars = maps.Clone(c.Variables)
	}
	z.chatOnline(p, h)
	z.socialOnline(p, h, friendsOf(c))
	// Other zones find the player from now on (playerworld.go).
	z.listPlayer(p)
	if h == nil {
		p.rewarp = 0
		p.touchOnEnter = true
		z.playerEvent("login", p)
	} else {
		p.temp = h.temp
		z.entered(p)
	}
	z.startNPCs()
	z.playerEvent("map_enter", p)
	// The player sees itself from the first frame.
	return joinResult{netID: p.netID(e), tick: z.tick}
}

func (z *Zone) startCell() (int, int) {
	if z.HasSpawn && !z.grid.NoPass(z.Spawn[0], z.Spawn[1]) {
		return z.Spawn[0], z.Spawn[1]
	}
	cx, cy := z.grid.Width/2, z.grid.Height/2
	for r := range max(z.grid.Width, z.grid.Height) {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if !z.grid.NoPass(cx+dx, cy+dy) {
					return cx + dx, cy + dy
				}
			}
		}
	}
	return cx, cy
}

func (p *Player) netID(e *entity) uint32 {
	if id, ok := p.netIDs.get(e); ok {
		return id
	}
	p.nextNet++
	p.netIDs.set(e, p.nextNet)
	return p.nextNet
}

func (z *Zone) leave(p *Player, reason wathenav1.VanishReason) {
	if p.entity == nil || !z.entities.has(p.entity) || p.leaving != nil {
		return
	}
	if reason == wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT && z.detachedLeave(p) {
		// An autotrader's connection closed: it stays with its store.
		return
	}
	if reason == wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT {
		// Handlers see the player before it goes. map_quit ends the
		// statuses that rAthena does not save, and timed bonuses that end
		// on logout. A tainted player's handlers do not run, since they
		// could move items that its last saved copy does not have.
		held := 0
		if !p.tainted {
			z.scriptEventFor("logout", modscript.Player(int64(p.charID)))
			// The NPCs of other maps hear it too (review decision 59), and
			// the logout waits for them, as npc_script_event runs before
			// map_quit (map.cpp 2266).
			held = z.sendPlayerEvent("logout", p, nil, nil, true)
		}
		if en := z.statusEngine(); en != nil && p.entity.sc != nil {
			en.Logout(z.unit(p.entity))
		}
		z.endTimedBonuses(p, "logout")
		z.runScripts()
		if held > 0 {
			p.leaving = &logoutHold{pending: held, until: z.now + logoutHoldMs}
			return
		}
		z.chatOffline(p)
		z.socialOffline(p)
	}
	z.leaveMap(p, reason)
}

// finishLogout ends a logout that waited for the NPC handlers of other
// zones (playerworld.go): their calls on the player ran first.
func (z *Zone) finishLogout(p *Player) {
	if p.leaving == nil {
		return
	}
	p.leaving = nil
	z.chatOffline(p)
	z.socialOffline(p)
	z.leaveMap(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
}

// leaveMap is the end of leave: the player's stores, trades and storage
// close, it is saved and it vanishes.
func (z *Zone) leaveMap(p *Player, reason wathenav1.VanishReason) {
	z.tradeCancel(p, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED)
	z.closeStores(p)
	if p.autotrade {
		p.autotrade = false
		if t := z.Autotraders; t != nil {
			t.remove(p.record.ID, p.sess)
		}
	}
	z.storagesLeave(p, reason == wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	if reason == wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT {
		z.mailLeave(p)
	}
	z.castCancel(p.entity, false)
	z.clearUnitGroups(p.entity)
	z.removeMap(p.entity)
	z.stopPassives(p)
	z.save(p, true)
	z.dropStatuses(p.entity)
	z.remove(p, reason)
	z.unwearAll(p)
	// A conversation's pending call fails once the player is gone, so a
	// script that goes on cannot change the saved character.
	z.closeTalk(p)
	z.drop(p)
	p.netIDs.reset()
}

// remove takes a player out of the zone. Viewers see it vanish for a
// reason.
func (z *Zone) remove(p *Player, reason wathenav1.VanishReason) {
	z.unlistPlayer(p)
	z.untouchAll(p)
	z.entities.remove(p.entity)
	delete(z.players, p.charID)
	z.sharedCells()
	z.forget(p.entity)
	for e := range z.entities.all() {
		if e.player == nil {
			continue
		}
		if id, ok := e.player.netIDs.get(p.entity); ok {
			e.player.netIDs.del(p.entity)
			z.queue(e.player, wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{
				Tick: z.tick, AckSeq: e.player.ackSeq, Events: []*wathenav1.Event{vanish(id, reason)},
			}.Build()}.Build())
		}
	}
}

// drop closes a player's messages. Its connection then closes.
func (z *Zone) drop(p *Player) {
	if !p.dropped {
		p.dropped = true
		z.closeOut(p)
	}
}

func (z *Zone) handle(c command) {
	p := c.player
	if c.queued {
		p.queued.Add(-1)
	}
	if p.gone() || !z.entities.has(p.entity) {
		return
	}
	if c.seq > p.ackSeq {
		p.ackSeq = c.seq
	}
	if c.raise != nil {
		// rAthena lets a dead player raise stats.
		z.raiseStat(p, c.raise.stat, c.raise.amount)
	}
	if c.chat != nil {
		// Dead players can talk too: clif_process_message only checks
		// for mutes.
		z.say(p, c.chat)
	}
	if c.staff != nil {
		c.staff(z, p)
	}
	if c.channel != nil {
		z.channelCommand(p, c.channel)
	}
	if c.block != nil {
		z.block(p, c.block)
	}
	if c.report != nil {
		z.report(p, c.report)
	}
	// Dead players may use their party and friend lists, as rAthena.
	if c.party != nil {
		z.partyCommand(p, c.party)
	}
	if c.friend != nil {
		z.friendCommand(p, c.friend)
	}
	if c.booking != nil {
		z.bookingCommand(p, c.booking)
	}
	if c.guild != nil {
		z.guildCommand(p, c.guild)
	}
	if c.storage != nil {
		z.storageCommand(p, c.storage)
	}
	if c.trade != nil {
		z.tradeCommand(p, c.trade)
	}
	if c.npcReply != nil {
		z.reply(p, c.npcReply)
	}
	if c.shopClose != nil {
		if z.shopFor(p, *c.shopClose) != nil {
			z.closeShop(p, true)
		}
	}
	if c.shopBuy != nil {
		z.buy(p, c.shopBuy.npc, c.shopBuy.lines)
	}
	if c.shopSell != nil {
		z.sell(p, c.shopSell.npc, c.shopSell.lines)
	}
	if c.mail != nil {
		z.mailCommand(p, c.mail)
	}
	if c.vending != nil {
		z.vendingCommand(p, c.vending)
	}
	if c.buying != nil {
		z.buyingCommand(p, c.buying)
	}
	if c.search != nil {
		z.searchCommand(p, c.search)
	}
	if c.memo {
		z.memo(p)
	}
	if c.questActive != nil {
		z.questActive(p, c.questActive)
	}
	me := p.entity
	if me.dead {
		return
	}
	if c.npcClick != nil && p.trade == nil {
		// npc_click: a player who trades cannot talk to NPCs.
		z.click(p, *c.npcClick)
	}
	if p.talk != nil {
		// pc_cant_act: a player who talks to an NPC cannot walk,
		// attack, pick up or drop items. A walk cancels a progress bar.
		if c.walkTo != nil {
			z.walkDuringTalk(p)
		}
		z.duringTalk(p, c)
		return
	}
	if (c.walkTo != nil || c.attack != nil) && (p.storage != nil || p.guild.storage != nil) {
		// rAthena's pc_cant_act refuses a walk or an attack while a
		// storage is open, so a player uses its storage only where it
		// opened it and out of combat. The web client lets the player
		// walk with the window open, so the walk or the attack closes
		// the storage and goes on.
		z.closeStorage(p)
		z.closeGuildStorage(p, true)
	}
	if c.walkTo != nil && p.hasStore() {
		// A vendor or a buying store owner does not walk, as
		// clif_parse_WalkToXY with state.vending or state.buyingstore.
		c.walkTo = nil
	}
	if c.walkTo != nil && !me.cant().Move && (me.cast == nil || freeCast(me)) && z.now >= me.canMove && !p.isBlocked(blockMove) {
		// unit_walktoxy: statuses that stop moving refuse the walk, and
		// Confusion picks another cell.
		me.target, me.pickItem, me.pending = nil, nil, nil
		x, y := c.walkTo[0], c.walkTo[1]
		z.cloakingWalk(me)
		if me.sc.Has(statuschange.SCConfusion) {
			x, y = z.randomDir(me, x, y)
		}
		z.walk(me, x, y)
	}
	if c.use != nil && p.trade != nil {
		// pc_useitem: MSI_BUSY while trading.
		z.busy(p)
	} else if c.use != nil && !p.isBlocked(blockUseItem) {
		// clif_parse_UseItem refuses PCBLOCK_USEITEM (clif.cpp 12103).
		z.useItem(p, *c.use)
	}
	if c.drop != nil {
		z.dropItem(p, c.drop.slot, c.drop.amount)
	}
	// clif_parse_EquipItem and clif_parse_UnequipItem refuse
	// PCBLOCK_EQUIP (clif.cpp 12141, 12189).
	if c.equip != nil && !p.isBlocked(blockEquip) {
		z.equipItem(p, c.equip.slot, c.equip.position)
	}
	if c.unequip != nil && !p.isBlocked(blockEquip) {
		z.unequipItem(p, *c.unequip)
	}
	if c.insertCard != nil {
		z.insertCard(p, c.insertCard.card, c.insertCard.equip)
	}
	if c.learnSkill != nil {
		z.learnSkill(p, *c.learnSkill)
	}
	if c.useSkill != nil && !p.isBlocked(blockSkill) {
		// clif_parse_skill_toid and clif_parse_UseSkillToPosSub refuse
		// PCBLOCK_SKILL (clif.cpp 12897, 13023).
		z.useSkill(p, c.useSkill)
	}
	// The game client does not attack while SC_WEIGHT90 is on, which
	// rAthena relies on. The zone checks it itself. A warg rider does
	// not attack, as unit_attack.
	if c.attack != nil && p.overweight != item.Weight90 && !me.cant().Attack && p.mountRule() != companion.RuleWugRider && !p.isBlocked(blockAttack) {
		for e, id := range p.netIDs.all() {
			// unit_attack's battle_check_target: a player attacks only
			// enemies. Other players are not enemies, since wAthena has
			// no PvP maps, and summons are enemies only as
			// alchemist_summon_setting allows.
			if id == c.attack.target && e != me && e.status != nil && !e.dead && isEnemy(me, e) {
				me.target, me.repeat, me.pickItem = e, c.attack.repeat, nil
			}
		}
	}
	if c.pickUp != nil {
		z.startPickUp(p, *c.pickUp)
	}
}

// walk starts or changes a walk. A unit in the middle of a step finishes
// it first, so the new path starts from the cell it is stepping into.
func (z *Zone) walk(e *entity, x, y int) {
	fromX, fromY := e.x, e.y
	var current []path.Dir
	if len(e.path) > 0 {
		dx, dy := e.path[0].Step()
		fromX, fromY = e.x+dx, e.y+dy
		current = e.path[:1]
	}
	steps, ok := path.Search(z.grid, fromX, fromY, x, y, false)
	if !ok || len(steps) == 0 {
		return
	}
	if len(current) == 0 {
		e.stepStart = z.now
	}
	e.path = append(append([]path.Dir(nil), current...), steps...)
	e.walkChanged = true
}

// step advances one tick: walks, then what each player sees.
func (z *Zone) step() { z.stepAt(time.Since(z.start).Milliseconds()) }

// stepAt advances one tick to a time in zone milliseconds.
func (z *Zone) stepAt(now int64) {
	began := time.Now()
	z.tick++
	z.now = now
	if z.scripts != nil {
		// Each mod's Lua CPU budget is whole again (review decision 52).
		z.scripts.BeginTick()
	}
	var warped []warpTouch
	for e := range z.entities.all() {
		touched := false
		for len(e.path) > 0 {
			d := e.path[0]
			end := e.stepStart + int64(path.StepTime(d, e.speed))
			if z.now < end {
				break
			}
			dx, dy := d.Step()
			e.x += dx
			e.y += dy
			e.dir = facing(d)
			e.path = e.path[1:]
			e.stepStart = end
			z.skillUnitMove(e)
			// map_moveblock: SC_PROPERTYWALK leaves a unit on the cell
			// left.
			z.propertyWalkMoved(e, e.x-dx, e.y-dy)
			if e.monster != nil && !e.dead {
				// unit_walktoxy_timer: the monster's next step, or the end
				// of its walk, then the touch areas it walks into.
				z.mobStepped(e)
				z.monsterTouched(e)
			}
			if e.npc != nil {
				// A walking NPC's touch area moves with it (npcpresent.go).
				z.npcStepped(e)
			}
			// unit_walktoxy_timer touches the warp on each cell a
			// player enters. A warp that takes the player ends the
			// walk. Otherwise the player steps into and out of NPCs'
			// touch areas.
			if p := e.player; p != nil {
				if w := z.warpAt(e.x, e.y); w != nil && z.target(p, w) != nil {
					e.path = e.path[:0]
					warped = append(warped, warpTouch{p, w})
					touched = true
				} else {
					z.touchAreas(p)
				}
			}
		}
		// clif_parse_LoadEndAck touches the cell a player entered on,
		// once its first frame went out.
		if p := e.player; p != nil && p.touchOnEnter && !p.fresh {
			p.touchOnEnter = false
			if w := z.warpAt(e.x, e.y); w != nil && !touched && len(e.path) == 0 && z.target(p, w) != nil {
				warped = append(warped, warpTouch{p, w})
			} else if !touched {
				z.touchAreas(p)
			}
		}
	}
	for _, t := range warped {
		z.warp(t.p, t.w)
	}
	moved := time.Now()
	z.Metrics.Phase(metrics.PhaseMove, moved.Sub(began))
	z.revive()
	z.think()
	z.fight()
	z.pickUps()
	z.dropItems()
	z.expireItems()
	z.advanceCasts()
	z.advanceTimerSkills()
	z.advanceSpiritBalls()
	z.advanceUnits()
	z.advanceTimerSkills()
	z.revealTick()
	z.naturalHeal()
	z.advanceStatuses()
	z.checkSanctions()
	z.expireRentals()
	z.partyUpdates()
	z.guildUpdates()
	z.autosave()
	z.runScripts()
	z.talkTimers()
	z.queuedTouches()
	if z.scripts != nil {
		z.scripts.Advance(z.now)
		z.flushDeferred()
		z.runSchedules()
		z.runScripts()
	}
	// pc_show_questinfo for the players whose items, levels, jobs or
	// quests changed this step (npcpresent.go).
	z.refreshQuestInfo()
	// Changes that no script's call made, such as a timer's bonuses that
	// end, are computed here.
	z.flushStats()
	z.mailNotices()
	acted := time.Now()
	z.Metrics.Phase(metrics.PhaseAct, acted.Sub(moved))
	if len(z.players) > 0 {
		z.blocks.build(z.grid.Width, z.grid.Height, &z.entities)
	}
	for e := range z.entities.all() {
		if e.player != nil {
			z.updateSafely(e.player)
		}
	}
	z.Metrics.Phase(metrics.PhaseSend, time.Since(acted))
	for e := range z.entities.all() {
		e.walkChanged = false
		e.stopped = false
		e.lookChanged = false
		e.guildChanged = false
		e.shopChanged = false
		e.optionChanged = false
		e.hpChanged = false
		if e.item != nil {
			e.item.fresh = false
		}
	}
	// clear drops the entities from the arrays too, so a player that
	// left is not kept in memory by the last tick's hits.
	clear(z.hits)
	clear(z.removed)
	z.hits = z.hits[:0]
	z.removed = z.removed[:0]
	// Every player has seen the removals now, those of this tick and those
	// of commands handled since the last one, so their slots may be used
	// again. Freed at the start of a tick instead, a slot of a monster
	// that a command killed could go to its drop before the players saw
	// the death, and they would keep the monster.
	z.entities.compact()
	z.takes = z.takes[:0]
	z.effects = z.effects[:0]
	z.emotions = z.emotions[:0]
	clear(z.skillEvents)
	z.skillEvents = z.skillEvents[:0]
	z.used = z.used[:0]
	clear(z.chats)
	z.chats = z.chats[:0]
	z.sharedCells()
	z.publishPlayers()
	z.publishMonsters()
	z.heldLogouts()
}

// warpTouch is a player that stepped onto a warp this tick.
type warpTouch struct {
	p *Player
	w *Warp
}

// update sends a player what changed in view this tick.
func (z *Zone) update(p *Player) {
	var events []*wathenav1.Event
	if p.mapChange {
		p.mapChange = false
		events = append(events, z.mapChange(p))
	}
	if z.viewsForTest != nil {
		events = z.viewsForTest(p, events)
	} else {
		events = z.viewEvents(p, events)
	}
	events = append(events, z.skillEventsFor(p)...)
	events = append(events, z.combatEvents(p)...)
	events = append(events, z.effectEvents(p)...)
	events = append(events, z.emotionEvents(p)...)
	events = append(events, z.usedEvents(p)...)
	events = append(events, z.chatEvents(p)...)
	events = append(events, p.pending...)
	p.pending = nil
	if len(events) == 0 {
		z.flushPages(p)
		return
	}
	z.queueFrames(p, events)
	p.fresh = false
	z.flushPages(p)
}

// frameRoom is what a frame keeps free for its tick and its ack, and
// eventHeader the most bytes an event's field tag and length take in a
// frame under schema.MaxMessageBytes.
const (
	frameRoom   = 64
	eventHeader = 4
)

// queueFrames sends a tick's events in one frame, or in several frames
// of the same tick when one would pass schema.MaxMessageBytes, which the
// gateway refuses to send. The client applies the frames in order, as it
// does the extra frames that vanish a player who left. This happens when
// a player first sees a crowd or many ground items at once.
func (z *Zone) queueFrames(p *Player, events []*wathenav1.Event) {
	frame := func(events []*wathenav1.Event) *wathenav1.ServerMessage {
		return wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{
			Tick: z.tick, AckSeq: p.ackSeq, Events: events,
		}.Build()}.Build()
	}
	m := frame(events)
	if proto.Size(m) <= schema.MaxMessageBytes {
		z.queue(p, m)
		return
	}
	limit := schema.MaxMessageBytes - frameRoom
	start, size := 0, 0
	for i, ev := range events {
		n := proto.Size(ev) + eventHeader
		if n > limit {
			// No frame can carry it. It cannot happen with the events
			// there are, and sending it would end the connection.
			z.logError("a frame event is over the message size limit", "player", p.Name, "bytes", n)
			if start < i {
				z.queue(p, frame(events[start:i]))
			}
			start, size = i+1, 0
			continue
		}
		if size+n > limit && start < i {
			z.queue(p, frame(events[start:i]))
			start, size = i, 0
		}
		size += n
	}
	if start < len(events) {
		z.queue(p, frame(events[start:]))
	}
}

// viewEvents adds the appear, move, stop and vanish events of what the
// player sees this tick.
func (z *Zone) viewEvents(p *Player, events []*wathenav1.Event) []*wathenav1.Event {
	me := p.entity
	// Entities in view: those of the blocks the view touches. No entity
	// moves while players update, so the blocks built for this tick
	// hold every entity where it is.
	bx0, bx1 := span(me.x, ViewRange, z.blocks.w)
	by0, by1 := span(me.y, ViewRange, z.blocks.h)
	for by := by0; by <= by1; by++ {
		for _, block := range z.blocks.blocks[by*z.blocks.w+bx0 : by*z.blocks.w+bx1+1] {
			for _, e := range block {
				if abs(e.x-me.x) > ViewRange || abs(e.y-me.y) > ViewRange {
					continue
				}
				if e.unit != nil && (e.unit.hidden || trapHiddenFrom(e, me)) {
					continue
				}
				if hiddenFrom(e, me) {
					continue
				}
				id, known := p.netIDs.get(e)
				switch {
				case !known || e == me && p.fresh:
					id = p.netID(e)
					if e.unit != nil {
						events = append(events, z.unitAppear(p, id, e))
						break
					}
					events = append(events, z.appear(id, e))
					if len(e.path) > 0 {
						events = append(events, z.move(id, e))
					}
					if m := e.monster; m != nil && m.spawn.size != 0 {
						// A small or big monster of a script, as
						// clif_getareachar_unit (clif.cpp 5118).
						events = append(events, effectEvent(id, map[int]uint32{sizeSmall: efBabyBody2, sizeBig: efGiantBody2}[m.spawn.size]))
					}
					if icon, ok := p.questIcons[e]; ok {
						events = append(events, questIconEvent(id, icon.icon, icon.mark))
					}
					if e.npc != nil {
						events = append(events, z.npcBarEvents(id, e)...)
					}
					if e.player != nil && e.player.hasStore() {
						events = append(events, entityShop(id, e.player))
					}
				case e.walkChanged:
					events = append(events, z.move(id, e))
				case e.stopped:
					events = append(events, stop(id, e))
				}
				if known && e.lookChanged && e.player != nil {
					events = append(events, lookChange(id, e.player))
				}
				if known && e.lookChanged && e.npc != nil && (e != me || !p.fresh) {
					// unit_refresh: the NPC appears again with its new view,
					// name or title (npcpresent.go).
					events = append(events, z.appear(id, e))
					if len(e.path) > 0 {
						events = append(events, z.move(id, e))
					}
					events = append(events, z.npcBarEvents(id, e)...)
				}
				if known && e.guildChanged && e.player != nil {
					events = append(events, entityGuild(id, e.player))
				}
				if known && e.shopChanged && e.player != nil {
					events = append(events, entityShop(id, e.player))
				}
				if known && e.optionChanged && (e != me || !p.fresh) {
					events = append(events, optionChange(id, e))
				}
			}
		}
	}
	// Entities the player knew that are still in the zone and went out
	// of view.
	for i := 0; i < len(p.netIDs.known); {
		e := p.netIDs.known[i]
		if (abs(e.x-me.x) <= ViewRange && abs(e.y-me.y) <= ViewRange && !hiddenFrom(e, me)) || !z.entities.has(e) {
			i++
			continue
		}
		// del moves the last known entity to i.
		id := p.netIDs.id(e)
		p.netIDs.del(e)
		events = append(events, vanish(id, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT))
	}
	return events
}

// hiddenFrom reports whether a viewer must not be sent a player or a
// monster at all, so a modified client cannot show it:
//   - A GM under @hide, to everyone else. rAthena sends it as
//     JT_INVISIBLE, which no client draws, to staff too.
//   - A player under Hiding, Cloaking or Chase Walk, to everyone but
//     itself, its party and detectors. rAthena's clif_send holds such a
//     unit's packets from enemies without intravision (clif_ally_only),
//     and status_check_visibility lets detectors see it. Party members
//     are its allies.
//   - A monster under the same options, to every viewer it is an enemy
//     of, unless the viewer is a detector. A player's summon is not an
//     enemy of players, so they still see it.
func hiddenFrom(e, viewer *entity) bool {
	if e.npc != nil && viewer.player != nil {
		// An NPC that is disabled, hidden, cloaked for the viewer or
		// without a view (npcworld.go).
		return !e.npc.visibleTo(viewer.player)
	}
	if e == viewer || e.player == nil && e.monster == nil {
		return false
	}
	if gmHidden(e) {
		return true
	}
	if l := e.sc; l == nil || l.Option&(statuschange.OptionHide|statuschange.OptionCloak|optionChasewalk) == 0 {
		return false
	}
	if e.player != nil && sameParty(e, viewer) {
		return false
	}
	if e.monster != nil && !isEnemy(e, viewer) {
		return false
	}
	return viewer.status == nil || viewer.status.Mode&modeDetector == 0
}

func (z *Zone) appear(id uint32, e *entity) *wathenav1.Event {
	if e.npc != nil {
		return z.npcAppear(id, e)
	}
	if e.warp != nil {
		// A warp portal is an NPC with the warp sprite, as rAthena's
		// JT_WARPNPC. It has no name to show.
		return wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{
			Id:   id,
			Kind: wathenav1.EntityKind_ENTITY_KIND_NPC,
			X:    uint32(e.x), // #nosec G115 -- map cells are at most 1024.
			Y:    uint32(e.y), // #nosec G115 -- as above.
			Look: wathenav1.Look_builder{Body: WarpSprite}.Build(),
		}.Build()}.Build()
	}
	if it := e.item; it != nil {
		return wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{
			Id:   id,
			Kind: wathenav1.EntityKind_ENTITY_KIND_ITEM,
			X:    uint32(e.x), // #nosec G115 -- map cells are at most 1024.
			Y:    uint32(e.y), // #nosec G115 -- as above.
			Item: wathenav1.GroundItem_builder{
				ItemId:  it.id,
				Amount:  uint32(it.amount), // #nosec G115 -- a stack of at most 30000.
				SubX:    uint32(it.subX),   // #nosec G115 -- 0 to 15.
				SubY:    uint32(it.subY),   // #nosec G115 -- as above.
				Dropped: it.fresh,
			}.Build(),
		}.Build()}.Build()
	}
	if m := e.monster; m != nil {
		return wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{
			Id:         id,
			Kind:       wathenav1.EntityKind_ENTITY_KIND_MONSTER,
			X:          uint32(e.x),   // #nosec G115 -- map cells are at most 1024.
			Y:          uint32(e.y),   // #nosec G115 -- as above.
			Dir:        uint32(e.dir), // #nosec G115 -- 0 to 7.
			Look:       wathenav1.Look_builder{Body: cmp.Or(m.spawn.Sprite, m.spawn.Mob)}.Build(),
			Name:       m.spawn.Name,
			WalkMs:     uint32(e.speed), // #nosec G115 -- a small speed.
			HpPermille: e.hpPermille(),
			Options:    e.options(),
		}.Build()}.Build()
	}
	p := e.player
	return wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{
		Id:         id,
		Kind:       wathenav1.EntityKind_ENTITY_KIND_PLAYER,
		X:          uint32(e.x),   // #nosec G115 -- map cells are at most 1024.
		Y:          uint32(e.y),   // #nosec G115 -- as above.
		Dir:        uint32(e.dir), // #nosec G115 -- 0 to 7.
		Look:       p.Look.message(),
		Name:       p.Name,
		WalkMs:     uint32(e.speed), // #nosec G115 -- a small speed.
		HpPermille: e.hpPermille(),
		Options:    e.options(),
		Guild:      p.guild.badge,
	}.Build()}.Build()
}

func (z *Zone) move(id uint32, e *entity) *wathenav1.Event {
	// Paths do not change while players update, so the steps are packed
	// once per tick. Messages only read them.
	if e.packed == nil || e.packedTick != z.tick {
		e.packed, e.packedTick = PackSteps(e.path), z.tick
	}
	tickStart := z.now
	return wathenav1.Event_builder{Move: wathenav1.EntityMove_builder{
		Id:      id,
		FromX:   uint32(e.x), // #nosec G115 -- map cells are at most 1024.
		FromY:   uint32(e.y), // #nosec G115 -- as above.
		Steps:   e.packed,
		StartMs: int32(e.stepStart - tickStart), // #nosec G115 -- within one step of now.
		WalkMs:  uint32(e.speed),                // #nosec G115 -- a small speed.
	}.Build()}.Build()
}

func vanish(id uint32, reason wathenav1.VanishReason) *wathenav1.Event {
	return wathenav1.Event_builder{Vanish: wathenav1.EntityVanish_builder{Id: id, Reason: reason}.Build()}.Build()
}

// facing turns a walk direction (0 north, counterclockwise) into the
// protocol's facing (0 south, clockwise).
func facing(d path.Dir) int { return (4 - int(d) + 8) % 8 }

// PackSteps encodes walk directions as the protocol's steps: one facing
// per 4 bits, low bits first, padded with 0xF.
func PackSteps(steps []path.Dir) []byte {
	out := make([]byte, (len(steps)+1)/2)
	for i, d := range steps {
		f := byte(facing(d)) // #nosec G115 -- 0 to 7.
		if i%2 == 0 {
			out[i/2] = f | 0xF0
		} else {
			out[i/2] = out[i/2]&0x0F | f<<4
		}
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
