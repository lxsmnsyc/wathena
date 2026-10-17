// Package content makes the procedural content of a procedural world
// when `wactl world new` makes it (proposal 0002, review decision 46,
// decision 0044, strategy sections 8.6 and 11.4):
//
//   - Generation layers: the mods' generation.on("map") functions add
//     named places, objects and bosses to each generated map. They cannot
//     change terrain cells, which the client generates.
//   - Quests: each town's quest board offers quests that its quest
//     templates make from the stored spawns of its nearby fields, chosen
//     with the secret branch. Rewards scale with the town's band.
//   - Job guilds: one per class family, each in a town, at a place
//     tagged guild:<family>. A guild's test for a stage is a hunt and a
//     collect quest from the templates with uses: [guild], then a talk
//     with the guild master.
//   - Rebirth: a place tagged rebirth in the largest town of the highest
//     band.
//
// The database keeps the result, so a new generator version or a mod
// change never changes a made world's content. The game server reads it
// and never sees a secret key. Content uses integer math and sorted
// slices only, so it gives the same result on every platform.
package content

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Quest IDs of a world's quests: core's quests keep rAthena's IDs, below
// 900,000, and mods' quests start at 1,000,000 (mod.QuestIDs).
const (
	FirstQuestID uint32 = 900_000
	LastQuestID  uint32 = 999_999
)

// BoardQuests is how many quests each town's quest board offers.
const BoardQuests = 6

// NearbyWarps is how many warps from a town a nearby field may be. A
// town whose nearby fields hold no monster for a template looks further,
// up to MaxNearbyWarps.
const (
	NearbyWarps    = 2
	MaxNearbyWarps = 6
)

// Families are the class families, each with one job guild (strategy
// section 11.4). They are wa.jobs names of the first jobs.
var Families = []string{"swordman", "mage", "archer", "acolyte", "merchant", "thief"}

// Stages of a job guild's test: a Novice's change into the first job,
// and a first job's change into a second job. A transcendent player
// changes without a test, as rAthena's job NPCs let it.
const (
	StageFirst  = "first"
	StageSecond = "second"
)

// Stages lists the stages in order.
var Stages = []string{StageFirst, StageSecond}

// stageLevel is the monster level each stage's test aims for: about the
// base level of a Novice at job level 10, and of a first job at job
// level 40.
var stageLevel = map[string]int{StageFirst: 10, StageSecond: 45}

// Place tags of procedural content.
const (
	// TagQuestBoard is each town's quest board.
	TagQuestBoard = "quest_board"
	// TagGuild is a town template's guild hall, where guilds stand.
	TagGuild = "guild"
	// TagRebirth is the rebirth NPC's place.
	TagRebirth = "rebirth"
	// TagSquare is a town's square.
	TagSquare = "square"
)

// GuildTag is the place tag of a family's guild, such as guild:swordman.
func GuildTag(family string) string { return TagGuild + ":" + family }

// Source of content that core makes, beside the mods' IDs.
const SourceCore = "core"

// Purposes of quests.
const (
	PurposeBoard = "board"
	PurposeGuild = "guild"
)

// Quest is one quest a world made: a quest board's, or a step of a job
// guild's test. The server makes a quest record of each, with its ID.
type Quest struct {
	ID uint32
	// Town is the map ID of the town whose board or guild offers it.
	Town string
	// Purpose is board or guild. Guild, Stage and Step name a guild
	// test's step: 1 for its hunt and 2 for its collect quest.
	Purpose string
	Guild   string
	Stage   string
	Step    int
	// Template is the template's full name, and Kind its kind.
	Template string
	Kind     string
	Title    string
	// Monster is a hunt's or a collect quest's monster, by name. Item is
	// a collect quest's item, or a delivery's carried item, or "".
	Monster string
	Item    string
	// Count is the kills or items it asks for, 0 for a delivery.
	Count int
	// DropRate is a collect quest's drop rate, out of 10,000.
	DropRate int
	// To is a delivery's town, as a map ID.
	To string
	// BaseExp, JobExp and Zeny are the reward.
	BaseExp, JobExp, Zeny int64
}

// Place is a named place that content or a generation layer added to a
// map. Source is core or the mod's ID.
type Place struct {
	Map    string
	X, Y   int
	Tags   []string
	Source string
}

// Object is an object a generation layer put on a map: a prop of the
// map's tileset, which the client draws. Turn is 0 to 3 quarter turns.
type Object struct {
	Map    string
	Model  string
	X, Y   int
	Turn   int
	Source string
}

