package zone

import (
	"errors"
	"slices"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The player calls of review decision 58, area B7 (decision 0064):
//   - give_exp is getexp (script.cpp 11142) and getexp2 (24713): getexp
//     multiplies by quest_exp_rate and pays the guild's tax, as
//     pc_gainexp with flag 1; getexp2 gives raw exp without the tax
//     (flag 2), and a negative amount takes exp (pc_lostexp).
//   - heal is status_heal with flag 1, and set_hp is pc_setparam's
//     SP_HP.
//   - set_blocked keeps rAthena's PCBLOCK_ flags (block_action). Each
//     check sits where rAthena's does.
//   - A conversation's items are locked by default in renewal and
//     unlocked in pre-renewal, as item_enabled_npc (battle.cpp 8685).
//   - transform is SC_MONSTER_TRANSFORM: the player looks like the
//     monster until the status ends.

// PCBLOCK_ flags of rAthena (script.hpp 2221).
const (
	blockMove     = 0x1
	blockAttack   = 0x2
	blockSkill    = 0x4
	blockUseItem  = 0x8
	blockChat     = 0x10
	blockImmune   = 0x20
	blockSitStand = 0x40
	blockCommands = 0x80
	blockNPCClick = 0x100
	blockEmotion  = 0x200
	blockEquip    = 0x400
)

// isBlocked reports whether a player has a PCBLOCK_ flag.
func (p *Player) isBlocked(flag uint32) bool { return p.blocked&flag != 0 }

// The statuses of player:transform, by rAthena's IDs.
const (
	scMonsterTransform       = 566
	scActiveMonsterTransform = 638
	// monsterEmperium is MOBID_EMPERIUM, which montransform refuses.
	monsterEmperium = 1288
)

// GainExp is player:give_exp.
func (h scriptHost) GainExp(player, base, job int64, raw bool) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	z := h.z
	if z.Progress == nil {
		return nil
	}
	if !raw {
		// buildin_getexp: each amount times quest_exp_rate/100, as a
		// double, capped to 0..MAX_EXP.
		rate := float64(z.Progress.QuestExpRate) / 100
		scale := func(v int64) int64 {
			f := float64(v) * rate
			if f >= float64(modscript.MaxExpAmount) {
				return modscript.MaxExpAmount
			}
			return max(int64(f), 0)
		}
		base, job = scale(base), scale(job)
		if !p.entity.dead {
			z.gainExpFlags(p, base, job, true)
		}
		return nil
	}
	// buildin_getexp2: a positive amount alone through pc_gainexp with
	// flag 2, and a negative one through pc_lostexp, which takes exp
	// from the dead too.
	if base > 0 && !p.entity.dead {
		z.gainExpFlags(p, base, 0, false)
	}
	if job > 0 && !p.entity.dead {
		z.gainExpFlags(p, 0, job, false)
	}
	if base < 0 {
		z.loseBaseExp(p, -base)
	}
	if job < 0 {
		z.loseJobExp(p, -job)
	}
	return nil
}

// loseJobExp is the job part of pc_lostexp.
func (z *Zone) loseJobExp(p *Player, exp int64) {
	exp = min(exp, p.progress.JobExp)
	if exp <= 0 {
		return
	}
	before := z.progressStats(p, nil)
	p.progress.JobExp -= exp
	if changed := z.progressStats(p, before); changed != nil {
		p.pending = append(p.pending, changed)
	}
}

// HealFixed is heal: status_heal with flag 1 (status.cpp 1803). A dead
// player gets nothing. A negative amount is passive damage, which may
// kill. A positive one heals up to the maximum, whatever Frenzy says.
func (h scriptHost) HealFixed(player, hp, sp int64) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	z, e := h.z, p.entity
	if e.dead || e.hp <= 0 || e.status == nil {
		return nil
	}
	if hp < 0 || sp < 0 {
		z.zap(e, max(-hp, 0), max(-sp, 0))
	}
	if e.dead {
		return nil
	}
	if hp > 0 {
		scEnv{z}.ForceHeal(z.unit(e), hp)
	}
	if sp > 0 {
		p.sp = min(p.sp+sp, int64(e.status.MaxSP))
		if s := z.battleStats(p); s != nil {
			p.pending = append(p.pending, s)
		}
	}
	return nil
}

