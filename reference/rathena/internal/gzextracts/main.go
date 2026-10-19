// Command gzextracts writes the gzipped copies of the embedded extracts
// that a build with the gzextracts tag embeds (review decision 56).
// go generate runs it in reference/rathena:
//
//	go generate ./reference/rathena
//
// It reads the files that a plain build embeds, so both builds hold the
// same files, and writes each one as <path>.gz below -out, which it
// empties first. The gzip headers carry no name and no time, so two
// runs with the same Go version give the same bytes.
package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/lxsmnsyc/wathena/reference/rathena"
)

func main() {
	out := flag.String("out", "gz", "folder to write the gzipped copies into; it is emptied first")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "gzextracts:", err)
		os.Exit(1)
	}
}

func run(out string) error {
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	var in, written int64
	count := 0
	err := fs.WalkDir(rathena.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(rathena.Files, p)
		if err != nil {
			return err
		}
		if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
			return errors.New(p + " is gzipped already: run go generate without the gzextracts tag")
		}
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			return err
		}
		if _, err := zw.Write(data); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		dst := filepath.Join(out, filepath.FromSlash(p)+rathena.GzipSuffix)
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(dst, buf.Bytes(), 0o600); err != nil {
			return err
		}
		in += int64(len(data))
		written += int64(buf.Len())
		count++
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("gzextracts: %d files, %.1f MB of JSON in %.1f MB of gzip, in %s\n",
		count, float64(in)/1e6, float64(written)/1e6, out)
	return nil
}
