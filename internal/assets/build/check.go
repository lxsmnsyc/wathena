package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"golang.org/x/image/webp"

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
	// Broken lists files that are missing, do not match their hash, or do
	// not decode, as "name: reason".
	Broken []string
	// Missing maps each name that is referred to but not in the build to
	// the names that refer to it.
	Missing map[string][]string
}

// Manifest entries, loaded from a build folder.
func loadEntries(root *os.Root) (*Manifest, map[string]entry, error) {
	data, err := root.ReadFile("manifest.json")
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
		sb, err := readJSON(root, h)
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
	m, all, err := loadEntries(root)
	if err != nil {
		return nil, err
	}
	rep := &CheckReport{Build: m.Build, Assets: len(all), Missing: map[string][]string{}}

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

	var mu sync.Mutex
	found := map[string][]string{} // referring name -> names it refers to
	work := make(chan job)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Go(func() {
			for j := range work {
				out, err := checkFile(root, j.e)
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
	rep.Broken = append(rep.Broken, checkIndexes(root, names, all, refsOf)...)
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
func checkFile(root *os.Root, e entry) ([]string, error) {
	ext := extOf[e.T]
	if ext == "" {
		return nil, fmt.Errorf("unknown type %q", e.T)
	}
	f, err := root.Open(StoredPath(e.H, ext))
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
func checkIndexes(root *os.Root, names []string, all map[string]entry, refsOf map[string][]string) []string {
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
			data, err := readJSON(root, e.D)
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
