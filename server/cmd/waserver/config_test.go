package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/gateway"
)

// assetDir makes a folder that passes the assets check.
func assetDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "waserver.yml")
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

var dbSecrets = secrets{Database: "postgres://game@db/wathena", AccountDatabase: "postgres://account@db/wathena"}

func TestConfigFileAndFlags(t *testing.T) {
	assets := assetDir(t)
	p := writeConfig(t, `
listen: 0.0.0.0:8300
public_url: https://game.example.com
assets: `+assets+`
start_map: prt_fild08
maps: [prontera, "prt_fild08,prt_in"]
trusted_proxies: 172.30.80.10
origins:
  - https://fork.example.com
`)
	o, _, err := parseOptions("waserver all", []string{"--config", p, "--map", "prontera", "--addr", "0.0.0.0:9000"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if o.StartMap != "prontera" || o.Listen != "0.0.0.0:9000" {
		t.Errorf("flags did not win: %+v", o)
	}
	if strings.Join(o.Maps, " ") != "prontera prt_fild08 prt_in" || o.Mode != "renewal" {
		t.Errorf("file: %+v", o)
	}
	st, err := validate(o, dbSecrets)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"https://game.example.com", "https://fork.example.com"}; !slices.Equal(st.Origins, want) {
		t.Errorf("origins %v, want %v", st.Origins, want)
	}
	if st.Public != "https://game.example.com" || len(st.TrustedProxies) != 1 {
		t.Errorf("settings %+v", st)
	}
}

func TestOriginDefaults(t *testing.T) {
	assets := assetDir(t)
	// No public URL and nothing listed: the Vite dev server.
	st, err := validate(options{Listen: "127.0.0.1:8300", Assets: assets, Mode: "none", Captcha: "none", Build: "all_1", Dev: true}, secrets{})
	if err != nil || !slices.Equal(st.Origins, devOrigins) {
		t.Errorf("dev: %v, %v", st.Origins, err)
	}
	// A public URL alone: only its origin.
	o := defaultOptions()
	o.Assets, o.PublicURL = assets, "http://localhost:8300/"
	st, err = validate(o, dbSecrets)
	if err != nil || !slices.Equal(st.Origins, []string{"http://localhost:8300"}) {
		t.Errorf("public: %v, %v", st.Origins, err)
	}
	// An empty list given on the command line stays empty.
	o, _, err = parseOptions("waserver", []string{"--assets", assets, "--dev", "--origins", ""}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if st, err = validate(o, secrets{}); err != nil || len(st.Origins) != 0 {
		t.Errorf("empty: %v, %v", st.Origins, err)
	}
}

func TestValidateListsEveryProblem(t *testing.T) {
	o := options{
		Listen: "0.0.0.0:8300", PublicURL: "http://game.example.com/play", Assets: t.TempDir(),
		Mode: "classic", Captcha: "hcaptcha", Build: "all_1", Dev: true,
		DebugAddr: "0.0.0.0:8301", TrustedProxies: list{"nope"}, StartItems: "501",
	}
	l := list{"https://ok.example.com", "https://bad.example.com/path"}
	o.Origins = &l
	_, err := validate(o, secrets{OldSecretKeys: "x"})
	if err == nil {
		t.Fatal("accepted")
	}
	msg := err.Error()
	for _, want := range []string{"assets:", "dev:", "debug_addr:", "public_url:", "origins:", "trusted_proxies:", "captcha:", "mode:", "start_items:", oldSecretKeysEnv} {
		if !strings.Contains(msg, want) {
			t.Errorf("no %q in:\n%s", want, msg)
		}
	}
}

func TestValidatePublicURL(t *testing.T) {
	assets := assetDir(t)
	for raw, ok := range map[string]bool{
		"https://game.example.com":       true,
		"https://game.example.com:8443/": true,
		"http://localhost:8300":          true,
		"http://127.0.0.1:8300":          true,
		"http://game.example.com":        false,
		"https://game.example.com/play":  false,
		"https://game.example.com/?a=1":  false,
		"https://user@game.example.com":  false,
		"game.example.com":               false,
	} {
		o := defaultOptions()
		o.Assets, o.PublicURL, o.Listen = assets, raw, "0.0.0.0:8300"
		_, err := validate(o, dbSecrets)
		if (err == nil) != ok {
			t.Errorf("%s: %v", raw, err)
		}
	}
	// A public listen address needs a public URL.
	o := defaultOptions()
	o.Assets, o.Listen = assets, "0.0.0.0:8300"
	if _, err := validate(o, dbSecrets); err == nil || !strings.Contains(err.Error(), "public_url") {
		t.Errorf("no public URL: %v", err)
	}
	// Accounts need both database URLs.
	o.PublicURL = "https://game.example.com"
	if _, err := validate(o, secrets{Database: "x"}); err == nil || !strings.Contains(err.Error(), accountDatabaseEnv) {
		t.Errorf("one database: %v", err)
	}
}

func TestCanonicalOrigin(t *testing.T) {
	for raw, want := range map[string]string{
		"https://Game.Example.com:443/": "https://game.example.com",
		"https://localhost:8443":        "https://localhost:8443",
		"http://localhost:80":           "http://localhost",
		"http://[::1]:8300":             "http://[::1]:8300",
	} {
		if got := canonicalOrigin(raw); got != want {
			t.Errorf("%s: %s, want %s", raw, got, want)
		}
	}
}

func TestConfigRejectsSecretsAndTypos(t *testing.T) {
	o := defaultOptions()
	err := loadConfig(writeConfig(t, "database: postgres://game:hunter2@db/wathena\n"), &o)
	if err == nil || !strings.Contains(err.Error(), databaseEnv) || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("a secret: %v", err)
	}
	if err := loadConfig(writeConfig(t, "public_ulr: https://game.example.com\n"), &o); err == nil {
		t.Error("a typo was accepted")
	}
	if err := loadConfig(writeConfig(t, "# only comments\n"), &o); err != nil {
		t.Errorf("an empty file: %v", err)
	}
}

func TestSecretsFromFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "game_url")
	if err := os.WriteFile(file, []byte("postgres://game:pw@db/wathena\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(databaseEnv, "")
	t.Setenv(databaseEnv+"_FILE", file)
	t.Setenv(accountDatabaseEnv, "postgres://account:pw@db/wathena")
	t.Setenv(accountDatabaseEnv+"_FILE", "")
	t.Setenv(secretKeyEnv, "")
	t.Setenv(secretKeyEnv+"_FILE", "")
	t.Setenv(oldSecretKeysEnv, "")
	t.Setenv(oldSecretKeysEnv+"_FILE", "")
	s, err := loadSecrets()
	if err != nil || s.Database != "postgres://game:pw@db/wathena" || s.AccountDatabase != "postgres://account:pw@db/wathena" {
		t.Errorf("%+v %v", s, err)
	}
}

func TestDispatch(t *testing.T) {
	if code, err := dispatch([]string{"nope"}); code != 2 || err == nil {
		t.Errorf("unknown command: %d %v", code, err)
	}
	if code, err := dispatch([]string{"all", "--help"}); code != 0 || err != nil {
		t.Errorf("help: %d %v", code, err)
	}
	// The flags alone run `all`, which then finds no assets.
	t.Setenv(databaseEnv, "")
	t.Setenv(databaseEnv+"_FILE", "")
	if code, err := dispatch([]string{"--dev"}); code != 1 || err == nil || !strings.Contains(err.Error(), "assets:") {
		t.Errorf("legacy flags: %d %v", code, err)
	}
}

func TestModsConfig(t *testing.T) {
	assets := assetDir(t)
	mods := t.TempDir()
	p := writeConfig(t, `
assets: `+assets+`
dev: true
mods:
  dir: `+mods+`
  ruleset: example.classic-plus
  strict: true
  approve:
    example.a: [player.give_items]
  disable: [example.b]
`)
	o, _, err := parseOptions("waserver all", []string{"--config", p, "--ruleset", "core.ruleset.pre_renewal"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	m := o.Mods
	if m.Dir != mods || m.Ruleset != "core.ruleset.pre_renewal" || !m.Strict || !slices.Equal(m.Approve["example.a"], []string{"player.give_items"}) || !slices.Equal(m.Disable, []string{"example.b"}) {
		t.Errorf("mods %+v", m)
	}
	if _, err := validate(o, secrets{}); err != nil {
		t.Error(err)
	}
	o.Mods.Dir = filepath.Join(mods, "missing")
	if _, err := validate(o, secrets{}); err == nil || !strings.Contains(err.Error(), "mods.dir") {
		t.Errorf("a missing mods folder: %v", err)
	}
	if got := rulesetBase("none", "pre-renewal"); got != "pre_renewal" {
		t.Errorf("base from the world row: %q", got)
	}
	if got := rulesetBase("renewal", ""); got != "renewal" {
		t.Errorf("base from the mode: %q", got)
	}
}

func TestTimeZone(t *testing.T) {
	o := defaultOptions()
	o.Assets, o.Dev = assetDir(t), true
	st, err := validate(o, secrets{})
	if err != nil || st.Location != time.UTC {
		t.Fatalf("default: %v %v", st.Location, err)
	}
	o.TimeZone = "Asia/Manila"
	if st, err = validate(o, secrets{}); err != nil || st.Location.String() != "Asia/Manila" {
		t.Fatalf("manila: %v %v", st.Location, err)
	}
	for _, bad := range []string{"Local", "Mars/Olympus"} {
		o.TimeZone = bad
		if _, err := validate(o, secrets{}); err == nil || !strings.Contains(err.Error(), "time_zone") {
			t.Errorf("%s: %v", bad, err)
		}
	}
}

func TestParseStartJob(t *testing.T) {
	sj, err := parseStartJob("1:50:40")
	if err != nil || sj != (startJob{job: 1, base: 50, jobLevel: 40}) {
		t.Fatalf("%+v, %v", sj, err)
	}
	for _, bad := range []string{"", "1:50", "a:1:1", "1:0:1", "1:1:-1", "1:1:1:1"} {
		if _, err := parseStartJob(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSeedNeedsDev(t *testing.T) {
	o := defaultOptions()
	o.Assets, o.Seed = assetDir(t), "test"
	if _, err := validate(o, dbSecrets); err == nil || !strings.Contains(err.Error(), "seed:") {
		t.Errorf("a seed without dev: %v", err)
	}
	o.Dev = true
	if _, err := validate(o, secrets{}); err != nil {
		t.Errorf("a seed with dev: %v", err)
	}
	o, _, err := parseOptions("waserver", []string{"--seed", "abc"}, io.Discard)
	if err != nil || o.Seed != "abc" {
		t.Errorf("--seed: %q, %v", o.Seed, err)
	}
}

// TestStaffConfig checks the staff section: the default roles, a list
// that replaces them, and the load errors for unknown names.
func TestStaffConfig(t *testing.T) {
	assets := assetDir(t)
	p := writeConfig(t, `
assets: `+assets+`
staff:
  owners: [Alice]
  roles:
    helper: [audit.read, moderation.*]
`)
	o, _, err := parseOptions("waserver all", []string{"--config", p}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	st, err := validate(o, dbSecrets)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(st.Staff.Owners, []string{"Alice"}) || !slices.Equal(st.Roles.Names(), []string{"helper"}) ||
		!st.Roles.Has("helper", "moderation.ban") || st.Roles.Has("helper", "staff.assign") {
		t.Errorf("staff %+v %v", st.Staff, st.Roles.Names())
	}
	o = defaultOptions()
	o.Assets = assets
	if st, err := validate(o, dbSecrets); err != nil || !slices.Equal(st.Roles.Names(), []string{"admin", "game_master", "moderator"}) {
		t.Errorf("default roles: %v", err)
	}
	o.Staff.Roles = map[string][]string{"helper": {"audit.write"}, "owner": {"audit.read"}}
	o.Staff.Owners = []string{"no"}
	_, err = validate(o, dbSecrets)
	for _, want := range []string{`staff.roles.helper: "audit.write" is not a permission`, "owner comes only from staff.owners", `staff.owners: "no" is not an account name`} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("error %v lacks %q", err, want)
		}
	}
}

// TestPrivacyConfig reads the privacy notice's keys, and refuses texts
// that are too long (decision 0032).
func TestPrivacyConfig(t *testing.T) {
	p := writeConfig(t, `
assets: `+assetDir(t)+`
dev: true
privacy:
  contact: privacy@example.com
  text: |
    We run this server.

    We keep little.
`)
	o, _, err := parseOptions("waserver all", []string{"--config", p}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validate(o, secrets{}); err != nil {
		t.Fatal(err)
	}
	if o.Privacy.Contact != "privacy@example.com" || !strings.Contains(o.Privacy.Text, "\n\nWe keep little.") {
		t.Errorf("privacy %+v", o.Privacy)
	}
	o.Privacy.Contact = strings.Repeat("x", 1001)
	o.Privacy.Text = strings.Repeat("x", 20001)
	_, err = validate(o, secrets{})
	if err == nil || !strings.Contains(err.Error(), "privacy.contact") || !strings.Contains(err.Error(), "privacy.text") {
		t.Errorf("long texts: %v", err)
	}
}

func TestModAssetLimitConfig(t *testing.T) {
	p := writeConfig(t, `
assets: `+assetDir(t)+`
dev: true
mods:
  max_asset_mb: 50
`)
	o, _, err := parseOptions("waserver all", []string{"--config", p}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validate(o, secrets{}); err != nil || o.Mods.MaxAssetMB != 50 {
		t.Fatalf("%v, %d", err, o.Mods.MaxAssetMB)
	}
	o.Mods.MaxAssetMB = -1
	if _, err := validate(o, secrets{}); err == nil || !strings.Contains(err.Error(), "mods.max_asset_mb") {
		t.Errorf("a negative limit: %v", err)
	}
}

// The mods' Lua budgets come from mods.budget and mods.max_budget
// (review decision 52).
func TestModBudgetConfig(t *testing.T) {
	p := writeConfig(t, `
assets: `+assetDir(t)+`
dev: true
mods:
  budget: {memory_mb: 8, cpu_per_tick: 1000000}
  max_budget: {memory_mb: 32}
`)
	o, _, err := parseOptions("waserver all", []string{"--config", p}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validate(o, secrets{}); err != nil || o.Mods.Budget.MemoryMB != 8 || o.Mods.Budget.CPUPerTick != 1_000_000 || o.Mods.MaxBudget.MemoryMB != 32 {
		t.Fatalf("%v, %+v, %+v", err, o.Mods.Budget, o.Mods.MaxBudget)
	}
	o.Mods.Budget.MemoryMB = 64
	if _, err := validate(o, secrets{}); err == nil || !strings.Contains(err.Error(), "mods.budget") {
		t.Errorf("a default above the most: %v", err)
	}
}

// TestDevRefusesProxySettings checks that dev, which lets anyone play
// any character, refuses the settings of a server behind a proxy on the
// same host, which the loopback listen check lets through (review
// 2026-10).
func TestDevRefusesProxySettings(t *testing.T) {
	assets := assetDir(t)
	for name, c := range map[string]struct {
		proxies list
		public  string
		ok      bool
	}{
		"plain":                 {ok: true},
		"loopback public url":   {public: "http://127.0.0.1:8300", ok: true},
		"trusted proxies":       {proxies: list{"127.0.0.1"}},
		"public public url":     {public: "https://game.example.com"},
		"public url, localhost": {public: "http://localhost:8300", ok: true},
	} {
		o := defaultOptions()
		o.Assets, o.Dev, o.TrustedProxies, o.PublicURL = assets, true, c.proxies, c.public
		_, err := validate(o, secrets{})
		if c.ok != (err == nil) {
			t.Errorf("%s: %v", name, err)
		}
		if err != nil && !strings.Contains(err.Error(), "dev:") {
			t.Errorf("%s: no dev: in %v", name, err)
		}
	}
}

// TestConnectionsConfig checks the connection limits per address: the
// defaults, a file that sets them, and the ranges.
func TestConnectionsConfig(t *testing.T) {
	assets := assetDir(t)
	o := defaultOptions()
	o.Assets, o.Dev = assets, true
	st, err := validate(o, secrets{})
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Connections.limits(); got != gateway.DefaultConnLimits() {
		t.Errorf("defaults %+v", got)
	}
	p := writeConfig(t, "dev: true\nassets: "+assets+"\nconnections:\n  per_address: 4\n  hello_burst: 2\n  hello_refill: 10s\n")
	o, _, err = parseOptions("waserver", []string{"--config", p}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	st, err = validate(o, secrets{})
	if err != nil {
		t.Fatal(err)
	}
	want := gateway.ConnLimits{PerAddress: 4, Hello: gateway.Rate{Burst: 2, Every: 10 * time.Second}}
	if got := st.Connections.limits(); got != want {
		t.Errorf("from the file %+v, want %+v", got, want)
	}
	o.Connections = connectionOptions{PerAddress: -1, HelloBurst: 5000, HelloRefill: time.Millisecond}
	_, err = validate(o, secrets{})
	for _, key := range []string{"connections.per_address", "connections.hello_burst", "connections.hello_refill"} {
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("no %s in %v", key, err)
		}
	}
}