// SetHP is Hp = n: pc_setparam's SP_HP (pc.cpp 10503) caps the value to
// 1 and the maximum, so it never kills. rAthena also sets the HP of a
// dead player without reviving it; wAthena leaves the dead alone.
func (h scriptHost) SetHP(player, hp int64) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	scEnv(h).SetHP(h.z.unit(p.entity), hp)
	return nil
}

// SetBlocked is setpcblock (script.cpp 18964).
func (h scriptHost) SetBlocked(player int64, mask uint32, blocked bool) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	if blocked {
		p.blocked |= mask
	} else {
		p.blocked &^= mask
	}
	if p.blocked&blockMove != 0 {
		// unit_can_move refuses the walk's next step too.
		p.entity.path = p.entity.path[:0]
		p.entity.stopped = true
	}
	return nil
}

// Blocked is getpcblock.
func (h scriptHost) Blocked(player int64) (uint32, error) {
	p, err := h.player(player)
	if err != nil {
		return 0, err
	}
	return p.blocked, nil
}

// itemsAllowed reports whether a player who talks to an NPC may use and
// equip items: enable_items and disable_items, or item_enabled_npc.
func (z *Zone) itemsAllowed(t *talk) bool {
	switch t.items {
	case itemsUnlocked:
		return true
	case itemsLocked:
		return false
	}
	return !z.renewal()
}

// The item states of a conversation (talk.items).
const (
	itemsDefault = iota
	itemsLocked
	itemsUnlocked
)

// SetItemsLocked is disable_items and enable_items.
func (h scriptHost) SetItemsLocked(conv *modscript.Conversation, locked bool) error {
	p, err := h.player(conv.Player)
	if err != nil {
		return err
	}
	if p.talk == nil || p.talk.conv != conv {
		return nil
	}
	p.talk.items = itemsUnlocked
	if locked {
		p.talk.items = itemsLocked
	}
	return nil
}

// UnequipAll is nude (script.cpp 15715): pc_unequipitem with flag 2, so
// statuses that keep equipment on do not stop it.
func (h scriptHost) UnequipAll(player int64) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	for _, i := range wornSlots(p) {
		h.z.unequipItem(p, i)
	}
	return nil
}

// Push is pushpc (script.cpp 22604): unit_blown in the direction, with
// no immunity check. A negative count pushes the other way.
func (h scriptHost) Push(player int64, direction, cells int) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	if cells == 0 {
		return nil
	}
	if cells < 0 {
		direction, cells = (direction+4)%8, -cells
	}
	z, e := h.z, p.entity
	// skillBlown pushes away from dir: the push's own direction is the
	// opposite of that.
	if z.skillBlown(e, e, cells, (direction+4)%8, true) > 0 {
		// unit_blown touches the NPC areas of the cell it ends on.
		z.touchAreas(p)
	}
	return nil
}

// Transform is transform (script.cpp 23777): SC_MONSTER_TRANSFORM with
// the monster in val1. A monster that does not exist, or the Emperium,
// is refused. 0 milliseconds does nothing.
func (h scriptHost) Transform(player, monster, ms int64) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	z := h.z
	if monster == monsterEmperium || !z.isMonsterView(uint32(monster)) { // #nosec G115 -- checked by the runtime.
		return false, errors.New("the monster does not exist, or is the Emperium")
	}
	if ms == 0 {
		return true, nil
	}
	en := z.statusEngine()
	if en == nil || z.Statuses == nil || z.Statuses.Get(scMonsterTransform) == nil {
		return false, errors.New("the statuses are not loaded")
	}
	u := z.unit(p.entity)
	en.End(u, scMonsterTransform, sc.ReasonReplaced)
	ok := en.Start(u, u, sc.Start{Type: scMonsterTransform, Rate: 10000, Val1: int(monster), Duration: ms, Reason: sc.ReasonScript,
		Flag: sc.NoAvoid | sc.NoTickDef | sc.NoRateDef})
	if ok {
		p.transform = uint32(monster) // #nosec G115 -- checked by the runtime.
		z.updateLook(p)
	}
	return ok, nil
}