// World is a world's content.
type World struct {
	Quests  []Quest
	Places  []Place
	Objects []Object
	// Spawns are the spawn lists with the layers' bosses, in the order
	// of the input.
	Spawns []spawns.Planned
	// Templates are the quest templates the world drew from, the
	// layers' included.
	Templates []Template
	// Notes are what the layers did that the rules left out.
	Notes []string
}

// MonsterInfo is what content reads of a monster.
type MonsterInfo struct {
	// Name is the AegisName or a mod monster's full name, as spawns
	// store it. DisplayName is the name players see.
	Name, DisplayName string
	Level             int
	// Boss is set for boss class monsters and MVPs.
	Boss bool
	// Drops are its drops, by item name.
	Drops []Drop
}

// Drop is one drop of a monster: an item name and its rate out of
// 10,000.
type Drop struct {
	Item string
	Rate int
}

// ItemInfo is what content reads of an item.
type ItemInfo struct {
	// DisplayName is the name players see.
	DisplayName string
	// Collectable reports whether a collect quest may ask for it: an etc
	// item that stacks.
	Collectable bool
}

// Data is the game data content reads.
type Data struct {
	Monster func(name string) (MonsterInfo, bool)
	Item    func(name string) (ItemInfo, bool)
	// BaseExp is the base exp from a level to the next, of a first job.
	BaseExp func(level int) int64
}

// Input is what a world's content comes from.
type Input struct {
	Generator gen.Generator
	Layout    *layout.Layout
	// Maps are the generated maps, by map ID.
	Maps map[string]*gen.Field
	// SpawnMaps are the spawn inputs of the fields and floors, and
	// Spawns their stored lists, as the spawn_table hooks left them.
	SpawnMaps []spawns.Map
	Spawns    []spawns.Planned
	// SpawnMonsters are what spawns read of each monster, by name, for
	// the bosses of layers.
	SpawnMonsters map[string]spawns.Monster
	Data          Data
	// Templates are core's and the mods' quest templates.
	Templates []Template
	// Secret is the secret branch of the world's seed.
	Secret rng.Node
	// Layers runs the mods' generation layers, or is nil without any.
	Layers Layers
}

// Node returns the secret content node of the world.
func Node(secret rng.Node) rng.Node { return secret.Child("content") }

// TownNode returns a town's secret quest node (strategy section 8.2,
// town(n)/secret/quests).
func TownNode(secret rng.Node, town int64) rng.Node {
	return Node(secret).Child("town", town).Child("quests")
}

// GuildNode returns a guild's secret node for a stage.
func GuildNode(secret rng.Node, family, stage string) rng.Node {
	return Node(secret).Child("guild").Child(family).Child(stage)
}

// ErrContent is the error of a world whose content cannot be made.
var ErrContent = errors.New("content")

// Generate makes a world's content:
//  1. Each generated map's layers, in the layout's order, with the
//     layers' bosses joined to the spawn lists.
//  2. The quest templates with those the layers added.
//  3. Each town's quest board, place and quests, in town order.
//  4. The job guilds' towns, places and tests, in family order.
//  5. The rebirth place.
//
// Quests get IDs from FirstQuestID in that order.
func Generate(in Input) (*World, error) {
	if in.Layout == nil {
		return nil, fmt.Errorf("%w: no layout", ErrContent)
	}
	w := &World{Spawns: make([]spawns.Planned, len(in.Spawns))}
	for i, p := range in.Spawns {
		w.Spawns[i] = spawns.Planned{Map: p.Map, Spawns: slices.Clone(p.Spawns)}
	}
	g := &generator{in: in, w: w, atlas: layout.NewAtlas(in.Layout, layout.AtlasOptions{}), nextID: FirstQuestID}
	g.occupied = map[string][][2]int{}
	for id, f := range in.Maps {
		for _, p := range f.Places {
			g.occupied[id] = append(g.occupied[id], [2]int{p.X, p.Y})
		}
	}
	if err := g.layers(); err != nil {
		return nil, err
	}
	w.Templates = append(slices.Clone(in.Templates), g.layerTemplates...)
	if err := checkTemplateNames(w.Templates); err != nil {
		return nil, err
	}
	if err := g.boards(); err != nil {
		return nil, err
	}
	if err := g.guilds(); err != nil {
		return nil, err
	}
	g.rebirth()
	return w, nil
}

// checkTemplateNames refuses two templates with one name.
func checkTemplateNames(list []Template) error {
	seen := map[string]bool{}
	for _, t := range list {
		if seen[t.Name] {
			return fmt.Errorf("%w: two quest templates are named %s", ErrContent, t.Name)
		}
		seen[t.Name] = true
	}
	return nil
}

