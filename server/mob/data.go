// Package mob holds monster data, monster stats and the monster AI rules,
// as rAthena's mob.cpp, mob_db.yml, mob_skill_db.txt and the monster parts
// of status.cpp (GPL-3.0). Function comments name the rAthena functions
// they port.
//
// The zone runs the AI on its entities. This package gives it the data,
// the stats, the AI types and states, and the rules of the AI that do
// not need the map: target changes, skill state matches and skill
// conditions.
package mob

import (
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// Def is one monster of mob_db.yml as MobDatabase loads it, before
// status_calc_misc: its base stats, AI and skills. It decodes from an
// entry of extracts/<mode>/monsters.json.
type Def struct {
	ID        uint32 `json:"id"`
	AegisName string `json:"aegis_name"`
	Name      string `json:"name"`
	Level     int    `json:"level"`
	HP        int    `json:"hp"`
	SP        int    `json:"sp"`
	BaseExp   int64  `json:"base_exp"`
	JobExp    int64  `json:"job_exp"`
	MvpExp    int64  `json:"mvp_exp"`
	// Attack is mob_db's Attack. Attack2 is rhw.matk in renewal and
	// rhw.atk2 in pre-renewal.
	Attack          int `json:"attack"`
	Attack2         int `json:"attack2"`
	Defense         int `json:"defense"`
	MagicDefense    int `json:"magic_defense"`
	Resistance      int `json:"resistance"`
	MagicResistance int `json:"magic_resistance"`
	Str             int `json:"str"`
	Agi             int `json:"agi"`
	Vit             int `json:"vit"`
	Int             int `json:"int"`
	Dex             int `json:"dex"`
	Luk             int `json:"luk"`
	AttackRange     int `json:"attack_range"`
	// SkillRange is range2, the view range. ChaseRange is range3.
	SkillRange         int `json:"skill_range"`
	ChaseRange         int `json:"chase_range"`
	Size               int `json:"size"`
	Race               int `json:"race"`
	Element            int `json:"element"`
	ElementLevel       int `json:"element_level"`
	WalkSpeed          int `json:"walk_speed"`
	AttackDelay        int `json:"attack_delay"`
	AttackMotion       int `json:"attack_motion"`
	ClientAttackMotion int `json:"client_attack_motion"`
	DamageMotion       int `json:"damage_motion"`
	DamageTaken        int `json:"damage_taken"`
	// AI is the Aegis AI type, such as "06", or a special one, such as
	// "ABR_PASSIVE".
	AI    string `json:"ai"`
	Class int    `json:"class"`
	// ModesOn and ModesOff are the Modes that mob_db sets and clears over
	// the AI type's modes. Mode is the result with the class's modes.
	ModesOn  []uint32 `json:"modes_on"`
	ModesOff []uint32 `json:"modes_off"`
	Mode     uint32   `json:"mode"`
	// Skills are the monster's mob_skill_db.txt rows.
	Skills []RawSkill `json:"skills"`
}

// RawSkill is one mob_skill_db.txt row as mob_parse_row_mobskilldb reads
// it, with rAthena's words for the state, the target and the condition.
type RawSkill struct {
	State          string `json:"state"`
	Skill          int    `json:"skill"`
	SkillName      string `json:"skill_name"`
	Level          int    `json:"level"`
	Rate           int    `json:"rate"`
	CastTime       int    `json:"cast_time"`
	Delay          int    `json:"delay"`
	Cancelable     bool   `json:"cancelable"`
	Target         string `json:"target"`
	Condition      string `json:"condition"`
	ConditionValue int    `json:"condition_value"`
	Values         [5]int `json:"values"`
	Emotion        int    `json:"emotion"`
	Chat           int    `json:"chat"`
}

// Chat is a line of mob_chat_db.yml.
type Chat struct {
	ID    int    `json:"id"`
	Color uint32 `json:"color"`
	Text  string `json:"text"`
}

// Data is extracts/<mode>/monsters.json.
type Data struct {
	Mode     string `json:"mode"`
	Chats    []Chat `json:"chats"`
	Monsters []Def  `json:"monsters"`
}

// Load reads the monsters of a mode.
func Load(mode reference.Mode) (*Data, error) {
	var d Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/monsters.json", &d); err != nil {
		return nil, err
	}
	if d.Mode != string(mode) {
		return nil, fmt.Errorf("mob: monsters.json is for %q, not %q", d.Mode, mode)
	}
	return &d, nil
}

// ChatLine returns a mob_chat_db line, or the zero Chat for an unknown
// ID.
func (d *Data) ChatLine(id int) Chat {
	for _, c := range d.Chats {
		if c.ID == id {
			return c
		}
	}
	return Chat{}
}
