package statuschange

import (
	"maps"
	"slices"
)

// InfiniteTick is rAthena's INFINITE_TICK: a status that does not end by
// itself.
const InfiniteTick = -1

// Entry is one active status: status_change_entry.
type Entry struct {
	Type                   int
	Val1, Val2, Val3, Val4 int
	// timer is when status_change_timer next runs for the status, in
	// zone milliseconds, or -1 for none (INVALID_TIMER): the status
	// lasts until something ends it. seq tells the engine's current timer
	// from older ones.
	timer int64
	seq   uint64
	// End is when the status ends, in zone milliseconds, or -1 for
	// never. For a status that ticks, the timer runs before it.
	End int64
	// Source is who caused the status, or the zero Source.
	Source Source
	// Reason is why the status started.
	Reason Reason
	// HideIcon is SCSTART_NOICON: the status sends no icon.
	HideIcon bool
	// Bonuses are the bonuses of a mod status, computed when it started.
	Bonuses []Bonus
}

// Values returns val1 to val4.
func (e *Entry) Values() [4]int { return [4]int{e.Val1, e.Val2, e.Val3, e.Val4} }

// Remaining is how long the status still lasts at a time, or -1 for a
// status without an end.
func (e *Entry) Remaining(now int64) int64 {
	if e.End < 0 {
		return InfiniteTick
	}
	return max(e.End-now, 0)
}

// Source is the unit that caused a status: its kind and the zone's
// handle for it.
type Source struct {
	Kind Kind
	ID   int64
}

// Kind is the kind of a unit, as rAthena's bl_type.
type Kind uint8

const (
	KindNone Kind = iota
	KindPlayer
	KindMonster
	KindNPC
)

// String is the kind's name in the Lua API.
func (k Kind) String() string {
	switch k {
	case KindPlayer:
		return "player"
	case KindMonster:
		return "monster"
	case KindNPC:
		return "npc"
	}
	return ""
}

// Reason says why a status started or ended (proposal 0002, review
// decision 19).
type Reason string

// Reasons a status starts.
const (
	ReasonSkill  Reason = "skill"
	ReasonItem   Reason = "item"
	ReasonBonus  Reason = "bonus"
	ReasonScript Reason = "script"
	ReasonLogin  Reason = "login"
)

// Reasons a status ends.
const (
	ReasonExpired   Reason = "expired"
	ReasonRemoved   Reason = "removed"
	ReasonReplaced  Reason = "replaced"
	ReasonDeath     Reason = "death"
	ReasonDispell   Reason = "dispell"
	ReasonClearance Reason = "clearance"
	ReasonLogout    Reason = "logout"
	ReasonDamage    Reason = "damage"
)

// Cant is status_change's cant: what the unit's statuses keep it from
// doing.
type Cant struct {
	Move, PickUp, Drop, Cast, Chat, Equip, Unequip, Consume, Attack, Warp, DeathPenalty, Interact bool
}

// List is a unit's status_change: its active statuses, and the options
// they show.
type List struct {
	entries map[int]*Entry
	// Opt1 is the body state (OPT1_), Opt2 the health state bits
	// (OPT2_), Opt3 the effect bits (OPT3_), and Option the option bits
	// (OPTION_) the statuses give.
	Opt1, Opt2, Opt3, Option int
	// Cant is what the statuses keep the unit from doing.
	Cant Cant
	// lastEffect is the opt1 status that damage just ended, which keeps
	// the same attack from starting a status its Fail list names.
	lastEffect   int
	lastEffectAt int64
}

// NewList returns an empty list.
func NewList() *List { return &List{entries: map[int]*Entry{}, lastEffect: -1} }

// Get returns an active status, or nil.
func (l *List) Get(t int) *Entry {
	if l == nil {
		return nil
	}
	return l.entries[t]
}

// Has reports whether a status is active.
func (l *List) Has(t int) bool { return l.Get(t) != nil }

// Empty reports whether no status is active.
func (l *List) Empty() bool { return l == nil || len(l.entries) == 0 }

// Len is the number of active statuses.
func (l *List) Len() int {
	if l == nil {
		return 0
	}
	return len(l.entries)
}