// generator holds the state of one Generate.
type generator struct {
	in    Input
	w     *World
	atlas *layout.Atlas
	// occupied are the cells of each map's places, generated and added,
	// which new places keep away from.
	occupied       map[string][][2]int
	layerTemplates []Template
	nextID         uint32
}

// newID returns the next quest ID.
func (g *generator) newID() (uint32, error) {
	if g.nextID > LastQuestID {
		return 0, fmt.Errorf("%w: more than %d quests", ErrContent, LastQuestID-FirstQuestID+1)
	}
	id := g.nextID
	g.nextID++
	return id, nil
}

// towns returns the layout's towns in number order.
func (g *generator) towns() []*layout.Map { return g.in.Layout.Towns() }

// addPlace adds a place to a map near a cell: the first free walkable
// cell from distance near to far of (x, y), by rings, each ring from its
// top row down and left to right. It returns false when none is free.
func (g *generator) addPlace(mapID string, x, y, near, far int, tags []string, source string) (Place, bool) {
	f := g.in.Maps[mapID]
	if f == nil {
		return Place{}, false
	}
	for d := near; d <= far; d++ {
		for dy := -d; dy <= d; dy++ {
			for dx := -d; dx <= d; dx++ {
				if max(abs(dx), abs(dy)) != d {
					continue
				}
				cx, cy := x+dx, y+dy
				if !walkable(f, cx, cy) || g.near(mapID, cx, cy, 1) {
					continue
				}
				p := Place{Map: mapID, X: cx, Y: cy, Tags: slices.Clone(tags), Source: source}
				g.w.Places = append(g.w.Places, p)
				g.occupied[mapID] = append(g.occupied[mapID], [2]int{cx, cy})
				return p, true
			}
		}
	}
	return Place{}, false
}

// near reports whether a place of the map is within r cells of a cell.
func (g *generator) near(mapID string, x, y, r int) bool {
	for _, c := range g.occupied[mapID] {
		if abs(c[0]-x) <= r && abs(c[1]-y) <= r {
			return true
		}
	}
	return false
}

// walkable reports whether a cell is open ground: walkable and not
// water.
func walkable(f *gen.Field, x, y int) bool {
	return x >= 0 && y >= 0 && x < f.Width && y < f.Height && f.Types[y*f.Width+x] == gen.CellWalkable
}

// placeOf returns a map's first generated place with a tag.
func placeOf(f *gen.Field, tag string) (gen.Place, bool) {
	if f == nil {
		return gen.Place{}, false
	}
	for _, p := range f.Places {
		if p.HasTag(tag) {
			return p, true
		}
	}
	return gen.Place{}, false
}

// anchor returns where a town's content place stands near: its first
// place with the tag, else its square, else its middle.
func (g *generator) anchor(town string, tag string) (int, int) {
	f := g.in.Maps[town]
	if p, ok := placeOf(f, tag); ok {
		return p.X, p.Y
	}
	if p, ok := placeOf(f, TagSquare); ok {
		return p.X, p.Y
	}
	if f == nil {
		return 0, 0
	}
	return f.Width / 2, f.Height / 2
}

// Content places stand 2 to 12 cells from their anchor.
const (
	placeNear = 2
	placeFar  = 12
)

// title fills a template's title.
func title(t Template, count int, monster, item, town string) string {
	r := strings.NewReplacer("{count}", strconv.Itoa(count), "{monster}", monster, "{item}", item, "{town}", town)
	s := r.Replace(t.Title)
	if len([]rune(s)) > MaxTitle {
		s = string([]rune(s)[:MaxTitle])
	}
	return s
}

// reward is a quest's reward in a band (decision 0044): base exp of
// Reward per mille of the exp from the band's lowest level to the next,
// at least 1, job exp three quarters of that, and Reward percent of 50
// zeny per level.
func (g *generator) reward(t Template, band [2]int) (base, job, zeny int64) {
	if t.Reward == 0 {
		return 0, 0, 0
	}
	level := max(band[0], 1)
	need := int64(0)
	if g.in.Data.BaseExp != nil {
		need = g.in.Data.BaseExp(level)
	}
	base = max(need*int64(t.Reward)/1000, 1)
	job = max(base*3/4, 1)
	zeny = int64(level) * 50 * int64(t.Reward) / 100
	return base, job, zeny
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// sortedTowns returns towns by their band's lowest level, then number.
func sortedTowns(list []*layout.Map) []*layout.Map {
	out := slices.Clone(list)
	slices.SortStableFunc(out, func(a, b *layout.Map) int {
		return cmp.Or(cmp.Compare(a.Levels[0], b.Levels[0]), cmp.Compare(a.Town, b.Town))
	})
	return out
}
