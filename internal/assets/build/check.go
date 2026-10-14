package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"golang.org/x/image/webp"

	"github.com/lxsmnsyc/wathena/internal/assets"
	"github.com/lxsmnsyc/wathena/internal/assets/effect"
	"github.com/lxsmnsyc/wathena/internal/assets/model"
	"github.com/lxsmnsyc/wathena/internal/assets/sound"
	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	"github.com/lxsmnsyc/wathena/internal/assets/wpal"
	"github.com/lxsmnsyc/wathena/internal/assets/wspr"
)

// Limits from assets-v1, "Limits", for types whose decoders do not check
// them.
const (
	maxTextureSize = 4096
	maxMusicBytes  = 32 << 20
	maxTableBytes  = 16 << 20
)

// Reference is a name that something refers to.
type Reference struct {
	// From is the name, or other source, that refers.
	From string
	Name string
}

// CheckReport describes a build's problems.
type CheckReport struct {
	Build  string
	Assets int
	Files  int
	// Bytes is the size of the build's distinct files, before
	// compression.
	Bytes int64
	// Broken lists files that are missing, do not match their hash, or do
	// not decode, as "name: reason".
	Broken []string
	// Missing maps each name that is referred to but not in the build to
	// the names that refer to it.
	Missing map[string][]string
}

// Manifest entries, loaded from a build folder.
func loadEntries(fsys fs.FS) (*Manifest, map[string]entry, error) {
	data, err := readLimited(fsys, "manifest.json", maxJSONBytes)
	if err != nil {
		return nil, nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, nil, fmt.Errorf("manifest.json: %w", err)
	}
	if len(m.Shards) != 256 {
		return nil, nil, fmt.Errorf("manifest.json has %d shards, want 256", len(m.Shards))
	}
	all := map[string]entry{}
	for i, h := range m.Shards {
		if !validHash(h) {
			return nil, nil, fmt.Errorf("manifest.json: shard %d hash %q", i, h)
		}
		sb, err := readJSON(fsys, h)
		if err != nil {
			return nil, nil, fmt.Errorf("shard %d: %w", i, err)
		}
		var sh map[string]entry
		if err := json.Unmarshal(sb, &sh); err != nil {
			return nil, nil, fmt.Errorf("shard %d: %w", i, err)
		}
		for n, e := range sh {
			if ShardOf(n) != i || !validHash(e.H) || (e.D != "" && !validHash(e.D)) {
				return nil, nil, fmt.Errorf("shard %d: entry %q", i, n)
			}
			all[n] = e
		}
	}
	return &m, all, nil
}

func validHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil && strings.ToLower(h) == h
}

// extOf maps manifest types to stored extensions. Each type has one.
var extOf = map[string]string{}

func init() {
	for ext, t := range types {
		extOf[t.t] = ext
	}
}

// Check verifies every file in a build and the references between
// assets. refs adds references from outside the build, such as client
// data tables.
func Check(dir string, refs []Reference) (*CheckReport, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return CheckFS(root.FS(), refs, CheckOptions{})
}

// CheckOptions are the extra rules of a mod's build.
type CheckOptions struct {
	// Prefix is what every name must start with, such as
	// mod/example.lantern/. Every name must then also be valid and in a
	// namespace of its type. Empty checks no names.
	Prefix string
	// MaxBytes limits the build's content: the sizes of its distinct
	// files, before compression. 0 is no limit.
	MaxBytes int64
	// Workers is the number of files checked at once. 0 means one per CPU.
	Workers int
}

