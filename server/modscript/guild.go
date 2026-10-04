package modscript

import (
	"errors"
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Guilds (proposal 0002, review decision 28): guild handles, wa.guilds
// and the guild events.

// GuildPosition is a member's position as guild:get_position returns it.
type GuildPosition struct {
	Name                          string
	CanInvite, CanExpel, CanStore bool
	ExpTax                        int
}

// GuildHost is a Host with guilds. A Host without it raises an error on
// these calls.
type GuildHost interface {
	// GuildField reads a field of a guild handle: id, name, master,
	// master_name, level, exp, notice or member_count. master is a player
	// ID of the zone, or nil.
	GuildField(guild int64, field string) (script.Value, error)
	// GuildMembers are the player IDs of the online members in the zone,
	// in slot order. GuildMemberNames are every member's name.
	GuildMembers(guild int64) ([]int64, error)
	GuildMemberNames(guild int64) ([]string, error)
	// GuildPositionOf is a member's position. ok is false for a player
	// who is not a member.
	GuildPositionOf(guild, player int64) (GuildPosition, bool, error)
	// GuildSkill is a guild skill's level.
	GuildSkill(guild int64, skill int) (int, error)
	// GuildRelated reports whether a guild has another as an ally, or as
	// an enemy.
	GuildRelated(guild, other int64, enemy bool) (bool, error)
	// CreateGuild makes a guild for a master and returns its ID, or 0
	// when the name is taken or the master has a guild.
	CreateGuild(master int64, name string) (int64, error)
	// GuildAdd, GuildRemove and GuildSetMaster change a guild's members.
	// They return false when nothing changed.
	GuildAdd(guild, player int64) (bool, error)
	GuildRemove(guild, player int64) (bool, error)
	GuildSetMaster(guild, player int64) (bool, error)
	// GuildSetNotice sets the notice. It returns false for a notice that
	// is too long.
	GuildSetNotice(guild int64, title, text string) (bool, error)
	// GuildGiveExp adds exp to a guild.
	GuildGiveExp(guild, exp int64) error
	// GuildDisband breaks a guild up.
	GuildDisband(guild int64) error
	// FindGuild finds a guild by ID or by name.
	FindGuild(id int64) bool
	FindGuildNamed(name string) (int64, bool)
}

// GuildEvent is a guild_join, guild_leave or guild_change event.
type GuildEvent struct {
	// Kind is "guild_join", "guild_leave" or "guild_change".
	Kind  string
	Guild int64
	// Player and Reason are set for guild_join and guild_leave.
	Player int64
	Reason string
	// Change is "master", "level", "notice" or "emblem", for
	// guild_change.
	Change string
}

// FireGuild runs the handlers of a guild event.
func (r *Runtime) FireGuild(e GuildEvent) {
	fields := []script.Field{script.KV("guild", script.Int(e.Guild))}
	if e.Kind == "guild_change" {
		fields = append(fields, script.KV("change", script.String(e.Change)))
	} else {
		fields = append(fields, script.KV("player", script.Int(e.Player)), script.KV("reason", script.String(e.Reason)))
	}
	r.Fire(e.Kind, Arg{"guild_event", script.NewTable(fields...)})
}

func (r *Runtime) guildHost(what string) (GuildHost, error) {
	h, ok := r.host.(GuildHost)
	if !ok {
		return nil, errors.New(what + ": guilds do not exist here")
	}
	return h, nil
}

// needGuildManage checks guild.manage. A test case's own code needs no
// permission to set up guilds, as for parties.
func (r *Runtime) needGuildManage(what string) error {
	if r.opts.Testing && r.current == nil {
		return nil
	}
	return r.need(what, "guild.manage")
}

func (r *Runtime) guildField(c *script.Call) error {
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return errors.New("not a guild: read fields of a guild handle, such as guild.name")
	}
	if err := r.need("reading a guild", "game.read"); err != nil {
		return err
	}
	h, err := r.guildHost("reading a guild")
	if err != nil {
		return err
	}
	v, err := h.GuildField(id, c.String(1))
	if err != nil {
		return err
	}
	c.Return(v)
	return nil
}

func intList(ids []int64) script.Value {
	vals := make([]script.Value, len(ids))
	for i, n := range ids {
		vals[i] = script.Int(n)
	}
	return script.List(vals...)
}

func stringList(names []string) script.Value {
	vals := make([]script.Value, len(names))
	for i, n := range names {
		vals[i] = script.String(n)
	}
	return script.List(vals...)
}

