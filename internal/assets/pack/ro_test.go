package pack

import (
	"bytes"
	"encoding/binary"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/assets/effect"
	"github.com/lxsmnsyc/wathena/internal/assets/model"
	"github.com/lxsmnsyc/wathena/internal/assets/wspr"
	"github.com/lxsmnsyc/wathena/internal/grf"
)

// spr writes a version 2.1 spr with one 2 × 2 indexed image.
func spr() []byte {
	var b bytes.Buffer
	w := func(v ...any) {
		for _, x := range v {
			_ = binary.Write(&b, binary.LittleEndian, x)
		}
	}
	b.WriteString("SP")
	b.Write([]byte{1, 2})
	w(uint16(1), uint16(0))
	w(uint16(2), uint16(2))
	rle := []byte{1, 1, 0, 1, 1}
	w(uint16(len(rle)))
	b.Write(rle)
	pal := make([]byte, 1024)
	pal[4], pal[5], pal[6] = 200, 100, 50
	b.Write(pal)
	return b.Bytes()
}

// act writes a version 2.5 act with one action of one frame, whose event
// is a sound.
func act(event string) []byte {
	var b bytes.Buffer
	w := func(v ...any) {
		for _, x := range v {
			_ = binary.Write(&b, binary.LittleEndian, x)
		}
	}
	b.WriteString("AC")
	b.Write([]byte{5, 2})
	w(uint16(1))
	b.Write(make([]byte, 10))
	w(uint32(1))
	b.Write(make([]byte, 32))
	w(uint32(1))
	w(int32(0), int32(-4), int32(0), uint32(0))
	b.Write([]byte{255, 255, 255, 255})
	w(float32(1), float32(1), int32(0), int32(0), int32(0), int32(0))
	w(int32(0))  // event 0
	w(uint32(0)) // anchors
	w(uint32(1)) // events
	ev := make([]byte, 40)
	copy(ev, event)
	b.Write(ev)
	w(float32(4))
	return b.Bytes()
}

// str writes an STR effect with one layer, one key, and one texture.
func str(tex string) []byte {
	var b bytes.Buffer
	w := func(v ...any) {
		for _, x := range v {
			_ = binary.Write(&b, binary.LittleEndian, x)
		}
	}
	b.WriteString("STRM")
	w(uint32(0x94), uint32(30), uint32(1), uint32(1))
	b.Write(make([]byte, 16))
	w(uint32(1))
	f := make([]byte, 128)
	copy(f, tex)
	b.Write(f)
	w(uint32(1))
	k := effect.Key{Pos: [2]float32{320, 320}, XY: [8]float32{-35, 35, 35, -35, -35, -35, 35, 35}, Color: [4]float32{255, 255, 255, 255}, SrcBlend: 5, DstBlend: 2}
	w(k.Frame, k.Type, k.Pos, k.UV, k.XY, k.AniFrame, k.AniType, k.Delay, k.Angle, k.Color, k.SrcBlend, k.DstBlend, k.MTPreset)
	return b.Bytes()
}

func TestLooseRO(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	write(t, dir, map[string][]byte{
		"sprite/mob/Lantern_Ghost.spr":  spr(),
		"sprite/mob/Lantern_Ghost.act":  act("effect/boo.wav"),
		"sprite/npc/Keeper.spr":         spr(),
		"sprite/npc/Keeper.act":         act("ef_hit1.wav"),
		"sound/effect/boo.wav":          wav(),
		"effect/glow/spark.str":         str("Spark.bmp"),
		"texture/effect/glow/spark.bmp": bmp(),
		"texture/wall.bmp":              bmp(),
		"texture/wall.tga":              tga(),
		"sprite/mob/Half.spr":           spr(),
		"map/lonely.rsw":                []byte("GRSW"),
		"icon/item/Lamp.png":            rgbaPNG(t, 4, 4),
	})
	src := &Source{Dir: dir, Prefix: "mod/example.lantern/", Formats: RO}
	rep, err := build.Run(build.Options{Out: out, Sources: []build.Source{src}, Brotli: 1})
	if err != nil {
		t.Fatal(err)
	}
	list, err := build.List(out)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for n := range list {
		names = append(names, n)
	}
	sort.Strings(names)
	want := []string{
		"mod/example.lantern/effect/glow/spark",
		"mod/example.lantern/sound/effect/boo",
		"mod/example.lantern/sprite/mob/Lantern_Ghost",
		"mod/example.lantern/sprite/npc/Keeper",
		"mod/example.lantern/texture/effect/glow/spark",
		"mod/example.lantern/texture/wall",
		"mod/example.lantern/texture/wall~tga",
	}
	if !slices.Equal(names, want) {
		t.Errorf("names %v, want %v", names, want)
	}
	rejected := strings.Join(rep.Rejected, "\n")
	for _, p := range []string{"Half.spr", "lonely.rsw", "Lamp.png"} {
		if !strings.Contains(rejected, p) {
			t.Errorf("%s was not rejected:\n%s", p, rejected)
		}
	}
	// A sprite's sound finds the mod's own sound, and otherwise core's.
	for name, ev := range map[string]string{
		"mod/example.lantern/sprite/mob/Lantern_Ghost": "mod/example.lantern/sound/effect/boo",
		"mod/example.lantern/sprite/npc/Keeper":        "sound/ef_hit1",
	} {
		data, _, err := build.ReadAsset(out, name)
		if err != nil {
			t.Fatal(err)
		}
		s, err := wspr.Decode(data)
		if err != nil || !slices.Equal(s.Events, []string{ev}) {
			t.Errorf("%s: %v events %v, want %s", name, err, s.Events, ev)
		}
	}
	// The effect's texture, in any case, is the mod's own.
	data, _, err := build.ReadAsset(out, "mod/example.lantern/effect/glow/spark")
	if err != nil {
		t.Fatal(err)
	}
	e, err := effect.Decode(data)
	if err != nil || !slices.Equal(e.Textures, []string{"mod/example.lantern/texture/effect/glow/spark"}) {
		t.Errorf("effect: %v %v", err, e.Textures)
	}
}

