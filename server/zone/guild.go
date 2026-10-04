package zone

import (
	"context"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Guilds (phase 5 step 5, proposal 0002 review decision 28, decision
// 0021):
//   - The world's social registry holds every guild. A zone runs its
//     players' guild commands through it, and turns its notices into
//     protocol events and Lua events.
//   - Others see a player's guild as a badge with its name: the guild's
//     name, the position's name and the emblem's version, as clif_name
//     and clif_guild_emblem_area. A change goes to the players in sight.
//   - Members on the same map see each other's cells every 5 seconds,
//     as guild_send_xy_timer.
//   - A member pays the guild its position's exp tax of base exp, as
//     pc_gainexp's guild_payexp.

// guildXYInterval is GUILD_SEND_XY_INTERVAL.
const guildXYInterval = 5000

// itemEmperium is ITEMID_EMPERIUM, which makes a guild.
const itemEmperium = 714

// guildState is a player's guild as this zone knows it.
type guildState struct {
	// view is the player's guild from the registry, or nil.
	view *social.GuildView
	// badge is what others see of it, or nil.
	badge *wathenav1.GuildBadge
	// storage is the guild storage the player has open, or nil.
	storage *social.GuildStorage
	// sentX, sentY and nextXY are the cell the members on the map last
	// got, and when it goes out next.
	sentX, sentY int
	nextXY       int64
}

type guildCommand struct {
	create      string
	invite      string
	reply       *guildReply
	leave       *string
	expel       *guildExpel
	positions   []social.PositionChange
	memberPos   *guildMemberPos
	notice      *social.GuildNotice
	skillUp     int
	ally        string
	allyReply   *guildReply
	enemy       string
	endRelation *guildRelationEnd
	disband     string
}

type guildReply struct {
	guild  int64
	accept bool
}

type guildExpel struct{ name, reason string }

type guildMemberPos struct {
	name     string
	position int
}

type guildRelationEnd struct {
	guild int64
	enemy bool
}

type storageCommand struct {
	move  *storageMove
	close bool
	log   bool
}

type storageMove struct {
	kind         StorageMoveKind
	slot, amount int
}

// StorageMoveKind is which way a storage command moves items.
type StorageMoveKind int

// The storage moves.
const (
	// MovePut is from the inventory into the open storage.
	MovePut StorageMoveKind = iota
	// MoveTake is from the open storage into the inventory.
	MoveTake
	// MoveCartPut is from the inventory into the cart.
	MoveCartPut
	// MoveCartTake is from the cart into the inventory.
	MoveCartTake
	// MoveCartToStorage is from the cart into the open storage.
	MoveCartToStorage
	// MoveStorageToCart is from the open storage into the cart.
	MoveStorageToCart
)

// The guild commands of a session.

// GuildCreate makes a guild with the player as its master.
func (s *Session) GuildCreate(seq uint32, name string) bool {
	return s.send(command{seq: seq, guild: &guildCommand{create: name}})
}

// GuildInvite invites a player by name.
func (s *Session) GuildInvite(seq uint32, name string) bool {
	return s.send(command{seq: seq, guild: &guildCommand{invite: name}})
}

// GuildReply answers a guild invite.
func (s *Session) GuildReply(seq uint32, guild int64, accept bool) bool {
	return s.send(command{seq: seq, guild: &guildCommand{reply: &guildReply{guild, accept}}})
}

// GuildLeave leaves the player's guild.
func (s *Session) GuildLeave(seq uint32, reason string) bool {
	return s.send(command{seq: seq, guild: &guildCommand{leave: &reason}})
}

// GuildExpel removes a member.
func (s *Session) GuildExpel(seq uint32, name, reason string) bool {
	return s.send(command{seq: seq, guild: &guildCommand{expel: &guildExpel{name, reason}}})
}

// GuildPositions changes the guild's positions.
func (s *Session) GuildPositions(seq uint32, changes []social.PositionChange) bool {
	return s.send(command{seq: seq, guild: &guildCommand{positions: changes}})
}

// GuildMemberPosition gives a member a position. Position 0 hands the
// guild over.
func (s *Session) GuildMemberPosition(seq uint32, name string, position int) bool {
	return s.send(command{seq: seq, guild: &guildCommand{memberPos: &guildMemberPos{name, position}}})
}

// GuildNotice changes the guild notice.
func (s *Session) GuildNotice(seq uint32, n social.GuildNotice) bool {
	return s.send(command{seq: seq, guild: &guildCommand{notice: &n}})
}

// GuildSkillUp raises a guild skill.
func (s *Session) GuildSkillUp(seq uint32, skill int) bool {
	return s.send(command{seq: seq, guild: &guildCommand{skillUp: skill}})
}

// GuildAlly asks a player's guild to be allies.
func (s *Session) GuildAlly(seq uint32, name string) bool {
	return s.send(command{seq: seq, guild: &guildCommand{ally: name}})
}

// GuildAllyReply answers an alliance request.
func (s *Session) GuildAllyReply(seq uint32, guild int64, accept bool) bool {
	return s.send(command{seq: seq, guild: &guildCommand{allyReply: &guildReply{guild, accept}}})
}

// GuildEnemy makes a player's guild an enemy.
func (s *Session) GuildEnemy(seq uint32, name string) bool {
	return s.send(command{seq: seq, guild: &guildCommand{enemy: name}})
}

// GuildEndRelation ends an alliance or an enemy relation.
func (s *Session) GuildEndRelation(seq uint32, guild int64, enemy bool) bool {
	return s.send(command{seq: seq, guild: &guildCommand{endRelation: &guildRelationEnd{guild, enemy}}})
}

// GuildDisband breaks the guild up.
func (s *Session) GuildDisband(seq uint32, name string) bool {
	return s.send(command{seq: seq, guild: &guildCommand{disband: name}})
}

// GuildChat says a line to the player's guild.
func (s *Session) GuildChat(seq uint32, text string) bool {
	return s.send(command{seq: seq, chat: &chatCommand{kind: chatGuild, text: text}})
}

// StoragePut moves an amount from an inventory slot into the open
// storage.
func (s *Session) StoragePut(seq uint32, slot, amount int) bool {
	return s.StorageMove(seq, MovePut, slot, amount)
}

// StorageTake moves an amount from a storage slot into the inventory.
func (s *Session) StorageTake(seq uint32, slot, amount int) bool {
	return s.StorageMove(seq, MoveTake, slot, amount)
}

// StorageMove moves an amount between the inventory, the open storage
// and the cart. slot is in the holder the amount leaves.
func (s *Session) StorageMove(seq uint32, kind StorageMoveKind, slot, amount int) bool {
	return s.send(command{seq: seq, storage: &storageCommand{move: &storageMove{kind, slot, amount}}})
}

// StorageClose closes the storage window.
func (s *Session) StorageClose(seq uint32) bool {
	return s.send(command{seq: seq, storage: &storageCommand{close: true}})
}

// StorageLog asks for the guild storage log.
func (s *Session) StorageLog(seq uint32) bool {
	return s.send(command{seq: seq, storage: &storageCommand{log: true}})
}

// guildRules are the world's guild rules.
func (z *Zone) guildRules() rules.Guild { return rules.GuildOrDefault(z.worldRules()) }

// guildTables are the world's guild tables.
func (z *Zone) guildTables() *social.GuildTables {
	if z.Mods != nil && z.Mods.Guilds != nil {
		return z.Mods.Guilds
	}
	return social.CoreGuildTables(z.renewal())
}

func guildResultEvent(o social.GuildOutcome, name string) *wathenav1.Event {
	return wathenav1.Event_builder{GuildResult: wathenav1.GuildResult_builder{
		Outcome: wathenav1.GuildOutcome(o), Name: name, // #nosec G115 -- the outcomes match the protocol's.
	}.Build()}.Build()
}

// guildCommand runs a guild command.
func (z *Zone) guildCommand(p *Player, c *guildCommand) {
	if p.soc == nil {
		return
	}
	reg := z.socialHub()
	switch {
	case c.create != "":
		z.createGuild(p, c.create)
	case c.invite != "":
		reg.GuildInvite(p.soc, c.invite)
	case c.reply != nil:
		reg.GuildReply(p.soc, c.reply.guild, c.reply.accept)
	case c.leave != nil:
		reg.GuildLeave(p.soc, *c.leave)
	case c.expel != nil:
		reg.GuildExpel(p.soc, c.expel.name, c.expel.reason)
	case c.positions != nil:
		reg.GuildSetPositions(p.soc, c.positions)
	case c.memberPos != nil:
		if _, ch := reg.GuildSetMemberPosition(p.soc, c.memberPos.name, c.memberPos.position); ch != nil {
			z.guildChanged(ch)
		}
	case c.notice != nil:
		if _, ch := reg.GuildSetNotice(p.soc, *c.notice); ch != nil {
			z.guildChanged(ch)
		}
	case c.skillUp != 0:
		reg.GuildSkillUp(p.soc, c.skillUp)
	case c.ally != "":
		reg.GuildAlly(p.soc, c.ally)
	case c.allyReply != nil:
		reg.GuildAllyReply(p.soc, c.allyReply.guild, c.allyReply.accept)
	case c.enemy != "":
		reg.GuildEnemy(p.soc, c.enemy)
	case c.endRelation != nil:
		reg.GuildEndRelation(p.soc, c.endRelation.guild, c.endRelation.enemy)
	case c.disband != "":
		id := reg.GuildIDOf(p.soc)
		if o, left := reg.GuildDisband(p.soc, c.disband); o == social.GuildDisbanded {
			z.storageLost(p, id, left)
		}
	}
}

// createGuild is guild_create from the client: the name passes the chat
// filter, and with guild_emperium_check the player gives an Emperium,
// as guild_created.
func (z *Zone) createGuild(p *Player, name string) {
	if err := z.chatHub().Filter.CheckName(name); err != nil || !social.ValidGuildName(name) {
		p.pending = append(p.pending, guildResultEvent(social.GuildBadName, name))
		return
	}
	emperium := -1
	if z.guildRules().EmperiumCheck {
		for i := range p.inventory.Slots {
			if s := &p.inventory.Slots[i]; s.ID == itemEmperium && s.Amount > 0 {
				emperium = i
				break
			}
		}
		if emperium < 0 {
			p.pending = append(p.pending, guildResultEvent(social.GuildNoEmperium, name))
			return
		}
	}
	if id, _ := z.socialHub().GuildCreate(p.soc, name); id != 0 && emperium >= 0 {
		z.deleteItem(p, emperium, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_GUILD)
	}
}

// storageLost logs the items of a guild storage that went with its
// guild, as ActionGuild lines, and saves the empty storage.
func (z *Zone) storageLost(p *Player, guild int64, left []social.StorageSlot) {
	if guild == 0 || len(left) == 0 {
		return
	}
	now := z.wallClock()
	var logs transfer.Logs
	for _, s := range left {
		logs.Items = append(logs.Items, transfer.ItemLog{At: now, Action: transfer.ActionGuild,
			From: transfer.Owner{Kind: transfer.KindGuildStorage, ID: guild}, To: transfer.End(transfer.KindUsed),
			Serial: s.Serial, ItemID: s.ID, Amount: s.Amount, Refine: s.Refine, Cards: s.Cards, Extra: s.Extra})
	}
	z.txLogs.Add(logs)
	z.txStorages = append(z.txStorages, StorageCommit{Guild: guild, Base: left})
	if p != nil && !p.transferred {
		p.transferred = true
		z.txPlayers = append(z.txPlayers, p)
	}
}

// guildChanged queues the guild_change event of a change this zone made.
func (z *Zone) guildChanged(ch *social.GuildChange) {
	z.scriptEvents = append(z.scriptEvents, scriptEvent{guild: &modscript.GuildEvent{Kind: "guild_change", Guild: ch.Guild, Change: ch.What}})
}

// receiveGuild shows a guild notice from the registry to its player. It
// returns false for a notice that is not a guild's.
func (z *Zone) receiveGuild(p *Player, n social.Notice) bool {
	switch n.Kind {
	case social.NoticeGuild, social.NoticeGuildExp:
		p.guild.view = n.Guild
		p.pending = append(p.pending, guildInfo(n.Guild, p.Name))
		if n.Kind == social.NoticeGuild {
			z.updateBadge(p)
		}
	case social.NoticeGuildMember:
		if v := p.guild.view; v != nil {
			for i := range v.Members {
				if chat.Key(v.Members[i].Name) == chat.Key(n.GuildMember.Name) {
					v.Members[i] = n.GuildMember
				}
			}
		}
		p.pending = append(p.pending, wathenav1.Event_builder{GuildMember: wathenav1.GuildMemberUpdate_builder{
			Member: guildMember(n.GuildMember)}.Build()}.Build())
	case social.NoticeGuildInvite:
		p.pending = append(p.pending, wathenav1.Event_builder{GuildInvite: wathenav1.GuildInvite_builder{
			GuildId: uint64(n.Invite.Party), GuildName: n.Invite.PartyName, From: n.Invite.From, // #nosec G115 -- guild IDs are positive.
		}.Build()}.Build())
	case social.NoticeGuildAllyRequest:
		p.pending = append(p.pending, wathenav1.Event_builder{GuildAllyRequest: wathenav1.GuildAllyRequest_builder{
			GuildId: uint64(n.Invite.Party), GuildName: n.Invite.PartyName, From: n.Invite.From, // #nosec G115 -- guild IDs are positive.
		}.Build()}.Build())
	case social.NoticeGuildResult:
		p.pending = append(p.pending, guildResultEvent(n.GuildOutcome, n.Name))
	case social.NoticeGuildChat:
		if p.blocking(n.Line.Name) {
			return true
		}
		p.pending = append(p.pending, wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
			Channel: wathenav1.ChatChannel_CHAT_CHANNEL_GUILD, FromName: n.Line.Name, Text: n.Line.Text, LineId: n.Line.ID[:],
		}.Build()}.Build())
		p.see(n.Line)
	case social.NoticeGuildEvent:
		ev := &modscript.GuildEvent{Kind: "guild_join", Guild: n.Event.Party, Player: int64(p.charID), Reason: n.Event.Reason}
		if n.Event.Kind == social.EventLeave {
			ev.Kind = "guild_leave"
			// guild_member_withdraw ends the guild auras of a member who
			// left. They do not exist yet.
			z.guildBoundLeft(p, n.Event.Party, n.Event.Reason)
		}
		z.scriptEvents = append(z.scriptEvents, scriptEvent{guild: ev})
	case social.NoticeGuildChange:
		z.guildChanged(&n.Change)
	case social.NoticeGuildBadge:
		z.updateBadge(p)
	case social.NoticeGuildSkillBlock:
		z.blockGuildSkills(p, int64(z.guildRules().SkillRelogDelay), true)
	case social.NoticeEmergencyCall:
		z.emergencyCalled(p, n.Call)
	case social.NoticeStorageClose:
		if p.guild.storage != nil {
			z.closeGuildStorage(p, false)
		}
	default:
		return false
	}
	return true
}

