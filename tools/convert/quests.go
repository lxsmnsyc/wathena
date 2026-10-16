package convert

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/quest"
)

// Quests (proposal 0002, review decision 45): rAthena's quest_db.yml,
// read from the quests.json extract, becomes core's data/quests.yml.
//
//   - A record's name is the quest's ID as text, such as "7104", so
//     converted scripts and mods name core's quests by their rAthena IDs.
//   - title, time_limit, targets and drops keep rAthena's values.
//     Monsters and items keep their AegisNames. Races, sizes and elements
//     take the readable names of wa.races, wa.sizes and wa.elements.
//   - A TimeLimit with "+" becomes a duration, such as 3h. One without
//     becomes { at, days } or { at, weekday }.

// QuestData is the quests.json extract.
type QuestData struct {
	Mode   string       `json:"mode"`
	Quests []questEntry `json:"quests"`
	// Skipped are the IDs of entries rAthena refuses.
	Skipped []int `json:"skipped"`
}

type questEntry struct {
	ID        uint32 `json:"id"`
	Title     string `json:"title"`
	TimeLimit *struct {
		Seconds int64  `json:"seconds"`
		At      *int   `json:"at"`
		Days    int    `json:"days"`
		Weekday string `json:"weekday"`
	} `json:"time_limit"`
	Targets []struct {
		Mob      string `json:"mob"`
		Count    int    `json:"count"`
		MinLevel int    `json:"min_level"`
		MaxLevel int    `json:"max_level"`
		Race     string `json:"race"`
		Size     string `json:"size"`
		Element  string `json:"element"`
		Location string `json:"location"`
		MapName  string `json:"map_name"`
		Mobs     []struct {
			Name string `json:"name"`
		} `json:"mobs"`
	} `json:"targets"`
	Drops []struct {
		Mob   string `json:"mob"`
		Item  string `json:"item"`
		Count int    `json:"count"`
		Rate  int    `json:"rate"`
	} `json:"drops"`
}

// QuestResult is the converted quests of one mode.
type QuestResult struct {
	Mode string
	// Records are the quests, by ID, as data/quests.yml writes them.
	Records []QuestRecord
	// Timed holds the IDs of the quests with a time limit. The NPC
	// converter reads it for checkquest (npcquest.go).
	Timed map[uint32]bool
	// Skipped are the IDs of entries rAthena refuses, which core leaves
	// out too.
	Skipped []int

	ids map[uint32]bool
}

// QuestRecord is one record of data/quests.yml, in its field order.
type QuestRecord struct {
	Name      string        `yaml:"name"`
	Title     string        `yaml:"title"`
	TimeLimit any           `yaml:"time_limit,omitempty"`
	Targets   []QuestTarget `yaml:"targets,omitempty"`
	Drops     []QuestDrop   `yaml:"drops,omitempty"`
}

// QuestTarget is a hunt objective of a record.
type QuestTarget struct {
	Monster  string   `yaml:"monster,omitempty"`
	Count    int      `yaml:"count"`
	MinLevel int      `yaml:"min_level,omitempty"`
	MaxLevel int      `yaml:"max_level,omitempty"`
	Race     string   `yaml:"race,omitempty"`
	Size     string   `yaml:"size,omitempty"`
	Element  string   `yaml:"element,omitempty"`
	Map      string   `yaml:"map,omitempty"`
	MapName  string   `yaml:"map_name,omitempty"`
	Monsters []string `yaml:"monsters,omitempty,flow"`
}

// QuestDrop is an item drop of a record.
type QuestDrop struct {
	Monster string `yaml:"monster,omitempty"`
	Item    string `yaml:"item"`
	Count   int    `yaml:"count,omitempty"`
	Rate    int    `yaml:"rate"`
}

