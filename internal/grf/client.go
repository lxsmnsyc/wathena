package grf

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Client is a game client folder: the GRFs listed in DATA.INI and an
// optional loose data folder.
//
// Lookups follow the official client. A file in the data folder wins over
// every GRF, and GRFs listed with a lower number in DATA.INI win over
// those listed with a higher number.
type Client struct {
	root     *os.Root
	archives []*Archive
	names    []string
	files    []*os.File
	// loose maps normalized keys to files under the data folder.
	loose map[string]looseFile
}

// Source says where a file comes from.
type looseFile struct {
	path string
	size uint32
}

type Source struct {
	// Archive is the GRF file name, or "" for the loose data folder.
	Archive string
	Entry   Entry
	path    string
	arc     *Archive
}

// OpenClient opens a client folder.
func OpenClient(dir string) (*Client, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	c := &Client{root: root, loose: map[string]looseFile{}}
	names, err := readDataINI(root)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	for _, name := range names {
		// os.Root keeps a crafted DATA.INI from opening files outside dir.
		f, err := root.Open(name)
		if err != nil {
			_ = c.Close()
			return nil, err
		}
		c.files = append(c.files, f)
		st, err := f.Stat()
		if err != nil {
			_ = c.Close()
			return nil, err
		}
		a, err := Open(f, st.Size())
		if err != nil {
			_ = c.Close()
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		c.archives = append(c.archives, a)
		c.names = append(c.names, name)
	}
	if err := c.scanLoose(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// readDataINI returns the GRF names in DATA.INI, highest priority first.
func readDataINI(root *os.Root) ([]string, error) {
	f, err := root.Open("DATA.INI")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	type item struct {
		n    int
		name string
	}
	var items []item
	section := ""
	sc := bufio.NewScanner(io.LimitReader(f, 1<<20))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(line[1 : len(line)-1])
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if section != "data" || !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(k))
		if err != nil {
			continue
		}
		items = append(items, item{n, strings.TrimSpace(v)})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("grf: DATA.INI lists no GRF files")
	}
	slices.SortStableFunc(items, func(a, b item) int { return a.n - b.n })
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.name
	}
	return names, nil
}

func (c *Client) scanLoose() error {
	err := fs.WalkDir(c.root.FS(), "data", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		c.loose[Normalize(path)] = looseFile{path: path, size: uint32(min(info.Size(), maxEntrySize))} // #nosec G115 -- clamped to maxEntrySize.
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (f looseFile) source(key string) Source {
	return Source{Entry: Entry{Name: filepath.ToSlash(f.path), Key: key, Flags: FlagFile, Size: f.size}, path: f.path}
}

// Archives returns the GRF file names, highest priority first.
func (c *Client) Archives() []string { return c.names }

// Lookup finds the file the game would use for a path.
func (c *Client) Lookup(name string) (Source, bool) {
	key := Normalize(name)
	if f, ok := c.loose[key]; ok {
		return f.source(key), true
	}
	for i, a := range c.archives {
		if e, ok := a.Lookup(key); ok {
			return Source{Archive: c.names[i], Entry: e, arc: a}, true
		}
	}
	return Source{}, false
}

// Files returns the file each path resolves to, one per path, in no
// particular order.
func (c *Client) Files() []Source {
	seen := map[string]bool{}
	var out []Source
	for key, f := range c.loose {
		seen[key] = true
		out = append(out, f.source(key))
	}
	for i, a := range c.archives {
		for _, e := range a.Entries() {
			if !e.IsFile() || seen[e.Key] {
				continue
			}
			seen[e.Key] = true
			out = append(out, Source{Archive: c.names[i], Entry: e, arc: a})
		}
	}
	return out
}

// ReadFile returns a file's uncompressed data.
func (c *Client) ReadFile(s Source) ([]byte, error) {
	if s.arc == nil {
		return c.root.ReadFile(s.path)
	}
	return s.arc.ReadFile(s.Entry)
}

// Repaired returns how many entries were read with the lenient fallback.
func (c *Client) Repaired() int64 {
	var n int64
	for _, a := range c.archives {
		n += a.Repaired()
	}
	return n
}

// Close closes the GRF files.
func (c *Client) Close() error {
	var errs []error
	for _, f := range c.files {
		errs = append(errs, f.Close())
	}
	errs = append(errs, c.root.Close())
	return errors.Join(errs...)
}