// updateBadge works out what others see of a player's guild, and marks
// the player so the players in sight see a change.
func (z *Zone) updateBadge(p *Player) {
	var b *wathenav1.GuildBadge
	if v := p.guild.view; v != nil {
		if m, ok := v.Member(p.Name); ok {
			pos := ""
			if m.Position >= 0 && m.Position < len(v.Positions) {
				pos = v.Positions[m.Position].Name
			}
			b = wathenav1.GuildBadge_builder{GuildId: uint64(v.ID), Name: v.Name, // #nosec G115 -- guild IDs are positive.
				EmblemVersion: uint32(max(v.Emblem, 0)), Position: pos}.Build() // #nosec G115 -- a version count.
		}
	}
	old := p.guild.badge
	if sameBadge(old, b) {
		return
	}
	p.guild.badge = b
	if p.entity != nil {
		p.entity.guildChanged = true
	}
}

func sameBadge(a, b *wathenav1.GuildBadge) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.GetGuildId() == b.GetGuildId() && a.GetName() == b.GetName() && a.GetEmblemVersion() == b.GetEmblemVersion() &&
		a.GetPosition() == b.GetPosition()
}

// entityGuild is the event that tells a viewer an entity's guild badge
// changed.
func entityGuild(id uint32, p *Player) *wathenav1.Event {
	return wathenav1.Event_builder{EntityGuild: wathenav1.EntityGuild_builder{Id: id, Guild: p.guild.badge}.Build()}.Build()
}