// bmp writes a 2 × 2 24-bit BMP with a magenta pixel.
func bmp() []byte {
	var b bytes.Buffer
	w := func(v ...any) {
		for _, x := range v {
			_ = binary.Write(&b, binary.LittleEndian, x)
		}
	}
	b.WriteString("BM")
	w(uint32(54+16), uint32(0), uint32(54))
	w(uint32(40), int32(2), int32(2), uint16(1), uint16(24), uint32(0), uint32(16), int32(0), int32(0), uint32(0), uint32(0))
	b.Write([]byte{255, 0, 255, 10, 20, 30, 0, 0})
	b.Write([]byte{40, 50, 60, 70, 80, 90, 0, 0})
	return b.Bytes()
}

// tga writes a 1 × 1 uncompressed 32-bit TGA.
func tga() []byte {
	h := make([]byte, 18)
	h[2] = 2
	h[12], h[14], h[16], h[17] = 1, 1, 32, 8
	return append(h, 1, 2, 3, 255)
}

// TestClientFiles builds loose files taken from a game client: a monster
// sprite, a model with its textures, an effect, a sound, and the smallest
// map, each placed as a mod would. WATHENA_CLIENT names the client
// folder.
func TestClientFiles(t *testing.T) {
	client := os.Getenv("WATHENA_CLIENT")
	if client == "" {
		t.Skip("set WATHENA_CLIENT to a game client folder to test loose files from it")
	}
	c, err := grf.OpenClient(client)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	dir, out := t.TempDir(), t.TempDir()
	put := func(grfPath, rel string) []byte {
		s, ok := c.Lookup(grfPath)
		if !ok {
			t.Fatalf("the client has no %s", grfPath)
		}
		data, err := c.ReadFile(s)
		if err != nil {
			t.Fatal(err)
		}
		write(t, dir, map[string][]byte{rel: data})
		return data
	}
	put(`data\sprite\몬스터\poring.spr`, "sprite/mob/Poring.spr")
	put(`data\sprite\몬스터\poring.act`, "sprite/mob/Poring.act")
	// The smallest map, a model, an effect and a sound.
	var smallest, rsm, strFile, wavFile grf.Source
	for _, s := range c.Files() {
		k := s.Entry.Key
		switch {
		case strings.HasSuffix(k, ".gat") && !strings.Contains(strings.TrimPrefix(k, "data/"), "/"):
			base := strings.TrimSuffix(k, ".gat")
			_, ok1 := c.Lookup(base + ".rsw")
			_, ok2 := c.Lookup(base + ".gnd")
			if ok1 && ok2 && (smallest.Entry.Key == "" || s.Entry.Size < smallest.Entry.Size) {
				smallest = s
			}
		case strings.HasPrefix(k, "data/model/") && strings.HasSuffix(k, ".rsm") && (rsm.Entry.Key == "" || k < rsm.Entry.Key):
			rsm = s
		case k == "data/texture/effect/firewall.str":
			strFile = s
		case strings.HasPrefix(k, "data/wav/") && strings.HasSuffix(k, ".wav") && s.Entry.Size > 1000 && s.Entry.Size < 50000 &&
			(wavFile.Entry.Key == "" || k < wavFile.Entry.Key):
			wavFile = s
		}
	}
	put(wavFile.Entry.Name, "sound/effect/ef_hit1.wav")
	base := strings.TrimSuffix(smallest.Entry.Key, ".gat")
	mapName := path.Base(base)
	for _, ext := range []string{".gat", ".gnd", ".rsw"} {
		put(base+ext, "map/"+mapName+ext)
	}
	data := put(rsm.Entry.Name, "model/thing.rsm")
	m, err := model.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, tex := range m.Textures {
		if _, ok := c.Lookup(`data\texture\` + tex); ok {
			put(`data\texture\`+tex, "texture/"+strings.ReplaceAll(tex, `\`, "/"))
		}
	}
	if strFile.Entry.Key != "" {
		put(strFile.Entry.Name, "effect/firewall.str")
	}
	src := &Source{Dir: dir, Prefix: "mod/example.client/", Formats: RO}
	rep, err := build.Run(build.Options{Out: out, Sources: []build.Source{src}, Brotli: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rejected) > 0 {
		t.Errorf("rejected: %v", rep.Rejected)
	}
	for _, n := range []string{"sprite/mob/Poring", "sound/effect/ef_hit1", "model/thing", "map/" + mapName, "lightmap/" + mapName} {
		if _, _, err := build.ReadAsset(out, "mod/example.client/"+n); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
	cr, err := build.Check(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cr.Broken) > 0 {
		t.Errorf("broken: %v", cr.Broken)
	}
	// The model's textures are the mod's own.
	for missing := range cr.Missing {
		if strings.HasPrefix(missing, "texture/") && len(m.Textures) > 0 {
			for _, tex := range m.Textures {
				if strings.EqualFold("texture/"+strings.TrimSuffix(strings.ReplaceAll(tex, `\`, "/"), filepath.Ext(tex)), missing) {
					t.Errorf("the model's texture %s points to core", missing)
				}
			}
		}
	}
	t.Logf("map %s, model %s, %d assets", mapName, rsm.Entry.Name, rep.Assets)
}
