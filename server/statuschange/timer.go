package statuschange

// timer is status_change_timer: the timer of an active status runs at
// tick, the time it was due. A status that ticks does its work and waits
// for its next tick; any other ends.
func (en *Engine) timer(u Unit, e *Entry, due int64) {
	typ := e.Type
	def := en.DB.Get(typ)
	sc := u.Changes()
	st := u.Status()
	interval := int64(scInterval(typ))
	isPC := u.Kind() == KindPlayer
	next := func(at int64) { en.setTimer(u, e, at) }
	// tick fires status_tick for a status that ticks and goes on. The
	// reveal loop of Sight and Ruwach, every 20 ms, fires none.
	tick := func(at int64) {
		en.env.Ticked(u, e)
		if sc.Get(typ) == e {
			next(at)
		}
	}

	if def != nil && def.Mod != "" && def.TickMs > 0 {
		// A mod status's tick: the status_tick event, then the next tick
		// or the end.
		en.env.Ticked(u, e)
		if sc.Get(typ) != e {
			return
		}
		if e.End < 0 {
			next(due + def.TickMs)
			return
		}
		if left := e.End - due; left >= 100 {
			next(due + min(left, def.TickMs))
			return
		}
		en.end(u, typ, true, ReasonExpired)
		return
	}

	switch typ {
	case SCBlind:
		// Blindness lasts while the unit stands in Blinding Mist.
		if sc.Has(SCFogwall) {
			next(5000 + due)
			return
		}
	case SCDancing, SCMarionette, SCMarionette2, SCWhistle, SCAssncros, SCPoembragi, SCAppleidun, SCHumming,
		SCDontforgetme, SCFortune, SCService4u:
		// The Bard and Dancer statuses, in bard.go.
		if en.performanceTimer(u, e, due) {
			return
		}
	case SCMaximizepower, SCCloaking:
		// Not enough SP to go on ends it. rAthena takes the SP with
		// status_damage, which takes none from a unit that cannot
		// consume SP, such as a monster, and so ends the status at its
		// first timer, 10 seconds in. charge would let a monster go on,
		// with a val2 of 0: a timer due at once, forever.
		if isPC && en.charge(u, 0, 1) {
			tick(int64(e.Val2) + due)
			return
		}
	case SCChasewalk:
		// Not enough SP to go on ends it. The first drain gives the STR
		// bonus, ten times as long with the Rogue's Soul Link.
		if !en.charge(u, 0, int64(e.Val4)) {
			break
		}
		if !sc.Has(SCChasewalk2) {
			d := int64(def.Duration2(e.Val1))
			if s := sc.Get(SCSpirit); s != nil && s.Val2 == skillSLRogue {
				d *= 10
			}
			en.Start(u, u, Start{Type: SCChasewalk2, Rate: 10000, Val1: 1 << (e.Val1 - 1), Duration: d,
				Reason: ReasonSkill, Source: u.Ref()})
		}
		if sc.Get(typ) == e {
			tick(int64(e.Val2) + due)
		}
		return
	case SCHiding:
		e.Val2--
		if e.Val2 > 0 {
			if e.Val4 != 0 && e.Val2%e.Val4 == 0 && !en.charge(u, 0, 1) {
				break
			}
			tick(1000 + due)
			return
		}
	case SCSight, SCRuwach, SCSightblaster:
		// Revealing hidden units waits for hiding to hide units from
		// others; the timer still counts down.
		e.Val2--
		if e.Val2 > 0 {
			e.Val4 += 20
			next(20 + due)
			return
		}
	case SCAutoberserk:
		if p := sc.Get(SCProvoke); p != nil && p.Val4 == 1 {
			// Above a quarter of max HP, the Provoke of Auto Berserk ends.
			if u.HP() > int64(st.MaxHP)/4 {
				en.End(u, SCProvoke, ReasonRemoved)
			}
		} else if u.HP() <= int64(st.MaxHP)/4 {
			// At a quarter of max HP or less, Provoke level 10 starts with
			// no end. It replaces a Provoke from the skill.
			en.Start(u, u, Start{Type: SCProvoke, Rate: 10000, Val1: 10, Val4: 1, Duration: InfiniteTick,
				Reason: ReasonSkill, Source: u.Ref()})
		}
		if sc.Get(typ) == e {
			next(100 + due)
		}
		return
	case SCStone:
		if e.Val4 >= 0 && u.HP() > int64(st.MaxHP)/4 {
			// 1% of max HP, never the last HP.
			en.percentDamage(u, 1)
		}
	case SCPoison, SCDpoison:
		if e.Val4 >= 0 && !sc.Has(SCSlowpoison) {
			var damage int64
			maxHP := int64(st.MaxHP)
			if isPC {
				if typ == SCDpoison {
					damage = 2 + maxHP/50
				} else {
					damage = 2 + maxHP*3/200
				}
			} else {
				if typ == SCDpoison {
					damage = 2 + maxHP/100
				} else {
					damage = 2 + maxHP/200
				}
			}
			// Poison stops at a quarter of max HP.
			if u.HP() > max(maxHP/4, damage) {
				en.env.Damage(u, damage, 0, true, false)
			}
		}
	case SCBleeding:
		if e.Val4 >= 0 {
			damage := int64(en.env.RandN(600) + 200)
			if !isPC && damage >= u.HP() {
				// Bleeding does not kill monsters.
				damage = u.HP() - 1
			}
			en.env.Damage(u, damage, 0, true, false)
		}
	case SCDeathhurt:
		if e.Val4 >= 0 && u.HP() < int64(st.MaxHP) {
			en.env.Heal(u, int64(st.MaxHP)/100, 0)
		}
	case SCGradualGravity:
		// status_zap of val2 percent of max HP, which may kill. rAthena
		// multiplies the unsigned max_hp.
		if e.Val4 >= 0 {
			en.env.Damage(u, int64(uint32(st.MaxHP)*uint32(e.Val2))/100, 0, true, false) // #nosec G115 -- rAthena's max_hp is a uint32.
		}
	case SCSplasher:
		// Venom Splasher counts down every 500 ms, then explodes when it
		// ends.
		e.Val4 -= 500
		if e.Val4 > 0 {
			tick(500 + due)
			return
		}
	case SCOblivioncurse:
		e.Val4--
		if e.Val4 >= 0 {
			tick(3000 + due)
			return
		}
	case SCWeaponblocking:
		e.Val4--
		if e.Val4 >= 0 {
			if !en.charge(u, 0, 3) {
				break
			}
			tick(5000 + due)
			return
		}
	case SCCrystalize:
		e.Val4--
		if e.Val4 >= 0 {
			// 2% of HP and 1% of SP every second.
			if !en.charge(u, int64(st.MaxHP)*2/100, int64(st.MaxSP)/100) {
				break
			}
			tick(1000 + due)
			return
		}
	case SCTensionrelax:
		// Goes on while HP is not full, for its number of ticks.
		if u.HP() < int64(st.MaxHP) {
			e.Val3--
			if e.Val3 >= 0 {
				tick(10000 + due)
				return
			}
		}
	case SCBerserk:
		// 5% of max HP every 10 seconds, until HP is 100 or less.
		e.Val3--
		if e.Val3 > 0 && en.charge(u, int64(e.Val2), 0) && u.HP() > 100 {
			tick(int64(e.Val4) + due)
			return
		}
	case SCGospel:
		// The caster's Battle Chant costs 30 HP and 20 SP every 10
		// seconds, or 45 HP and 35 SP above level 5.
		if e.Val4 == BCTSelf {
			e.Val2--
			if e.Val2 > 0 {
				hp, sp := int64(30), int64(20)
				if e.Val1 > 5 {
					hp, sp = 45, 35
				}
				if !en.charge(u, hp, sp) {
					break
				}
				tick(10000 + due)
				return
			}
		}
	case SCStomachache:
		e.Val4--
		if e.Val4 >= 0 {
			// 8 SP every 10 seconds. rAthena also makes a player sit,
			// which waits for sitting.
			en.charge(u, 0, int64(e.Val2))
			tick(10000 + due)
			return
		}
	}

	if interval > 0 && sc.Get(typ) == e {
		en.env.Ticked(u, e)
	}
	// With an interval and at least 100 ms left, wait for the next one.
	if interval > 0 && sc.Get(typ) == e && e.Val4 >= 100 {
		next(min(int64(e.Val4), interval) + due)
		e.Val4 -= int(interval)
		return
	}
	en.end(u, typ, true, ReasonExpired)
}

// charge is status_charge: a player pays HP and SP, as status_damage
// with flags 1 and 2. It fails without enough, or with nothing to pay.
// Other units always pay.
func (en *Engine) charge(u Unit, hp, sp int64) bool {
	if u.Kind() != KindPlayer {
		return true
	}
	if hp > 0 && hp >= u.HP() {
		return false
	}
	if sp > u.SP() {
		return false
	}
	if hp == 0 && sp == 0 {
		return false
	}
	en.env.Damage(u, hp, sp, true, true)
	return true
}

// percentDamage is status_percent_damage with a negative rate and kill
// false: a percent of max HP, at least 1, that never kills.
func (en *Engine) percentDamage(u Unit, pct int64) {
	maxHP := int64(u.Status().MaxHP)
	var hp int64
	if maxHP > 100000 {
		hp = maxHP / 100 * pct
	} else {
		hp = maxHP * pct / 100
	}
	if hp == 0 {
		hp = 1
	}
	if hp >= u.HP() {
		hp = u.HP() - 1
	}
	en.env.Damage(u, hp, 0, true, true)
}