// CheckFS checks a build in a file system, such as a folder inside a
// mod's .zip file.
func CheckFS(fsys fs.FS, refs []Reference, opts CheckOptions) (*CheckReport, error) {
	m, all, err := loadEntries(fsys)
	if err != nil {
		return nil, err
	}
	rep := &CheckReport{Build: m.Build, Assets: len(all), Missing: map[string][]string{}}
	if opts.Prefix != "" {
		for n, e := range all {
			switch {
			case !strings.HasPrefix(n, opts.Prefix):
				rep.Broken = append(rep.Broken, fmt.Sprintf("%s: the name is not under %s", n, opts.Prefix))
			case assets.ValidName(n) != nil:
				rep.Broken = append(rep.Broken, fmt.Sprintf("%s: the name is not valid", n))
			case assets.TypeOf(n) != e.T:
				rep.Broken = append(rep.Broken, fmt.Sprintf("%s: a %s in the namespace of %ss", n, e.T, assets.TypeOf(n)))
			}
		}
	}

	// Each stored file is checked once, for the first name that uses it.
	type job struct {
		name string
		e    entry
	}
	byHash := map[string]job{}
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		e := all[n]
		if _, ok := byHash[e.H+e.T]; !ok {
			byHash[e.H+e.T] = job{n, e}
		}
	}
	rep.Files = len(byHash)
	for _, j := range byHash {
		rep.Bytes += int64(j.e.S)
	}
	if opts.MaxBytes > 0 && rep.Bytes > opts.MaxBytes {
		// Nothing is read: the build is refused as a whole.
		rep.Broken = append(rep.Broken, fmt.Sprintf("the build holds %d bytes, over the limit of %d", rep.Bytes, opts.MaxBytes))
		sort.Strings(rep.Broken)
		return rep, nil
	}
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	var mu sync.Mutex
	found := map[string][]string{} // referring name -> names it refers to
	work := make(chan job)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for j := range work {
				out, err := checkFile(fsys, j.e)
				mu.Lock()
				if err != nil {
					rep.Broken = append(rep.Broken, fmt.Sprintf("%s: %v", j.name, err))
				} else if len(out) > 0 {
					found[j.e.H+j.e.T] = out
				}
				mu.Unlock()
			}
		})
	}
	for _, j := range byHash {
		work <- j
	}
	close(work)
	wg.Wait()

	// Every name shares its file's references.
	refsOf := map[string][]string{}
	for _, n := range names {
		e := all[n]
		refsOf[n] = found[e.H+e.T]
		for _, to := range refsOf[n] {
			refs = append(refs, Reference{From: n, Name: to})
		}
	}
	rep.Broken = append(rep.Broken, checkIndexes(fsys, names, all, refsOf)...)
	for _, r := range refs {
		if _, ok := all[r.Name]; !ok {
			rep.Missing[r.Name] = append(rep.Missing[r.Name], r.From)
		}
	}
	for k := range rep.Missing {
		sort.Strings(rep.Missing[k])
	}
	sort.Strings(rep.Broken)
	return rep, nil
}