// guildCall is a method of a guild handle.
func (r *Runtime) guildCall(c *script.Call) error {
	op, id := c.String(0), c.Int(1)
	what := "guild:" + op
	h, err := r.guildHost(what)
	if err != nil {
		return err
	}
	switch op {
	case "get_members", "get_member_names", "get_position", "get_skill_level", "is_allied", "is_enemy":
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
		return r.guildRead(c, h, op, id)
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.needGuildManage(what); err != nil {
		return err
	}
	switch op {
	case "add", "remove", "set_master":
		player, ok := c.Arg(2).AsInt()
		if !ok {
			return errors.New(what + ": want a player handle")
		}
		f := map[string]func(int64, int64) (bool, error){"add": h.GuildAdd, "remove": h.GuildRemove, "set_master": h.GuildSetMaster}[op]
		changed, err := f(id, player)
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		c.Return(script.Bool(changed))
		return nil
	case "set_notice":
		title, ok1 := c.Arg(2).AsString()
		text, ok2 := c.Arg(3).AsString()
		if !ok1 || !ok2 {
			return errors.New(what + ": want a title and a text")
		}
		ok, err := h.GuildSetNotice(id, title, text)
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		c.Return(script.Bool(ok))
		return nil
	case "give_exp":
		n, ok := c.Arg(2).AsInt()
		if !ok || n < 1 || n > 1<<31-1 {
			return errors.New(what + ": the exp is 1 to 2147483647")
		}
		if err := h.GuildGiveExp(id, n); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		return nil
	case "disband":
		if err := h.GuildDisband(id); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		return nil
	}
	return fmt.Errorf("guild: no method %q", op)
}

// guildRead runs the guild methods that only read.
func (r *Runtime) guildRead(c *script.Call, h GuildHost, op string, id int64) error {
	what := "guild:" + op
	switch op {
	case "get_members":
		ids, err := h.GuildMembers(id)
		if err != nil {
			return err
		}
		c.Return(intList(ids))
	case "get_member_names":
		names, err := h.GuildMemberNames(id)
		if err != nil {
			return err
		}
		c.Return(stringList(names))
	case "get_position":
		player, ok := c.Arg(2).AsInt()
		if !ok {
			return errors.New(what + ": want a player handle")
		}
		p, member, err := h.GuildPositionOf(id, player)
		if err != nil {
			return err
		}
		if !member {
			c.Return(script.Nil)
			return nil
		}
		c.Return(script.NewTable(script.KV("name", script.String(p.Name)), script.KV("can_invite", script.Bool(p.CanInvite)),
			script.KV("can_expel", script.Bool(p.CanExpel)), script.KV("can_use_storage", script.Bool(p.CanStore)),
			script.KV("exp_tax", script.Int(int64(p.ExpTax)))))
	case "get_skill_level":
		skill, ok := c.Arg(2).AsInt()
		if !ok {
			return errors.New(what + ": want a skill ID, such as skills.id(\"GD_EXTENSION\")")
		}
		lv, err := h.GuildSkill(id, int(skill))
		if err != nil {
			return err
		}
		c.Return(script.Int(int64(lv)))
	case "is_allied", "is_enemy":
		other, ok := c.Arg(2).AsInt()
		if !ok {
			return errors.New(what + ": want a guild handle")
		}
		b, err := h.GuildRelated(id, other, op == "is_enemy")
		if err != nil {
			return err
		}
		c.Return(script.Bool(b))
	}
	return nil
}

func (r *Runtime) guildsCreate(c *script.Call) error {
	const what = "guilds.create"
	master, ok := c.Arg(0).AsInt()
	if !ok {
		return errors.New(what + ": want a player handle as the master")
	}
	name, ok := c.Arg(1).AsString()
	if !ok {
		return errors.New(what + ": the name must be a string")
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.needGuildManage(what); err != nil {
		return err
	}
	h, err := r.guildHost(what)
	if err != nil {
		return err
	}
	id, err := h.CreateGuild(master, name)
	if err != nil {
		return err
	}
	if id == 0 {
		c.Return(script.Nil)
		return nil
	}
	c.Return(script.Int(id))
	return nil
}

// guildsFind is guilds.get(id) and guilds.find(name).
func (r *Runtime) guildsFind(c *script.Call) error {
	by := c.String(0)
	what := "guilds." + by
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	h, err := r.guildHost(what)
	if err != nil {
		return err
	}
	if by == "get" {
		id, ok := c.Arg(1).AsInt()
		if !ok {
			return errors.New(what + ": the ID must be an integer")
		}
		if id <= 0 || !h.FindGuild(id) {
			c.Return(script.Nil)
			return nil
		}
		c.Return(script.Int(id))
		return nil
	}
	name, ok := c.Arg(1).AsString()
	if !ok {
		return errors.New(what + ": the name must be a string")
	}
	id, found := h.FindGuildNamed(name)
	if !found {
		c.Return(script.Nil)
		return nil
	}
	c.Return(script.Int(id))
	return nil
}

// GuildEventNames are the guild events.
var GuildEventNames = []string{"guild_join", "guild_leave", "guild_change"}

// IsGuildEvent reports whether an event is a guild event.
func IsGuildEvent(name string) bool { return slices.Contains(GuildEventNames, name) }

// CoreGuildHost is what core's converted scripts use through wa.internal
// and the API has no call for yet.
type CoreGuildHost interface {
	// OpenGuildStorage opens a player's guild storage, as
	// guildopenstorage, and returns rAthena's GSTORAGE_ value.
	OpenGuildStorage(player int64) (int, error)
	// GuildStorageLog sends a player its guild's storage log, as
	// guildopenstorage_log.
	GuildStorageLog(player int64) error
}

// coreHost checks that the core mod runs, and returns its host calls.
func (r *Runtime) coreHost(what string) (CoreGuildHost, error) {
	if r.actor() != "core" {
		return nil, errors.New(what + " is for core's scripts only")
	}
	h, ok := r.host.(CoreGuildHost)
	if !ok {
		return nil, errors.New(what + ": guild storage does not exist here")
	}
	return h, nil
}

func (r *Runtime) coreGuildStorage(c *script.Call) error {
	const what = "open_guild_storage"
	h, err := r.coreHost(what)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	n, err := h.OpenGuildStorage(c.Int(0))
	if err != nil {
		return err
	}
	c.Return(script.Int(int64(n)))
	return nil
}

func (r *Runtime) coreGuildStorageLog(c *script.Call) error {
	h, err := r.coreHost("guild_storage_log")
	if err != nil {
		return err
	}
	return h.GuildStorageLog(c.Int(0))
}
