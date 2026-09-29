// Package grf reads GRF archives, the container format of the game's
// data files. It supports version 0x200, the version current clients use.
//
// GRF files can come from anywhere, so every size and offset is checked
// before use, and no read can allocate more than the limits below.
package grf

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"

	"golang.org/x/text/encoding/korean"
)

// Entry flags.
const (
	// FlagFile marks a file. Entries without it are directories.
	FlagFile uint8 = 0x01
	// FlagMixedCrypt marks an entry with DES and shuffled blocks.
	FlagMixedCrypt uint8 = 0x02
	// FlagHeaderCrypt marks an entry whose first 20 blocks use DES.
	FlagHeaderCrypt uint8 = 0x04
)

const (
	magic      = "Master of Magic\x00"
	headerSize = 46
	version200 = 0x200
)

// Limits on sizes read from the file. Real files are far below them.
const (
	maxTableSize = 256 << 20
	maxEntrySize = 1 << 30
)

var (
	ErrFormat  = errors.New("grf: not a GRF file")
	ErrVersion = errors.New("grf: unsupported version")
	ErrCorrupt = errors.New("grf: corrupt file")
)

// Entry is one file in an archive.
type Entry struct {
	// Name is the path as stored, decoded from CP949, with backslashes.
	Name string
	// Key is Name normalized for lookups. See Normalize.
	Key string
	// CompressedSize is the zlib data size. AlignedSize is the stored size,
	// padded to 8 bytes for encrypted entries.
	CompressedSize uint32
	AlignedSize    uint32
	// Size is the uncompressed size.
	Size   uint32
	Flags  uint8
	Offset int64
}

// IsFile reports whether the entry is a file and not a directory.
func (e Entry) IsFile() bool { return e.Flags&FlagFile != 0 }

// Archive is an open GRF file.
type Archive struct {
	r       io.ReaderAt
	size    int64
	version uint32
	entries []Entry
	index   map[string]int
	// BadNames counts names that were not valid CP949.
	BadNames int
	// repaired counts entries read with the lenient fallback in ReadFile.
	repaired atomic.Int64
}

