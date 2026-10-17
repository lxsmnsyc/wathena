package zone

import (
	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Generation layers (proposal 0002, review decision 46, decision 0044):
// `wactl world new` runs the mods' generation.on("map") layers in the
// Lua state of the spawn hooks, once on each generated map, and the test
// worlds of wactl mod test in theirs.

// HasGeneration reports whether a mod may add generation layers or quest
// templates: a mod with world.generate and an entry script.
func HasGeneration(mods *Mods) bool {
	if mods == nil {
		return false
	}
	for _, sc := range mods.Scripts {
		if sc.Grants.Has("world.generate") && len(sc.Entries) > 0 {
			return true
		}
	}
	return false
}

// GenerationLayers runs a Lua state's generation layers for content: it
// implements content.Layers.
type GenerationLayers struct {
	r      *modscript.Runtime
	secret rng.Node
	idx    SpawnIndex
}

// NewGenerationLayers runs r's layers with the world's secret branch.
// idx names the bosses' monsters. A nil r runs none.
func NewGenerationLayers(r *modscript.Runtime, secret rng.Node, idx SpawnIndex) *GenerationLayers {
	return &GenerationLayers{r: r, secret: secret, idx: idx}
}

// Map runs every layer on a map.
func (g *GenerationLayers) Map(m content.LayerMap) ([]content.LayerResult, []string) {
	if g == nil || g.r == nil {
		return nil, nil
	}
	return g.r.RunGeneration(m, g.secret, func(id int64) (string, bool) {
		name, ok := g.idx.Names[id]
		if !ok {
			return "", false
		}
		_, ok = g.idx.Monsters[name]
		return name, ok
	})
}

// Templates are the quest templates of generation.quest_template.
func (g *GenerationLayers) Templates() []content.Template {
	if g == nil || g.r == nil {
		return nil
	}
	return g.r.GenerationTemplates()
}

// Layers returns the generation layers of the spawn hooks' state.
func (h *SpawnHooks) Layers(secret rng.Node, idx SpawnIndex) *GenerationLayers {
	return NewGenerationLayers(h.z.scripts, secret, idx)
}