func guildMember(m social.GuildMemberView) *wathenav1.GuildMember {
	return wathenav1.GuildMember_builder{
		Name: m.Name, Position: uint32(max(m.Position, 0)), Online: m.Online, Map: m.Map, // #nosec G115 -- below 40.
		Job: uint32(max(m.Job, 0)), BaseLevel: uint32(max(m.Level, 0)), Exp: m.Exp, // #nosec G115 -- small values.
		X: uint32(max(m.X, 0)), Y: uint32(max(m.Y, 0)), Master: m.Master, // #nosec G115 -- map cells.
	}.Build()
}

func guildPosition(i int, p social.Position) *wathenav1.GuildPosition {
	return wathenav1.GuildPosition_builder{Index: uint32(i), Name: p.Name, CanInvite: p.Invite, CanExpel: p.Expel, // #nosec G115 -- below 40.
		CanUseStorage: p.Storage, ExpTax: uint32(max(p.ExpTax, 0))}.Build() // #nosec G115 -- at most 99.
}

// guildInfo is the GuildInfo event of a guild for one of its members.
func guildInfo(v *social.GuildView, me string) *wathenav1.Event {
	b := wathenav1.GuildInfo_builder{}
	if v != nil {
		b.Id, b.Name, b.Master = uint64(v.ID), v.Name, v.Master                                   // #nosec G115 -- guild IDs are positive.
		b.Level, b.Exp, b.NextExp = uint32(max(v.Level, 0)), v.Exp, v.NextExp                     // #nosec G115 -- 1 to 50.
		b.SkillPoints, b.MaxMembers = uint32(max(v.SkillPoints, 0)), uint32(max(v.MaxMembers, 0)) // #nosec G115 -- small counts.
		b.Notice = wathenav1.GuildNotice_builder{Title: v.Notice.Title, Text: v.Notice.Text}.Build()
		b.EmblemVersion, b.AverageLevel = uint32(max(v.Emblem, 0)), uint32(max(v.AverageLevel, 0)) // #nosec G115 -- small values.
		for _, m := range v.Members {
			b.Members = append(b.Members, guildMember(m))
			if chat.Key(m.Name) == chat.Key(me) {
				b.Position = uint32(max(m.Position, 0)) // #nosec G115 -- below 40.
			}
		}
		for i, p := range v.Positions {
			b.Positions = append(b.Positions, guildPosition(i, p))
		}
		for _, r := range v.Relations {
			b.Relations = append(b.Relations, wathenav1.GuildRelation_builder{GuildId: uint64(r.Guild), Name: r.Name, Enemy: r.Enemy}.Build()) // #nosec G115 -- guild IDs are positive.
		}
		for _, s := range v.Skills {
			b.Skills = append(b.Skills, wathenav1.GuildSkill_builder{SkillId: uint32(max(s.ID, 0)), Level: uint32(max(s.Level, 0)), // #nosec G115 -- skill IDs and levels.
				MaxLevel: uint32(max(s.Max, 0)), CanRaise: s.CanRaise}.Build()) // #nosec G115 -- small levels.
		}
	}
	return wathenav1.Event_builder{GuildInfo: b.Build()}.Build()
}

