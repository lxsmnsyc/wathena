package convert

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mob"
)

// Monster skills: rAthena's mob_skill_db.txt becomes the skills list of
// core's monster records in data/monsters.yml (proposal 0002, review
// decision 22), with readable names for states, targets, conditions,
// statuses, emotions and summons.

// MonsterInput are the extracts the monster converter reads.
type MonsterInput struct {
	// Monsters is monsters.json, Constants script_constants.json, and
	// Skills skills.json, for the skills that are ground skills.
	Monsters, Constants, Skills io.Reader
}

// MonsterResult is core's converted monster skills of one mode.
type MonsterResult struct {
	Mode string
	// Records are the monsters with skills, in ID order.
	Records []MonsterRecord
	// Entries counts every mob_skill_db row, Full the rows that convert
	// whole, and Partial those that lose a part. Lost counts the parts by
	// kind, such as "the mode change of NPC_EMOTION".
	Entries, Full, Partial int
	Lost                   map[string]int
	// Skills counts the rows of each skill, by name.
	Skills map[string]int
}

// MonsterRecord is one monster's converted skills.
type MonsterRecord struct {
	ID        uint32
	AegisName string
	Skills    []mob.Skill
	// Lines are the YAML of each skill.
	Lines []string
}

// monsterNames resolves the names of data/monsters.yml from the
// extracts.
type monsterNames struct {
	skills   map[string]int
	skillOf  map[int]string
	ground   map[int]bool
	monsters map[string]uint32
	aegis    map[uint32]string
	statuses *constants.Module
	emotions *constants.Module
}

func (n *monsterNames) SkillID(name string) (int, bool) {
	id, ok := n.skills[name]
	return id, ok
}
func (n *monsterNames) SkillName(id int) (string, bool) {
	s, ok := n.skillOf[id]
	return s, ok
}
func (n *monsterNames) Ground(id int) bool { return n.ground[id] }
func (n *monsterNames) MonsterID(name string) (uint32, bool) {
	id, ok := n.monsters[name]
	return id, ok
}
func (n *monsterNames) MonsterName(id uint32) (string, bool) {
	s, ok := n.aegis[id]
	return s, ok
}
func (n *monsterNames) StatusID(name string) (int, bool) {
	v, ok := n.statuses.Values[name]
	return int(v), ok
}
func (n *monsterNames) StatusName(id int) (string, bool) {
	s, ok := n.statuses.Names[int64(id)]
	return s, ok
}
func (n *monsterNames) EmotionID(name string) (int, bool) {
	v, ok := n.emotions.Values[name]
	return int(v), ok
}
func (n *monsterNames) EmotionName(id int) (string, bool) {
	s, ok := n.emotions.Names[int64(id)]
	return s, ok
}

