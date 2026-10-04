package social

import "slices"

// guildMemberView is what members see of a member. r.mu must be held.
func (r *Registry) guildMemberView(g *Guild, m *guildMember) GuildMemberView {
	p := m.p
	return GuildMemberView{Name: p.name, Position: m.position, Online: p.online, Map: p.mapName, Job: p.job, Level: p.level,
		Exp: m.exp, X: p.x, Y: p.y, Master: g.members[0] == m}
}

// guildView is what members see of a guild. r.mu must be held.
func (r *Registry) guildView(g *Guild) *GuildView {
	t := r.tables()
	v := &GuildView{ID: g.ID, Name: g.Name, Level: g.level, Exp: g.exp, NextExp: t.Next(g.level), SkillPoints: g.skillPoints,
		MaxMembers: r.maxMembers(g), Notice: g.notice, Emblem: g.emblem.Version, Positions: slices.Clone(g.positions),
		Relations: slices.Clone(g.relations)}
	if g.level >= MaxGuildLevel {
		v.NextExp = 0
	}
	if m := g.master(); m != nil {
		v.Master = m.p.name
	}
	sum, n := 0, 0
	for _, m := range g.members {
		if m == nil {
			continue
		}
		v.Members = append(v.Members, r.guildMemberView(g, m))
		sum += m.p.level
		n++
	}
	if n > 0 {
		v.AverageLevel = sum / n
	}
	ids := make([]int, 0, len(t.Skills))
	for _, d := range t.Skills {
		ids = append(ids, d.ID)
	}
	slices.Sort(ids)
	for _, id := range ids {
		v.Skills = append(v.Skills, GuildSkillView{ID: id, Level: g.skill(id), Max: t.MaxLevel(id), CanRaise: r.canRaise(g, id)})
	}
	return v
}

// guildOnline gives a player that logged in or moved its guild, and its
// guild sees it online. A master who logged in has its guild skills
// blocked in pre-renewal, as guild_member_joined. r.mu must be held.
func (r *Registry) guildOnline(p *Player, login bool) {
	g := p.guild
	if g == nil {
		deliver(p, Notice{Kind: NoticeGuild})
		return
	}
	deliver(p, Notice{Kind: NoticeGuild, Guild: r.guildView(g)})
	if m := g.member(p); m != nil {
		r.guildMemberChanged(g, m)
	}
	if login && g.isMaster(p) {
		deliver(p, Notice{Kind: NoticeGuildSkillBlock})
	}
}

// GuildViewOf returns the guild of a player as its members see it, or
// nil.
func (r *Registry) GuildViewOf(p *Player) *GuildView {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p == nil || p.guild == nil {
		return nil
	}
	return r.guildView(p.guild)
}

// GuildViewByID returns a running guild as its members see it, or nil.
func (r *Registry) GuildViewByID(id int64) *GuildView {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g := r.guilds[id]; g != nil {
		return r.guildView(g)
	}
	return nil
}
