package botclient

import (
	"context"
	"maps"
	"slices"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Guilds and guild storage (phase 5 step 5): what a bot knows of them
// from the server's events, and the commands it sends.

// GuildMember is one member of the bot's guild, as the server last told
// the bot.
type GuildMember struct {
	Name string
	// Position is the member's position index.
	Position       int
	Online, Master bool
	Map            string
	Job, BaseLevel int
	// Exp is what the member paid the guild through the exp tax.
	Exp  int64
	X, Y int
}

// Guild is the bot's guild: the last GuildInfo, with the
// GuildMemberUpdate events since merged in.
type Guild struct {
	ID   uint64
	Name string
	// Level is 1 to 50. Exp is the exp toward the next level, and
	// NextExp what the next level needs.
	Level        int
	Exp, NextExp int64
	SkillPoints  int
	MaxMembers   int
	Master       string
	NoticeTitle  string
	NoticeText   string
	// Emblem is the emblem's version, or 0.
	Emblem uint32
	// Position is the bot's own position index.
	Position int
	// Members are in slot order, the master first.
	Members   []GuildMember
	Positions []*wathenav1.GuildPosition
	// Skills are the guild skills' levels, by skill ID.
	Skills    map[uint32]int
	Relations []*wathenav1.GuildRelation
}

// Member returns the member with a name, and false when there is none.
func (g *Guild) Member(name string) (GuildMember, bool) {
	if g == nil {
		return GuildMember{}, false
	}
	for _, m := range g.Members {
		if strings.EqualFold(m.Name, name) {
			return m, true
		}
	}
	return GuildMember{}, false
}

// Skill is the guild's level of a guild skill.
func (g *Guild) Skill(id uint32) int {
	if g == nil {
		return 0
	}
	return g.Skills[id]
}

// Storage is the storage window the bot has open.
type Storage struct {
	Kind wathenav1.StorageKind
	// Size is how many slots the storage has.
	Size int
	// Items are the filled slots, by slot.
	Items map[int]Item
}

// Count is the amount of an item in the storage.
func (s *Storage) Count(id uint32) int {
	if s == nil {
		return 0
	}
	n := 0
	for _, it := range s.Items {
		if it.ID == id {
			n += it.Amount
		}
	}
	return n
}

// guildState is what a bot knows of its guild and its storage window.
// The caller of apply holds the bot's mu.
type guildState struct {
	guild *Guild
	// invite is the last guild invite the bot has not answered.
	invite  *wathenav1.GuildInvite
	storage *Storage
}

func guildMemberOf(m *wathenav1.GuildMember) GuildMember {
	return GuildMember{Name: m.GetName(), Position: int(m.GetPosition()), Online: m.GetOnline(), Master: m.GetMaster(), Map: m.GetMap(),
		Job: int(m.GetJob()), BaseLevel: int(m.GetBaseLevel()), Exp: m.GetExp(), X: int(m.GetX()), Y: int(m.GetY())}
}

func (s *guildState) apply(e *wathenav1.Event) {
	switch {
	case e.HasGuildInfo():
		info := e.GetGuildInfo()
		if info.GetId() == 0 {
			s.guild = nil
			return
		}
		g := &Guild{ID: info.GetId(), Name: info.GetName(), Level: int(info.GetLevel()), Exp: info.GetExp(), NextExp: info.GetNextExp(),
			SkillPoints: int(info.GetSkillPoints()), MaxMembers: int(info.GetMaxMembers()), Master: info.GetMaster(),
			NoticeTitle: info.GetNotice().GetTitle(), NoticeText: info.GetNotice().GetText(), Emblem: info.GetEmblemVersion(),
			Position: int(info.GetPosition()), Positions: info.GetPositions(), Relations: info.GetRelations(), Skills: map[uint32]int{}}
		for _, m := range info.GetMembers() {
			g.Members = append(g.Members, guildMemberOf(m))
		}
		for _, sk := range info.GetSkills() {
			g.Skills[sk.GetSkillId()] = int(sk.GetLevel())
		}
		s.guild = g
		s.invite = nil
	case e.HasGuildMember():
		m := guildMemberOf(e.GetGuildMember().GetMember())
		if s.guild == nil {
			return
		}
		for i := range s.guild.Members {
			if strings.EqualFold(s.guild.Members[i].Name, m.Name) {
				s.guild.Members[i] = m
			}
		}
	case e.HasGuildInvite():
		s.invite = e.GetGuildInvite()
	case e.HasStorageOpen():
		o := e.GetStorageOpen()
		st := &Storage{Kind: o.GetKind(), Size: int(o.GetSize()), Items: map[int]Item{}}
		for _, it := range o.GetItems() {
			st.Items[int(it.GetSlot())] = Item{Slot: int(it.GetSlot()), ID: it.GetItemId(), Amount: int(it.GetAmount())}
		}
		s.storage = st
	case e.HasStorageAdd():
		a := e.GetStorageAdd()
		if s.storage == nil {
			return
		}
		it := a.GetItem()
		slot := int(it.GetSlot())
		// item is the slot after the change.
		s.storage.Items[slot] = Item{Slot: slot, ID: it.GetItemId(), Amount: int(it.GetAmount())}
	case e.HasStorageRemove():
		r := e.GetStorageRemove()
		if s.storage == nil {
			return
		}
		slot := int(r.GetSlot())
		it := s.storage.Items[slot]
		it.Amount -= int(r.GetAmount())
		if it.Amount <= 0 {
			delete(s.storage.Items, slot)
		} else {
			s.storage.Items[slot] = it
		}
	case e.HasStorageClose():
		s.storage = nil
	}
}

// clone copies the state for Game.
func (s *guildState) clone() (*Guild, *wathenav1.GuildInvite, *Storage) {
	var g *Guild
	if s.guild != nil {
		c := *s.guild
		c.Members = slices.Clone(s.guild.Members)
		c.Positions = slices.Clone(s.guild.Positions)
		c.Relations = slices.Clone(s.guild.Relations)
		c.Skills = maps.Clone(s.guild.Skills)
		g = &c
	}
	var st *Storage
	if s.storage != nil {
		c := *s.storage
		c.Items = maps.Clone(s.storage.Items)
		st = &c
	}
	return g, s.invite, st
}

func (b *Bot) guild(ctx context.Context, c wathenav1.GuildCommand_builder) error {
	return b.command(ctx, wathenav1.Command_builder{Guild: c.Build()})
}

// GuildCreate makes a guild with the bot as its master. With the default
// rules it takes an Emperium.
func (b *Bot) GuildCreate(ctx context.Context, name string) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{Create: &name})
}

