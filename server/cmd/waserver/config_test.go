package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
