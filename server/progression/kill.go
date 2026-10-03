package progression

import (
	"slices"
	"sort"
)

// DamageEntry is one attacker in a monster's damage log, as rAthena's
// s_dmglog for a player. CharID 0 is never logged.
type DamageEntry struct {
	CharID uint32
	// Damage is the damage the player dealt. Tanked is the damage the
	// monster dealt to the player, which only MVP rewards read.
	Damage, Tanked int64
}

// DamageLog is a monster's dmglog: the players who fought it, oldest
// first, at most DamageLogSize.
type DamageLog struct {
	Entries []DamageEntry
}

// Add is mob_log_damage for a player: it adds damage dealt and damage
// tanked to the player's entry, or adds an entry, dropping the oldest when
// the log is full. A hit for 0 still adds the player.
func (l *DamageLog) Add(charID uint32, damage, tanked int64) {
	if damage < 0 || tanked < 0 || charID == 0 {
		return
	}
	for i := range l.Entries {
		if e := &l.Entries[i]; e.CharID == charID {
			e.Damage = addCap(e.Damage, damage, MaxExp)
			e.Tanked = addCap(e.Tanked, tanked, MaxExp)
			return
		}
	}
	if len(l.Entries) == DamageLogSize {
		l.Entries = append(l.Entries[:0], l.Entries[1:]...)
	}
	l.Entries = append(l.Entries, DamageEntry{CharID: charID, Damage: damage, Tanked: tanked})
}

// LoggedDamage is the damage status_damage passes to the damage log for a
// hit on a monster with hp left. It is the full damage, capped to 32 bits,
// or the HP actually lost with exp_calc_type 1.
func (t *Tables) LoggedDamage(damage, hp int64) int64 {
	damage = min(damage, 1<<31-1)
	if t.ExpCalcType == 1 {
		return min(damage, hp)
	}
	return damage
}

// Attacker is what a kill reads about a player in the damage log.
type Attacker struct {
	// Online is false for a player who left or is on another map.
	Online    bool
	Dead      bool
	BaseLevel int
	// Party is the player's party ID, or 0, and EvenShare whether its
	// party shares exp evenly.
	Party     int64
	EvenShare bool
}

// Reward is the exp one player gets for a kill, before GainExp.
type Reward struct {
	CharID          uint32
	BaseExp, JobExp int64
}

// Kill is the outcome of a monster's death.
type Kill struct {
	// Rewards are in damage log order.
	Rewards []Reward
	// First, Second and Third have loot priority, as mob_dead's first_sd,
	// second_sd and third_sd. 0 is no one.
	First, Second, Third uint32
	// DropModifier is the renewal drop penalty for the first of them, in
	// percent.
	DropModifier int
	// Pots are the exp of parties that share evenly, in the order the
	// damage log first names each party, as mob_dead's pt[]. ShareExp
	// splits each one.
	Pots []PartyPot
}

// PartyPot is the exp a party with even share earned from one kill:
// the sum of its members' shares, before the level penalty.
type PartyPot struct {
	Party     int64
	Base, Job int64
	// Shares are each fighting member's own share, by character.
	Shares map[uint32][2]int64
}

// Kill is the exp and loot priority part of mob_dead for a monster
// killed by a player's normal attack. killer is the player who dealt the
// last hit, or 0. who reports on a player in the log.
//
// It leaves out what needs parties, homunculi, pets, mercenaries, guilds,
// MVP rewards, Star Gladiator bonuses and zeny from monsters.
func (t *Tables) Kill(m *Mob, log *DamageLog, killer uint32, who func(charID uint32) Attacker) Kill {
	return t.KillWith(m, log, killer, who, nil)
}