// guildID is the ID of the player's guild as the zone knows it, or 0.
func (p *Player) guildID() int64 {
	if v := p.guild.view; v != nil {
		return v.ID
	}
	return 0
}

// isGuildMaster reports whether the player is its guild's master, as
// gmaster_flag.
func (p *Player) isGuildMaster() bool {
	v := p.guild.view
	return v != nil && chat.Key(v.Master) == chat.Key(p.Name)
}

// guildSkillLevel is pc_checkskill for a guild skill: the guild's level
// of it, for its master only, as clif_parse_UseSkillToId.
func (p *Player) guildSkillLevel(id int) int {
	v := p.guild.view
	if v == nil || !p.isGuildMaster() {
		return 0
	}
	for _, s := range v.Skills {
		if s.ID == id {
			return s.Level
		}
	}
	return 0
}

// guildOf is a unit's guild, as status_get_guild_id: a player's guild,
// or the guild of a monster's master.
func guildOf(e *entity) int64 {
	e = battleMaster(e)
	if e == nil || e.player == nil {
		return 0
	}
	return e.player.guildID()
}

// guildAllied reports whether a unit's guild has another as an ally, as
// guild_isallied, from the unit's own guild view.
func guildAllied(e *entity, other int64) bool {
	e = battleMaster(e)
	if e == nil || e.player == nil || e.player.guild.view == nil {
		return false
	}
	for _, r := range e.player.guild.view.Relations {
		if r.Guild == other && !r.Enemy {
			return true
		}
	}
	return false
}

