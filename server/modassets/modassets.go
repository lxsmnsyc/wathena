// Package modassets loads the mods' asset builds and client data tables,
// checks them, serves them, and tells the client which art each record
// uses (proposal 0002, review decision 38, decision 0033):
//   - Each mod's build, in assets/ of its folder or .zip file, is checked
//     at start: every file's hash, its decoding and the asset limits,
//     names under mod/<mod id>/ only, and the mod's size limit. A mod
//     whose build fails stops the start. Nothing is converted.
//   - A build is served at /assets/mods/<mod id>/<version>/, with the
//     headers and caching of core's build.
//   - The client data, /client-data.json, lists the active mods' builds,
//     the art of each record by ID, and the client data tables merged
//     with the mods' client/*.yml.
package modassets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"

	clientdata "github.com/lxsmnsyc/wathena/client/data"
	"github.com/lxsmnsyc/wathena/internal/assets"
	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/assets/serve"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// DefaultMaxBytes is the default limit of one mod's build: the sizes of
// its distinct files before compression.
const DefaultMaxBytes = 200 << 20

// Paths the server serves.
const (
	// Prefix is where the mods' builds are, as
	// /assets/mods/<mod id>/<version>/.
	Prefix = "/assets/mods/"
	// ClientDataPath is the client data.
	ClientDataPath = "/client-data.json"
)

// ClientDataVersion is the version of the client data's format. A field
// that goes, changes name or changes meaning raises it. New fields keep
// it.
const ClientDataVersion = 1

// Options configure Load.
type Options struct {
	// MaxBytes limits each mod's build. 0 uses DefaultMaxBytes.
	MaxBytes int64
	// Workers is the number of files checked at once. 0 means one per CPU.
	Workers int
	Log     *slog.Logger
}

// Build is one mod's asset build.
type Build struct {
	Mod, Version string
	// URL is the build's path on the server, ending in "/".
	URL string
	// Names are the names in the build.
	Names map[string]bool
	// Bytes is the build's size before compression.
	Bytes   int64
	handler *serve.Handler
}

// Assets are the loaded builds and the client data.
type Assets struct {
	Builds []*Build
	// ClientData is /client-data.json.
	ClientData []byte
	etag       string
	byMod      map[string]*Build
	closers    []func() error
}

// Close closes the mods' files.
func (a *Assets) Close() error {
	var errs []error
	for _, c := range a.closers {
		errs = append(errs, c())
	}
	return errors.Join(errs...)
}

// Load checks every mod's build and makes the client data. A mod with
// art/ and no build is noted in the log, and its records show
// placeholders.
func Load(m *gamedata.Mods, opts Options) (*Assets, error) {
	if opts.MaxBytes == 0 {
		opts.MaxBytes = DefaultMaxBytes
	}
	log := opts.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	a := &Assets{byMod: map[string]*Build{}}
	var errs []error
	var overrides []clientdata.Override
	for _, md := range m.Set.Mods {
		if md.Builtin {
			continue
		}
		fsys, closeFS, err := md.Open()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", md.ID(), err))
			continue
		}
		a.closers = append(a.closers, closeFS)
		tables, err := ClientTables(fsys, md.ID())
		if err != nil {
			errs = append(errs, err)
		}
		overrides = append(overrides, tables...)
		b, err := loadBuild(fsys, md, opts)
		switch {
		case err != nil:
			errs = append(errs, err)
		case b == nil:
			if fi, err := fs.Stat(fsys, mod.ArtDir); err == nil && fi.IsDir() {
				log.Warn("mods: the mod has art/ but no asset build; its art shows placeholders until wactl mod build-art builds it", "mod", md.ID())
			}
		default:
			a.Builds = append(a.Builds, b)
			a.byMod[b.Mod] = b
			log.Info("mods: asset build checked", "mod", b.Mod, "assets", len(b.Names), "mb", fmt.Sprintf("%.1f", float64(b.Bytes)/(1<<20)))
		}
	}
	if len(errs) > 0 {
		_ = a.Close()
		return nil, errors.Join(errs...)
	}
	merged, err := clientdata.Merge(overrides)
	if err != nil {
		_ = a.Close()
		return nil, err
	}
	doc := a.clientData(m)
	if len(overrides) > 0 {
		doc.Tables = merged
	}
	b, err := json.Marshal(doc)
	if err != nil {
		_ = a.Close()
		return nil, err
	}
	a.ClientData = b
	sum := sha256.Sum256(b)
	a.etag = `"` + hex.EncodeToString(sum[:12]) + `"`
	return a, nil
}

