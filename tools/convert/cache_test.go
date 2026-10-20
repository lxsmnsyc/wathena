package convert

import (
	"bytes"
	"io/fs"
	"os"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
)

// TestCoreCache: a second load uses the cache file, and the same files
// come back. A changed extract or converter version misses, and a
// damaged file is ignored and written again.
func TestCoreCache(t *testing.T) {
	testrace.SkipUnderRace(t, "it reads and writes the cache on one goroutine")
	want := loadCore(t, reference.Renewal)
	converted := 0
	conv := func(*coreInputs) (*Core, error) {
		converted++
		return want, nil
	}
	o := CacheOptions{Dir: t.TempDir(), Version: "a"}
	read := referenceReader(reference.Renewal)
	load := func(o CacheOptions, read func(string) ([]byte, error), status CacheStatus) *Core {
		t.Helper()
		before := converted
		c, r, err := loadCoreCached(reference.Renewal, o, read, conv)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != status {
			t.Fatalf("status %s, want %s (read error: %v)", r.Status, status, r.ReadErr)
		}
		if r.WriteErr != nil {
			t.Fatal(r.WriteErr)
		}
		if hit := converted == before; hit != (status == CacheHit) {
			t.Fatalf("status %s, but converted %d times", status, converted-before)
		}
		return c
	}

	load(o, read, CacheMiss)
	got := load(o, read, CacheHit)
	sameCore(t, got, want)

	t.Run("changed extract", func(t *testing.T) {
		changed := func(name string) ([]byte, error) {
			data, err := read(name)
			if name == "quests" {
				data = append(bytes.Clone(data), '\n')
			}
			return data, err
		}
		load(o, changed, CacheMiss)
		load(o, changed, CacheHit)
		load(o, read, CacheMiss)
	})
	t.Run("changed version", func(t *testing.T) {
		load(CacheOptions{Dir: o.Dir, Version: "b"}, read, CacheMiss)
		load(o, read, CacheMiss)
		load(o, read, CacheHit)
	})
	path := o.Dir + "/core-renewal.cache"
	damage := func(t *testing.T, change func([]byte) []byte) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, change(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("flipped byte", func(t *testing.T) {
		damage(t, func(b []byte) []byte { b[len(b)-100] ^= 1; return b })
		load(o, read, CacheBad)
		load(o, read, CacheHit)
	})
	t.Run("cut short", func(t *testing.T) {
		damage(t, func(b []byte) []byte { return b[:len(b)/2] })
		load(o, read, CacheBad)
		damage(t, func(b []byte) []byte { return b[:30] })
		load(o, read, CacheBad)
		load(o, read, CacheHit)
	})
	t.Run("not a cache file", func(t *testing.T) {
		damage(t, func([]byte) []byte { return []byte("hello\n") })
		load(o, read, CacheBad)
		load(o, read, CacheHit)
	})
	t.Run("old format", func(t *testing.T) {
		damage(t, func(b []byte) []byte { return append([]byte(cacheMagic+" 0\n"), b...) })
		load(o, read, CacheMiss)
		load(o, read, CacheHit)
	})
	t.Run("off", func(t *testing.T) {
		load(CacheOptions{}, read, CacheOff)
	})
}

// sameCore checks that a Core from the cache has what waserver reads.
func sameCore(t *testing.T, got, want *Core) {
	t.Helper()
	n := 0
	err := fs.WalkDir(want.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		n++
		w, _ := fs.ReadFile(want.Files, p)
		g, err := fs.ReadFile(got.Files, p)
		if err != nil {
			return err
		}
		if !bytes.Equal(g, w) {
			t.Errorf("%s differs", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	gotN := 0
	_ = fs.WalkDir(got.Files, ".", func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			gotN++
		}
		return err
	})
	if gotN != n {
		t.Errorf("%d files, want %d", gotN, n)
	}
	if len(got.Result.Items) != len(want.Result.Items) || got.Result.Mode != want.Result.Mode {
		t.Errorf("%d items in mode %s, want %d in %s", len(got.Result.Items), got.Result.Mode, len(want.Result.Items), want.Result.Mode)
	}
	if len(got.Monsters) != len(want.Monsters) || len(got.Groups.Groups) != len(want.Groups.Groups) || len(got.Constants.Constants) != len(want.Constants.Constants) {
		t.Error("the tables differ")
	}
	_, wantFailed, err := want.LoadStatuses(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	_, gotFailed, err := got.LoadStatuses(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotFailed) != len(wantFailed) {
		t.Errorf("%d status scripts fail, want %d", len(gotFailed), len(wantFailed))
	}
	t.Logf("%d files, %d items", n, len(got.Result.Items))
}