// ConvertQuests converts a quests.json extract.
func ConvertQuests(r io.Reader) (*QuestResult, error) {
	var d QuestData
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return nil, fmt.Errorf("quests.json: %w", err)
	}
	out := &QuestResult{Mode: d.Mode, Timed: map[uint32]bool{}, Skipped: d.Skipped}
	for _, q := range d.Quests {
		rec := QuestRecord{Name: strconv.FormatUint(uint64(q.ID), 10), Title: q.Title}
		if l := q.TimeLimit; l != nil {
			out.Timed[q.ID] = true
			switch {
			case l.Seconds > 0:
				rec.TimeLimit = quest.DurationText(l.Seconds)
			case l.At != nil:
				lim := quest.Limit{At: *l.At, HasAt: true, Days: l.Days}
				if l.Weekday != "" {
					for i, w := range quest.Weekdays {
						if w == l.Weekday {
							lim.Weekday, lim.HasWeekday = time.Weekday(i), true
						}
					}
				}
				rec.TimeLimit = quest.LimitText(lim)
			}
		}
		for _, t := range q.Targets {
			qt := QuestTarget{Monster: t.Mob, Count: t.Count, MinLevel: t.MinLevel, MaxLevel: t.MaxLevel, Map: t.Location, MapName: t.MapName}
			if t.Race != "" {
				qt.Race = constants.Readable("RC_"+strings.ToUpper(t.Race), "RC_")
			}
			if t.Size != "" {
				qt.Size = strings.ToLower(t.Size)
			}
			if t.Element != "" {
				qt.Element = strings.ToLower(t.Element)
			}
			for _, m := range t.Mobs {
				qt.Monsters = append(qt.Monsters, m.Name)
			}
			rec.Targets = append(rec.Targets, qt)
		}
		for _, dr := range q.Drops {
			qd := QuestDrop{Monster: dr.Mob, Item: dr.Item, Rate: dr.Rate}
			if dr.Count > 1 {
				qd.Count = dr.Count
			}
			rec.Drops = append(rec.Drops, qd)
		}
		out.Records = append(out.Records, rec)
	}
	return out, nil
}

// Files returns core's data/quests.yml.
func (r *QuestResult) Files() map[string]string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Core's quests, converted from rAthena's quest_db.yml (%s) by tools/convert.\n", r.Mode)
	b.WriteString("# A record's name is the quest's rAthena ID.\n")
	if len(r.Records) > 0 {
		data, err := yaml.Marshal(r.Records)
		if err != nil {
			panic(fmt.Errorf("convert: quests: %w", err))
		}
		b.Write(data)
	}
	return map[string]string{quest.File: b.String()}
}

// Report is the coverage report of the quests.
func (r *QuestResult) Report() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Quests: %s\n\n", r.Mode)
	b.WriteString("Generated by `wactl convert quests`. Do not edit.\n\n")
	b.WriteString("rAthena's `quest_db.yml` becomes core's `data/quests.yml` (proposal 0002, review decision 45, [decision 0043](../decisions/0043-quests.md)).\n\n")
	var timed, targets, filters, drops int
	for _, q := range r.Records {
		if q.TimeLimit != nil {
			timed++
		}
		if len(q.Targets) > 0 {
			targets++
		}
		for _, t := range q.Targets {
			if t.Monster == "" {
				filters++
			}
		}
		if len(q.Drops) > 0 {
			drops++
		}
	}
	b.WriteString("| Count | Quests |\n|---|---:|\n")
	fmt.Fprintf(&b, "| Converted | %d |\n", len(r.Records))
	fmt.Fprintf(&b, "| With a time limit | %d |\n", timed)
	fmt.Fprintf(&b, "| With hunt objectives | %d |\n", targets)
	fmt.Fprintf(&b, "| Objectives with filters | %d |\n", filters)
	fmt.Fprintf(&b, "| With item drops | %d |\n", drops)
	fmt.Fprintf(&b, "| Refused by rAthena, left out | %d |\n", len(r.Skipped))
	return b.String()
}