// sameGuildOrAlly is battle_check_target's guild test: the same guild, or
// an allied one unless same is set (BCT_SAMEGUILD).
func sameGuildOrAlly(src, t *entity, same bool) bool {
	sg, tg := guildOf(src), guildOf(t)
	return sg != 0 && tg != 0 && (sg == tg || !same && guildAllied(src, tg))
}

// guildTax returns the player's guild and the exp tax of its position.
func (z *Zone) guildTax(p *Player) (int64, int) {
	if p.soc == nil || z.Social == nil {
		return 0, 0
	}
	return z.Social.GuildTax(p.soc)
}

// payGuildTax is guild_payexp in pc_gainexp: the player's position's tax
// of base exp goes to its guild. It returns the base exp the player
// keeps.
func (z *Zone) payGuildTax(p *Player, base int64) int64 {
	if base <= 0 {
		return base
	}
	guild, tax := z.guildTax(p)
	if guild == 0 || tax < 1 {
		return base
	}
	paid := social.TaxOf(base, tax)
	if paid <= 0 {
		return base
	}
	if ch := z.Social.PayExp(p.soc, paid); ch != nil {
		z.guildChanged(ch)
	}
	return base - paid
}

// guildUpdates sends the members on this map each other's cells every
// 5 seconds, as guild_send_xy_timer.
func (z *Zone) guildUpdates() {
	if z.Social == nil {
		return
	}
	for _, p := range z.players {
		id := p.guildID()
		if id == 0 || p.gone() {
			continue
		}
		st := &p.guild
		e := p.entity
		if z.now < st.nextXY || e.x == st.sentX && e.y == st.sentY {
			continue
		}
		st.sentX, st.sentY, st.nextXY = e.x, e.y, z.now+guildXYInterval
		m, ok := p.guild.view.Member(p.Name)
		if !ok {
			continue
		}
		m.Online, m.Map, m.X, m.Y, m.Level, m.Job = true, z.Map, e.x, e.y, p.progress.BaseLevel, p.char.Job
		ev := wathenav1.Event_builder{GuildMember: wathenav1.GuildMemberUpdate_builder{Member: guildMember(m)}.Build()}.Build()
		for _, q := range z.sortedPlayers() {
			if q != p && !q.gone() && q.guildID() == id {
				q.pending = append(q.pending, ev)
			}
		}
	}
}

