package progression

// Penalty types, as the Type names of rAthena's level_penalty.yml in
// lower case.
const (
	PenaltyExp     = "exp"
	PenaltyDrop    = "drop"
	PenaltyMVPExp  = "mvpexp"
	PenaltyMVPDrop = "mvpdrop"
)

// Monster modes and classes this package reads, as rAthena's e_mode and
// e_aegis_monsterclass.
const (
	ModeMVP           = 0x0080000
	ModeFixedItemDrop = 0x1000000
	ClassBoss         = 1
)

// BossType is rAthena's e_mob_bosstype.
type BossType int

const (
	BossNone BossType = iota
	BossMini
	BossMVP
)

// Mob is what a kill reads about a monster.
type Mob struct {
	ID    uint32
	Level int
	MaxHP int
	// BaseExp and JobExp are the exp after BaseExpRate and JobExpRate. Use
	// Tables.MobExp to scale them.
	BaseExp, JobExp int64
	Mode            uint32
	Class           int
	// Drops are the drops after Tables.AdjustDrops.
	Drops []Drop
}

// Boss is s_mob_db::get_bosstype.
func (m *Mob) Boss() BossType {
	switch {
	case m.Mode&ModeMVP == ModeMVP:
		return BossMVP
	case m.Class == ClassBoss:
		return BossMini
	}
	return BossNone
}

// fillPenalty is PenaltyDatabase::loadingFinished. Unset rates (-1) take
// the nearest set rate toward a difference of 0, and 100 when there is
// none. Index MaxLevel-1 is a difference of 0.
func fillPenalty(p *[2*MaxLevel - 1]int) {
	last := 100
	for i := MaxLevel - 1; i < len(p); i++ {
		if p[i] < 0 {
			p[i] = last
		} else {
			last = p[i]
		}
	}
	// This pass starts at difference 0 again, which the first pass set.
	last = 100
	for i := MaxLevel - 1; i >= 0; i-- {
		if p[i] < 0 {
			p[i] = last
		} else {
			last = p[i]
		}
	}
}

// Penalty is pc_level_penalty_mod: the percent of exp or drop rate a
// player of a base level gets from a monster. It is 100 for a type the
// mode's level_penalty.yml does not list, so always 100 in pre-renewal,
// and for drops from monsters with MD_FIXEDITEMDROP.
func (t *Tables) Penalty(typ string, playerLevel int, m *Mob) int {
	if (typ == PenaltyDrop || typ == PenaltyMVPDrop) && m.Mode&ModeFixedItemDrop != 0 {
		return 100
	}
	p := t.penalty[typ]
	if p == nil {
		return 100
	}
	// rAthena does not check the bounds. Differences beyond MaxLevel take
	// the last rate.
	i := min(max(m.Level-playerLevel+MaxLevel-1, 0), len(p)-1)
	return p[i]
}