// ClientTables reads a mod's client/*.yml tables.
func ClientTables(fsys fs.FS, modID string) ([]clientdata.Override, error) {
	entries, err := fs.ReadDir(fsys, mod.ClientDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %s: %w", modID, mod.ClientDir, err)
	}
	var out []clientdata.Override
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if path.Ext(e.Name()) != ".yml" {
			return nil, fmt.Errorf("%s: client/%s: client data tables are .yml files", modID, e.Name())
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		if info.Size() > clientdata.MaxTableBytes {
			return nil, fmt.Errorf("%s: client/%s: %d bytes, over the limit of %d", modID, e.Name(), info.Size(), clientdata.MaxTableBytes)
		}
		data, err := fs.ReadFile(fsys, mod.ClientDir+"/"+e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, clientdata.Override{Mod: modID, File: e.Name(), Data: data})
	}
	return out, nil
}

// loadBuild checks a mod's build. It returns nil for a mod without one.
func loadBuild(fsys fs.FS, md *mod.Mod, opts Options) (*Build, error) {
	if _, err := fs.Stat(fsys, mod.AssetsDir+"/manifest.json"); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	sub, err := fs.Sub(fsys, mod.AssetsDir)
	if err != nil {
		return nil, err
	}
	prefix := assets.ModPrefix(md.ID())
	rep, err := build.CheckFS(sub, nil, build.CheckOptions{Prefix: prefix, MaxBytes: opts.MaxBytes, Workers: opts.Workers})
	if err != nil {
		return nil, fmt.Errorf("%s: asset build: %w", md.ID(), err)
	}
	if len(rep.Broken) > 0 {
		shown := rep.Broken[:min(len(rep.Broken), 10)]
		more := ""
		if len(rep.Broken) > len(shown) {
			more = fmt.Sprintf("\n  and %d more", len(rep.Broken)-len(shown))
		}
		return nil, fmt.Errorf("%s: the asset build has %d problems; build it again with wactl mod build-art or wactl mod pack:\n  %s%s",
			md.ID(), len(rep.Broken), strings.Join(shown, "\n  "), more)
	}
	list, err := build.ListFS(sub)
	if err != nil {
		return nil, err
	}
	names := make(map[string]bool, len(list))
	for n := range list {
		names[n] = true
	}
	v := md.Manifest.Version
	return &Build{Mod: md.ID(), Version: v, URL: Prefix + md.ID() + "/" + v + "/", Names: names, Bytes: rep.Bytes, handler: serve.NewFS(sub)}, nil
}

// ServeHTTP serves the builds under Prefix and the client data.
func (a *Assets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == ClientDataPath {
		a.serveClientData(w, r)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, Prefix)
	modID, rest, ok2 := strings.Cut(rest, "/")
	version, file, ok3 := strings.Cut(rest, "/")
	b := a.byMod[modID]
	if !ok || !ok2 || !ok3 || b == nil || version != b.Version {
		http.NotFound(w, r)
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/" + file
	b.handler.ServeHTTP(w, r2)
}

func (a *Assets) serveClientData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Access-Control-Allow-Origin", "*")
	// The client data changes when the server starts with other mods.
	h.Set("Cache-Control", "no-cache")
	h.Set("ETag", a.etag)
	if r.Header.Get("If-None-Match") == a.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Length", strconv.Itoa(len(a.ClientData)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(a.ClientData)
}

// ClientData is the client data's JSON form.
type ClientData struct {
	Version int `json:"version"`
	// Mods are the builds to load after core's, in load order.
	Mods []ModBuild `json:"mods"`
	// The art of records, by ID as a decimal string.
	Items      map[string]ItemArt             `json:"items"`
	Monsters   map[string]gamedata.MonsterArt `json:"monsters"`
	NPCSprites map[string]string              `json:"npc_sprites"`
	Jobs       map[string]map[string]string   `json:"jobs"`
	Skills     map[string]SkillArt            `json:"skills"`
	Companions map[string]string              `json:"companions"`
	Mounts     map[string]string              `json:"mounts"`
	// Tables are the client data tables merged with the mods', by file
	// name, when a mod has any.
	Tables clientdata.Merged `json:"tables,omitempty"`
}

