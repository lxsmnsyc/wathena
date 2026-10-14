package gamedata

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
)

// overlaySet makes a set of core and one mod with a monsters.yml, and
// the given script files.
func overlaySet(t *testing.T, monsters string, scripts ...string) *mod.Set {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"a/mod.yaml":          "id: example.a\nname: A\nversion: 1.0.0\napi: 1.0\n",
		"a/data/monsters.yml": monsters,
	}
	for _, p := range scripts {
		files["a/"+p] = "return function() end\n"
	}
	for p, s := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	found, err := mod.Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	set, err := mod.Resolve(found, mod.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// baseMonsters is a renewal Poring, as mob_db.yml has it.
func baseMonsters() map[uint32]Monster {
	d := &mob.Def{ID: 1002, AegisName: "PORING", Name: "Poring", Level: 1, HP: 55, SP: 1, BaseExp: 150, JobExp: 40,
		Attack: 8, Attack2: 1, Defense: 2, MagicDefense: 5, Str: 6, Agi: 1, Vit: 1, Dex: 6, Luk: 5,
		AttackRange: 1, SkillRange: 10, ChaseRange: 12, Size: 1, Race: 3, Element: 1, ElementLevel: 1,
		WalkSpeed: 400, AttackDelay: 1872, AttackMotion: 672, DamageMotion: 480, DamageTaken: 100, AI: "02", Mode: 0x83}
	st := mob.Calc(d, true)
	return map[uint32]Monster{1002: {Def: d, Status: &st, AegisName: "PORING", Name: "Poring", renewal: true,
		drops:  []progression.Drop{{Item: 909, Rate: 7000}},
		Reward: &progression.Mob{ID: 1002, Level: 1, MaxHP: 55, BaseExp: 150, JobExp: 40, Mode: 0x83, Drops: []progression.Drop{{Item: 909, Rate: 7000}}}}}
}

// testSkills loads the renewal skill database.
func testSkills(t *testing.T) *skill.DB {
	t.Helper()
	data, _, err := LoadSkillData(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	db, err := skill.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMonsterOverlay(t *testing.T) {
	set := overlaySet(t, `
- name: example.a:BIG_PORING
  sprite: PORING
  display_name: Big Poring
  level: 20
  hp: 900
  attack: 30
  magic_attack: 10
  str: 50
  dex: 30
  class: boss
  ai: 4
  modes: [mvp]
  base_exp: 100
  mvp_exp: 50
  skill_range: 12
  chase_range: 14
  damage_taken: 50
  drops: [{item: Jellopy, rate: 5000}]
  mvp_drops: [{item: Jellopy, rate: 1000}]
  skills:
    - {skill: NPC_SUMMONSLAVE, state: idle, rate: 1000, target: self, condition: slaves_below, condition_value: 2, summons: [PORING, example.a:SMALL_PORING]}
    - {skill: NPC_EMOTION, state: attack, emotion: [surprise, anger], modes: [can_move, aggressive], chat: "Boing!", chat_color: 0xFF8000}
    - {skill: AL_HEAL, level: 5, target: self, condition: hp_between, condition_value: [10, 50]}
  script: scripts/monsters/big.lua
- name: example.a:SMALL_PORING
  sprite: PORING
- name: PORING
  hp: 80
  vit: 30
`, "scripts/monsters/big.lua")
	ids, _ := mod.LoadIDs(context.Background(), &mod.MemoryIDs{})
	tables := progression.New(&progression.Data{}, map[uint32]string{909: "Etc"}, progression.DefaultConfig)
	base := baseMonsters()
	out, names, _, err := monsterOverlay(context.Background(), set, base, testConstants(t), map[string]uint32{"Jellopy": 909}, testSkills(t), tables, ids)
	if err != nil {
		t.Fatal(err)
	}
	big := out[100_000]
	if names["EXAMPLE.A:BIG_PORING"] != 100_000 || big.Name != "Big Poring" || big.Sprite != 1002 || big.Status.MaxHP != 900 {
		t.Fatalf("big poring %+v %+v", big, big.Status)
	}
	// status_calc_misc computes the combat values from the base stats.
	if st := big.Status; st.RHW.Atk != 24 || st.RHW.Atk2 != 36 || st.BAtk != 70 || st.Hit != 20+30+150 || st.MAtkMin != 0+20+7 || st.DamageTaken != 50 {
		t.Errorf("status %+v", st)
	}
	// AI type 04 with mvp and the boss modes.
	if want := uint32(0x3885) | mob.ModeMvp | mob.ClassModes(1); big.Status.Class != 1 || big.Status.Mode != want {
		t.Errorf("class %d mode %#x, want %#x", big.Status.Class, big.Status.Mode, want)
	}
	if r := big.Reward; r.ID != 100_000 || r.Level != 20 || r.BaseExp != 100 || r.JobExp != 0 || r.MvpExp != 50 || len(r.Drops) != 1 || r.Drops[0].Rate != 5000 || len(r.MvpDrops) != 1 {
		t.Errorf("reward %+v", r)
	}
	if len(big.Skills) != 3 || big.Script != "example.a:scripts/monsters/big.lua" {
		t.Fatalf("skills %+v script %q", big.Skills, big.Script)
	}
	if s := big.Skills[0]; s.ID != mob.IDSummonSlave || s.State != mob.StateIdle || s.Condition != mob.CondSlavesBelow || s.Value != 2 || len(s.Summons) != 2 || s.Summons[1] != 100_001 {
		t.Errorf("summon %+v", s)
	}
	if s := big.Skills[1]; !slices.Equal(s.Emotions, []int{0, 7}) || s.Chat != "Boing!" || s.ChatColor != 0xFF8000 ||
		s.State != mob.StateBerserk || !s.HasModes || s.Modes != mob.ModeCanMove|mob.ModeAggressive {
		t.Errorf("emotion %+v", s)
	}
	if s := big.Skills[2]; s.Level != 5 || s.Value != 10 || s.Value2 != 50 || s.State != mob.StateAny {
		t.Errorf("heal %+v", s)
	}
	if p := out[1002]; p.Status.MaxHP != 80 || p.Status.Def2 != (1+30)/2 || base[1002].Status.MaxHP != 55 {
		t.Errorf("the core poring has %d HP and %d soft DEF, and the original %d HP", p.Status.MaxHP, p.Status.Def2, base[1002].Status.MaxHP)
	}
}

func TestMonsterOverlayErrors(t *testing.T) {
	set := overlaySet(t, `
- name: example.a:NO_SPRITE
  hp: 10
- name: example.a:BAD
  sprite: PORING
  hp: -1
  attack: [9, 3]
  class: king
  modes: [flying]
  colour: red
  hit: 50
  ai: 14
  drops: [{item: Gold_Brick, rate: 10}]
  skills:
    - {skill: NO_SUCH_SKILL}
    - {skill: AL_HEAL, target: around1, condition: hp_below}
  script: scripts/missing.lua
`)
	ids, _ := mod.LoadIDs(context.Background(), &mod.MemoryIDs{})
	tables := progression.New(&progression.Data{}, nil, progression.DefaultConfig)
	_, _, _, err := monsterOverlay(context.Background(), set, baseMonsters(), testConstants(t), map[string]uint32{}, testSkills(t), tables, ids)
	if err == nil {
		t.Fatal("no error")
	}
	_, _, _, err2 := monsterOverlay(context.Background(), overlaySet(t, `
- name: example.a:BAD
  sprite: PORING
  hp: -1
  attack: [9, 3]
  class: king
  modes: [flying]
  colour: red
  hit: 50
  ai: 14
  drops: [{item: Gold_Brick, rate: 10}]
  skills:
    - {skill: NO_SUCH_SKILL}
    - {skill: AL_HEAL, target: around1, condition: hp_below}
  script: scripts/missing.lua
`), baseMonsters(), testConstants(t), map[string]uint32{}, testSkills(t), tables, ids)
	if !strings.Contains(err.Error(), "a new monster needs sprite") {
		t.Errorf("missing the sprite error in %v", err)
	}
	for _, want := range []string{"hp must be", "attack must be one number in renewal", `class: "king"`, `"flying" is not a mode`,
		`"colour" is not a field`, `"hit" is not a field`, "ai must be one of", `there is no item "Gold_Brick"`,
		`there is no skill "NO_SUCH_SKILL"`, "around1 is for ground skills", "hp_below needs a number", "script: scripts/missing.lua does not exist"} {
		if err2 == nil || !strings.Contains(err2.Error(), want) {
			t.Errorf("missing %q in %v", want, err2)
		}
	}
}

// TestMonsterArt: a monster's sprite may name art, and a new monster
// with art starts from a Poring. sounds names its sounds (decision 0033).
func TestMonsterArt(t *testing.T) {
	ids, _ := mod.LoadIDs(context.Background(), &mod.MemoryIDs{})
	tables := progression.New(&progression.Data{}, nil, progression.DefaultConfig)
	out, _, _, err := monsterOverlay(context.Background(), overlaySet(t, `
- name: example.a:GHOST
  sprite: mod/example.a/sprite/mob/Ghost
  sounds: {attack: mod/example.a/sound/boo, die: sound/die}
  hp: 30
- name: PORING
  sprite: mod/example.a/sprite/mob/Shiny
`), baseMonsters(), testConstants(t), map[string]uint32{}, testSkills(t), tables, ids)
	if err != nil {
		t.Fatal(err)
	}
	ghost := out[mod.MonsterIDs.Start]
	if ghost.Art.Sprite != "mod/example.a/sprite/mob/Ghost" || ghost.Art.Sounds["attack"] != "mod/example.a/sound/boo" || ghost.Sprite != 0 ||
		ghost.Def.Level != out[1002].Def.Level {
		t.Errorf("ghost %+v %+v", ghost.Art, ghost.Def)
	}
	if out[1002].Art.Sprite != "mod/example.a/sprite/mob/Shiny" {
		t.Errorf("poring art %+v", out[1002].Art)
	}
	_, _, _, err = monsterOverlay(context.Background(), overlaySet(t, `
- name: example.a:BAD
  sprite: mod/example.a/icon/item/Ghost
  sounds: {sing: sound/x, die: texture/y}
`), baseMonsters(), testConstants(t), map[string]uint32{}, testSkills(t), tables, ids)
	for _, want := range []string{"is not a sprite name", "is not a sound name"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}
