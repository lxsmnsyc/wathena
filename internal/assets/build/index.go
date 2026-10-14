package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/andybalholm/brotli"
)

// Dependency indexes. An asset that refers to other names, such as a map
// or a model, gets an index: the manifest entries of every name it uses,
// directly or through the assets it uses. The client loads a map's index
// with the map, so it can fetch everything the map needs without loading
// manifest shards. See docs/specs/assets-v1.md, "Dependency indexes".

// refTypes are the manifest types whose files refer to other names.
var refTypes = map[string]bool{"sprite": true, "model": true, "map": true, "effect": true}

// maxJSONBytes is the largest shard or index a build reads back.
const maxJSONBytes = 16 << 20

// cacheEntry is a cache record: an asset's entry and the names it refers
// to.
type cacheEntry struct {
	entry
	Refs []string `json:"r,omitempty"`
	// HasRefs is set once Refs is known, which tells an asset without
	// references from a record an older build wrote.
	HasRefs bool `json:"k,omitempty"`
}

// fileRefs returns the names a stored file refers to. Files that do not
// decode have none; `wactl assets check` reports them.
func fileRefs(t string, data []byte) []string {
	if !refTypes[t] {
		return nil
	}
	refs, err := decodeRefs(t, data)
	if err != nil {
		return nil
	}
	return refs
}

// readStored reads a stored asset and removes its Brotli layer.
func readStored(fsys fs.FS, e entry) ([]byte, error) {
	ext := extOf[e.T]
	f, err := fsys.Open(filepath.ToSlash(StoredPath(e.H, ext)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var r io.Reader = f
	if types[ext].brotli {
		r = brotli.NewReader(f)
	}
	return io.ReadAll(io.LimitReader(r, 64<<20+1))
}

// readLimited reads a file of at most limit bytes.
func readLimited(fsys fs.FS, p string, limit int64) ([]byte, error) {
	f, err := fsys.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is over %d bytes", p, limit)
	}
	return data, nil
}

// jsonPath is where a shard or index is stored. Both are stored only as
// Brotli.
func jsonPath(h string) string {
	return objectPath(h, "json") + ".br"
}

// storeJSON stores a shard or index under its hash, unless it is already
// stored, and returns the hash.
func storeJSON(root *os.Root, data []byte, quality int) (string, error) {
	sum := sha256.Sum256(data)
	h := hex.EncodeToString(sum[:])
	p := jsonPath(h)
	if _, err := root.Stat(p); err == nil {
		return h, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	var b bytes.Buffer
	w := brotli.NewWriterLevel(&b, quality)
	if _, err := w.Write(data); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return h, writeFile(root, p, b.Bytes())
}

// readJSON reads a stored shard or index and checks its hash.
func readJSON(fsys fs.FS, h string) ([]byte, error) {
	f, err := fsys.Open(filepath.ToSlash(jsonPath(h)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(brotli.NewReader(f), maxJSONBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxJSONBytes {
		return nil, fmt.Errorf("%s is too large", h)
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != h {
		return nil, fmt.Errorf("%s does not match its hash", h)
	}
	return data, nil
}

// closure returns the entries of every name that name uses, directly or
// through other assets, that the build has.
func closure(name string, refs map[string][]string, entries map[string]entry) map[string]entry {
	out := map[string]entry{}
	stack := append([]string(nil), refs[name]...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, done := out[n]; done || n == name {
			continue
		}
		e, ok := entries[n]
		if !ok {
			continue
		}
		e.D = ""
		out[n] = e
		stack = append(stack, refs[n]...)
	}
	return out
}

// writeIndexes stores an index for every asset that uses other names, and
// records it in the asset's entry.
func writeIndexes(root *os.Root, entries map[string]entry, refs map[string][]string, quality, workers int) error {
	names := make([]string, 0, len(refs))
	for n, r := range refs {
		if len(r) > 0 {
			names = append(names, n)
		}
	}
	// Workers only read entries. The results are applied after they stop.
	var mu sync.Mutex
	var firstErr error
	index := map[string]string{}
	work := make(chan string)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for name := range work {
				deps := closure(name, refs, entries)
				if len(deps) == 0 {
					continue
				}
				// encoding/json sorts map keys, so the bytes are stable.
				data, err := json.Marshal(deps)
				var h string
				if err == nil {
					h, err = storeJSON(root, data, quality)
				}
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = fmt.Errorf("index for %s: %w", name, err)
				}
				index[name] = h
				mu.Unlock()
			}
		})
	}
	for _, n := range names {
		work <- n
	}
	close(work)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	for n, h := range index {
		e := entries[n]
		e.D = h
		entries[n] = e
	}
	return nil
}
