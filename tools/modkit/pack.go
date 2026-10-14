package modkit

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
)

// Packing (strategy section 6.10): a mod's folder becomes
// <id>-<version>.zip, with the files in one top folder named after the
// ID, and <id>-<version>.zip.sha256, in sha256sum's format. Names that
// start with ".", such as .git, .luarc.json and .wathena, are left out.
// The archive is the same for the same files: entries are sorted and
// have a fixed time.

// zipTime is the modification time of every entry.
var zipTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// Pack builds a mod's .zip and checksum file in outDir. It returns
// their paths and the SHA-256 in hex.
func Pack(dir, outDir string) (zipPath, sumPath, sum string, err error) {
	m, err := mod.Read(dir)
	if err != nil {
		return "", "", "", err
	}
	files, err := packFiles(dir, outDir)
	if err != nil {
		return "", "", "", err
	}
	// The art is built again from art/, so the package holds the build of
	// its own sources. A build left in the folder's assets/ is not packed.
	files = slices.DeleteFunc(files, func(p string) bool { return strings.HasPrefix(p, mod.AssetsDir+"/") })
	type entry struct{ name, from string }
	var entries []entry
	for _, rel := range files {
		entries = append(entries, entry{rel, filepath.Join(dir, filepath.FromSlash(rel))})
	}
	if HasArt(dir) {
		tmp, err := os.MkdirTemp("", "wathena-art-")
		if err != nil {
			return "", "", "", err
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		if _, err := buildArt(dir, tmp, 11, zipTime); err != nil {
			return "", "", "", fmt.Errorf("art: %w; run wactl mod build-art for the list", err)
		}
		built, err := buildFiles(tmp)
		if err != nil {
			return "", "", "", err
		}
		for _, rel := range built {
			entries = append(entries, entry{mod.AssetsDir + "/" + rel, filepath.Join(tmp, filepath.FromSlash(rel))})
		}
		slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.name, b.name) })
	}
	name := m.ID() + "-" + m.Manifest.Version + ".zip"
	zipPath = filepath.Join(outDir, name)
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return "", "", "", err
	}
	f, err := os.Create(zipPath) // #nosec G304 -- the folder the user named.
	if err != nil {
		return "", "", "", err
	}
	h := sha256.New()
	zw := zip.NewWriter(io.MultiWriter(f, h))
	for _, e := range entries {
		// Built files are Brotli or WebP already, so they are stored.
		method := zip.Deflate
		if strings.HasPrefix(e.name, mod.AssetsDir+"/") {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: m.ID() + "/" + e.name, Method: method, Modified: zipTime})
		if err != nil {
			_ = f.Close()
			return "", "", "", err
		}
		data, err := os.ReadFile(e.from) // #nosec G304 -- a file of the mod or of its build.
		if err != nil {
			_ = f.Close()
			return "", "", "", err
		}
		if _, err := w.Write(data); err != nil {
			_ = f.Close()
			return "", "", "", err
		}
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return "", "", "", err
	}
	if err := f.Close(); err != nil {
		return "", "", "", err
	}
	sum = hex.EncodeToString(h.Sum(nil))
	sumPath = zipPath + ".sha256"
	if err := os.WriteFile(sumPath, []byte(sum+"  "+name+"\n"), 0o600); err != nil {
		return "", "", "", err
	}
	return zipPath, sumPath, sum, nil
}

// packFiles lists a mod's files, with forward slashes, sorted. Names
// that start with "." are left out, and the folder skip, where packs
// go.
func packFiles(dir, skip string) ([]string, error) {
	var out []string
	skipAbs, _ := filepath.Abs(skip)
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil || rel == "." {
			return err
		}
		if abs, _ := filepath.Abs(p); d.IsDir() && abs == skipAbs {
			return fs.SkipDir
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symbolic link; a mod holds plain files", rel)
		}
		if !d.IsDir() {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	slices.Sort(out)
	return out, err
}
