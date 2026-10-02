package skill

import (
	"fmt"
	"slices"
)

// Skill trees: which skills a job shows and may learn, as rAthena's
// skill_tree.yml, pc_calc_skilltree and pc_skillup.

// TreeData is skill_trees.json: skill_tree.yml as SkillTreeDatabase loads
// it, with inherited skills.
type TreeData struct {
	Mode  string `json:"mode"`
	Trees []struct {
		Job     int         `json:"job"`
		Inherit []int       `json:"inherit"`
		Skills  []TreeSkill `json:"skills"`
	} `json:"trees"`
}

// TreeSkill is one skill of a job's tree: s_skill_tree_entry.
type TreeSkill struct {
	Skill     int    `json:"skill"`
	Name      string `json:"name"`
	MaxLevel  int    `json:"max_level"`
	BaseLevel int    `json:"base_level"`
	JobLevel  int    `json:"job_level"`
	// Requires lists [skill, level] pairs the skill needs.
	Requires [][2]int `json:"requires"`
	Exclude  bool     `json:"exclude"`
}

// Tree is one job's skill tree.
type Tree struct {
	Job int
	// Skills are by skill ID, in ID order.
	Skills  []*TreeSkill
	bySkill map[int]*TreeSkill
}

// Skill returns a skill of the tree, or nil.
func (t *Tree) Skill(id int) *TreeSkill {
	if t == nil {
		return nil
	}
	return t.bySkill[id]
}

// Trees are the skill trees of every job.
type Trees struct {
	byJob map[int]*Tree
}

// LoadTrees builds the trees from skill_trees.json.
func LoadTrees(d *TreeData) *Trees {
	t := &Trees{byJob: map[int]*Tree{}}
	for _, tr := range d.Trees {
		tree := &Tree{Job: tr.Job, bySkill: map[int]*TreeSkill{}}
		for i := range tr.Skills {
			s := tr.Skills[i]
			tree.Skills = append(tree.Skills, &s)
			tree.bySkill[s.Skill] = &s
		}
		t.byJob[tr.Job] = tree
	}
	return t
}

// Job returns a job's tree, or nil.
func (t *Trees) Job(job int) *Tree {
	if t == nil {
		return nil
	}
	return t.byJob[job]
}

// Set changes a job's tree entry for a skill, or adds it, as a mod's
// data/skill_trees.yml does.
func (t *Trees) Set(job int, s TreeSkill) {
	tree := t.byJob[job]
	if tree == nil {
		tree = &Tree{Job: job, bySkill: map[int]*TreeSkill{}}
		t.byJob[job] = tree
	}
	if old := tree.bySkill[s.Skill]; old != nil {
		*old = s
		return
	}
	e := &s
	tree.bySkill[s.Skill] = e
	tree.Skills = append(tree.Skills, e)
	slices.SortFunc(tree.Skills, func(a, b *TreeSkill) int { return a.Skill - b.Skill })
}

// MaxLevel is skill_tree_get_max: a skill's maximum level in a job's
// tree, or the skill's own maximum when the tree does not have it.
func (t *Trees) MaxLevel(db *DB, skill, job int) int {
	if e := t.Job(job).Skill(skill); e != nil {
		return e.MaxLevel
	}
	if d := db.Get(skill); d != nil {
		return d.MaxLevel
	}
	return 0
}

// Jobs answers the job questions of the trees from the job tables.
type Jobs interface {
	// MapID is pc_jobid2mapid.
	MapID(job int) (uint64, bool)
	// JobOf is pc_mapid2jobid.
	JobOf(mapID uint64, male bool) (int, bool)
	// MaxJobLevel is a job's max_job_level.
	MaxJobLevel(job int) int
}

// Learner is what pc_calc_skilltree reads of a player.
type Learner struct {
	Job                 int
	Male                bool
	BaseLevel, JobLevel int
	// Learned are the skills learned with skill points, by ID.
	Learned map[int]int
	// ChangeLevel2nd is change_level_2nd: the job level the player had
	// when it changed from a first class to a second class, or 0 when it
	// is not known, which counts the first class's maximum job level.
	ChangeLevel2nd int
}

// Settings are the conf/battle/player.conf options of skill trees.
type Settings struct {
	// QuestSkillLearn is quest_skill_learn: quest skills are learned with
	// skill points. It is off by default: quest skills come from quests.
	QuestSkillLearn bool
	// SkillFree is player_skillfree: skills need no other skills.
	SkillFree bool
	// NoSkillupLimit turns off player_skillup_limit, which is on by
	// default: a job sees the tree of its earlier job until it spent
	// that job's skill points.
	NoSkillupLimit bool
}

// Map job ID flags and masks, as rAthena's e_mapid.
const (
	mapidFirstMask  = 0xff
	mapidSecondMask = 0xfff
	mapidThirdMask  = 0xffff
	mapidNovice     = 0x0
	mapidSummoner   = 0xa
	joblSecond1     = 0x100
	joblSecond2     = 0x200
	joblThird       = 0x1000
	joblFourth      = 0x10000
	joblUpper       = 0x100000
	joblBaby        = 0x200000
	jobNovice       = 0
)