// GuildInvite invites a player by name to the bot's guild.
func (b *Bot) GuildInvite(ctx context.Context, name string) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{Invite: &name})
}

// GuildReply answers a guild invite. The bot forgets the invite.
func (b *Bot) GuildReply(ctx context.Context, guild uint64, accept bool) error {
	b.mu.Lock()
	if b.guildSt.invite.GetGuildId() == guild {
		b.guildSt.invite = nil
	}
	b.mu.Unlock()
	return b.guild(ctx, wathenav1.GuildCommand_builder{Reply: wathenav1.GuildReply_builder{GuildId: guild, Accept: accept}.Build()})
}

// GuildLeave leaves the bot's guild.
func (b *Bot) GuildLeave(ctx context.Context, reason string) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{Leave: &reason})
}

// GuildExpel removes a member by name.
func (b *Bot) GuildExpel(ctx context.Context, name, reason string) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{Expel: wathenav1.GuildExpel_builder{Name: name, Reason: reason}.Build()})
}

// GuildPositions changes the guild's positions.
func (b *Bot) GuildPositions(ctx context.Context, positions ...*wathenav1.GuildPosition) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{Positions: wathenav1.GuildPositions_builder{Positions: positions}.Build()})
}

// GuildMemberPosition gives a member a position. Position 0 hands the
// guild to the member.
func (b *Bot) GuildMemberPosition(ctx context.Context, name string, position uint32) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{MemberPosition: wathenav1.GuildMemberPosition_builder{Name: name, Position: position}.Build()})
}

// GuildNotice changes the guild notice.
func (b *Bot) GuildNotice(ctx context.Context, title, text string) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{Notice: wathenav1.GuildNotice_builder{Title: title, Text: text}.Build()})
}

// GuildSkillUp raises a guild skill.
func (b *Bot) GuildSkillUp(ctx context.Context, skill uint32) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{SkillUp: &skill})
}

// GuildAlly asks the guild of an online player by name to be allies.
func (b *Bot) GuildAlly(ctx context.Context, name string) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{Ally: &name})
}

// GuildAllyReply answers an alliance request.
func (b *Bot) GuildAllyReply(ctx context.Context, guild uint64, accept bool) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{AllyReply: wathenav1.GuildReply_builder{GuildId: guild, Accept: accept}.Build()})
}

// GuildEnemy makes the guild of an online player by name an enemy.
func (b *Bot) GuildEnemy(ctx context.Context, name string) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{Enemy: &name})
}

// GuildEndRelation ends an alliance, or an enemy relation with enemy
// set.
func (b *Bot) GuildEndRelation(ctx context.Context, guild uint64, enemy bool) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{EndRelation: wathenav1.GuildRelationEnd_builder{GuildId: guild, Enemy: enemy}.Build()})
}

// GuildDisband breaks the guild up. The master names it.
func (b *Bot) GuildDisband(ctx context.Context, name string) error {
	return b.guild(ctx, wathenav1.GuildCommand_builder{Disband: &name})
}

// GuildChat says a line to the bot's guild.
func (b *Bot) GuildChat(ctx context.Context, text string) error {
	return b.command(ctx, wathenav1.Command_builder{Chat: wathenav1.Chat_builder{
		Channel: wathenav1.ChatChannel_CHAT_CHANNEL_GUILD, Text: text,
	}.Build()})
}

func (b *Bot) storage(ctx context.Context, c wathenav1.StorageCommand_builder) error {
	return b.command(ctx, wathenav1.Command_builder{Storage: c.Build()})
}

// StoragePut moves an amount from an inventory slot into the open
// storage.
func (b *Bot) StoragePut(ctx context.Context, slot, amount int) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{Put: wathenav1.StorageMove_builder{
		Slot: uint32(max(slot, 0)), Amount: uint32(max(amount, 0)), // #nosec G115 -- slots and amounts.
	}.Build()})
}

// StorageTake moves an amount from a storage slot into the inventory.
func (b *Bot) StorageTake(ctx context.Context, slot, amount int) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{Take: wathenav1.StorageMove_builder{
		Slot: uint32(max(slot, 0)), Amount: uint32(max(amount, 0)), // #nosec G115 -- slots and amounts.
	}.Build()})
}

// StorageClose closes the storage window.
func (b *Bot) StorageClose(ctx context.Context) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{Close: new(true)})
}

// StorageLog asks for the guild storage log.
func (b *Bot) StorageLog(ctx context.Context) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{Log: new(true)})
}
