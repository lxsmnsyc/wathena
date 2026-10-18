package zone

import (
	"fmt"
	"log/slog"

	"github.com/lxsmnsyc/wathena/internal/script"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/status"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Companions, mounts and carts (proposal 0002, review decision 25). A
// player has at most one companion and one mount, records of
// data/companions.yml and data/mounts.yml. Each record's acts_as picks
// the core rules it follows, and its look the option bits that show it,
// as rAthena's OPTION_FALCON, OPTION_RIDING and the others. The look's
// bits and the cart's are the saved options (migration 00012), and the
// record names are saved beside them (migration 00014).

// companionSet is the zone's companions and mounts.
func (z *Zone) companionSet() *companion.Set {
	if z.Mods != nil && z.Mods.Companions != nil {
		return z.Mods.Companions
	}
	return companion.Core()
}

// warn logs a warning to the mods' log, or the default log.
func (z *Zone) warn(msg string, args ...any) {
	if z.Mods != nil && z.Mods.Log != nil {
		z.Mods.Log.Warn(msg, args...)
		return
	}
	slog.Warn(msg, args...)
}

// logError logs an error to the mods' log, or the default log.
func (z *Zone) logError(msg string, args ...any) {
	if z.Mods != nil && z.Mods.Log != nil {
		z.Mods.Log.Error(msg, args...)
		return
	}
	slog.Error(msg, args...)
}

// mountRule is the rule of the player's mount, or RuleNone.
func (p *Player) mountRule() companion.Rule {
	if p.mount == nil {
		return companion.RuleNone
	}
	return p.mount.Rule
}

// isFalcon is pc_isfalcon: the player's companion acts as a falcon.
func isFalcon(p *Player) bool { return p.companion != nil && p.companion.Rule == companion.RuleFalcon }

// riding is pc_isriding: the player's mount acts as a Peco Peco.
func (p *Player) riding() bool { return p.mountRule() == companion.RuleRiding }

// ridingDragon is pc_isridingdragon.
func (p *Player) ridingDragon() bool { return p.mountRule() == companion.RuleDragon }

// lookOptions are the option bits of the player's companion, mount and
// cart, over the other bits of o.
func (p *Player) lookOptions(o int) int {
	o &^= companion.OptionLooks
	if p.companion != nil {
		o |= p.companion.Option
	}
	if p.mount != nil {
		o |= p.mount.Option
	}
	return o
}

// setCompanion gives the player a companion, or removes it with nil. It
// returns false, and changes nothing, when the player does not meet the
// record's requires, as pc_setfalcon without Falconry Mastery.
func (z *Zone) setCompanion(p *Player, r *companion.Record) bool {
	if r != nil && !r.Meets(p.skillLevel) {
		return false
	}
	if r == p.companion {
		return true
	}
	p.companion = r
	p.record.Companion = recordName(r)
	z.setOptions(p, p.lookOptions(p.record.Options))
	return true
}

// setMount puts the player on a mount, or takes it off with nil. It
// returns false, and changes nothing, when the player does not meet the
// record's requires, as pc_setriding without Riding.
func (z *Zone) setMount(p *Player, r *companion.Record) bool {
	if r != nil && !r.Meets(p.skillLevel) {
		return false
	}
	if r == p.mount {
		return true
	}
	old := p.mountRule()
	p.mount = r
	p.record.Mount = recordName(r)
	z.setOptions(p, p.lookOptions(p.record.Options))
	// pc_setoption starts SC_MADOGEAR for the Madogear and ends it after.
	switch now := p.mountRule(); {
	case now == companion.RuleMadogear && old != companion.RuleMadogear:
		z.scStart(p.entity, p.entity, sc.SCMadogear, 100, 0, sc.InfiniteTick)
	case now != companion.RuleMadogear && old == companion.RuleMadogear:
		z.scEnd(p.entity, sc.SCMadogear)
	}
	return true
}

// recordName is a record's name, or "" for none.
func recordName(r *companion.Record) string {
	if r == nil {
		return ""
	}
	return r.Name
}

// setFalcon is pc_setfalcon: a player with Falconry Mastery gets a
// falcon, or loses its companion.
func (z *Zone) setFalcon(p *Player, on bool) {
	if !on {
		z.setCompanion(p, nil)
		return
	}
	z.setCompanion(p, z.companionSet().Companions.ByName("falcon"))
}

// setRiding is pc_setriding: a player with Riding (KN_RIDING) mounts a
// Peco Peco, and any rider can dismount. It reports whether the state
// changed.
func (z *Zone) setRiding(p *Player, on bool) bool {
	if on == p.riding() {
		return false
	}
	if !on {
		return z.setMount(p, nil)
	}
	r := z.companionSet().Mounts.ByName("peco_peco")
	return r != nil && r.Meets(p.skillLevel) && z.setMount(p, r)
}

// restoreCompanions finds a loaded character's companion and mount: by
// their saved names, or, for a character saved before the names, by the
// look bits of its options. A name whose record is gone, such as one of
// a removed mod, is dropped with a warning.
func (z *Zone) restoreCompanions(p *Player, c *character.Character) {
	set := z.companionSet()
	find := func(t *companion.Table, name, what string) *companion.Record {
		if name == "" {
			return nil
		}
		r := t.ByName(name)
		if r == nil {
			z.warn("character "+what+" dropped on load: the world has no such record", "character", c.Name, what, name)
		}
		return r
	}
	comp, mount := c.Companion, c.Mount
	if comp == "" && c.Options&companion.OptionFalcon != 0 {
		comp = "falcon"
	}
	if mount == "" {
		switch o := c.Options; {
		case o&companion.OptionRiding != 0:
			mount = "peco_peco"
		case o&companion.OptionDragon != 0:
			mount = "dragon"
		case o&companion.OptionWugRider != 0:
			mount = "warg"
		case o&companion.OptionMadogear != 0:
			mount = "madogear"
		}
	}
	p.companion = find(&set.Companions, comp, "companion")
	p.mount = find(&set.Mounts, mount, "mount")
	p.record.Companion, p.record.Mount = recordName(p.companion), recordName(p.mount)
	p.record.Options = p.lookOptions(c.Options)
	// clif_parse_LoadEndAck shows the icons of the looks at login.
	p.pending = append(p.pending, lookIconEvents(0, p.record.Options)...)
}

// dropCompanionsWithoutSkills removes the companion, mount and cart
// whose requires the player no longer meets, as pc_jobchange, or, with
// resetting set, those whose skills the player has, as pc_resetskill
// before it resets the skills. A job change (jobChange) always takes the
// Madogear away, as pc_jobchange does.
func (z *Zone) dropCompanionsWithoutSkills(p *Player, resetting, jobChange bool) {
	drop := func(r *companion.Record) bool {
		if r == nil {
			return false
		}
		if resetting {
			return len(r.Requires) > 0 && r.Meets(p.skillLevel)
		}
		return !r.Meets(p.skillLevel) || (jobChange && r.Rule == companion.RuleMadogear)
	}
	if drop(p.companion) {
		z.setCompanion(p, nil)
	}
	if drop(p.mount) {
		z.setMount(p, nil)
	}
	if p.record.Options&companion.OptionCart != 0 && (p.skillLevel(skillPushCart) > 0) == resetting {
		z.setOptions(p, p.record.Options&^companion.OptionCart)
	}
}

// mountInput is what status.Calc reads of the player's mount and
// companion.
func (p *Player) mountInput(c *status.Character) {
	c.Riding = p.riding()
	c.Dragon = p.ridingDragon()
	c.WugRider = p.mountRule() == companion.RuleWugRider
	c.Madogear = p.mountRule() == companion.RuleMadogear
	c.MountSpeed = nil
	if p.mount != nil && p.mount.HasSpeed {
		s := p.mount.Speed
		c.MountSpeed = &s
	}
	c.CompanionBonuses = nil
	for _, r := range []*companion.Record{p.companion, p.mount} {
		if r == nil {
			continue
		}
		for _, b := range r.Bonuses {
			c.CompanionBonuses = append(c.CompanionBonuses, status.FromBonus(b))
		}
	}
}

// Status icons of the looks, as rAthena's EFST_ values.
const (
	iconRiding   = 27
	iconFalcon   = 28
	iconWugRider = 357
)

// lookIcons are the icons that option bits show, as pc_setoption's
// clif_status_load calls.
var lookIcons = []struct{ bits, icon int }{
	{companion.OptionRiding | companion.OptionDragon, iconRiding},
	{companion.OptionFalcon, iconFalcon},
	{companion.OptionWugRider, iconWugRider},
}

// lookIconEvents are the icon changes between two option values.
func lookIconEvents(old, o int) []*wathenav1.Event {
	var out []*wathenav1.Event
	for _, x := range lookIcons {
		if on := o&x.bits != 0; on != (old&x.bits != 0) {
			out = append(out, wathenav1.Event_builder{StatusIcon: wathenav1.StatusIcon_builder{
				Icon: uint32(x.icon), On: on, // #nosec G115 -- EFST_ IDs are small.
			}.Build()}.Build())
		}
	}
	return out
}

// table is the zone's records of a kind, "companion" or "mount".
func (z *Zone) companionTable(kind string) *companion.Table {
	set := z.companionSet()
	switch kind {
	case "companion":
		return &set.Companions
	case "mount":
		return &set.Mounts
	}
	return nil
}

// CompanionID is companions.id and mounts.id for scripts.
func (h scriptHost) CompanionID(kind, name string) (int, bool) {
	r := h.z.companionTable(kind).ByName(name)
	if r == nil {
		return 0, false
	}
	return r.ID, true
}

// SetCompanion is player:set_companion and player:set_mount for scripts.
func (h scriptHost) SetCompanion(id int64, kind string, rec int) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	t := h.z.companionTable(kind)
	var r *companion.Record
	if rec != 0 {
		if r = t.Get(rec); r == nil {
			return false, fmt.Errorf("player:set_%s: there is no %s %d", kind, kind, rec)
		}
	}
	if kind == "mount" {
		return h.z.setMount(p, r), nil
	}
	return h.z.setCompanion(p, r), nil
}

// SetCart is player:set_cart for scripts.
func (h scriptHost) SetCart(id int64, look int) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	return h.z.setCart(p, look), nil
}

// companionField reads player.companion or player.mount: the record's
// ID, or nil.
func companionField(r *companion.Record) script.Value {
	if r == nil {
		return script.Nil
	}
	return script.Int(int64(r.ID))
}

// cartField reads player.cart: the cart's look, or nil.
func cartField(p *Player) script.Value {
	if l := companion.CartLook(p.record.Options); l > 0 {
		return script.Int(int64(l))
	}
	return script.Nil
}
