package modscript

import (
	"errors"
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Generation layers (proposal 0002, review decision 46, decision 0044):
// wa.generation, which runs when `wactl world new` makes a procedural
// world, on the server only.
//
//   - generation.on("map", function(map, node) ... end) adds a layer. It
//     runs on every generated map, in the mods' load order, and returns
//     nil or { places, objects, bosses }.
//   - The node is the layer's own node of the seed tree's secret branch,
//     for that map: node:child(label), node:stream() and node:hash(x, y).
//     node:hash is HMAC-SHA256 keyed by the node (review decision 54).
//     A layer has no other source of randomness (strategy section 8.7),
//     so wa.random raises an error while it runs.
//   - generation.quest_template(record) adds a quest template.
//
// Both need world.generate. In a running zone the calls record what they
// get, and nothing runs it.

// genLayer is one generation.on("map") function.
type genLayer struct {
	ctx *Context
	fn  *script.Ref
}

// generationOn is generation.on.
func (r *Runtime) generationOn(c *script.Call) error {
	ctx, err := r.needContext("generation.on")
	if err != nil {
		return err
	}
	if err := r.need("generation.on", "world.generate"); err != nil {
		return err
	}
	if ctx.Kind != "mod" {
		return fmt.Errorf("generation.on: layers belong to a mod context, and this is a %s context", ctx.Kind)
	}
	if c.String(0) != "map" {
		return errors.New(`generation.on: the event is "map"`)
	}
	if len(r.generation) >= MaxRegistrations {
		return errors.New("generation.on: too many layers")
	}
	ref, err := c.State().Keep(c.Arg(1))
	if err != nil {
		return err
	}
	r.generation = append(r.generation, &genLayer{ctx: ctx, fn: ref})
	return nil
}

// generationQuestTemplate is generation.quest_template: it reads the
// record as data/quest_templates.yml's, named with the mod's ID.
func (r *Runtime) generationQuestTemplate(c *script.Call) error {
	if err := r.need("generation.quest_template", "world.generate"); err != nil {
		return err
	}
	modID := r.actor()
	t := c.Table(0)
	name, ok := t.Get("name").AsString()
	if !ok || name == "" || len(name) > 64 {
		return errors.New("generation.quest_template: the record needs a name of 1 to 64 characters, such as \"shrine_hunt\"")
	}
	fields := map[string]any{}
	for _, f := range t.Fields {
		k, isString := f.Key.AsString()
		if !isString {
			return errors.New("generation.quest_template: the record's keys are field names")
		}
		if k == "name" {
			continue
		}
		v, err := plainValue(f.Value, 0)
		if err != nil {
			return fmt.Errorf("generation.quest_template: %s: %w", k, err)
		}
		fields[k] = v
	}
	full := modID + ":" + name
	if slices.ContainsFunc(r.genTemplates, func(x content.Template) bool { return x.Name == full }) {
		return fmt.Errorf("generation.quest_template: %s is added already", full)
	}
	if len(r.genTemplates) >= content.MaxTemplates {
		return errors.New("generation.quest_template: too many templates")
	}
	tp, err := content.ParseTemplate(full, modID, fields, nil)
	if err != nil {
		return fmt.Errorf("generation.quest_template %s: %w", full, err)
	}
	r.genTemplates = append(r.genTemplates, tp)
	return nil
}

// plainValue turns a Lua value into what YAML records hold: int,
// string, bool, or []any for a list.
func plainValue(v script.Value, depth int) (any, error) {
	switch v.Kind() {
	case script.KindInt:
		n, _ := v.AsInt()
		return int(n), nil
	case script.KindString:
		s, _ := v.AsString()
		return s, nil
	case script.KindBool:
		b, _ := v.AsBool()
		return b, nil
	case script.KindTable:
		if depth > 0 {
			return nil, errors.New("want a whole number, text or a list")
		}
		t, _ := v.AsTable()
		var out []any
		for i := int64(1); i <= int64(len(t.Fields)); i++ {
			e := t.Index(i)
			if e.IsNil() {
				return nil, errors.New("a list must have no gaps")
			}
			x, err := plainValue(e, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, x)
		}
		if len(out) != len(t.Fields) {
			return nil, errors.New("want a list")
		}
		return out, nil
	}
	return nil, fmt.Errorf("want a whole number, text or a list, not %s", v.Kind())
}

// HasGeneration reports whether a mod added a generation layer or a
// quest template.
func (r *Runtime) HasGeneration() bool { return len(r.generation) > 0 || len(r.genTemplates) > 0 }

// GenerationTemplates returns the quest templates that the mods added
// with generation.quest_template, in the order they came.
func (r *Runtime) GenerationTemplates() []content.Template { return slices.Clone(r.genTemplates) }

// genNodes and genStreams are the nodes and streams of the layer that
// runs, by handle ID from 1.
type genState struct {
	nodes   []rng.Node
	streams []*rng.Rand
}

// RunGeneration runs every layer on a map, in the order they came, and
// returns each layer's result with its mod. secret is the world's secret
// branch, and monsterName names a monster ID for the bosses. A layer that
// fails, or returns what the API refuses, is left out with a note.
func (r *Runtime) RunGeneration(m content.LayerMap, secret rng.Node, monsterName func(int64) (string, bool)) ([]content.LayerResult, []string) {
	var out []content.LayerResult
	var notes []string
	places := make([]script.Value, len(m.Places))
	for i, p := range m.Places {
		tags := make([]script.Value, len(p.Tags))
		for k, t := range p.Tags {
			tags[k] = script.String(t)
		}
		places[i] = script.NewTable(script.KV("tags", script.List(tags...)),
			script.KV("x", script.Int(int64(p.X))), script.KV("y", script.Int(int64(p.Y))))
	}
	fields := script.NewTable(
		script.KV("id", script.String(m.ID)), script.KV("kind", script.String(m.Kind)),
		script.KV("biome", script.String(m.Biome)), script.KV("tileset", script.String(m.Tileset)),
		script.KV("band", script.List(script.Int(int64(m.Band[0])), script.Int(int64(m.Band[1])))),
		script.KV("width", script.Int(int64(m.Width))), script.KV("height", script.Int(int64(m.Height))),
		script.KV("places", script.List(places...)),
	)
	for _, l := range r.generation {
		if l.ctx.ended || l.ctx.retiring || r.off(l.ctx) {
			continue
		}
		r.gen = &genState{nodes: []rng.Node{content.LayerNode(secret, l.ctx.Mod, m.ID)}}
		prev := r.current
		r.setCurrent(l.ctx)
		vals, err := r.s.Call(r.fns.runGeneration.Value(), l.fn.Value(), fields, script.Int(1))
		r.setCurrent(prev)
		r.gen = nil
		if err != nil {
			r.afterError()
			r.failed(l.ctx, "generation layer", err)
			notes = append(notes, fmt.Sprintf("%s on %s: the layer failed: %v", l.ctx.Mod, m.ID, err))
			continue
		}
		res := content.LayerResult{Mod: l.ctx.Mod}
		if len(vals) > 0 {
			if t, ok := vals[0].AsTable(); ok {
				res, notes = readGenResult(l.ctx.Mod, m.ID, t, monsterName, notes)
			}
		}
		out = append(out, res)
	}
	return out, notes
}

// readGenResult reads the result the prelude checked.
func readGenResult(modID, mapID string, t *script.Table, monsterName func(int64) (string, bool), notes []string) (content.LayerResult, []string) {
	res := content.LayerResult{Mod: modID}
	num := func(e *script.Table, k string) int {
		n, _ := e.Get(k).AsInt()
		return int(n)
	}
	each := func(k string, fn func(e *script.Table)) {
		list, ok := t.Get(k).AsTable()
		if !ok {
			return
		}
		for i := int64(1); ; i++ {
			e, ok := list.Index(i).AsTable()
			if !ok {
				return
			}
			fn(e)
		}
	}
	each("places", func(e *script.Table) {
		p := content.LayerPlace{X: num(e, "x"), Y: num(e, "y")}
		if tags, ok := e.Get("tags").AsTable(); ok {
			for i := int64(1); ; i++ {
				s, ok := tags.Index(i).AsString()
				if !ok {
					break
				}
				p.Tags = append(p.Tags, s)
			}
		}
		res.Places = append(res.Places, p)
	})
	each("objects", func(e *script.Table) {
		model, _ := e.Get("model").AsString()
		res.Objects = append(res.Objects, content.LayerObject{Model: model, X: num(e, "x"), Y: num(e, "y"), Turn: num(e, "turn")})
	})
	each("bosses", func(e *script.Table) {
		id, _ := e.Get("monster").AsInt()
		name, ok := monsterName(id)
		if !ok {
			notes = append(notes, fmt.Sprintf("%s on %s: boss monster %d does not spawn", modID, mapID, id))
			return
		}
		role, _ := e.Get("role").AsString()
		b := content.LayerBoss{Monster: name, Role: role, RespawnMs: num(e, "respawn")}
		if !e.Get("x").IsNil() {
			b.X, b.Y, b.HasCell = num(e, "x"), num(e, "y"), true
		}
		res.Bosses = append(res.Bosses, b)
	})
	return res, notes
}

// genNode returns the node of a handle of the layer that runs.
func (r *Runtime) genNode(c *script.Call, what string) (rng.Node, error) {
	id := c.Int(0)
	if r.gen == nil || id < 1 || id > int64(len(r.gen.nodes)) {
		return rng.Node{}, errors.New(what + ": the node belongs to a layer that has ended")
	}
	return r.gen.nodes[id-1], nil
}

// MaxGenerationHandles is the most nodes and streams one layer's run
// makes.
const MaxGenerationHandles = 100_000

func (r *Runtime) genNodeChild(c *script.Call) error {
	n, err := r.genNode(c, "node:child")
	if err != nil {
		return err
	}
	if len(r.gen.nodes) >= MaxGenerationHandles {
		return errors.New("node:child: too many nodes")
	}
	r.gen.nodes = append(r.gen.nodes, n.Child(c.String(1)))
	c.Return(script.Int(int64(len(r.gen.nodes))))
	return nil
}

func (r *Runtime) genNodeStream(c *script.Call) error {
	n, err := r.genNode(c, "node:stream")
	if err != nil {
		return err
	}
	if len(r.gen.streams) >= MaxGenerationHandles {
		return errors.New("node:stream: too many streams")
	}
	r.gen.streams = append(r.gen.streams, n.SecretStream())
	c.Return(script.Int(int64(len(r.gen.streams))))
	return nil
}

func (r *Runtime) genNodeHash(c *script.Call) error {
	n, err := r.genNode(c, "node:hash")
	if err != nil {
		return err
	}
	// A layer's node is secret, so its hash is HMAC-SHA256 keyed by the
	// node and cannot be inverted (review decision 54).
	c.Return(script.Int(int64(n.SecretHash(c.Int(1), c.Int(2))))) // #nosec G115 -- the hash's bits as a Lua integer.
	return nil
}

// genStream returns the stream of a handle of the layer that runs.
func (r *Runtime) genStream(c *script.Call, what string) (*rng.Rand, error) {
	id := c.Int(0)
	if r.gen == nil || id < 1 || id > int64(len(r.gen.streams)) {
		return nil, errors.New(what + ": the stream belongs to a layer that has ended")
	}
	return r.gen.streams[id-1], nil
}

func (r *Runtime) genStreamInteger(c *script.Call) error {
	s, err := r.genStream(c, "stream:integer")
	if err != nil {
		return err
	}
	lo, hi := c.Int(1), c.Int(2)
	if hi < lo || hi-lo < 0 || hi-lo >= 1<<62 {
		return errors.New("stream:integer: the range is too large")
	}
	c.Return(script.Int(s.Range(lo, hi)))
	return nil
}

func (r *Runtime) genStreamChance(c *script.Call) error {
	s, err := r.genStream(c, "stream:chance")
	if err != nil {
		return err
	}
	num, den := c.Int(1), c.Int(2)
	if den <= 0 || den >= 1<<62 {
		return errors.New("stream:chance: the denominator must be from 1 to 2^62")
	}
	c.Return(script.Bool(int64(s.Uint64N(uint64(den))) < num)) // #nosec G115 -- den is positive, and the draw is below it.
	return nil
}

// generating reports whether a layer runs, when wa.random is refused.
func (r *Runtime) generating() bool { return r.gen != nil }