// Normalize returns the lookup key for a path: lowercase ASCII, with
// forward slashes. The game treats paths without regard to ASCII case.
func Normalize(name string) string {
	b := []byte(name)
	for i, c := range b {
		switch {
		case c == '\\':
			b[i] = '/'
		case 'A' <= c && c <= 'Z':
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// Open reads the header and file table of a GRF of the given size.
func Open(r io.ReaderAt, size int64) (*Archive, error) {
	var hdr [headerSize]byte
	if size < headerSize {
		return nil, ErrFormat
	}
	if _, err := r.ReadAt(hdr[:], 0); err != nil {
		return nil, err
	}
	if string(hdr[:len(magic)]) != magic {
		return nil, ErrFormat
	}
	tableOffset := int64(binary.LittleEndian.Uint32(hdr[30:])) + headerSize
	seed := binary.LittleEndian.Uint32(hdr[34:])
	rawCount := binary.LittleEndian.Uint32(hdr[38:])
	version := binary.LittleEndian.Uint32(hdr[42:])
	if version != version200 {
		return nil, fmt.Errorf("%w: %#x", ErrVersion, version)
	}
	count := int64(rawCount) - int64(seed) - 7
	if count < 0 {
		return nil, fmt.Errorf("%w: negative entry count", ErrCorrupt)
	}

	if tableOffset+8 > size {
		return nil, fmt.Errorf("%w: file table offset past the end", ErrCorrupt)
	}
	var th [8]byte
	if _, err := r.ReadAt(th[:], tableOffset); err != nil {
		return nil, err
	}
	csize := int64(binary.LittleEndian.Uint32(th[0:]))
	usize := int64(binary.LittleEndian.Uint32(th[4:]))
	if csize > maxTableSize || usize > maxTableSize || tableOffset+8+csize > size {
		return nil, fmt.Errorf("%w: file table size", ErrCorrupt)
	}
	ctable := make([]byte, csize)
	if _, err := r.ReadAt(ctable, tableOffset+8); err != nil {
		return nil, err
	}
	table, err := inflate(ctable, usize)
	if err != nil {
		return nil, fmt.Errorf("%w: file table: %v", ErrCorrupt, err)
	}

	a := &Archive{r: r, size: size, version: version, index: map[string]int{}}
	if count > int64(len(table)/18) {
		// Each entry needs at least 18 bytes, so the count is wrong.
		return nil, fmt.Errorf("%w: entry count %d does not fit the file table", ErrCorrupt, count)
	}
	a.entries = make([]Entry, 0, count)
	dec := korean.EUCKR.NewDecoder()
	for p := 0; p < len(table); {
		end := bytes.IndexByte(table[p:], 0)
		if end < 0 || p+end+18 > len(table) {
			return nil, fmt.Errorf("%w: file table entry %d", ErrCorrupt, len(a.entries))
		}
		raw := table[p : p+end]
		p += end + 1
		f := table[p : p+17]
		p += 17
		name, err := dec.String(string(raw))
		if err != nil || strings.ContainsRune(name, '�') {
			a.BadNames++
			name = strings.ToValidUTF8(string(raw), "�")
		}
		e := Entry{
			Name:           name,
			Key:            Normalize(name),
			CompressedSize: binary.LittleEndian.Uint32(f[0:]),
			AlignedSize:    binary.LittleEndian.Uint32(f[4:]),
			Size:           binary.LittleEndian.Uint32(f[8:]),
			Flags:          f[12],
			Offset:         int64(binary.LittleEndian.Uint32(f[13:])) + headerSize,
		}
		if e.IsFile() {
			if e.Offset+int64(e.AlignedSize) > size || e.CompressedSize > e.AlignedSize || e.Size > maxEntrySize {
				return nil, fmt.Errorf("%w: entry %q is out of range", ErrCorrupt, name)
			}
		}
		// A later entry with the same key replaces the earlier one.
		if i, ok := a.index[e.Key]; ok {
			a.entries[i] = e
			continue
		}
		a.index[e.Key] = len(a.entries)
		a.entries = append(a.entries, e)
	}
	return a, nil
}

// Version returns the GRF version.
func (a *Archive) Version() uint32 { return a.version }

// Entries returns every entry in file table order.
func (a *Archive) Entries() []Entry { return a.entries }

// Lookup finds an entry by path. The path is normalized first.
func (a *Archive) Lookup(name string) (Entry, bool) {
	i, ok := a.index[Normalize(name)]
	if !ok {
		return Entry{}, false
	}
	return a.entries[i], true
}

// ReadRaw returns an entry's stored bytes, with encryption removed but
// still compressed. The asset build uses it to detect changed entries.
func (a *Archive) ReadRaw(e Entry) ([]byte, error) {
	if !e.IsFile() {
		return nil, fmt.Errorf("grf: %q is a directory", e.Name)
	}
	if e.Offset < headerSize || e.Offset+int64(e.AlignedSize) > a.size || e.CompressedSize > e.AlignedSize {
		return nil, fmt.Errorf("%w: entry %q is out of range", ErrCorrupt, e.Name)
	}
	buf := make([]byte, e.AlignedSize)
	if _, err := a.r.ReadAt(buf, e.Offset); err != nil {
		return nil, err
	}
	codeEntry(buf, e.Flags, e.CompressedSize, false)
	return buf[:e.CompressedSize], nil
}

// ReadFile returns an entry's uncompressed data.
func (a *Archive) ReadFile(e Entry) ([]byte, error) {
	if e.Size > maxEntrySize {
		return nil, fmt.Errorf("%w: entry %q is too large", ErrCorrupt, e.Name)
	}
	raw, err := a.ReadRaw(e)
	if err != nil {
		return nil, err
	}
	data, err := inflate(raw, int64(e.Size))
	if err == nil {
		return data, nil
	}
	// Some official GRFs record a compressed size a few bytes too small.
	// The zlib stream then runs past the entry, and its checksum reads the
	// next entry's bytes. The official client still loads these files.
	if e.Flags&(FlagMixedCrypt|FlagHeaderCrypt) == 0 {
		if data, lerr := a.inflateLenient(e); lerr == nil {
			a.repaired.Add(1)
			return data, nil
		}
	}
	return nil, fmt.Errorf("%w: entry %q: %v", ErrCorrupt, e.Name, err)
}

// Repaired returns how many entries ReadFile read with the lenient fallback.
func (a *Archive) Repaired() int64 { return a.repaired.Load() }

// inflateLenient reads an entry's deflate stream to its end, past the
// recorded compressed size, and skips the zlib checksum. It succeeds only
// if the stream ends and gives exactly the recorded uncompressed size.
func (a *Archive) inflateLenient(e Entry) ([]byte, error) {
	const zlibHeader = 2
	start := e.Offset + zlibHeader
	if start > a.size {
		return nil, ErrCorrupt
	}
	// Deflate expands at most about 1032 to 1, and stored blocks add 5 bytes
	// per 64 KB, so the stream cannot be longer than this.
	limit := min(a.size-start, int64(e.Size)+int64(e.Size)/64+1024)
	fr := flate.NewReader(io.NewSectionReader(a.r, start, limit))
	defer func() { _ = fr.Close() }()
	var out bytes.Buffer
	n, err := io.Copy(&out, io.LimitReader(fr, int64(e.Size)+1))
	if err != nil {
		return nil, err
	}
	if n != int64(e.Size) {
		return nil, fmt.Errorf("lenient read gave %d bytes, want %d", n, e.Size)
	}
	return out.Bytes(), nil
}

// inflate decompresses zlib data that must be exactly size bytes.
// Memory grows with the real output, not the stated size, so a crafted
// size cannot force a large allocation.
func inflate(src []byte, size int64) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	var out bytes.Buffer
	out.Grow(int(min(size, int64(len(src))*4)))
	n, err := io.Copy(&out, io.LimitReader(zr, size+1))
	if err != nil {
		return nil, err
	}
	if n != size {
		return nil, fmt.Errorf("data is %d bytes, want %d", n, size)
	}
	return out.Bytes(), nil
}