// emergencyCalled warps a member to its master, as GD_EMERGENCYCALL's
// pc_setpos with CLR_RESPAWN. A dead member stays.
func (z *Zone) emergencyCalled(p *Player, c social.Call) {
	if p.entity.dead || p.gone() {
		return
	}
	z.warp(p, &Warp{To: c.Map, ToX: c.X, ToY: c.Y})
}

// blockGuildSkills is guild_block_skill: the master's Battle Orders,
// Regeneration, Restore and Emergency Call wait. Only pre-renewal blocks
// a master who logs in or becomes master (relog is set), as rAthena's
// #ifndef RENEWAL; the skill_cooldown flag picks the mode.
func (z *Zone) blockGuildSkills(p *Player, ms int64, relog bool) {
	if ms <= 0 || relog && z.worldRules().SkillCooldown {
		return
	}
	if p.cooldowns == nil {
		p.cooldowns = map[int]int64{}
	}
	for _, id := range []int{social.SkillBattleOrder, social.SkillRegeneration, social.SkillRestore, social.SkillEmergencyCall} {
		p.cooldowns[id] = max(p.cooldowns[id], z.now+ms)
		p.pending = append(p.pending, cooldownEvent(id, ms))
	}
}

// Guild storage.

func storageResult(o wathenav1.StorageOutcome) *wathenav1.Event {
	return wathenav1.Event_builder{StorageResult: wathenav1.StorageResult_builder{Outcome: o}.Build()}.Build()
}

