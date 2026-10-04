package social

import "slices"

// Guild alliances and enemies, as guild.cpp's guild_reqalliance,
// guild_reply_reqalliance, guild_opposition and guild_delalliance, and
// the char server's mapif_parse_GuildAlliance:
//   - A master asks the guild of an online player to be allies. The
//     player answers. With guild_alliance_onlygm, the player must be a
//     master. An accepted alliance puts each guild on the other's list
//     and ends any enemy relation between them.
//   - A master makes another guild an enemy at once. Enemies are on one
//     side only.
//   - Ending an alliance ends it on both sides. Ending an enemy relation
//     ends it on the master's side.
//   - guild_allies and guild_enemies limit each kind. rAthena counts both
//     against max_guild_alliance.
//   - Sieges do not exist yet, so the siege checks of rAthena do not
//     apply.

type allyInvite struct {
	guild int64
	from  string
}

func (g *Guild) count(enemy bool) int {
	n := 0
	for _, rel := range g.relations {
		if rel.Enemy == enemy {
			n++
		}
	}
	return n
}

// GuildAlly asks the guild of an online player to be allies, as
// guild_reqalliance. Only the master may ask.
func (r *Registry) GuildAlly(master *Player, name string) GuildOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, who := r.guildAlly(master, name)
	guildResult(master, o, who)
	return o
}

func (r *Registry) guildAlly(master *Player, name string) (GuildOutcome, string) {
	g := master.guild
	switch {
	case g == nil:
		return GuildNoGuild, ""
	case !g.isMaster(master):
		return GuildNotMaster, ""
	}
	to := r.online(name)
	switch {
	case to == nil:
		return GuildNotOnline, name
	case to.guild == nil:
		return GuildNoGuild, to.name
	case to.guild == g:
		return GuildSameGuild, to.name
	case r.guildRules.AllianceOnlyMaster && !to.guild.isMaster(to):
		return GuildTargetNotMaster, to.name
	case g.count(false) >= r.limits.GuildAllies:
		return GuildAllyLimit, to.name
	case to.guild.count(false) >= r.limits.GuildAllies:
		return GuildTheirAllyLimit, to.name
	case to.allyInvite != nil:
		return GuildTargetBusy, to.name
	case r.related(g.ID, to.guild.ID, false):
		return GuildAlreadyAllied, to.guild.Name
	case r.blocks(to, master):
		return GuildAllyRefused, to.name
	}
	to.allyInvite = &allyInvite{guild: g.ID, from: master.key}
	deliver(to, Notice{Kind: NoticeGuildAllyRequest, Invite: InviteView{Party: g.ID, PartyName: g.Name, From: master.name}})
	return GuildAllyRequestSent, to.name
}

// GuildAllyReply answers an alliance request, as
// guild_reply_reqalliance.
func (r *Registry) GuildAllyReply(p *Player, guild int64, accept bool) GuildOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv := p.allyInvite
	if inv == nil || inv.guild != guild {
		return 0
	}
	p.allyInvite = nil
	from := r.players[inv.from]
	g, tg := r.guilds[guild], p.guild
	if !accept || g == nil || tg == nil || from == nil || !from.online {
		if from != nil {
			guildResult(from, GuildAllyRefused, p.name)
		}
		return GuildAllyRefused
	}
	switch {
	case tg.count(false) >= r.limits.GuildAllies:
		guildResult(p, GuildAllyLimit, g.Name)
		guildResult(from, GuildTheirAllyLimit, tg.Name)
		return GuildAllyLimit
	case g.count(false) >= r.limits.GuildAllies:
		guildResult(p, GuildTheirAllyLimit, g.Name)
		guildResult(from, GuildAllyLimit, tg.Name)
		return GuildTheirAllyLimit
	}
	// An alliance ends the enemy relations between the two guilds.
	r.endRelation(g, tg.ID, true)
	r.endRelation(tg, g.ID, true)
	g.relations = append(g.relations, Relation{Guild: tg.ID, Name: tg.Name})
	tg.relations = append(tg.relations, Relation{Guild: g.ID, Name: g.Name})
	r.save(func(s Store) { s.SaveGuild(g.record()); s.SaveGuild(tg.record()) })
	guildResult(from, GuildAllied, tg.Name)
	guildResult(p, GuildAllied, g.Name)
	r.guildChanged(g)
	r.guildChanged(tg)
	return GuildAllied
}

func (r *Registry) endRelation(g *Guild, other int64, enemy bool) bool {
	n := len(g.relations)
	g.relations = slices.DeleteFunc(g.relations, func(rel Relation) bool { return rel.Guild == other && rel.Enemy == enemy })
	return len(g.relations) != n
}

// GuildEnemy makes the guild of an online player an enemy, as
// guild_opposition. An alliance with it ends first. Only the master may
// do it.
func (r *Registry) GuildEnemy(master *Player, name string) GuildOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := master.guild
	o, who := GuildEnemyAdded, name
	to := r.online(name)
	switch {
	case g == nil:
		o, who = GuildNoGuild, ""
	case !g.isMaster(master):
		o, who = GuildNotMaster, ""
	case to == nil:
		o = GuildNotOnline
	case to.guild == nil:
		o, who = GuildNoGuild, to.name
	case to.guild == g:
		o, who = GuildSameGuild, to.name
	case r.related(g.ID, to.guild.ID, true):
		o, who = GuildAlreadyEnemy, to.guild.Name
	case g.count(true) >= r.limits.GuildEnemies:
		o, who = GuildEnemyLimit, to.guild.Name
	default:
		tg := to.guild
		who = tg.Name
		if r.endRelation(g, tg.ID, false) {
			r.endRelation(tg, g.ID, false)
			r.save(func(s Store) { s.SaveGuild(tg.record()) })
			r.guildChanged(tg)
		}
		g.relations = append(g.relations, Relation{Guild: tg.ID, Name: tg.Name, Enemy: true})
		r.save(func(s Store) { s.SaveGuild(g.record()) })
		r.guildChanged(g)
	}
	guildResult(master, o, who)
	return o
}

// GuildEndRelation ends an alliance on both sides, or an enemy relation
// on the master's side, as guild_delalliance. Only the master may do it.
func (r *Registry) GuildEndRelation(master *Player, other int64, enemy bool) GuildOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := master.guild
	switch {
	case g == nil:
		guildResult(master, GuildNoGuild, "")
		return GuildNoGuild
	case !g.isMaster(master):
		guildResult(master, GuildNotMaster, "")
		return GuildNotMaster
	}
	i := slices.IndexFunc(g.relations, func(rel Relation) bool { return rel.Guild == other && rel.Enemy == enemy })
	if i < 0 {
		return 0
	}
	name := g.relations[i].Name
	r.endRelation(g, other, enemy)
	r.save(func(s Store) { s.SaveGuild(g.record()) })
	guildResult(master, GuildRelationEnded, name)
	r.guildChanged(g)
	if tg := r.guilds[other]; tg != nil && !enemy {
		r.endRelation(tg, g.ID, false)
		r.save(func(s Store) { s.SaveGuild(tg.record()) })
		r.guildChanged(tg)
	}
	return GuildRelationEnded
}
