package rathena

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"testing"
	"testing/fstest"
)

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestGzipFS checks that a gzipped tree reads as the files themselves,
// as a release build reads the extracts: fs.ReadFile, directory
// listings, and the checks of testing/fstest.
func TestGzipFS(t *testing.T) {
	src := fstest.MapFS{
		"extracts/renewal/a.json.gz":     {Data: gzipBytes(t, `{"a":1}`)},
		"extracts/renewal/b.json.gz":     {Data: gzipBytes(t, `[]`)},
		"extracts/pre-renewal/a.json.gz": {Data: gzipBytes(t, `{"a":2}`)},
		"extracts/renewal/notes.txt":     {Data: []byte("not part of the tree")},
	}
	g := GzipFS(src)
	got, err := fs.ReadFile(g, "extracts/renewal/a.json")
	if err != nil || string(got) != `{"a":1}` {
		t.Fatalf("a.json: %q, %v", got, err)
	}
	if _, err := fs.ReadFile(g, "extracts/renewal/notes.txt"); err == nil {
		t.Error("a file without the suffix reads")
	}
	if _, err := fs.ReadFile(g, "extracts/renewal/a.json.gz"); err == nil {
		t.Error("a gzipped name reads")
	}
	if err := fstest.TestFS(g, "extracts/renewal/a.json", "extracts/renewal/b.json", "extracts/pre-renewal/a.json"); err != nil {
		t.Fatal(err)
	}
}
