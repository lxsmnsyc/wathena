package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

// releaseWactl builds wactl as scripts/release.sh does: the procedural
// tag, no cgo and -trimpath. It returns the binary and a working folder
// outside the module, so nothing can find go.mod or reference/rathena.
func releaseWactl(t *testing.T) (bin, dir string) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds wactl; skipped in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command on PATH")
	}
	bin = filepath.Join(t.TempDir(), "wactl")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-tags", "procedural", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	dir = t.TempDir()
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			t.Skipf("the temporary folder %s is inside a module", dir)
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return bin, dir
}

// TestReleaseWactlOutsideRepository: a release wactl makes a procedural
// world in a folder outside the repository, from the reference data
// built into it. Before, it failed with "reference: go.mod not found
// above the working directory".
func TestReleaseWactlOutsideRepository(t *testing.T) {
	bin, dir := releaseWactl(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// sim economy makes a procedural world in memory, as world new does.
	sim := exec.CommandContext(ctx, bin, "sim", "economy", "--seed", "release-test")
	sim.Dir = dir
	if out, err := sim.CombinedOutput(); err != nil {
		t.Fatalf("wactl sim economy: %v\n%s", err, out)
	}

	if os.Getenv(storetest.Env) == "" {
		t.Log("set " + storetest.Env + " to also run world new against PostgreSQL")
		return
	}
	pool := storetest.Empty(t)
	url, err := databaseURL(os.Getenv(storetest.Env), pool.Config().ConnConfig.Database)
	if err != nil {
		t.Fatal(err)
	}
	pw := filepath.Join(dir, "admin-password")
	if err := os.WriteFile(pw, []byte("release test password 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The default mode of a procedural build is procedural.
	world := exec.CommandContext(ctx, bin, "world", "new", "--name", "Release", "--admin", "Owner1", "--admin-password-file", pw)
	world.Dir = dir
	world.Env = append(os.Environ(), migrateEnv+"="+url)
	out, err := world.CombinedOutput()
	if err != nil {
		t.Fatalf("wactl world new: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "made the procedural") {
		t.Errorf("wactl world new made no procedural world:\n%s", out)
	}
}
