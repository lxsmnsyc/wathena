package zone

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/social"
)

// The script calls of guilds (modscript.GuildHost, review decision 28).
// Guild handles hold the guild's ID. Players are handles of this zone, so
// a master or member on another map is nil or left out.

func (h scriptHost) guildInfo(id int64) (social.GuildInfo, error) {
	in, ok := h.z.socialHub().Guild(id)
	if !ok {
		return social.GuildInfo{}, errors.New("there is no such guild")
	}
	return in, nil
}

// GuildField reads a field of a guild handle.
func (h scriptHost) GuildField(id int64, field string) (script.Value, error) {
	in, err := h.guildInfo(id)
	if err != nil {
		return script.Nil, err
	}
	switch field {
	case "id":
		return script.Int(in.ID), nil
	case "name":
		return script.String(in.Name), nil
	case "master":
		if !in.MasterOnline {
			return script.Nil, nil
		}
		if pid := h.z.playerNamed(in.Master); pid != 0 {
			return script.Int(pid), nil
		}
		return script.Nil, nil
	case "master_name":
		return script.String(in.Master), nil
	case "level":
		return script.Int(int64(in.Level)), nil
	case "exp":
		return script.Int(in.Exp), nil
	case "notice":
		return script.NewTable(script.KV("title", script.String(in.Notice.Title)), script.KV("text", script.String(in.Notice.Text))), nil
	case "member_count":
		return script.Int(int64(len(in.Members))), nil
	}
	return script.Nil, fmt.Errorf("guild has no field %q", field)
}

// GuildMembers are the online members in this zone, in slot order.
func (h scriptHost) GuildMembers(id int64) ([]int64, error) {
	in, err := h.guildInfo(id)
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, name := range in.Online {
		if pid := h.z.playerNamed(name); pid != 0 {
			out = append(out, pid)
		}
	}
	return out, nil
}

// GuildMemberNames are every member's name.
func (h scriptHost) GuildMemberNames(id int64) ([]string, error) {
	in, err := h.guildInfo(id)
	if err != nil {
		return nil, err
	}
	return in.Members, nil
}

// GuildPositionOf is a member's position.
func (h scriptHost) GuildPositionOf(id, player int64) (modscript.GuildPosition, bool, error) {
	p, err := h.socialPlayer(player)
	if err != nil {
		return modscript.GuildPosition{}, false, err
	}
	if _, err := h.guildInfo(id); err != nil {
		return modscript.GuildPosition{}, false, err
	}
	pos, ok := h.z.socialHub().MemberPosition(id, p.soc)
	if !ok {
		return modscript.GuildPosition{}, false, nil
	}
	return modscript.GuildPosition{Name: pos.Name, CanInvite: pos.Invite, CanExpel: pos.Expel, CanStore: pos.Storage, ExpTax: pos.ExpTax}, true, nil
}

// GuildSkill is a guild skill's level.
func (h scriptHost) GuildSkill(id int64, skill int) (int, error) {
	if _, err := h.guildInfo(id); err != nil {
		return 0, err
	}
	return h.z.socialHub().GuildSkillLevel(id, skill), nil
}

// GuildRelated reports whether a guild has another as an ally or enemy.
func (h scriptHost) GuildRelated(id, other int64, enemy bool) (bool, error) {
	if _, err := h.guildInfo(id); err != nil {
		return false, err
	}
	if enemy {
		return h.z.socialHub().Enemies(id, other), nil
	}
	return h.z.socialHub().Allied(id, other), nil
}

// CreateGuild makes a guild for a master, without an Emperium.
func (h scriptHost) CreateGuild(master int64, name string) (int64, error) {
	p, err := h.socialPlayer(master)
	if err != nil {
		return 0, err
	}
	return h.z.socialHub().ScriptGuildCreate(p.soc, name), nil
}

// GuildAdd puts a player in a guild.
func (h scriptHost) GuildAdd(id, player int64) (bool, error) {
	p, err := h.socialPlayer(player)
	if err != nil {
		return false, err
	}
	return h.z.socialHub().GuildAddMember(id, p.soc)
}

// GuildRemove takes a member out of a guild.
func (h scriptHost) GuildRemove(id, player int64) (bool, error) {
	p, err := h.socialPlayer(player)
	if err != nil {
		return false, err
	}
	return h.z.socialHub().GuildRemoveMember(id, p.soc)
}

// GuildSetMaster makes a member the master, as guildchangegm.
func (h scriptHost) GuildSetMaster(id, player int64) (bool, error) {
	p, err := h.socialPlayer(player)
	if err != nil {
		return false, err
	}
	ok, ch, err := h.z.socialHub().ScriptSetMaster(id, p.soc)
	if ch != nil {
		h.z.guildChanged(ch)
	}
	return ok, err
}

// GuildSetNotice sets the notice.
func (h scriptHost) GuildSetNotice(id int64, title, text string) (bool, error) {
	ok, ch, err := h.z.socialHub().ScriptSetNotice(id, social.GuildNotice{Title: title, Text: text})
	if ch != nil {
		h.z.guildChanged(ch)
	}
	return ok, err
}

// GuildGiveExp adds exp to a guild, as guildgetexp.
func (h scriptHost) GuildGiveExp(id, exp int64) error {
	ch, err := h.z.socialHub().GiveExp(id, exp)
	if ch != nil {
		h.z.guildChanged(ch)
	}
	return err
}

// GuildDisband breaks a guild up with its members. Its storage is lost.
func (h scriptHost) GuildDisband(id int64) error {
	left, err := h.z.socialHub().DisbandGuild(id)
	if err != nil {
		return err
	}
	h.z.storageLost(nil, id, left)
	return nil
}

// FindGuild reports whether a guild runs.
func (h scriptHost) FindGuild(id int64) bool { return h.z.socialHub().GuildRuns(id) }

// FindGuildNamed finds a guild by name.
func (h scriptHost) FindGuildNamed(name string) (int64, bool) { return h.z.socialHub().FindGuild(name) }

// OpenGuildStorageFor is player:open_guild_storage, as
// guildopenstorage. reason names rAthena's e_guild_storage_flags value:
// already_open is GSTORAGE_STORAGE_ALREADY_OPEN, in_use
// GSTORAGE_ALREADY_OPEN, and no_guild, no_storage and no_permission the
// others.
func (h scriptHost) OpenGuildStorageFor(player int64) (bool, string, error) {
	p, err := h.player(player)
	if err != nil {
		return false, "", err
	}
	personal := p.storage != nil
	switch h.z.openGuildStorage(p) {
	case social.StorageOpened:
		return true, "", nil
	case social.StorageNoGuild:
		return false, "no_guild", nil
	case social.StorageNoStorage:
		return false, "no_storage", nil
	case social.StorageNoPermission:
		return false, "no_permission", nil
	case social.StorageAlreadyOpen:
		if personal {
			return false, "already_open", nil
		}
	}
	return false, "in_use", nil
}

// GuildStorageLog sends a player its guild's storage log, for core's
// converted NPC scripts, as guildopenstorage_log.
func (h scriptHost) GuildStorageLog(player int64) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	h.z.storageCommand(p, &storageCommand{log: true})
	return nil
}