// ModBuild is one mod's build in the client data.
type ModBuild struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	// Assets is the build's URL path, ending in "/".
	Assets string `json:"assets"`
}

// ItemArt is an item's art in the client data.
type ItemArt = mod.ItemArt

// SkillArt is a skill's art in the client data.
type SkillArt struct {
	Icon    string              `json:"icon,omitempty"`
	Effects map[string][]string `json:"effects,omitempty"`
}

// clientData collects the art of every record: the fields records set,
// and for a mod's own records without a field, the art its build has
// under the record's name, such as mod/example.lantern/icon/item/Paper_Lantern.
func (a *Assets) clientData(m *gamedata.Mods) *ClientData {
	d := &ClientData{Version: ClientDataVersion, Mods: []ModBuild{}, Items: map[string]ItemArt{}, Monsters: map[string]gamedata.MonsterArt{},
		NPCSprites: map[string]string{}, Jobs: map[string]map[string]string{}, Skills: map[string]SkillArt{},
		Companions: map[string]string{}, Mounts: map[string]string{}}
	for _, b := range a.Builds {
		d.Mods = append(d.Mods, ModBuild{ID: b.Mod, Version: b.Version, Assets: b.URL})
	}
	id := func(n uint32) string { return strconv.FormatUint(uint64(n), 10) }
	// convention finds a record's art by its name in its mod's build.
	convention := func(record, namespace string) string {
		n, ok := mod.ConventionName(record, namespace)
		if !ok {
			return ""
		}
		modID, _, _ := assets.SplitMod(n)
		if b := a.byMod[modID]; b != nil && b.Names[n] {
			return n
		}
		return ""
	}
	// A job's body sprites are mod/<mod id>/sprite/body/<name>/<m or f>.
	conventionSex := func(record, sex string) string {
		n, ok := mod.ConventionName(record, "sprite/body")
		if !ok {
			return ""
		}
		modID, _, _ := assets.SplitMod(n)
		if b := a.byMod[modID]; b != nil && b.Names[n+"/"+sex] {
			return n + "/" + sex
		}
		return ""
	}
	if m.Items != nil {
		for _, itemID := range slices.Sorted(maps.Keys(m.Items.Art)) {
			d.Items[id(itemID)] = m.Items.Art[itemID]
		}
		if m.Items.Overlay != nil {
			for _, r := range m.Items.Overlay.Records {
				itemID, ok := m.Items.IDs[r.Name]
				if !ok || r.Mod == mod.Core {
					continue
				}
				art := d.Items[id(itemID)]
				art.Icon = cmpOr(art.Icon, convention(r.Name, "icon/item"))
				art.Image = cmpOr(art.Image, convention(r.Name, "image/item"))
				art.DropSprite = cmpOr(art.DropSprite, convention(r.Name, "sprite/item"))
				if art != (ItemArt{}) {
					d.Items[id(itemID)] = art
				}
			}
		}
	}
	for monsterID, mon := range m.Monsters {
		if mon.Art.Sprite != "" || len(mon.Art.Sounds) > 0 {
			d.Monsters[id(monsterID)] = mon.Art
		}
	}
	for spriteID, name := range m.NPCArt {
		d.NPCSprites[id(spriteID)] = name
	}
	if m.Jobs != nil {
		for jobID, r := range m.Jobs.Records {
			art := maps.Clone(r.Art)
			if r.Data.Info.Mod != "" && r.Data.Info.Mod != mod.Core {
				for _, sex := range []string{"m", "f"} {
					if art[sex] == "" {
						if c := conventionSex(r.Data.Info.Name, sex); c != "" {
							if art == nil {
								art = map[string]string{}
							}
							art[sex] = c
						}
					}
				}
			}
			if len(art) > 0 {
				d.Jobs[strconv.Itoa(jobID)] = art
			}
		}
	}
	if m.Skills != nil {
		for _, s := range m.Skills.All() {
			art := SkillArt{Icon: s.Art.Icon, Effects: s.Art.Effects}
			if s.Mod != "" && art.Icon == "" {
				art.Icon = convention(s.Mod+":"+s.Name, "icon/skill")
			}
			if art.Icon != "" || len(art.Effects) > 0 {
				d.Skills[strconv.Itoa(s.ID)] = art
			}
		}
	}
	if m.Companions != nil {
		for _, k := range []struct {
			kind companion.Kind
			to   map[string]string
		}{{companion.Companions, d.Companions}, {companion.Mounts, d.Mounts}} {
			t := m.Companions.Table(k.kind)
			if t == nil {
				continue
			}
			for _, n := range t.Names() {
				if r := t.ByName(n); r != nil && r.Art != "" {
					k.to[strconv.Itoa(r.ID)] = r.Art
				}
			}
		}
	}
	return d
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Refs lists the art names that records name in their fields, with the
// mod that set each, for wactl mod validate.
type Ref struct {
	// Mod is the mod whose record names the art, and Record the record,
	// such as items example.lantern:Paper_Lantern.
	Mod, Record, Name string
}

// ArtRefs lists the art names records name.
func ArtRefs(m *gamedata.Mods) []Ref {
	var out []Ref
	add := func(modID, record, name string) {
		if name != "" {
			out = append(out, Ref{Mod: modID, Record: record, Name: name})
		}
	}
	if m.Items != nil && m.Items.Overlay != nil {
		for _, r := range m.Items.Overlay.Records {
			art := m.Items.Art[m.Items.IDs[r.Name]]
			for _, f := range []string{"icon", "image", "drop_sprite", "view_sprite"} {
				if by := r.SetBy[f]; by != "" {
					add(by, "items "+r.Name, map[string]string{"icon": art.Icon, "image": art.Image, "drop_sprite": art.DropSprite, "view_sprite": art.ViewSprite}[f])
				}
			}
		}
	}
	if m.MonsterOverlay != nil {
		byName := make(map[string]gamedata.Monster, len(m.Monsters))
		for _, mon := range m.Monsters {
			byName[mon.AegisName] = mon
		}
		for _, r := range m.MonsterOverlay.Records {
			mon := byName[r.Name]
			add(r.SetBy["sprite"], "monsters "+r.Name, mon.Art.Sprite)
			for _, k := range slices.Sorted(maps.Keys(mon.Art.Sounds)) {
				add(r.SetBy["sounds"], "monsters "+r.Name, mon.Art.Sounds[k])
			}
		}
	}
	if m.NPCOverlay != nil {
		for _, r := range m.NPCOverlay.Records {
			if s, ok := r.Fields["sprite"].(string); ok && mod.IsArtName(s) {
				add(r.SetBy["sprite"], "npcs "+r.Name, s)
			}
		}
	}
	if m.Jobs != nil && m.Jobs.Overlay != nil {
		for _, r := range m.Jobs.Overlay.Records {
			if id, ok := m.Jobs.ID(r.Name); ok {
				rec := m.Jobs.Records[id]
				for _, sex := range []string{"m", "f"} {
					add(r.SetBy["look"], "jobs "+r.Name, rec.Art[sex])
				}
			}
		}
	}
	if m.Skills != nil && m.SkillOverlay != nil {
		for _, r := range m.SkillOverlay.Records {
			s := m.Skills.ByName(r.Name)
			if s == nil {
				continue
			}
			add(r.SetBy["icon"], "skills "+r.Name, s.Art.Icon)
			for _, k := range slices.Sorted(maps.Keys(s.Art.Effects)) {
				for _, e := range s.Art.Effects[k] {
					add(r.SetBy["effects"], "skills "+r.Name, e)
				}
			}
		}
	}
	for i, o := range m.CompanionOverlays {
		kind := []companion.Kind{companion.Companions, companion.Mounts}[min(i, 1)]
		t := m.Companions.Table(kind)
		for _, r := range o.Records {
			if rec := t.ByName(r.Name); rec != nil {
				add(r.SetBy["look"], kind.Name+" "+r.Name, rec.Art)
			}
		}
	}
	slices.SortFunc(out, func(x, y Ref) int { return strings.Compare(x.Record+x.Name, y.Record+y.Name) })
	return out
}
