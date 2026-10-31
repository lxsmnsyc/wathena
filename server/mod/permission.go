package mod

import (
	"slices"
	"sort"
)

// Permission is one thing a mod may do (strategy section 6.6).
type Permission struct {
	Name string
	// Approval is set when the server owner must approve the permission
	// in the config. Other permissions are granted to every mod.
	Approval bool
	// Flagged permissions can change the game for everyone. The loader
	// logs a warning for each mod that has one.
	Flagged bool
	// Allows says what the permission allows, for errors and docs.
	Allows string
}

// Permissions lists every permission, from strategy section 6.6.
var Permissions = []Permission{
	{Name: "game.read", Allows: "read game state"},
	{Name: "npc.create", Allows: "add NPCs"},
	{Name: "event.listen", Allows: "listen to events"},
	{Name: "storage.mod", Allows: "read and write the mod's own saved data"},
	{Name: "player.give_items", Approval: true, Allows: "give items to players"},
	{Name: "player.give_zeny", Approval: true, Allows: "give zeny to players"},
	{Name: "player.give_exp", Approval: true, Allows: "give exp to players"},
	{Name: "player.warp", Approval: true, Allows: "move players"},
	{Name: "player.status", Approval: true, Allows: "apply status effects to players"},
	{Name: "player.change_job", Approval: true, Allows: "change a player into any job, without the job change rules"},
	{Name: "world.announce", Approval: true, Allows: "announce to every player of the world"},
	{Name: "events.schedule", Approval: true, Allows: "add world events to the scheduler"},
	{Name: "rules.override", Approval: true, Flagged: true, Allows: "replace formulas such as damage or drop rates"},
	{Name: "admin.commands", Approval: true, Flagged: true, Allows: "add GM commands"},
	{Name: "net.http", Approval: true, Flagged: true, Allows: "call external HTTP services"},
	{Name: "items.modify_rolls", Approval: true, Flagged: true, Allows: "change the rolled values of existing items"},
	// player.sanction is flagged for review (proposal 0002, review
	// decision 26).
	{Name: "player.sanction", Approval: true, Flagged: true, Allows: "mute and jail players"},
	// party.manage is review decision 27's: wa.parties' changes.
	{Name: "party.manage", Approval: true, Allows: "make parties and change their members, leader and options"},
	// guild.manage is review decision 28's: wa.guilds' changes.
	{Name: "guild.manage", Approval: true, Allows: "make guilds and change their members, master, notice, exp, and break them up"},
	// mail.send, currencies.modify and shops.modify are review decision
	// 30's.
	{Name: "mail.send", Approval: true, Allows: "send mail with items and zeny to any character, online or not"},
	{Name: "currencies.modify", Approval: true, Allows: "change other mods' currencies of players, such as cash points"},
	{Name: "shops.modify", Approval: true, Allows: "change what shops sell, their costs and their stock"},
	// events.control is review decision 31's: world_events.start and
	// world_events.stop.
	{Name: "events.control", Approval: true, Allows: "start and stop world events at any time"},
	// player.expand_limits is review decision 32's: player:expand.
	{Name: "player.expand_limits", Approval: true, Allows: "raise and lower a player's inventory, cart and storage slots within the ruleset's limits"},
	// spawns.modify is review decision 44's: the spawn_table hook.
	{Name: "spawns.modify", Approval: true, Allows: "change the monsters a new procedural world spawns on its fields and dungeons"},
	// world.generate is review decision 46's: wa.generation.
	{Name: "world.generate", Approval: true, Allows: "add places, objects, bosses and quest templates to a new procedural world's maps"},
	// data.core_override is review decision 52's: data overlays that
	// change the prices, drops, exp or reserved chat words of records
	// other mods added, core's too.
	{Name: "data.core_override", Approval: true, Flagged: true, Allows: "change the prices, drops and exp of items, monsters and quests, and the chat filter's words, that other mods added, core's too"},
	// player.skills is review decision 57's: player:give_skill, which
	// gives skills outside the skill tree and skill points.
	{Name: "player.skills", Approval: true, Flagged: true, Allows: "give players skills and skill levels outside their skill tree, without skill points"},
	// npcs.control is review decision 58's: showing, hiding, cloaking and
	// removing another mod's NPCs, and sending them events.
	{Name: "npcs.control", Approval: true, Allows: "show, hide, cloak and remove other mods' NPCs, core's too, and send them events"},
	// maps.control is review decision 59's: maps.remove_monsters and
	// maps.set_cells change a map for every player on it.
	{Name: "maps.control", Approval: true, Flagged: true, Allows: "kill the monsters of any map and change its cells"},
	// castles.manage is proposal 0004's (section 8): changing castle
	// owners and values, placing guardians, and starting and ending
	// sieges. Another mod's castle, core's too, also needs npcs.control.
	{Name: "castles.manage", Approval: true, Allows: "change castle owners and values, place guardians, and start and end sieges"},
}

// LookupPermission returns a permission by name.
func LookupPermission(name string) (Permission, bool) {
	i := slices.IndexFunc(Permissions, func(p Permission) bool { return p.Name == name })
	if i < 0 {
		return Permission{}, false
	}
	return Permissions[i], true
}

// Grants is the set of permissions a mod holds.
type Grants map[string]bool

// Has reports whether the set holds a permission.
func (g Grants) Has(name string) bool { return g[name] }

// List returns the permissions in the set, sorted.
func (g Grants) List() []string {
	out := make([]string, 0, len(g))
	for p := range g {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// grant works out the permissions of a mod. A mod holds every permission
// that needs no approval, and the ones it requests that the owner
// approved. It returns the requested permissions that are not approved.
func grant(requested []string, approved []string) (Grants, []string) {
	g := Grants{}
	for _, p := range Permissions {
		if !p.Approval {
			g[p.Name] = true
		}
	}
	var missing []string
	for _, name := range requested {
		p, _ := LookupPermission(name)
		if !p.Approval {
			continue
		}
		if slices.Contains(approved, name) {
			g[name] = true
		} else {
			missing = append(missing, name)
		}
	}
	return g, missing
}