// spentPoints is pc_calc_skillpoint: the skill points the learned
// skills took. Quest skills count only when they are learned with
// points, and wedding skills never count.
func spentPoints(db *DB, l *Learner, set Settings) int {
	n := 0
	for id, lv := range l.Learned {
		d := db.Get(id)
		if d == nil || lv <= 0 {
			continue
		}
		if (!d.Has("IsQuest") || set.QuestSkillLearn) && (!d.Has("IsWedding") || d.Has("IsSpirit")) {
			n += lv
		}
	}
	return n
}

// treeJob is pc_calc_skilltree_normalize_job with pc_mapid2jobid: the
// job whose tree the player sees. With player_skillup_limit, a job sees
// the tree of the job before it until it spent that job's points: a
// first class needs the Novice's 9 points, and a second class the first
// class's, up to the job level it changed jobs at (change_level_2nd).
func treeJob(db *DB, jobs Jobs, l *Learner, set Settings) (int, error) {
	mapID, ok := jobs.MapID(l.Job)
	if !ok {
		return 0, fmt.Errorf("skill: job %d has no map job ID", l.Job)
	}
	c := mapID
	if !set.NoSkillupLimit {
		c = normalizeJob(db, jobs, l, mapID, set)
	}
	job, ok := jobs.JobOf(c, l.Male)
	if !ok {
		return 0, fmt.Errorf("skill: map job ID %#x has no job", c)
	}
	return job, nil
}

// normalizeJob is pc_calc_skilltree_normalize_job and its _sub for the
// jobs up to the third classes.
func normalizeJob(db *DB, jobs Jobs, l *Learner, mapID uint64, set Settings) uint64 {
	points := spentPoints(db, l, set)
	sub := func() uint64 {
		if mapID&mapidFirstMask != mapidSummoner {
			novice := jobs.MaxJobLevel(jobNovice) - 1
			if points < novice {
				return mapidNovice
			}
			points -= novice
		}
		if mapID&(joblSecond1|joblSecond2) != 0 && mapID&mapidSecondMask != joblSecond1|mapidNovice {
			first := mapID & mapidFirstMask
			if job, ok := jobs.JobOf(first, l.Male); ok && job > 0 {
				change := l.ChangeLevel2nd
				if change == 0 {
					change = jobs.MaxJobLevel(job)
				}
				if points < change-1 {
					return first
				}
				points -= change - 1
			}
		}
		if mapID&joblThird != 0 {
			second := mapID & mapidSecondMask
			if job, ok := jobs.JobOf(second, l.Male); ok && job > 0 {
				change := jobs.MaxJobLevel(job)
				if points < change-1 {
					return second
				}
			}
		}
		return mapID
	}
	c := sub()
	if mapID&joblUpper != 0 {
		if _, ok := jobs.JobOf(c|joblUpper, l.Male); ok {
			c |= joblUpper
		}
	}
	if mapID&joblBaby != 0 {
		if _, ok := jobs.JobOf(c|joblBaby, l.Male); ok {
			c |= joblBaby
		}
	}
	return c
}

// Entry is one skill of a player's skill list: its ID, the level learned,
// and whether a point raises it.
type Entry struct {
	Skill, Level int
	Upgradable   bool
}

// List is pc_calc_skilltree followed by the levels clif_skillinfoblock
// sends: every skill the player learned, and every skill of its tree
// whose requirements it meets, in skill ID order. Quest skills it has not
// learned stay out, unless quest_skill_learn is on.
func (t *Trees) List(db *DB, jobs Jobs, l *Learner, set Settings) ([]Entry, error) {
	job, err := treeJob(db, jobs, l, set)
	if err != nil {
		return nil, err
	}
	tree := t.Job(job)
	known := map[int]bool{}
	for id, lv := range l.Learned {
		if lv > 0 {
			known[id] = true
		}
	}
	if tree != nil {
		for changed := true; changed; {
			changed = false
			for _, e := range tree.Skills {
				if known[e.Skill] {
					continue
				}
				if !set.SkillFree && !meets(e, l) {
					continue
				}
				d := db.Get(e.Skill)
				if d == nil {
					continue
				}
				if l.Learned[e.Skill] == 0 && (d.Has("IsQuest") && !set.QuestSkillLearn || d.Has("IsWedding") || d.Has("IsSpirit")) {
					// Learned by other means only.
					continue
				}
				known[e.Skill] = true
				changed = true
			}
		}
	}
	var out []Entry
	for id := range known {
		lv := l.Learned[id]
		out = append(out, Entry{Skill: id, Level: lv, Upgradable: lv < t.MaxLevel(db, id, l.Job)})
	}
	slices.SortFunc(out, func(a, b Entry) int { return a.Skill - b.Skill })
	return out, nil
}

// meets is pc_calc_skilltree's check of one tree entry: the required
// skills at their levels, and the base and job levels.
func meets(e *TreeSkill, l *Learner) bool {
	for _, need := range e.Requires {
		if l.Learned[need[0]] < need[1] {
			return false
		}
	}
	return l.BaseLevel >= e.BaseLevel && l.JobLevel >= e.JobLevel
}

// SkillUp is pc_skillup's check: whether a skill point may raise a skill.
// The skill must be in the player's list, and below its maximum level in
// the job's tree.
func (t *Trees) SkillUp(db *DB, jobs Jobs, l *Learner, set Settings, points, skill int) bool {
	if points <= 0 {
		return false
	}
	list, err := t.List(db, jobs, l, set)
	if err != nil {
		return false
	}
	for _, e := range list {
		if e.Skill == skill {
			return e.Level < t.MaxLevel(db, skill, l.Job)
		}
	}
	return false
}
