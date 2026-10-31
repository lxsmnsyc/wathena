package zone

import (
	"github.com/lxsmnsyc/wathena/internal/script"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// Castles (proposal 0004): the zone hears the castle registry's notices
// through its feed. An owner change runs castle_change; a guild's new
// emblem, or its break up, updates the NPCs that show its badge
// (section 7).

// castleNotices takes the feed's notices: castle_change events wait
// with the other script events, and badges change at once.
func (z *Zone) castleNotices() {
	if z.castleFeed == nil {
		return
	}
	for _, n := range z.castleFeed.Take() {
		if c := n.Change; c != nil {
			z.scriptEvents = append(z.scriptEvents, scriptEvent{name: "castle_change",
				args: []modscript.Arg{modscript.CastleChangeArg(modscript.CastleChange{Castle: c.Castle.ID, Previous: c.Previous})}})
		}
		if n.Badge != 0 {
			z.refreshBadges(n.Badge)
		}
	}
}

// refreshBadges works out again the badge of each NPC of the zone that
// shows a guild.
func (z *Zone) refreshBadges(guild int64) {
	for _, e := range z.npcHandles {
		if e.npc.guild == guild {
			z.setNPCBadge(e, guild)
		}
	}
}

// setNPCBadge makes an NPC show a guild's badge, as flagemblem: its name
// and emblem version. A guild that does not run shows none, and so does
// 0.
func (z *Zone) setNPCBadge(e *entity, guild int64) {
	var b *wathenav1.GuildBadge
	if guild > 0 && z.Social != nil {
		if v := z.Social.GuildViewByID(guild); v != nil {
			b = wathenav1.GuildBadge_builder{GuildId: uint64(v.ID), Name: v.Name, // #nosec G115 -- guild IDs are positive.
				EmblemVersion: uint32(max(v.Emblem, 0))}.Build() // #nosec G115 -- a version count.
		}
	}
	e.npc.guild = guild
	if sameBadge(e.npc.badge, b) {
		return
	}
	e.npc.badge = b
	e.guildChanged = true
}

// SetNPCGuild is flagemblem: the NPC shows a guild's badge, or none for
// 0.
func (h scriptHost) SetNPCGuild(id, guild int64) error {
	return h.onNPC(id, true, func(z *Zone, e *entity) { z.setNPCBadge(e, guild) })
}

// npcGuild is the guild an NPC shows, as npc.guild reads it.
func npcGuild(_ scriptHost, e *entity) script.Value {
	if e.npc.guild == 0 {
		return script.Nil
	}
	return script.Int(e.npc.guild)
}