// openGuildStorage opens a player's guild storage, as
// storage_guild_storageopen. It returns the outcome.
func (z *Zone) openGuildStorage(p *Player) social.StorageOutcome {
	if p.soc == nil {
		return social.StorageNoGuild
	}
	if p.guild.storage != nil || p.storage != nil {
		p.pending = append(p.pending, storageResult(wathenav1.StorageOutcome_STORAGE_OUTCOME_ALREADY_OPEN))
		return social.StorageAlreadyOpen
	}
	st, size, o := z.socialHub().OpenGuildStorage(p.soc)
	if o != social.StorageOpened {
		p.pending = append(p.pending, storageResult(wathenav1.StorageOutcome(o))) // #nosec G115 -- the outcomes match the protocol's.
		return o
	}
	p.guild.storage = st
	st.Lock()
	box := st.Box()
	box.Items = z.Items
	items := z.storageItems(box)
	st.Unlock()
	z.sendStorageList(p, wathenav1.StorageOpen_builder{Kind: wathenav1.StorageKind_STORAGE_KIND_GUILD, Size: uint32(size), // #nosec G115 -- at most the ceiling.
		DisplayName: "Guild storage"}.Build(), items)
	return o
}

// closeGuildStorage closes the player's guild storage window, as
// storage_guild_storageclose. tell sends nothing when the registry
// already closed it.
func (z *Zone) closeGuildStorage(p *Player, release bool) {
	st := p.guild.storage
	if st == nil {
		return
	}
	p.guild.storage = nil
	if release && p.soc != nil {
		z.socialHub().CloseGuildStorage(p.soc, st)
	}
	p.pending = append(p.pending, wathenav1.Event_builder{StorageClose: wathenav1.StorageClose_builder{
		Kind: wathenav1.StorageKind_STORAGE_KIND_GUILD}.Build()}.Build())
}

