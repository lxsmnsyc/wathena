// Package build combines asset sources into a build, as described in
// docs/specs/assets-v1.md, "Builds".
package build

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/lxsmnsyc/wathena/internal/assets"
)

// Item is one asset a source can provide. Produce is only called for items
// that win the layering and are not in the cache.
type Item struct {
	Name string
	// Ext is the output extension, such as "wspr".
	Ext string
	// CacheKey identifies the input and converter version. Two items with
	// the same key must produce the same bytes. Empty disables caching.
	CacheKey string
	Produce  func() ([]byte, error)
}

// Source lists the items it provides.
type Source interface {
	Name() string
	Items() ([]Item, error)
}

// Rejecter is a source that leaves out files it cannot use, such as a
// pack's file in the wrong folder. Run adds them to the report.
type Rejecter interface {
	Rejected() []string
}

// Options control a build.
type Options struct {
	Out     string
	Sources []Source
	// Brotli is the quality for Brotli copies, 0 to 11.
	Brotli int
	// Workers is the number of parallel conversions. 0 means one per CPU.
	Workers int
	// Time is the build ID's timestamp. The zero time is now. A fixed
	// time makes the same sources give the same build, as a mod's
	// package needs.
	Time time.Time
}

// SourceReport counts what one source did.
type SourceReport struct {
	Name     string
	Provided int
	Replaced int
}

// Report describes a finished build.
type Report struct {
	BuildID   string
	Sources   []SourceReport
	Assets    int
	Converted int
	Cached    int
	// Bytes is the uncompressed size of all assets.
	Bytes    int64
	Rejected []string
	Duration time.Duration
}

// types maps output extensions to manifest types, and marks which are
// binary formats that get a Brotli copy.
var types = map[string]struct {
	t      string
	brotli bool
}{
	"wspr": {"sprite", true},
	"wpal": {"palette", true},
	"wmap": {"map", true},
	"weff": {"effect", true},
	"webp": {"texture", false},
	"glb":  {"model", true},
	"webm": {"sound", false},
	"mp3":  {"music", false},
	"json": {"table", true},
}

type entry struct {
	H string `json:"h"`
	T string `json:"t"`
	S int    `json:"s"`
	// D is the hash of the asset's dependency index, if it has one.
	D string `json:"d,omitempty"`
}

// Manifest is manifest.json.
type Manifest struct {
	Contract string   `json:"contract"`
	Build    string   `json:"build"`
	Shards   []string `json:"shards"`
}

const cacheFile = ".cache.json"

// Run builds the assets into opts.Out.
func Run(opts Options) (*Report, error) {
	start := time.Now()
	if opts.Out == "" {
		return nil, errors.New("build: no output folder")
	}
	// Assets are public files, and the server may run as another user,
	// such as the container image's.
	if err := os.MkdirAll(opts.Out, 0o755); err != nil { // #nosec G301 -- assets are public files.
		return nil, err
	}
	root, err := os.OpenRoot(opts.Out)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	rep := &Report{}
	// Layer the sources. A later source replaces names from earlier ones.
	winners := map[string]Item{}
	for _, src := range opts.Sources {
		items, err := src.Items()
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", src.Name(), err)
		}
		if r, ok := src.(Rejecter); ok {
			rep.Rejected = append(rep.Rejected, r.Rejected()...)
		}
		sr := SourceReport{Name: src.Name()}
		for _, it := range items {
			if err := assets.ValidName(it.Name); err != nil {
				rep.Rejected = append(rep.Rejected, fmt.Sprintf("%s: %q: invalid name", src.Name(), it.Name))
				continue
			}
			if _, ok := types[it.Ext]; !ok {
				rep.Rejected = append(rep.Rejected, fmt.Sprintf("%s: %s: unknown extension %q", src.Name(), it.Name, it.Ext))
				continue
			}
			if _, ok := winners[it.Name]; ok {
				sr.Replaced++
			}
			winners[it.Name] = it
			sr.Provided++
		}
		rep.Sources = append(rep.Sources, sr)
	}

	cache := loadCache(root)
	names := make([]string, 0, len(winners))
	for n := range winners {
		names = append(names, n)
	}
	sort.Strings(names)

	var mu sync.Mutex
	entries := make(map[string]entry, len(names))
	refs := map[string][]string{}
	newCache := map[string]cacheEntry{}
	work := make(chan string)
	var wg sync.WaitGroup
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	for range workers {
		wg.Go(func() {
			for name := range work {
				it := winners[name]
				ce, cached, err := produce(root, it, cache, opts.Brotli)
				e := ce.entry
				mu.Lock()
				switch {
				case err != nil:
					rep.Rejected = append(rep.Rejected, fmt.Sprintf("%s: %v", name, err))
				default:
					entries[name] = e
					refs[name] = ce.Refs
					if it.CacheKey != "" {
						newCache[it.CacheKey] = ce
					}
					rep.Bytes += int64(e.S)
					if cached {
						rep.Cached++
					} else {
						rep.Converted++
					}
				}
				mu.Unlock()
			}
		})
	}
	for _, n := range names {
		work <- n
	}
	close(work)
	wg.Wait()
	rep.Assets = len(entries)
	sort.Strings(rep.Rejected)

	if err := writeIndexes(root, entries, refs, opts.Brotli, workers); err != nil {
		return nil, err
	}
	id, err := writeManifest(root, entries, opts.Brotli, opts.Time)
	if err != nil {
		return nil, err
	}
	rep.BuildID = id
	if err := saveCache(root, newCache); err != nil {
		return nil, err
	}
	rep.Duration = time.Since(start)
	return rep, nil
}

