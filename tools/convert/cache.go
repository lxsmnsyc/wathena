package convert

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing/fstest"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// cacheFormat is the layout of a cache file. Change it when cacheEntry
// or the file's header changes, so old files are not read.
const cacheFormat = 1

// cacheMagic starts every cache file.
const cacheMagic = "wathena core cache"

// CacheOptions are where LoadCoreCached keeps converted output.
type CacheOptions struct {
	// Dir is the cache folder. Empty converts every time and writes
	// nothing.
	Dir string
	// Version names the converter: the build of the program that
	// converts. A file from another version is not used. waserver gives
	// its build information, or the hash of its executable.
	Version string
}

// CacheStatus says how LoadCoreCached got its Core.
type CacheStatus string

const (
	// CacheOff: no cache folder; the extracts were converted.
	CacheOff CacheStatus = "off"
	// CacheHit: the Core came from the cache file.
	CacheHit CacheStatus = "hit"
	// CacheMiss: there was no cache file, or it was for other extracts,
	// another converter version or another mode. The extracts were
	// converted and the file written.
	CacheMiss CacheStatus = "miss"
	// CacheBad: the cache file could not be read or was damaged. The
	// extracts were converted and the file written again.
	CacheBad CacheStatus = "bad"
)

// CacheResult says what LoadCoreCached did.
type CacheResult struct {
	Status CacheStatus
	// Path is the cache file, or "" when the cache is off.
	Path string
	// ReadErr is why a CacheBad file was not used.
	ReadErr error
	// WriteErr is why the cache file could not be written. The Core is
	// still good; the next start converts again.
	WriteErr error
}

// cacheEntry is what a cache file stores, gob encoded.
type cacheEntry struct {
	Mode  string
	Items []*Item
	Files map[string][]byte
}

// LoadCoreCached is LoadCore with a cache on disk. The cache file is
// keyed by a hash of the extracts it converts, o.Version and the mode.
// A file with another key, or one that is damaged, is converted again
// and written over. Errors in reading or writing the cache do not fail
// the load: CacheResult reports them.
func LoadCoreCached(mode reference.Mode, o CacheOptions) (*Core, CacheResult, error) {
	return loadCoreCached(mode, o, referenceReader(mode), convertCore)
}

// loadCoreCached is LoadCoreCached with the extracts read by read and
// converted by conv, for tests.
func loadCoreCached(mode reference.Mode, o CacheOptions, read func(string) ([]byte, error), conv func(*coreInputs) (*Core, error)) (*Core, CacheResult, error) {
	in, err := readCoreInputs(mode, read)
	if err != nil {
		return nil, CacheResult{}, err
	}
	if o.Dir == "" {
		c, err := conv(in)
		return c, CacheResult{Status: CacheOff}, err
	}
	key := cacheKey(mode, o.Version, in)
	r := CacheResult{Path: filepath.Join(o.Dir, "core-"+string(mode)+".cache")}
	entry, hit, err := readCache(r.Path, key)
	switch {
	case err != nil:
		r.Status, r.ReadErr = CacheBad, err
	case hit:
		c, err := coreFromCache(in, entry)
		if err == nil {
			r.Status = CacheHit
			return c, r, nil
		}
		r.Status, r.ReadErr = CacheBad, err
	default:
		r.Status = CacheMiss
	}
	c, err := conv(in)
	if err != nil {
		return nil, r, err
	}
	r.WriteErr = writeCache(r.Path, key, c)
	return c, r, nil
}

// cacheKey hashes everything the converted output depends on.
func cacheKey(mode reference.Mode, version string, in *coreInputs) string {
	h := sha256.New()
	field := func(s string) {
		_, _ = fmt.Fprintf(h, "%d:%s\n", len(s), s)
	}
	field(cacheMagic)
	field(strconv.Itoa(cacheFormat))
	field(version)
	field(string(mode))
	file := func(name string, data []byte) {
		sum := sha256.Sum256(data)
		field(name)
		field(hex.EncodeToString(sum[:]))
	}
	for i, name := range extractNames {
		file(name, in.files[i])
	}
	for i, name := range in.partNames {
		file(name, in.parts[i])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// readCache reads a cache file. hit is false, with no error, when there
// is no file or its key differs. A file that is not a cache file, or
// whose body does not match its hash, is an error.
//
// The file is three header lines, then the gob body:
//
//	wathena core cache <format>
//	<key>
//	<sha-256 of the body>
func readCache(path, key string) (*cacheEntry, bool, error) {
	f, err := os.Open(path) // #nosec G304 -- a file in the cache folder the owner named.
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = f.Close() }()
	br := bufio.NewReaderSize(f, 1<<20)
	line := func() (string, error) {
		s, err := br.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("%s: the header is cut short", path)
		}
		return s[:len(s)-1], nil
	}
	magic, err := line()
	if err != nil {
		return nil, false, err
	}
	if magic != cacheMagic+" "+strconv.Itoa(cacheFormat) {
		// Another format: a miss, written over.
		if len(magic) > len(cacheMagic) && magic[:len(cacheMagic)] == cacheMagic {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("%s is not a cache file", path)
	}
	fileKey, err := line()
	if err != nil {
		return nil, false, err
	}
	if fileKey != key {
		return nil, false, nil
	}
	sum, err := line()
	if err != nil {
		return nil, false, err
	}
	body, err := io.ReadAll(br)
	if err != nil {
		return nil, false, err
	}
	got := sha256.Sum256(body)
	if hex.EncodeToString(got[:]) != sum {
		return nil, false, fmt.Errorf("%s: the body does not match its hash", path)
	}
	var e cacheEntry
	if err := gob.NewDecoder(bytes.NewReader(body)).Decode(&e); err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	return &e, true, nil
}

// writeCache writes c's converted output to path, through a temporary
// file, so a reader never sees half a file.
func writeCache(path, key string, c *Core) error {
	e := cacheEntry{Mode: c.Result.Mode, Items: c.Result.Items, Files: map[string][]byte{}}
	err := fs.WalkDir(c.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(c.Files, p)
		e.Files[p] = data
		return err
	})
	if err != nil {
		return err
	}
	var body bytes.Buffer
	if err := gob.NewEncoder(&body).Encode(&e); err != nil {
		return err
	}
	sum := sha256.Sum256(body.Bytes())
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	w := bufio.NewWriterSize(tmp, 1<<20)
	_, _ = fmt.Fprintf(w, "%s %d\n%s\n%s\n", cacheMagic, cacheFormat, key, hex.EncodeToString(sum[:]))
	_, _ = w.Write(body.Bytes())
	err = w.Flush()
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// coreFromCache makes a Core from a cache entry and the extracts' tables.
func coreFromCache(in *coreInputs, e *cacheEntry) (*Core, error) {
	if len(e.Files) == 0 {
		return nil, errors.New("the cache entry has no files")
	}
	conv, err := newConverter(Input{Constants: bytes.NewReader(in.files[1]), Groups: bytes.NewReader(in.files[2]), ClientTables: bytes.NewReader(in.files[3])})
	if err != nil {
		return nil, err
	}
	c, err := coreTables(in)
	if err != nil {
		return nil, err
	}
	files := fstest.MapFS{}
	for p, data := range e.Files {
		files[p] = &fstest.MapFile{Data: data, Mode: 0o444}
	}
	for _, it := range e.Items {
		if it.Features == nil {
			it.Features = map[string]bool{}
		}
	}
	c.Result = &Result{Mode: e.Mode, Items: e.Items, conv: conv}
	c.Files = files
	return c, nil
}
