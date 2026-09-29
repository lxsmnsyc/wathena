package grf

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"

	"golang.org/x/text/encoding/korean"
)

// WriteEntry is one file for Write.
type WriteEntry struct {
	// Name is a UTF-8 path with backslashes. It is stored as CP949.
	Name string
	Data []byte
	// Crypt is 0, FlagMixedCrypt, or FlagHeaderCrypt.
	Crypt uint8
}

// Write writes a version 0x200 GRF. Tests use it to build files with every
// feature the reader supports, since real game files cannot be used in CI.
func Write(w io.Writer, entries []WriteEntry) error {
	// Sizes are stored as 32 bits. Test files are far smaller.
	const maxSize = 1 << 30
	if len(entries) > maxSize {
		return fmt.Errorf("grf: too many entries")
	}
	for _, e := range entries {
		if len(e.Data) > maxSize {
			return fmt.Errorf("grf: %q is too large", e.Name)
		}
	}
	enc := korean.EUCKR.NewEncoder()
	var body, table bytes.Buffer
	for _, e := range entries {
		name, err := enc.String(e.Name)
		if err != nil {
			return fmt.Errorf("grf: name %q is not CP949: %w", e.Name, err)
		}
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		if _, err := zw.Write(e.Data); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		data := z.Bytes()
		csize := len(data)
		if e.Crypt != 0 {
			// Encrypted entries are padded to whole 8-byte blocks.
			data = append(data, make([]byte, (8-csize%8)%8)...)
			codeEntry(data, e.Crypt, uint32(csize), true) // #nosec G115 -- sizes are checked against maxSize.
		}
		offset := body.Len()
		if offset+len(data) > maxSize {
			return fmt.Errorf("grf: archive is too large")
		}
		body.Write(data)

		table.WriteString(name)
		table.WriteByte(0)
		var f [17]byte
		binary.LittleEndian.PutUint32(f[0:], uint32(csize))       // #nosec G115 -- sizes are checked against maxSize.
		binary.LittleEndian.PutUint32(f[4:], uint32(len(data)))   // #nosec G115 -- sizes are checked against maxSize.
		binary.LittleEndian.PutUint32(f[8:], uint32(len(e.Data))) // #nosec G115 -- sizes are checked against maxSize.
		f[12] = FlagFile | e.Crypt
		binary.LittleEndian.PutUint32(f[13:], uint32(offset)) // #nosec G115 -- sizes are checked against maxSize.
		table.Write(f[:])
	}

	if table.Len() > maxSize {
		return fmt.Errorf("grf: file table is too large")
	}
	var ztable bytes.Buffer
	zw := zlib.NewWriter(&ztable)
	if _, err := zw.Write(table.Bytes()); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	var hdr [headerSize]byte
	copy(hdr[:], magic)
	for i := range 14 {
		hdr[16+i] = byte(i + 1)
	}
	const seed = 0x1234
	binary.LittleEndian.PutUint32(hdr[30:], uint32(body.Len())) // #nosec G115 -- sizes are checked against maxSize.
	binary.LittleEndian.PutUint32(hdr[34:], seed)
	binary.LittleEndian.PutUint32(hdr[38:], uint32(len(entries))+seed+7) // #nosec G115 -- sizes are checked against maxSize.
	binary.LittleEndian.PutUint32(hdr[42:], version200)

	var th [8]byte
	binary.LittleEndian.PutUint32(th[0:], uint32(ztable.Len())) // #nosec G115 -- sizes are checked against maxSize.
	binary.LittleEndian.PutUint32(th[4:], uint32(table.Len()))  // #nosec G115 -- sizes are checked against maxSize.
	for _, b := range [][]byte{hdr[:], body.Bytes(), th[:], ztable.Bytes()} {
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}