// storageCommand runs a storage command. Moves go to the open personal
// storage, else to the guild storage (storage.go).
func (z *Zone) storageCommand(p *Player, c *storageCommand) {
	switch {
	case c.close:
		if p.storage != nil {
			z.closeStorage(p)
			return
		}
		z.closeGuildStorage(p, true)
	case c.log:
		if p.soc == nil {
			return
		}
		entries, o := z.socialHub().StorageLog(p.soc)
		if o != social.StorageOpened {
			p.pending = append(p.pending, storageResult(wathenav1.StorageOutcome(o))) // #nosec G115 -- the outcomes match the protocol's.
			return
		}
		b := wathenav1.GuildStorageLog_builder{}
		for _, e := range entries {
			b.Entries = append(b.Entries, wathenav1.GuildStorageLogEntry_builder{AtMs: e.At.UnixMilli(), Name: e.Name, ItemId: e.ItemID,
				Amount: int32(e.Amount), Refine: uint32(max(e.Refine, 0)), Cards: e.Cards}.Build()) // #nosec G115 -- a stack and a refine level.
		}
		p.pending = append(p.pending, wathenav1.Event_builder{GuildStorageLog: b.Build()}.Build())
	case c.move != nil:
		z.storageMove(p, c.move)
		z.flushStorageLog()
	}
}

// storageLogBatch is guild storage log lines that wait for the storage's
// lock to be let go.
type storageLogBatch struct {
	guild   int64
	entries []social.StorageLogEntry
}

// flushStorageLog hands waiting guild storage log lines to the registry.
// No storage lock may be held.
func (z *Zone) flushStorageLog() {
	for _, b := range z.storageLog {
		z.socialHub().LogStorage(b.guild, b.entries)
	}
	z.storageLog = z.storageLog[:0]
}

// storageChanged puts a guild storage in the next commit.
func (z *Zone) storageChanged(st *social.GuildStorage) {
	for _, o := range z.txStorageRefs {
		if o == st {
			return
		}
	}
	z.txStorageRefs = append(z.txStorageRefs, st)
}

// StorageCommit is a storage a commit saves: what the last commit of it
// wrote, and what it holds now. Guild names a guild storage. Key names
// an account's storage, an extra storage or a cart (storage.go).
type StorageCommit struct {
	Guild     int64
	Key       storage.Key
	Base, Cur []social.StorageSlot
}

// storageCommits are the guild storages the transfers since the last
// commit changed.
func (z *Zone) storageCommits() []StorageCommit {
	out := append(z.txStorages, z.personalCommits()...)
	z.txStorages = nil
	for _, st := range z.txStorageRefs {
		st.Lock()
		base, cur := st.Commit()
		st.Unlock()
		out = append(out, StorageCommit{Guild: st.Guild(), Base: base, Cur: cur})
	}
	z.txStorageRefs = nil
	return out
}

// OpenGuildStorageFor opens the guild storage of a player of this zone by
// name, as an NPC would, for waserver --dev's debug endpoint. It returns
// false when no such player is here.
func (z *Zone) OpenGuildStorageFor(ctx context.Context, name string) (social.StorageOutcome, bool, error) {
	type result struct {
		o     social.StorageOutcome
		found bool
	}
	done := make(chan result, 1)
	f := func() {
		for _, p := range z.players {
			if chat.Key(p.Name) == chat.Key(name) && !p.gone() {
				done <- result{z.openGuildStorage(p), true}
				return
			}
		}
		done <- result{}
	}
	select {
	case z.control <- f:
	case <-ctx.Done():
		return 0, false, ctx.Err()
	case <-z.stopped:
		return 0, false, ErrStopped
	}
	select {
	case r := <-done:
		return r.o, r.found, nil
	case <-ctx.Done():
		return 0, false, ctx.Err()
	}
}
