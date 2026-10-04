package social

import "slices"

// Guild skill IDs, as rAthena's GD_ skills from GD_SKILLBASE.
const (
	SkillApproval          = 10000
	SkillKafraContract     = 10001
	SkillGuardResearch     = 10002
	SkillGuardUp           = 10003
	SkillExtension         = 10004
	SkillGloryGuild        = 10005
	SkillLeadership        = 10006
	SkillGloryWounds       = 10007
	SkillSoulCold          = 10008
	SkillHawkEyes          = 10009
	SkillBattleOrder       = 10010
	SkillRegeneration      = 10011
	SkillRestore           = 10012
	SkillEmergencyCall     = 10013
	SkillDevelopment       = 10014
	SkillItemEmergencyCall = 10015
	SkillGuildStorage      = 10016
	SkillChargeShoutFlag   = 10017
	SkillChargeShoutBeat   = 10018
	SkillEmergencyMove     = 10019
)

// IsGuildSkill reports whether a skill ID is a guild skill, as
// SKILL_CHK_GUILD with renewal's MAX_GUILDSKILL.
func IsGuildSkill(id int) bool { return id >= SkillApproval && id < SkillApproval+20 }

// MaxGuildLevel is MAX_GUILDLEVEL.
const MaxGuildLevel = 50

// MaxGuildExp is MAX_GUILD_EXP: a guild's and a member's exp stop there.
const MaxGuildExp = 1<<31 - 1

// GuildSkillDef is one skill of the guild skill tree, as
// guild_skill_tree.yml.
type GuildSkillDef struct {
	ID  int
	Max int
	// Needs are the skills and levels it needs.
	Needs []GuildSkillNeed
}

// GuildSkillNeed is one requirement of a guild skill.
type GuildSkillNeed struct {
	ID, Level int
}

// GuildTables are a world's guild data: exp_guild.yml and
// guild_skill_tree.yml.
type GuildTables struct {
	// NextExp holds, for each level from 1, the exp it needs to reach
	// the next level. A level without an entry is the top.
	NextExp []int64
	// Skills are the guild skill tree in file order.
	Skills []GuildSkillDef
}

// Next is the exp a level needs to reach the next one, or 0 at the top,
// as GuildExpDatabase::get_nextexp.
func (t *GuildTables) Next(level int) int64 {
	if level < 1 || level > len(t.NextExp) {
		return 0
	}
	return t.NextExp[level-1]
}

// Skill returns a skill of the tree, or nil.
func (t *GuildTables) Skill(id int) *GuildSkillDef {
	i := slices.IndexFunc(t.Skills, func(d GuildSkillDef) bool { return d.ID == id })
	if i < 0 {
		return nil
	}
	return &t.Skills[i]
}

// MaxLevel is guild_skill_get_max: a skill's maximum level, or 0 for a
// skill outside the tree.
func (t *GuildTables) MaxLevel(id int) int {
	if d := t.Skill(id); d != nil {
		return d.Max
	}
	return 0
}

// CoreGuildTables are rAthena's db/re or db/pre-re exp_guild.yml and
// guild_skill_tree.yml. The parity test checks them against the
// extracts.
func CoreGuildTables(renewal bool) *GuildTables {
	if renewal {
		return &renewalGuildTables
	}
	return &preRenewalGuildTables
}

var renewalGuildTables, preRenewalGuildTables = func() (GuildTables, GuildTables) {
	re := make([]int64, 49)
	for i := range re {
		l := int64(i + 1)
		re[i] = l * l * 100000
	}
	pre := []int64{2000000, 4000000, 8000000, 14000000, 22000000, 32000000, 44000000, 58000000, 74000000, 92000000,
		112000000, 134000000, 158000000, 184000000, 212000000, 242000000, 274000000, 308000000, 344000000, 382000000,
		422000000, 464000000, 508000000, 554000000, 602000000, 652000000, 704000000, 758000000, 814000000, 872000000,
		932000000, 994000000, 1058000000, 1124000000, 1192000000, 1262000000, 1334000000, 1408000000, 1484000000,
		1562000000, 1642000000, 1724000000, 1808000000, 1894000000, 1999999999, 1999999999, 1999999999, 1999999999,
		1999999999}
	tree := []GuildSkillDef{
		{ID: SkillApproval, Max: 1},
		{ID: SkillKafraContract, Max: 1, Needs: []GuildSkillNeed{{SkillApproval, 1}}},
		{ID: SkillGuardResearch, Max: 1, Needs: []GuildSkillNeed{{SkillApproval, 1}}},
		{ID: SkillGuardUp, Max: 3},
		{ID: SkillExtension, Max: 10},
		// require_glory_guild is off, so Glory of Guild stays at 0.
		{ID: SkillGloryGuild, Max: 0},
		{ID: SkillLeadership, Max: 5},
		{ID: SkillGloryWounds, Max: 5},
		{ID: SkillSoulCold, Max: 5, Needs: []GuildSkillNeed{{SkillGloryWounds, 1}}},
		{ID: SkillHawkEyes, Max: 5, Needs: []GuildSkillNeed{{SkillLeadership, 1}}},
		{ID: SkillBattleOrder, Max: 1, Needs: []GuildSkillNeed{{SkillApproval, 1}, {SkillExtension, 2}}},
		{ID: SkillRegeneration, Max: 3, Needs: []GuildSkillNeed{{SkillApproval, 1}, {SkillExtension, 5}, {SkillBattleOrder, 1}}},
		{ID: SkillRestore, Max: 1, Needs: []GuildSkillNeed{{SkillRegeneration, 1}}},
		{ID: SkillEmergencyCall, Max: 1, Needs: []GuildSkillNeed{{SkillApproval, 1}, {SkillGuardResearch, 1}, {SkillExtension, 5},
			{SkillBattleOrder, 1}, {SkillRegeneration, 1}}},
		{ID: SkillDevelopment, Max: 1},
	}
	reTree := append(slices.Clone(tree),
		GuildSkillDef{ID: SkillGuildStorage, Max: 5, Needs: []GuildSkillNeed{{SkillEmergencyCall, 1}, {SkillExtension, 10},
			{SkillSoulCold, 1}, {SkillHawkEyes, 1}}},
		GuildSkillDef{ID: SkillChargeShoutFlag, Max: 1, Needs: []GuildSkillNeed{{SkillEmergencyCall, 1}}},
		GuildSkillDef{ID: SkillChargeShoutBeat, Max: 1, Needs: []GuildSkillNeed{{SkillChargeShoutFlag, 1}}},
		GuildSkillDef{ID: SkillEmergencyMove, Max: 1, Needs: []GuildSkillNeed{{SkillLeadership, 1}, {SkillGloryWounds, 1}}},
	)
	return GuildTables{NextExp: re, Skills: reTree}, GuildTables{NextExp: pre, Skills: tree}
}()
