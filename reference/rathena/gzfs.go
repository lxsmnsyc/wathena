package rathena

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"
)

// GzipSuffix ends the name of each gzipped copy in a gzipped tree.
const GzipSuffix = ".gz"

// GzipFS reads a tree of gzipped files as the files themselves: the
// file a.json is a.json.gz in fsys, and reading it decompresses it. A
// directory lists its files without the suffix. Release builds read the
// extracts this way (embed_gz.go).
func GzipFS(fsys fs.FS) fs.FS { return gzipFS{fsys} }

type gzipFS struct{ fsys fs.FS }

// Open opens a file, decompressed, or a directory.
func (g gzipFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	data, info, err := g.read(name)
	if err == nil {
		return &gzipFile{Reader: bytes.NewReader(data), info: info}, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	f, err := g.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	d, ok := f.(fs.ReadDirFile)
	if !st.IsDir() || !ok {
		// A file without the suffix is not part of the tree.
		_ = f.Close()
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &gzipDir{ReadDirFile: d, g: g, name: name}, nil
}

// ReadFile reads a file, decompressed, with no copy.
func (g gzipFS) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}
	data, _, err := g.read(name)
	return data, err
}

// read decompresses the file name from name.gz.
func (g gzipFS) read(name string) ([]byte, fs.FileInfo, error) {
	if name == "." {
		return nil, nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	f, err := g.fsys.Open(name + GzipSuffix)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if st.IsDir() {
		return nil, nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		return nil, nil, &fs.PathError{Op: "read", Path: name, Err: err}
	}
	if err := zr.Close(); err != nil {
		return nil, nil, &fs.PathError{Op: "read", Path: name, Err: err}
	}
	return data, fileInfo{name: path.Base(name), size: int64(len(data)), mode: st.Mode(), mod: st.ModTime()}, nil
}

// gzipFile is a decompressed file.
type gzipFile struct {
	*bytes.Reader
	info fs.FileInfo
}

func (f *gzipFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *gzipFile) Close() error               { return nil }

// gzipDir lists a directory of the tree, with the files' own names.
type gzipDir struct {
	fs.ReadDirFile
	g    gzipFS
	name string
}

// ReadDir lists the directory. Files without the suffix are left out.
func (d *gzipDir) ReadDir(n int) ([]fs.DirEntry, error) {
	entries, err := d.ReadDirFile.ReadDir(n)
	out := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e)
			continue
		}
		base, ok := strings.CutSuffix(e.Name(), GzipSuffix)
		if !ok {
			continue
		}
		out = append(out, gzipEntry{g: d.g, dir: d.name, name: base})
	}
	return out, err
}

// gzipEntry is a file in a directory listing. Its size is known once it
// is decompressed, so Info reads it.
type gzipEntry struct {
	g         gzipFS
	dir, name string
}

func (e gzipEntry) Name() string      { return e.name }
func (e gzipEntry) IsDir() bool       { return false }
func (e gzipEntry) Type() fs.FileMode { return 0 }
func (e gzipEntry) Info() (fs.FileInfo, error) {
	_, info, err := e.g.read(path.Join(e.dir, e.name))
	return info, err
}

type fileInfo struct {
	name string
	size int64
	mode fs.FileMode
	mod  time.Time
}

func (i fileInfo) Name() string       { return i.name }
func (i fileInfo) Size() int64        { return i.size }
func (i fileInfo) Mode() fs.FileMode  { return i.mode }
func (i fileInfo) ModTime() time.Time { return i.mod }
func (i fileInfo) IsDir() bool        { return false }
func (i fileInfo) Sys() any           { return nil }