// KillWith is Kill with hooks: each reward goes through h.Experience.
func (t *Tables) KillWith(m *Mob, log *DamageLog, killer uint32, who func(charID uint32) Attacker, h *Hooks) Kill {
	k := Kill{First: killer, DropModifier: 100}
	entries := append([]DamageEntry(nil), log.Entries...)
	eligible := make([]Attacker, len(entries))
	ok := make([]bool, len(entries))
	type loot struct {
		id     uint32
		damage int64
	}
	var loots []loot
	count := 0
	var total int64
	for i, e := range entries {
		total = addCap(total, e.Damage, MaxExp)
		a := who(e.CharID)
		if !a.Online {
			continue
		}
		// Only players on the map count toward the total.
		count++
		if a.Dead {
			continue
		}
		eligible[i], ok[i] = a, true
		// Each player has one entry, so each gets its own loot entry.
		loots = append(loots, loot{e.CharID, e.Damage})
	}
	if len(loots) > 0 {
		// The first player in the log gets a share of the total damage
		// as a bonus for loot priority.
		loots[0].damage += total * int64(t.FirstAttackLootBonus) / 100
		// rAthena's std::sort is not stable. Ties keep log order here.
		sort.SliceStable(loots, func(i, j int) bool { return loots[i].damage > loots[j].damage })
		k.First = loots[0].id
		if len(loots) > 1 {
			k.Second = loots[1].id
		}
		if len(loots) > 2 {
			k.Third = loots[2].id
		}
	}
	for _, id := range []uint32{k.First, k.Second, k.Third} {
		if id != 0 {
			k.DropModifier = t.Penalty(PenaltyDrop, who(id).BaseLevel, m)
			break
		}
	}

	if t.ExpCalcType == 2 && count > 1 && len(entries) > 0 {
		// The first attacker counts twice.
		total = addCap(total, entries[0].Damage, MaxExp)
		entries[0].Damage = addCap(entries[0].Damage, entries[0].Damage, MaxExp)
	}
	for i, e := range entries {
		if !ok[i] {
			continue
		}
		var per float64
		if t.ExpCalcType == 1 || total == 0 {
			per = float64(e.Damage) / float64(max(m.MaxHP, 1))
		} else {
			per = float64(e.Damage) / float64(total)
		}
		per = min(per, 1)
		if count > 1 && t.ExpBonusAttacker != 0 {
			// More attackers give more exp, up to a cap.
			count = min(count, t.ExpBonusMaxAttacker)
			per += per * float64((count-1)*t.ExpBonusAttacker) / 100
		}
		var base, job int64
		// The common bonus, 100 unless a status adds to it.
		bonus := 100
		if h != nil {
			bonus += h.ExpBonus
		}
		withBonus := func(exp float64) float64 {
			if bonus != 100 {
				exp = exp * float64(bonus) / 100
			}
			return exp
		}
		if m.BaseExp != 0 {
			// apply_rate2 by the share, then the common bonus and the map
			// rate, which is 100.
			base = capFloat(withBonus(float64(m.BaseExp)*per), 1)
		}
		if m.JobExp != 0 {
			job = capFloat(withBonus(float64(m.JobExp)*per), 1)
		}
		if a := eligible[i]; a.Party != 0 && a.EvenShare {
			// mob_dead adds the share to its party's pot. party_exp_share
			// splits the pot, and the level penalty comes after the split.
			j := slices.IndexFunc(k.Pots, func(p PartyPot) bool { return p.Party == a.Party })
			if j < 0 {
				k.Pots = append(k.Pots, PartyPot{Party: a.Party, Shares: map[uint32][2]int64{}})
				j = len(k.Pots) - 1
			}
			pot := &k.Pots[j]
			pot.Base = addCap(pot.Base, base, MaxExp)
			pot.Job = addCap(pot.Job, job, MaxExp)
			pot.Shares[e.CharID] = [2]int64{base, job}
			continue
		}
		if base == 0 && job == 0 {
			continue
		}
		shareBase, shareJob := base, job
		if rate := t.Penalty(PenaltyExp, eligible[i].BaseLevel, m); rate != 100 {
			if base != 0 {
				base = min(max(applyRate(base, rate), 1), MaxExp)
			}
			if job != 0 {
				job = min(max(applyRate(job, rate), 1), MaxExp)
			}
		}
		if h != nil && h.Experience != nil {
			base, job = h.Experience(ExpGain{CharID: e.CharID, Base: base, Job: job, Party: eligible[i].Party,
				ShareBase: shareBase, ShareJob: shareJob})
			base, job = max(base, 0), max(job, 0)
			if base == 0 && job == 0 {
				continue
			}
		}
		k.Rewards = append(k.Rewards, Reward{CharID: e.CharID, BaseExp: base, JobExp: job})
	}
	return k
}

// ShareMember is a party member that party_exp_share counts: on the
// kill's map and alive.
type ShareMember struct {
	CharID    uint32
	BaseLevel int
}

// ShareExp is party_exp_share: a party pot split among the members on
// the map, in slot order. evenBonus is party_even_share_bonus. Each
// member's share then gets its own level penalty (RENEWAL_EXP, the
// level_penalty_exp flag) and the hooks.
func (t *Tables) ShareExp(m *Mob, pot PartyPot, members []ShareMember, evenBonus int, h *Hooks) []Reward {
	c := int64(len(members))
	if c < 1 {
		return nil
	}
	base, job := pot.Base/c, pot.Job/c
	if evenBonus != 0 && c > 1 {
		bonus := float64(100 + evenBonus*int(c-1))
		if base != 0 {
			base = min(max(int64(float64(base)*bonus/100), 0), MaxExp)
		}
		if job != 0 {
			job = min(max(int64(float64(job)*bonus/100), 0), MaxExp)
		}
	}
	var out []Reward
	for _, mb := range members {
		b, j := base, job
		if b != 0 || j != 0 {
			if rate := t.Penalty(PenaltyExp, mb.BaseLevel, m); rate != 100 {
				if b != 0 {
					b = min(max(applyRate(b, rate), 1), MaxExp)
				}
				if j != 0 {
					j = min(max(applyRate(j, rate), 1), MaxExp)
				}
			}
		}
		if h != nil && h.Experience != nil {
			sh := pot.Shares[mb.CharID]
			b, j = h.Experience(ExpGain{CharID: mb.CharID, Base: b, Job: j, Party: pot.Party, ShareBase: sh[0], ShareJob: sh[1]})
			b, j = max(b, 0), max(j, 0)
		}
		// pc_gainexp does nothing for 0 and 0.
		if b == 0 && j == 0 {
			continue
		}
		out = append(out, Reward{CharID: mb.CharID, BaseExp: b, JobExp: j})
	}
	return out
}