// transformEnded takes a player's monster look off when its transform
// status ends.
func (z *Zone) transformEnded(e *entity, typ int) {
	if p := e.player; p != nil && (typ == scMonsterTransform || typ == scActiveMonsterTransform) && p.transform != 0 {
		p.transform = 0
		z.updateLook(p)
	}
}

// OpenBook is readbook: clif_readbook to the player.
func (h scriptHost) OpenBook(player, book, page int64) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	p.pending = append(p.pending, wathenav1.Event_builder{BookOpen: wathenav1.BookOpen_builder{
		Book: uint32(book), Page: uint32(page), // #nosec G115 -- checked by the runtime.
	}.Build()}.Build())
	return nil
}

// SavePoint is getsavepoint: the saved map and cell, or the world's
// save map and its start cell for a character without one.
func (h scriptHost) SavePoint(player int64) (string, int, int, error) {
	p, err := h.player(player)
	if err != nil {
		return "", 0, 0, err
	}
	if m := p.record.SaveMap; m != "" {
		return strings.TrimPrefix(m, "map/"), p.record.SaveX, p.record.SaveY, nil
	}
	z := h.z
	if to := z.zoneOf(z.SaveMap); to != nil {
		x, y := to.StartCell()
		return strings.TrimPrefix(to.Map, "map/"), x, y, nil
	}
	return strings.TrimPrefix(z.SaveMap, "map/"), 0, 0, nil
}

// Inventory is getinventorylist (script.cpp 15029).
func (h scriptHost) Inventory(player int64) ([]modscript.InventoryEntry, error) {
	p, err := h.player(player)
	if err != nil {
		return nil, err
	}
	z := h.z
	var out []modscript.InventoryEntry
	for i := range p.inventory.Slots {
		s := &p.inventory.Slots[i]
		if s.ID == 0 || s.Amount <= 0 {
			continue
		}
		e := modscript.InventoryEntry{Slot: int64(i), Item: int64(s.ID), Amount: int64(s.Amount), Equip: int64(s.Equip),
			Refine: int64(s.Refine), Identified: !s.Unidentified, Broken: s.Broken, Expires: s.Expires, Bound: s.Bound.String()}
		for _, c := range s.Cards {
			e.Cards = append(e.Cards, int64(c))
		}
		for _, o := range s.Options {
			e.Options = append(e.Options, modscript.RandomOptionEntry{Name: o.Name, Value: int64(o.Value), Param: int64(o.Param)})
		}
		// pc_can_trade_item: not a rental, not bound, and no no_trade.
		e.Tradable = s.Expires == 0 && s.Bound == item.BoundNone && (z.Items == nil || !z.Items.Restricted(s, item.NoTrade))
		out = append(out, e)
	}
	return out, nil
}

// lookSetting reads a look limit of rules.get's settings.
func (z *Zone) lookSetting(name string) int {
	v, _ := scriptHost{z}.Setting(name)
	return v
}

