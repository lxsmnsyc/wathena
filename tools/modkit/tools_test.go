package modkit

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/blake2b"

	"github.com/lxsmnsyc/wathena/internal/testrace"
)

// TestNewModPasses makes a mod from the template, and checks that it
// validates, its tests pass, and it packs the same twice.
func TestNewModPasses(t *testing.T) {
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
	e := env(t)
	dir := filepath.Join(t.TempDir(), "example.demo-mod")
	if _, err := New("example.demo-mod", dir, e.Core.Constants); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, TypesDir, "wa", "events.lua")); err != nil {
		t.Fatal(err)
	}
	rep := e.Validate(context.Background(), dir, "")
	if len(rep.Problems) > 0 || rep.Cases != 2 || rep.Manifest.Name != "Demo Mod" {
		t.Fatalf("problems %q, cases %d, name %q", rep.Problems, rep.Cases, rep.Manifest.Name)
	}
	results, err := e.RunTests(context.Background(), rep.Loaded, "", func(CaseResult) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Failed() {
			t.Errorf("%s: %v %v", r.Name, r.Err, r.Logs)
		}
	}
	out := t.TempDir()
	zip1, _, sum1, err := Pack(dir, out)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := os.ReadFile(zip1)
	_, _, sum2, err := Pack(dir, out)
	if err != nil || sum1 != sum2 {
		t.Fatalf("packs differ: %v", err)
	}
	if bytes.Contains(b1, []byte(".luarc.json")) || bytes.Contains(b1, []byte(".wathena")) {
		t.Error("the pack holds editor files")
	}
	if _, err := New("example.demo-mod", dir, nil); err == nil {
		t.Error("a mod was made in a folder that is not empty")
	}
	if _, err := New("Bad ID", t.TempDir(), nil); err == nil {
		t.Error("a bad ID was accepted")
	}
}

// TestValidateFindsProblems checks that validate reports each problem
// with its file and line.
func TestValidateFindsProblems(t *testing.T) {
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
	dir := writeMod(t, map[string]string{
		"mod.yaml":              testManifest,
		"scripts/main.lua":      "local lib = require \"example.tested.lib.missing\"\nlocal x = require \"wa.nothing\"\nreturn function() end\n",
		"scripts/broken.lua":    "local x = = 1\n",
		"scripts/npc/thing.lua": "return 42\n",
		"data/npcs.yml":         "- name: Thing\n  map: prontera\n  x: 150\n  y: 150\n  sprite: 4_F_KAFRA1\n  script: scripts/npc/thing.lua\n",
		"data/items.yml":        "- name: Red_Potion\n  weight: -1\n",
	})
	rep := env(t).Validate(context.Background(), dir, "")
	all := strings.Join(rep.Problems, "\n")
	for _, want := range []string{
		"scripts/broken.lua:1:",
		"items Red_Potion: weight",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("no problem %q in:\n%s", want, all)
		}
	}
	// The data does not load, so the scripts are checked in a mod
	// without it.
	dir = writeMod(t, map[string]string{
		"mod.yaml":              testManifest,
		"scripts/main.lua":      "local lib = require \"example.tested.lib.missing\"\nlocal x = require \"wa.nothing\"\nreturn function() end\n",
		"scripts/npc/thing.lua": "return 42\n",
		"data/npcs.yml":         "- name: Thing\n  map: prontera\n  x: 150\n  y: 150\n  sprite: 4_F_KAFRA1\n  script: scripts/npc/thing.lua\n",
	})
	rep = env(t).Validate(context.Background(), dir, "")
	all = strings.Join(rep.Problems, "\n")
	for _, want := range []string{
		"scripts/main.lua:1: module example.tested.lib.missing not found",
		"scripts/main.lua:2: there is no API module wa.nothing",
		"example.tested/scripts/npc/thing.lua must return a setup function",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("no problem %q in:\n%s", want, all)
		}
	}
}

// TestMinisign checks signatures in minisign's format.
func TestMinisign(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	pubKey := "untrusted comment: minisign public key\n" + base64.StdEncoding.EncodeToString(append(append([]byte("Ed"), keyID...), pub...)) + "\n"
	data := []byte("the mod's zip")
	sign := func(alg string, data []byte, comment string) string {
		msg := data
		if alg == "ED" {
			h := blake2b.Sum512(data)
			msg = h[:]
		}
		sig := ed25519.Sign(priv, msg)
		global := ed25519.Sign(priv, append(append([]byte(nil), sig...), comment...))
		return "untrusted comment: signature\n" + base64.StdEncoding.EncodeToString(append(append([]byte(alg), keyID...), sig...)) +
			"\ntrusted comment: " + comment + "\n" + base64.StdEncoding.EncodeToString(global) + "\n"
	}
	for _, alg := range []string{"ED", "Ed"} {
		got, err := VerifyMinisign(data, sign(alg, data, "timestamp:1 file:x.zip"), pubKey)
		if err != nil || got != "timestamp:1 file:x.zip" {
			t.Errorf("%s: %q, %v", alg, got, err)
		}
	}
	if _, err := VerifyMinisign([]byte("changed"), sign("ED", data, "c"), pubKey); err == nil {
		t.Error("a changed file passed")
	}
	tampered := strings.Replace(sign("ED", data, "c"), "trusted comment: c", "trusted comment: d", 1)
	if _, err := VerifyMinisign(data, tampered, pubKey); err == nil {
		t.Error("a changed trusted comment passed")
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	otherKey := base64.StdEncoding.EncodeToString(append(append([]byte("Ed"), keyID...), other...))
	if _, err := VerifyMinisign(data, sign("ED", data, "c"), otherKey); err == nil {
		t.Error("another key passed")
	}
}

// TestInstallListsCoreOverrides checks that validate and install name
// the core records whose prices or drops a mod changes, which need
// data.core_override (review decision 52).
func TestInstallListsCoreOverrides(t *testing.T) {
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
	dir := writeMod(t, map[string]string{
		"mod.yaml":          "id: example.prices\nname: Prices\nversion: 1.0.0\napi: 1.0\ndepends:\n  - id: core\npermissions:\n  - data.core_override\n",
		"data/items.yml":    "- name: Red_Potion\n  buy: 40\n  sell: 20\n- name: Jellopy\n  weight: 2\n",
		"data/monsters.yml": "- name: PORING\n  base_exp: 90\n",
	})
	e := env(t)
	rep := e.Validate(context.Background(), dir, "")
	if len(rep.Problems) > 0 {
		t.Fatalf("problems: %q", rep.Problems)
	}
	want := []string{"monsters PORING: base_exp", "items Red_Potion: buy, sell"}
	if strings.Join(rep.Overrides, "|") != strings.Join(want, "|") {
		t.Errorf("overrides = %q, want %q", rep.Overrides, want)
	}
	var out bytes.Buffer
	if err := e.Install(context.Background(), InstallOptions{Source: dir, ModsDir: t.TempDir()}, &out); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"data.core_override", "changes to records that other mods added", "items Red_Potion: buy, sell"} {
		if !strings.Contains(out.String(), line) {
			t.Errorf("install output has no %q:\n%s", line, out.String())
		}
	}
}