// produce returns the manifest entry for an item and the names it refers
// to, converting it unless the cache has its key and the output file still
// exists.
func produce(root *os.Root, it Item, cache map[string]cacheEntry, quality int) (cacheEntry, bool, error) {
	info := types[it.Ext]
	if ce, ok := cache[it.CacheKey]; ok && it.CacheKey != "" && ce.T == info.t {
		if _, err := root.Stat(StoredPath(ce.H, it.Ext)); err == nil {
			if !ce.HasRefs {
				// Builds before dependency indexes did not record
				// references.
				data, err := readStored(root.FS(), ce.entry)
				if err != nil {
					return cacheEntry{}, false, err
				}
				ce.Refs, ce.HasRefs = fileRefs(ce.T, data), true
			}
			ce.D = ""
			return ce, true, nil
		}
	}
	data, err := it.Produce()
	if err != nil {
		return cacheEntry{}, false, err
	}
	sum := sha256.Sum256(data)
	h := hex.EncodeToString(sum[:])
	stored := data
	if info.brotli {
		// Binary formats are stored only as Brotli. The hash is of the
		// uncompressed content, which the client checks after decoding.
		var b bytes.Buffer
		w := brotli.NewWriterLevel(&b, quality)
		if _, err := w.Write(data); err != nil {
			return cacheEntry{}, false, err
		}
		if err := w.Close(); err != nil {
			return cacheEntry{}, false, err
		}
		stored = b.Bytes()
	}
	if err := writeFile(root, StoredPath(h, it.Ext), stored); err != nil {
		return cacheEntry{}, false, err
	}
	e := entry{H: h, T: info.t, S: len(data)}
	return cacheEntry{entry: e, Refs: fileRefs(info.t, data), HasRefs: true}, false, nil
}

// StoredPath is where an asset with a hash and extension is stored.
// Binary formats end in ".br".
func StoredPath(h, ext string) string {
	p := objectPath(h, ext)
	if types[ext].brotli {
		p += ".br"
	}
	return p
}

// Stored reports whether ext is an extension a build stores, and whether
// files with it are stored as Brotli. Tables, shards, and indexes are
// JSON, stored as Brotli.
func Stored(ext string) (known, brotli bool) {
	t, ok := types[ext]
	return ok, t.brotli
}

// objectPath is where content with a hash is stored. It uses only the hash
// and a fixed extension, never the asset name.
func objectPath(h, ext string) string {
	return filepath.Join(h[:2], h+"."+ext)
}

// writeFile writes a file through a temporary name, so a reader never sees
// a partial file.
func writeFile(root *os.Root, p string, data []byte) error {
	if err := root.MkdirAll(filepath.Dir(p), 0o755); err != nil { // #nosec G301 -- assets are public files.
		return err
	}
	var suffix [8]byte
	_, _ = rand.Read(suffix[:])
	tmp := p + ".tmp" + hex.EncodeToString(suffix[:])
	if err := root.WriteFile(tmp, data, 0o644); err != nil { // #nosec G306 -- assets are public files.
		return err
	}
	return root.Rename(tmp, p)
}

// ShardOf returns the manifest shard for a name: the first byte of its SHA-256.
func ShardOf(name string) int {
	sum := sha256.Sum256([]byte(name))
	return int(sum[0])
}

func writeManifest(root *os.Root, entries map[string]entry, quality int, at time.Time) (string, error) {
	shards := make([]map[string]entry, 256)
	for i := range shards {
		shards[i] = map[string]entry{}
	}
	for name, e := range entries {
		shards[ShardOf(name)][name] = e
	}
	m := Manifest{Contract: assets.Contract, Shards: make([]string, 256)}
	all := sha256.New()
	for i, sh := range shards {
		// encoding/json sorts map keys, so the bytes are stable.
		data, err := json.Marshal(sh)
		if err != nil {
			return "", err
		}
		h, err := storeJSON(root, data, quality)
		if err != nil {
			return "", err
		}
		m.Shards[i] = h
		all.Write([]byte(h))
	}
	if at.IsZero() {
		at = time.Now()
	}
	m.Build = at.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(all.Sum(nil))[:12]
	data, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		return "", err
	}
	return m.Build, writeFile(root, "manifest.json", append(data, '\n'))
}

func loadCache(root *os.Root) map[string]cacheEntry {
	c := map[string]cacheEntry{}
	data, err := root.ReadFile(cacheFile)
	if err != nil {
		return c
	}
	if json.Unmarshal(data, &c) != nil {
		return map[string]cacheEntry{}
	}
	return c
}

func saveCache(root *os.Root, c map[string]cacheEntry) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return writeFile(root, cacheFile, data)
}
