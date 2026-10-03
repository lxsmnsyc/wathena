package statuschange

// Devotion (CR_DEVOTION): a Crusader's guard statuses pass on to the
// players it protects, and end on them with it.

// Devotions is what an Env may also do for Devotion: the players a
// player protects (sd->devotion), in slot order. The engine passes no
// status on with an Env that does not have it.
type Devotions interface {
	Devoted(u Unit) []Unit
}

// devoted is sd->devotion of a player: the players it protects, or nil.
func (en *Engine) devoted(u Unit) []Unit {
	d, ok := en.env.(Devotions)
	if !ok || u.Kind() != KindPlayer {
		return nil
	}
	return d.Devoted(u)
}

// passToDevoted is the part of status_change_start for SC_ENDURE,
// SC_REFLECTSHIELD, SC_AUTOGUARD and SC_DEFENDER that gives the status
// to each player the unit protects, with no resistance and no icon (but
// SC_DEFENDER shows its icon).
func (v *setup) passToDevoted(typ int, val [4]int) {
	if v.flag&NoAvoid != 0 {
		return
	}
	flag := NoAvoid | NoIcon
	if typ == SCDefender {
		flag = NoAvoid
	}
	for _, t := range v.en.devoted(v.target) {
		v.en.Start(v.src, t, Start{Type: typ, Rate: 10000, Val1: val[0], Val2: val[1], Val3: val[2], Val4: val[3],
			Duration: v.tick, Flag: flag, Reason: v.reason, Source: v.source})
	}
}

// inheritDevotion is SC_DEVOTION's part of status_change_start: the
// devoted player gets the Crusader's SC_ENDURE, SC_REFLECTSHIELD,
// SC_DEFENDER and SC_AUTOGUARD, at their levels, for their skills'
// durations. A Shield Reflect passed on this way has val4 1. GvG and
// battleground maps, which leave SC_ENDURE out, do not exist yet.
func (v *setup) inheritDevotion(val1 int) {
	en := v.en
	d := en.env.UnitOf(Source{Kind: KindPlayer, ID: int64(val1)})
	if d == nil {
		return
	}
	dsc := d.Changes()
	if dsc.Empty() {
		return
	}
	for _, typ := range []int{SCEndure, SCDefender, SCReflectshield, SCAutoguard} {
		e := dsc.Get(typ)
		if e == nil {
			continue
		}
		val4 := 0
		if typ == SCReflectshield {
			val4 = 1
		}
		flag := NoAvoid | NoIcon
		if typ == SCDefender {
			flag = NoAvoid
		}
		dur := int64(0)
		if def := en.DB.Get(typ); def != nil {
			dur = int64(def.Duration(e.Val1))
		}
		en.Start(d, v.target, Start{Type: typ, Rate: 10000, Val1: e.Val1, Val4: val4, Duration: dur, Flag: flag,
			Reason: v.reason, Source: d.Ref()})
	}
}

// endOnDevoted is the part of status_change_end for SC_ENDURE,
// SC_DEFENDER, SC_REFLECTSHIELD and SC_AUTOGUARD on a player: the
// players it protects lose the status too.
func (en *Engine) endOnDevoted(u Unit, typ int, reason Reason) {
	for _, t := range en.devoted(u) {
		if t.Changes().Has(typ) {
			en.End(t, typ, reason)
		}
	}
}

// devoterDied is the part of pc_dead for Devotion: the players the dead
// player protects lose SC_DEVOTION.
func (en *Engine) devoterDied(u Unit) {
	for _, t := range en.devoted(u) {
		en.End(t, SCDevotion, ReasonRemoved)
	}
}