// ConvertMonsters converts the monster skills of a mode.
func ConvertMonsters(in MonsterInput) (*MonsterResult, error) {
	var data mob.Data
	if err := json.NewDecoder(in.Monsters).Decode(&data); err != nil {
		return nil, fmt.Errorf("monsters.json: %w", err)
	}
	consts, err := constants.Read(in.Constants)
	if err != nil {
		return nil, err
	}
	var skills struct {
		Skills []struct {
			ID     int    `json:"id"`
			Name   string `json:"name"`
			Target int    `json:"target"`
		} `json:"skills"`
	}
	if err := json.NewDecoder(in.Skills).Decode(&skills); err != nil {
		return nil, fmt.Errorf("skills.json: %w", err)
	}
	modules := consts.Modules()
	n := &monsterNames{skills: map[string]int{}, skillOf: map[int]string{}, ground: map[int]bool{},
		monsters: map[string]uint32{}, aegis: map[uint32]string{},
		statuses: modules["statuses"], emotions: modules["emotions"]}
	for _, s := range skills.Skills {
		n.skills[s.Name] = s.ID
		n.skillOf[s.ID] = s.Name
		// skill_get_casttype: INF_GROUND_SKILL makes a ground skill.
		n.ground[s.ID] = s.Target&2 != 0
	}
	for _, m := range data.Monsters {
		n.monsters[m.AegisName] = m.ID
		n.aegis[m.ID] = m.AegisName
	}
	chats := map[int]string{}
	for _, c := range data.Chats {
		chats[c.ID] = c.Text
	}
	out := &MonsterResult{Mode: data.Mode, Lost: map[string]int{}, Skills: map[string]int{}}
	for _, m := range data.Monsters {
		if len(m.Skills) == 0 {
			continue
		}
		rec := MonsterRecord{ID: m.ID, AegisName: m.AegisName}
		for _, r := range m.Skills {
			out.Entries++
			out.Skills[r.SkillName]++
			s, lost, err := mob.FromRaw(r, n.Ground, func(id uint32) bool { _, ok := n.aegis[id]; return ok },
				func(id int) string { return chats[id] })
			if err != nil {
				return nil, fmt.Errorf("monster %d (%s): %v", m.ID, m.AegisName, err)
			}
			if _, ok := n.EmotionName(s.Emotion); s.Emotion >= 0 && !ok {
				// A value past ET_MAX, such as a summon that spilled into
				// the Emotion column, shows nothing in rAthena.
				s.Emotion = -1
			}
			line, err := s.YAML(n)
			if err != nil {
				return nil, fmt.Errorf("monster %d (%s): %v", m.ID, m.AegisName, err)
			}
			if len(lost) > 0 {
				out.Partial++
				for _, l := range lost {
					out.Lost[l]++
				}
			} else {
				out.Full++
			}
			rec.Skills = append(rec.Skills, s)
			rec.Lines = append(rec.Lines, line)
		}
		out.Records = append(out.Records, rec)
	}
	return out, nil
}

// Files are core's data/monsters.yml.
func (r *MonsterResult) Files() map[string]string {
	var b strings.Builder
	b.WriteString("# Core's monster skills, converted from rAthena's mob_skill_db.txt.\n")
	b.WriteString("# Each record is a core monster by AegisName. Mods change a monster's\n")
	b.WriteString("# skills by setting skills on the same name.\n")
	for _, rec := range r.Records {
		fmt.Fprintf(&b, "- name: %s\n  skills:\n", rec.AegisName)
		for _, l := range rec.Lines {
			b.WriteString("    - " + l + "\n")
		}
	}
	return map[string]string{"data/monsters.yml": b.String()}
}

// MonsterReport writes the coverage report of the converted monster
// skills as Markdown.
func (r *MonsterResult) MonsterReport() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Monster skill coverage: %s\n\n", r.Mode)
	b.WriteString("Generated by `wactl convert monsters --report`; do not edit. `go test ./tools/convert` checks that it is current.\n\n")
	b.WriteString("Each row of rAthena's `mob_skill_db.txt` becomes an entry of a monster's `skills` in core's `data/monsters.yml`. A row converts fully when the entry holds all it does, and partly when a part is lost. Whether a skill takes effect depends on the server's port of it, which `server/README.md`, \"Monster skills\", lists.\n\n")
	fmt.Fprintf(&b, "| Rows | Monsters | Full | Partial |\n|---:|---:|---:|---:|\n| %d | %d | %d | %d |\n\n", r.Entries, len(r.Records), r.Full, r.Partial)
	if len(r.Lost) > 0 {
		b.WriteString("## Lost parts\n\n| Part | Rows |\n|---|---:|\n")
		var keys []string
		for k := range r.Lost {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "| %s | %d |\n", k, r.Lost[k])
		}
		b.WriteString("\n")
	}
	b.WriteString("## Rows per skill\n\n| Skill | Rows |\n|---|---:|\n")
	type kv struct {
		name string
		n    int
	}
	var list []kv
	for k, v := range r.Skills {
		list = append(list, kv{k, v})
	}
	slices.SortFunc(list, func(a, b kv) int {
		if a.n != b.n {
			return b.n - a.n
		}
		return strings.Compare(a.name, b.name)
	})
	for _, e := range list {
		fmt.Fprintf(&b, "| %s | %d |\n", e.name, e.n)
	}
	return b.String()
}

// MonsterSummary is a one-line summary of the conversion.
func (r *MonsterResult) MonsterSummary() string {
	return fmt.Sprintf("%s: %d monster skill rows of %d monsters, %d full, %d partial", r.Mode, r.Entries, len(r.Records), r.Full, r.Partial)
}