// Sorted returns the active statuses by ID, so walks happen in the same
// order every time.
func (l *List) Sorted() []*Entry {
	if l == nil {
		return nil
	}
	keys := slices.Sorted(maps.Keys(l.entries))
	out := make([]*Entry, len(keys))
	for i, k := range keys {
		out[i] = l.entries[k]
	}
	return out
}

// val returns a value of an active status, or 0.
func (l *List) val(t, n int) int {
	e := l.Get(t)
	if e == nil {
		return 0
	}
	return e.Values()[n-1]
}

// Val1 to Val4 return a value of an active status, or 0 when it is not
// active.
func (l *List) Val1(t int) int { return l.val(t, 1) }
func (l *List) Val2(t int) int { return l.val(t, 2) }
func (l *List) Val3(t int) int { return l.val(t, 3) }
func (l *List) Val4(t int) int { return l.val(t, 4) }

// calcState is status_calc_state for every state: a state holds while
// any active status has it. songs is the ensemble_songs flag, which
// picks SC_DANCING's conditions.
func (l *List) calcState(db *DB, songs bool) {
	var s States
	for _, e := range l.entries {
		if d := db.Get(e.Type); d != nil {
			s |= d.States
		}
	}
	l.Cant = Cant{
		Move: s.Has(StateNoMove), PickUp: s.Has(StateNoPickItem), Drop: s.Has(StateNoDropItem),
		Cast: s.Has(StateNoCast), Chat: s.Has(StateNoChat), Equip: s.Has(StateNoEquipItem),
		Unequip: s.Has(StateNoUnequipItem), Consume: s.Has(StateNoConsumeItem), Attack: s.Has(StateNoAttack),
		Warp: s.Has(StateNoWarp), DeathPenalty: s.Has(StateNoDeathPenalty), Interact: s.Has(StateNoInteract),
	}
	// status_calc_state_sub's conditions: the caster of a pre-renewal
	// Gravitation Field (val3 BCT_SELF) cannot move or use items.
	if e := l.Get(SCGravitation); e != nil && e.Val3 == bctSelf {
		l.Cant.Move, l.Cant.Consume = true, true
	}
	// SC_DANCING's conditions. An ensemble (val4 set) keeps its
	// performers in place: in renewal only for Moonlit Water Mill and
	// Wand of Hermode, in pre-renewal for every one unless Longing for
	// Freedom is on. In pre-renewal a song stops attacks, as does an
	// ensemble without Longing for Freedom.
	if e := l.Get(SCDancing); e != nil {
		id := e.Val1 & 0xFFFF
		special := id == skillMoonlit || id == skillHermode
		if e.Val4 != 0 && (songs && special || !songs && (!l.Has(SCLonging) || special)) {
			l.Cant.Move = true
		}
		if !songs && (e.Val4 == 0 || !l.Has(SCLonging)) {
			l.Cant.Attack = true
		}
	}
}

// bctSelf is BCT_SELF.
const bctSelf = 0x010000

// Regen is the status part of status_calc_regen_rate: whether natural HP
// and SP regen may run. The rate changes of skills come with them.
func (l *List) Regen() (hp, sp bool) {
	if l.Empty() {
		return true, true
	}
	if (l.Has(SCPoison) && !l.Has(SCSlowpoison)) || (l.Has(SCDpoison) && !l.Has(SCSlowpoison)) ||
		l.Has(SCBerserk) || l.Has(SCTrickdead) || l.Has(SCBleeding) ||
		(l.Has(SCMagicmushroom) && l.Val3(SCMagicmushroom) == 1) || l.Has(SCSaturdaynightfever) ||
		l.Has(SCRebound) || l.Has(SCNorecoverState) {
		return false, false
	}
	sp = !l.Has(SCDancing) && !l.Has(SCMaximizepower) && (!l.Has(SCOblivioncurse) || l.Val3(SCOblivioncurse) != 1)
	return true, sp
}

// RegenRate is the rate part of status_calc_regen_rate: the percent
// speed of natural HP and SP regen. Magnificat doubles SP regen.
func (l *List) RegenRate() (hp, sp int) {
	hp, sp = 100, 100
	if l.Empty() {
		return hp, sp
	}
	if l.Has(SCMagnificat) {
		sp += 100
	}
	return hp, sp
}

// Timer is when the status's timer next runs, in zone milliseconds, or
// -1 for none. Tests compare it with rAthena's timer.
func (e *Entry) Timer() int64 { return e.timer }