// checkFile reads, verifies, and decodes one stored file, and returns the
// names it refers to.
func checkFile(fsys fs.FS, e entry) ([]string, error) {
	ext := extOf[e.T]
	if ext == "" {
		return nil, fmt.Errorf("unknown type %q", e.T)
	}
	f, err := fsys.Open(filepath.ToSlash(StoredPath(e.H, ext)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var r io.Reader = f
	if types[ext].brotli {
		r = brotli.NewReader(f)
	}
	// Reading one byte past the largest allowed size is enough to tell
	// that a file is too big, without reading all of a huge file.
	data, err := io.ReadAll(io.LimitReader(r, 64<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) != e.S {
		return nil, fmt.Errorf("%d bytes, the manifest says %d", len(data), e.S)
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != e.H {
		return nil, fmt.Errorf("content does not match its hash")
	}
	return decodeRefs(e.T, data)
}

// decodeRefs decodes an asset of a manifest type and returns the names it
// refers to.
func decodeRefs(t string, data []byte) ([]string, error) {
	switch t {
	case "sprite":
		s, err := wspr.Decode(data)
		if err != nil {
			return nil, err
		}
		var refs []string
		for _, ev := range s.Events {
			// Events with a slash name sounds; others are markers.
			if strings.Contains(ev, "/") {
				refs = append(refs, ev)
			}
		}
		return refs, nil
	case "palette":
		_, err := wpal.Decode(data)
		return nil, err
	case "texture":
		c, err := webp.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if c.Width > maxTextureSize || c.Height > maxTextureSize {
			return nil, fmt.Errorf("texture is %d × %d", c.Width, c.Height)
		}
		return nil, nil
	case "model":
		return model.Inspect(data)
	case "map":
		mp, err := wmap.Decode(data)
		if err != nil {
			return nil, err
		}
		return mp.Deps, nil
	case "effect":
		ef, err := effect.Decode(data)
		if err != nil {
			return nil, err
		}
		return ef.Textures, nil
	case "sound":
		return nil, sound.CheckWebM(data)
	case "table":
		// A table is a JSON object. Its fields depend on the table.
		if len(data) > maxTableBytes {
			return nil, fmt.Errorf("table is %d bytes", len(data))
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, fmt.Errorf("table: %w", err)
		}
		return nil, nil
	case "music":
		if len(data) > maxMusicBytes {
			return nil, fmt.Errorf("music is %d bytes", len(data))
		}
		if !sound.IsMP3(data) {
			return nil, fmt.Errorf("not an mp3 file")
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unknown type %q", t)
}

// checkIndexes checks that each asset's dependency index lists exactly the
// entries of the names it uses.
func checkIndexes(fsys fs.FS, names []string, all map[string]entry, refsOf map[string][]string) []string {
	var broken []string
	indexes := map[string]map[string]entry{}
	for _, n := range names {
		e := all[n]
		want := closure(n, refsOf, all)
		if e.D == "" {
			if len(want) > 0 {
				broken = append(broken, n+": no dependency index")
			}
			continue
		}
		got, ok := indexes[e.D]
		if !ok {
			data, err := readJSON(fsys, e.D)
			if err == nil {
				err = json.Unmarshal(data, &got)
			}
			if err != nil {
				broken = append(broken, fmt.Sprintf("%s: dependency index: %v", n, err))
				continue
			}
			indexes[e.D] = got
		}
		if !maps.Equal(got, want) {
			broken = append(broken, n+": dependency index does not match the manifest")
		}
	}
	return broken
}

// Listed is one manifest entry, as List returns it.
type Listed struct {
	Hash string
	Type string
	Size int
}

// List reads a build's manifest and returns every name in it.
func List(dir string) (map[string]Listed, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return ListFS(root.FS())
}

// ListFS is List for a build in a file system.
func ListFS(fsys fs.FS) (map[string]Listed, error) {
	_, all, err := loadEntries(fsys)
	if err != nil {
		return nil, err
	}
	out := make(map[string]Listed, len(all))
	for n, e := range all {
		out[n] = Listed{Hash: e.H, Type: e.T, Size: e.S}
	}
	return out, nil
}

// ReadAsset reads one asset from a build by name, checks it against its
// hash, and returns its content and manifest type.
func ReadAsset(dir, name string) ([]byte, string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = root.Close() }()
	h := sha256.Sum256([]byte(name))
	m, err := root.ReadFile("manifest.json")
	if err != nil {
		return nil, "", err
	}
	var man Manifest
	if err := json.Unmarshal(m, &man); err != nil || len(man.Shards) != 256 {
		return nil, "", fmt.Errorf("manifest.json: invalid")
	}
	data, err := readJSON(root.FS(), man.Shards[h[0]])
	if err != nil {
		return nil, "", err
	}
	var shard map[string]entry
	if err := json.Unmarshal(data, &shard); err != nil {
		return nil, "", err
	}
	e, ok := shard[name]
	if !ok {
		return nil, "", fmt.Errorf("%s is not in the build", name)
	}
	content, err := readStored(root.FS(), e)
	if err != nil {
		return nil, "", err
	}
	if sum := sha256.Sum256(content); hex.EncodeToString(sum[:]) != e.H || len(content) != e.S {
		return nil, "", fmt.Errorf("%s does not match its hash", name)
	}
	return content, e.T, nil
}
