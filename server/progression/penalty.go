package progression

import "fmt"

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
	// MvpExp is the MVP exp after MvpExpRate.
	MvpExp int64
	Mode   uint32
	Class  int
	// Drops are the drops after Tables.AdjustDrops. MvpDrops are the
	// MVP prizes, which the MVP drop rates scale when they drop.
	Drops, MvpDrops []Drop
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

// penaltyTable is PenaltyDatabase's table of one type, from the rows of
// level_penalty.yml. It returns nil for a type the data does not list.
func penaltyTable(d *Data, typ string) *[2*MaxLevel - 1]int {
	rows, ok := d.LevelPenalty[typ]
	if !ok {
		return nil
	}
	p := &[2*MaxLevel - 1]int{}
	for i := range p {
		p[i] = -1
	}
	for _, r := range rows {
		if abs(r.Difference) > MaxLevel {
			continue
		}
		p[r.Difference+MaxLevel-1] = r.Rate
	}
	fillPenalty(p)
	return p
}

// WithLevelPenalty returns tables whose level penalty follows the
// level_penalty_exp and level_penalty_drop ruleset flags (RENEWAL_EXP and
// RENEWAL_DROP). exp covers the exp and MVP exp types, and drop the drop
// and MVP drop types:
//   - On, a type keeps the base's table. A base without one, as
//     pre-renewal, whose data has no level_penalty.yml, takes it from
//     renewal, renewal's progression data. renewal may be nil when the
//     base has every table it needs. A type neither lists, as the MVP
//     types today, gives 100%.
//   - Off, a type gives 100%, as in pre-renewal.
func (t *Tables) WithLevelPenalty(exp, drop bool, renewal *Data) (*Tables, error) {
	out := *t
	out.penalty = map[string]*[2*MaxLevel - 1]int{}
	for typ, on := range map[string]bool{PenaltyExp: exp, PenaltyMVPExp: exp, PenaltyDrop: drop, PenaltyMVPDrop: drop} {
		if !on {
			continue
		}
		p := t.penalty[typ]
		if p == nil && renewal != nil {
			p = penaltyTable(renewal, typ)
		}
		switch {
		case p != nil:
			out.penalty[typ] = p
		case typ == PenaltyExp || typ == PenaltyDrop:
			return nil, fmt.Errorf("progression: the %s level penalty needs renewal's level_penalty.yml", typ)
		}
	}
	return &out, nil
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
