package gateway

import (
	"math"
	"strings"
	"unicode/utf8"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// The guild and storage commands (phase 5 step 5). The gateway checks
// their form; the zone and the social registry check the rest. A command
// whose form is wrong is acknowledged and does nothing.

// maxStorageSlot is past the highest storage slot: the engine's ceiling
// of storage_slots and guild_storage. Extra storage records are at most
// the storage_slots ceiling too.
var maxStorageSlot = uint32(max(rules.Ceiling("guild_storage"), rules.Ceiling("storage_slots"))) // #nosec G115 -- 10000.

// maxCartSlot is past the highest cart slot, the engine's ceiling of
// cart_slots.
var maxCartSlot = uint32(rules.Ceiling("cart_slots")) // #nosec G115 -- 1000.

// guildID reads a guild ID from the protocol.
func guildID(v uint64) (int64, bool) {
	if v == 0 || v > math.MaxInt64 {
		return 0, false
	}
	return int64(v), true // #nosec G115 -- checked above.
}

// cleanText checks free text such as a reason or a notice: empty, or a
// chat line of at most n characters.
func cleanText(s string, n int) (string, bool) {
	if s == "" {
		return "", true
	}
	t, ok := CleanChat(s)
	if !ok || utf8.RuneCountInString(t) > n {
		return "", false
	}
	return t, true
}

func guildCommand(p *zone.Session, seq uint32, c *wathenav1.GuildCommand) bool {
	switch {
	case c.HasCreate():
		name := strings.TrimSpace(c.GetCreate())
		if !social.ValidGuildName(name) || strings.ContainsFunc(name, func(r rune) bool { return r < ' ' }) {
			return p.Ack(seq)
		}
		return p.GuildCreate(seq, name)
	case c.HasInvite():
		if !ValidName(c.GetInvite()) {
			return p.Ack(seq)
		}
		return p.GuildInvite(seq, c.GetInvite())
	case c.HasReply():
		id, ok := guildID(c.GetReply().GetGuildId())
		if !ok {
			return p.Ack(seq)
		}
		return p.GuildReply(seq, id, c.GetReply().GetAccept())
	case c.HasLeave():
		reason, ok := cleanText(c.GetLeave(), social.MaxGuildReason)
		if !ok {
			return p.Ack(seq)
		}
		return p.GuildLeave(seq, reason)
	case c.HasExpel():
		e := c.GetExpel()
		reason, ok := cleanText(e.GetReason(), social.MaxGuildReason)
		if !ok || !ValidName(e.GetName()) {
			return p.Ack(seq)
		}
		return p.GuildExpel(seq, e.GetName(), reason)
	case c.HasPositions():
		ps := c.GetPositions().GetPositions()
		b, _ := rules.FindBound(rules.LimitBounds, "guild_positions")
		if len(ps) == 0 || len(ps) > b.Max {
			return p.Ack(seq)
		}
		var changes []social.PositionChange
		for _, gp := range ps {
			name, ok := cleanText(gp.GetName(), social.MaxGuildName)
			if !ok || name == "" || gp.GetIndex() >= uint32(b.Max) || gp.GetExpTax() > 100 { // #nosec G115 -- 40.
				return p.Ack(seq)
			}
			changes = append(changes, social.PositionChange{Index: int(gp.GetIndex()), Position: social.Position{
				Name: name, Invite: gp.GetCanInvite(), Expel: gp.GetCanExpel(), Storage: gp.GetCanUseStorage(), ExpTax: int(gp.GetExpTax())}})
		}
		return p.GuildPositions(seq, changes)
	case c.HasMemberPosition():
		m := c.GetMemberPosition()
		if !ValidName(m.GetName()) || m.GetPosition() >= 40 {
			return p.Ack(seq)
		}
		return p.GuildMemberPosition(seq, m.GetName(), int(m.GetPosition()))
	case c.HasNotice():
		n := c.GetNotice()
		title, ok1 := cleanText(n.GetTitle(), social.MaxNoticeTitle)
		text, ok2 := cleanText(n.GetText(), social.MaxNoticeText)
		if !ok1 || !ok2 {
			return p.Ack(seq)
		}
		return p.GuildNotice(seq, social.GuildNotice{Title: title, Text: text})
	case c.HasSkillUp():
		id := c.GetSkillUp()
		if !social.IsGuildSkill(int(id)) { // #nosec G115 -- a skill ID.
			return p.Ack(seq)
		}
		return p.GuildSkillUp(seq, int(id))
	case c.HasAlly():
		if !ValidName(c.GetAlly()) {
			return p.Ack(seq)
		}
		return p.GuildAlly(seq, c.GetAlly())
	case c.HasAllyReply():
		id, ok := guildID(c.GetAllyReply().GetGuildId())
		if !ok {
			return p.Ack(seq)
		}
		return p.GuildAllyReply(seq, id, c.GetAllyReply().GetAccept())
	case c.HasEnemy():
		if !ValidName(c.GetEnemy()) {
			return p.Ack(seq)
		}
		return p.GuildEnemy(seq, c.GetEnemy())
	case c.HasEndRelation():
		id, ok := guildID(c.GetEndRelation().GetGuildId())
		if !ok {
			return p.Ack(seq)
		}
		return p.GuildEndRelation(seq, id, c.GetEndRelation().GetEnemy())
	case c.HasDisband():
		if !social.ValidGuildName(c.GetDisband()) {
			return p.Ack(seq)
		}
		return p.GuildDisband(seq, c.GetDisband())
	}
	return p.Ack(seq)
}

func storageCommand(p *zone.Session, seq uint32, c *wathenav1.StorageCommand) bool {
	move := func(m *wathenav1.StorageMove, limit uint32) (int, int, bool) {
		if m.GetSlot() >= limit || m.GetAmount() < 1 || m.GetAmount() > item.MaxAmount {
			return 0, 0, false
		}
		return int(m.GetSlot()), int(m.GetAmount()), true
	}
	var (
		kind         zone.StorageMoveKind
		slot, amount int
		ok           bool
	)
	switch {
	case c.HasPut():
		kind = zone.MovePut
		slot, amount, ok = move(c.GetPut(), item.MaxInventory)
	case c.HasTake():
		kind = zone.MoveTake
		slot, amount, ok = move(c.GetTake(), maxStorageSlot)
	case c.HasCartPut():
		kind = zone.MoveCartPut
		slot, amount, ok = move(c.GetCartPut(), item.MaxInventory)
	case c.HasCartTake():
		kind = zone.MoveCartTake
		slot, amount, ok = move(c.GetCartTake(), maxCartSlot)
	case c.HasCartToStorage():
		kind = zone.MoveCartToStorage
		slot, amount, ok = move(c.GetCartToStorage(), maxCartSlot)
	case c.HasStorageToCart():
		kind = zone.MoveStorageToCart
		slot, amount, ok = move(c.GetStorageToCart(), maxStorageSlot)
	case c.HasClose():
		return p.StorageClose(seq)
	case c.HasLog():
		return p.StorageLog(seq)
	}
	if !ok {
		return p.Ack(seq)
	}
	return p.StorageMove(seq, kind, slot, amount)
}

// tradeCommand checks the form of a trade command.
func tradeCommand(p *zone.Session, seq uint32, c *wathenav1.TradeCommand) bool {
	t := zone.TradeAction{}
	switch {
	case c.HasRequest():
		if c.GetRequest() == 0 {
			return p.Ack(seq)
		}
		t.Request = c.GetRequest()
	case c.HasRespond():
		t.Respond, t.Accept = true, c.GetRespond()
	case c.HasAddItem():
		m := c.GetAddItem()
		if m.GetSlot() >= item.MaxInventory || m.GetAmount() < 1 || m.GetAmount() > item.MaxAmount {
			return p.Ack(seq)
		}
		t.AddSlot, t.AddAmount = int(m.GetSlot()), int(m.GetAmount())
	case c.HasAddZeny():
		if int64(c.GetAddZeny()) > item.MaxZeny {
			return p.Ack(seq)
		}
		t.Zeny, t.HasZeny = int64(c.GetAddZeny()), true
	case c.HasLock():
		t.Lock = c.GetLock()
	case c.HasConfirm():
		t.Confirm = c.GetConfirm()
	case c.HasCancel():
		t.Cancel = c.GetCancel()
	default:
		return p.Ack(seq)
	}
	return p.Trade(seq, t)
}