// SetPlayerLook is setlook: pc_changelook (pc.cpp 11127). Hair, hair
// color and cloth color are capped to the look limits and saved. The
// headgear views hold until the player leaves the map, as setlook_head_*
// do, over the equipment's.
func (h scriptHost) SetPlayerLook(player int64, look modscript.PlayerLook) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	z := h.z
	capTo := func(v *int64, hi string) (int, bool) {
		if v == nil {
			return 0, false
		}
		return int(min(*v, int64(z.lookSetting(hi)))), true
	}
	if v, ok := capTo(look.HairStyle, "max_hair_style"); ok {
		p.record.Hair = v
		p.Look.Hair = uint32(v) // #nosec G115 -- capped above, 0 or more.
	}
	if v, ok := capTo(look.HairColor, "max_hair_color"); ok {
		p.record.HairColor = v
		p.Look.HairColor = uint32(v) // #nosec G115 -- capped above.
	}
	if v, ok := capTo(look.ClothColor, "max_cloth_color"); ok {
		p.record.ClothColor = v
		p.Look.ClothColor = uint32(v) // #nosec G115 -- capped above.
	}
	set := func(dst *uint32, v *int64) {
		if v != nil {
			*dst = uint32(min(*v, 1<<31-1)) // #nosec G115 -- 0 or more, checked by the runtime.
		}
	}
	set(&p.setLook[0], look.HeadTop)
	set(&p.setLook[1], look.HeadMiddle)
	set(&p.setLook[2], look.HeadBottom)
	z.updateLook(p)
	// clif_changelook sends the look even when it did not change.
	p.entity.lookChanged = true
	return nil
}

// HasOption is checkoption: the player's sc.option, which holds its
// statuses' options and its saved ones, and @hide's OPTION_INVISIBLE.
func (h scriptHost) HasOption(player int64, mask int) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	opt := p.record.Options
	if l := p.entity.sc; l != nil {
		opt |= int(l.Option)
	}
	if p.gmHidden {
		opt |= companion.OptionInvisible
	}
	return opt&mask != 0, nil
}

// Setting is a server setting of rules.get (review decision 58): the
// exp rates of the progression tables, and the look limits, rAthena's.
func (h scriptHost) Setting(name string) (int, bool) {
	if pr := h.z.Progress; pr != nil {
		switch name {
		case "base_exp_rate":
			return pr.BaseExpRate, true
		case "job_exp_rate":
			return pr.JobExpRate, true
		case "quest_exp_rate":
			return pr.QuestExpRate, true
		}
	}
	v, ok := modscript.DefaultSettings[name]
	return v, ok
}

// grantedSkills are the player's granted skills that it still has, in
// ID order, for the save.
func grantedSkills(p *Player) []int {
	var out []int
	for _, id := range p.granted {
		if p.char.Skills[id] > 0 && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// staffLevel is getgmlevel's pc_get_group_level for player.staff_level:
// 99 for the owner and admin roles, as rAthena's Admin group, 10 for
// other staff roles, as its Law Enforcement group, and 0 for players.
func (z *Zone) staffLevel(p *Player) int {
	switch role := z.staffRole(p); role {
	case "":
		return 0
	case "owner", "admin":
		return 99
	default:
		return 10
	}
}

var (
	_ modscript.PlayerActionHost = scriptHost{}
	_ modscript.SettingsHost     = scriptHost{}
)

// duringTalk is what a player who talks to an NPC may still do: use and
// equip items when the conversation's items are unlocked, and then only
// skills on itself (pc_useitem pc.cpp 6470, clif.cpp 12141, 12189 and
// 12922). Anything else refused tells the player it is busy (MSI_BUSY).
func (z *Zone) duringTalk(p *Player, c command) {
	if p.talk == nil {
		// A walk ended the conversation in the same command.
		return
	}
	allowed := z.itemsAllowed(p.talk)
	if c.use != nil {
		if allowed && !p.isBlocked(blockUseItem) && p.trade == nil {
			z.useItem(p, *c.use)
		} else {
			z.busy(p)
		}
	}
	if allowed && !p.isBlocked(blockEquip) {
		if c.equip != nil {
			z.equipItem(p, c.equip.slot, c.equip.position)
		}
		if c.unequip != nil {
			z.unequipItem(p, *c.unequip)
		}
	}
	if c.useSkill != nil {
		if allowed && !p.isBlocked(blockSkill) && z.selfSkill(c.useSkill.skill) {
			z.useSkill(p, c.useSkill)
		} else {
			z.busy(p)
		}
	}
}

// selfSkill reports whether a skill is INF_SELF_SKILL.
func (z *Zone) selfSkill(id int) bool {
	if z.Skills == nil {
		return false
	}
	d := z.Skills.Get(id)
	return d != nil && d.Target == skill.TargetSelf
}
