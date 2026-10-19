// Package e2e runs scripted scenarios against a real waserver
// (strategy section 12.1, decision 0017). README.md describes the tests.
//
// Start builds waserver and an asset build once per test binary, then
// runs a server in its own process for one test:
//   - with --dev sessions, the placeholder map as the start map, and a
//     second placeholder map, e2e_field, for warps;
//   - with the mod in testdata/mods/fixture, which places monsters, a
//     shop and NPCs, from a copy of testdata/mods with each mod's art
//     built;
//   - with a fixed --seed, so the zones' random streams repeat;
//   - with PostgreSQL when Options.Database is set and
//     WATHENA_TEST_DATABASE_URL names a server: each server gets a new
//     database, as the store's tests do.
//
// When the tests run with -race, the servers of the tests that set
// Options.Race are built with -race too.
//
// Join connects a bot (tools/botclient) as a player. Player's methods
// send commands and wait for the events that show they worked, so every
// check is on what the server sent.
package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
	"github.com/lxsmnsyc/wathena/tools/modkit"
)

// Maps of the test world. Players start at the middle of StartMap.
const (
	StartMap = "placeholder/map"
	FieldMap = "map/e2e_field"
)

// Defaults of Options.
const (
	// DefaultStartJob makes every new character a Swordman at base level
	// 50 and job level 40, with the skill points of that job level.
	DefaultStartJob = "1:50:40"
	// DefaultStartItems are 10 Red Potions and 20 Jellopies.
	DefaultStartItems = "501:10,909:20"
)

// Items and skills the tests use.
const (
	RedPotion uint32 = 501
	Apple     uint32 = 512
	Jellopy   uint32 = 909
	// SkillBash is SM_BASH.
	SkillBash uint32 = 5
	// JobKnight is rAthena's Knight.
	JobKnight = 7
)

// Charm is the fixture mod's only item, so it gets the first mod item
// ID.
var Charm = mod.ItemIDs.Start

// Options configure a server.
type Options struct {
	// Seed fixes the random streams. Empty uses the test's name.
	Seed string
	// StartJob and StartItems are waserver's --start-job and
	// --start-items. Empty uses the defaults.
	StartJob, StartItems string
	// Database runs the server on a new PostgreSQL database. The test is
	// skipped when WATHENA_TEST_DATABASE_URL is not set.
	Database bool
	// Race builds the server with the race detector when the tests run
	// with -race. A server with it starts about ten times slower, so
	// only the tests that move items at the same time ask for it.
	Race bool
	// Ruleset is waserver's --ruleset, such as e2e.guildrules from
	// testdata/mods. Empty uses the core renewal ruleset.
	Ruleset string
	// Config is more of waserver's config, in YAML, after the test
	// config, such as a jail map.
	Config string
	// Maps is waserver's --maps. Empty runs StartMap and FieldMap.
	Maps string
	// Procedural runs a procedural world (decisions 0041 and 0042):
	// waserver and wactl built with -tags procedural, the Home Village
	// and Mossy Fields samples, and a world that
	// `wactl world new --mode procedural --config` makes with the same
	// config, with those samples beside the test mods. Config's
	// procedural.layout and spawns shape it. It needs Database.
	Procedural bool
	// Classic runs a classic world on the binaries of a release build:
	// waserver and wactl built with -tags procedural, and a world that
	// `wactl world new --mode classic` makes (decision 0047). It needs
	// Database.
	Classic bool
	// Assets is an asset build to run in place of the test's, such as a
	// full kRO build (review decision 55). With it, and Maps empty, the
	// server gets no --map and --maps, so Config's classic block sets
	// the maps and the start map.
	Assets string
}

// Server is a running waserver.
type Server struct {
	t testing.TB
	// WS is the game's WebSocket URL, and Debug the debug address's
	// base URL.
	WS, Debug string
	// DB is an admin pool on the server's database, or nil.
	DB *pgxpool.Pool

	bin     string
	args    []string
	env     []string
	cmd     *exec.Cmd
	logs    *logBuffer
	exited  chan struct{}
	stopped bool
}

// built is what every server of a test binary shares.
type built struct {
	dir, assets, mods string
	err               error
	// bins are the waserver binaries, built when first needed: without
	// and with the race detector, and with the procedural tag. wactl is
	// wactl with the procedural tag.
	bins  [3]binary
	wactl binary
	// homeMods is the mods with the Home Village sample, for procedural
	// worlds.
	homeMods string
}

type binary struct {
	once sync.Once
	path string
	err  error
}

var (
	buildOnce sync.Once
	shared    built
	// servers limits how many servers start and run at once, so a
	// machine with other work does not starve them. raceServers runs
	// servers with the race detector one at a time.
	servers     chan struct{}
	raceServers = make(chan struct{}, 1)
)

// Main runs a package's tests and removes the shared build afterwards.
// Call it from TestMain.
func Main(m *testing.M) int {
	code := m.Run()
	if shared.dir != "" {
		_ = os.RemoveAll(shared.dir)
	}
	return code
}

// prepare makes the assets once, and builds waserver, with the race
// detector when race is set, once per kind.
func prepare(race bool) (string, *built, error) {
	buildOnce.Do(func() {
		servers = make(chan struct{}, max(runtime.NumCPU()/2, 1))
		shared.err = func() error {
			src, err := filepath.Abs(filepath.Join("testdata", "mods"))
			if err != nil {
				return err
			}
			if _, err := os.Stat(filepath.Join(src, "fixture", "mod.yaml")); err != nil {
				return fmt.Errorf("the fixture mod is missing: %w", err)
			}
			if shared.dir, err = os.MkdirTemp("", "wathena-e2e-"); err != nil {
				return err
			}
			// The mods are copied, so their art builds into the copy and
			// not into the repository.
			shared.mods = filepath.Join(shared.dir, "mods")
			if err := os.CopyFS(shared.mods, os.DirFS(src)); err != nil {
				return fmt.Errorf("mods: %w", err)
			}
			entries, err := os.ReadDir(shared.mods)
			if err != nil {
				return err
			}
			for _, e := range entries {
				dir := filepath.Join(shared.mods, e.Name())
				if !modkit.HasArt(dir) {
					continue
				}
				if _, err := modkit.BuildArt(dir, "", 1); err != nil {
					return fmt.Errorf("art of %s: %w", e.Name(), err)
				}
			}
			shared.assets = filepath.Join(shared.dir, "assets")
			if _, err := build.Run(build.Options{Out: shared.assets, Sources: []build.Source{build.Placeholder{}, fieldSource{}}, Brotli: 1}); err != nil {
				return fmt.Errorf("assets: %w", err)
			}
			return nil
		}()
	})
	if shared.err != nil {
		return "", nil, shared.err
	}
	i := 0
	if race {
		i = 1
	}
	b := &shared.bins[i]
	b.once.Do(func() {
		name := "waserver"
		args := []string{"build"}
		if race {
			name += "-race"
			args = append(args, "-race")
		}
		b.path, b.err = goBuild(name, args, "github.com/lxsmnsyc/wathena/server/cmd/waserver")
	})
	return b.path, &shared, b.err
}

// prepareProcedural builds waserver and wactl with the procedural tag,
// and copies the mods with the Home Village and Mossy Fields samples,
// once.
func prepareProcedural() (waserver, wactl string, b *built, err error) {
	if _, _, err := prepare(false); err != nil {
		return "", "", nil, err
	}
	w := &shared.bins[2]
	w.once.Do(func() {
		w.path, w.err = goBuild("waserver-procedural", []string{"build", "-tags", "procedural"}, "github.com/lxsmnsyc/wathena/server/cmd/waserver")
		if w.err != nil {
			return
		}
		shared.homeMods = filepath.Join(shared.dir, "home-mods")
		if err := os.CopyFS(shared.homeMods, os.DirFS(shared.mods)); err != nil {
			w.err = err
			return
		}
		for _, name := range []string{"home-village", "mossy-fields"} {
			src, err := filepath.Abs(filepath.Join("..", "mods", "examples", name))
			if err != nil {
				w.err = err
				return
			}
			if w.err = os.CopyFS(filepath.Join(shared.homeMods, name), os.DirFS(src)); w.err != nil {
				return
			}
		}
	})
	c := &shared.wactl
	c.once.Do(func() {
		c.path, c.err = goBuild("wactl-procedural", []string{"build", "-tags", "procedural"}, "github.com/lxsmnsyc/wathena/tools/wactl")
	})
	return w.path, c.path, &shared, errors.Join(w.err, c.err)
}

// goBuild builds a command into the shared folder.
func goBuild(name string, args []string, pkg string) (string, error) {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(shared.dir, name)
	args = append(args, "-o", out, pkg)
	gobuild := exec.Command("go", args...) // #nosec G204 -- fixed arguments.
	if b, err := gobuild.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build %s: %w\n%s", name, err, b)
	}
	return out, nil
}

// fieldSource adds a copy of the placeholder map as map/e2e_field, so
// the world has two maps.
type fieldSource struct{}

func (fieldSource) Name() string { return "e2e field" }

func (fieldSource) Items() ([]build.Item, error) {
	items, err := build.Placeholder{}.Items()
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.Name == StartMap {
			it.Name, it.CacheKey = FieldMap, it.CacheKey+"|e2e_field"
			return []build.Item{it}, nil
		}
	}
	return nil, errors.New("the placeholder pack has no map")
}

// Start runs a server for a test and stops it when the test ends. It
// skips the test in -short mode, when go is not on PATH, and for
// Options.Database without WATHENA_TEST_DATABASE_URL.
func Start(t testing.TB, o Options) *Server {
	t.Helper()
	s := newServer(t, o)
	s.launch()
	return s
}

// StartFailing runs a server that must stop at its start, such as one
// whose config names a map that does not run, and returns its log. It
// fails the test when the server serves, or does not stop within two
// minutes: a start that failed after the server variables ran used to
// wait for a signal (decision 0045).
func StartFailing(t testing.TB, o Options) string {
	t.Helper()
	s := newServer(t, o)
	cmd := exec.Command(s.bin, s.args...) // #nosec G204 -- a binary built by prepare.
	cmd.Env, cmd.Stdout, cmd.Stderr = s.env, s.logs, s.logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err == nil {
			t.Errorf("the server exited without an error:\n%s", s.logs.tail(20))
		}
	case <-time.After(2 * time.Minute):
		_ = cmd.Process.Kill()
		<-exited
		t.Fatalf("the server did not stop in 2 minutes:\n%s", s.logs.tail(20))
	}
	text := strings.Join(s.logs.lines(), "\n")
	if strings.Contains(text, "msg=serving") {
		t.Errorf("the server served:\n%s", s.logs.tail(20))
	}
	return text
}

// newServer prepares a server's binary, database, config and arguments.
func newServer(t testing.TB, o Options) *Server {
	t.Helper()
	if testing.Short() {
		t.Skip("end-to-end tests run a server; skipped in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("end-to-end tests build waserver and need the go command on PATH")
	}
	var dbURL string
	var admin *pgxpool.Pool
	if o.Database {
		if os.Getenv(storetest.Env) == "" {
			t.Skip("set " + storetest.Env + " to a PostgreSQL 18 server to run the end-to-end tests with a database")
		}
		admin = storetest.Migrated(t)
		dbURL = gameURL(t, admin)
	}
	race := testrace.Enabled && o.Race
	bin, b, err := prepare(race)
	if err != nil {
		t.Fatal(err)
	}
	var wactl string
	mods := b.mods
	if o.Procedural || o.Classic {
		if !o.Database {
			t.Fatal("Options.Procedural and Options.Classic need Options.Database")
		}
		if bin, wactl, b, err = prepareProcedural(); err != nil {
			t.Fatal(err)
		}
		if o.Procedural {
			mods = b.homeMods
		}
	}
	if o.Assets != "" {
		// Another build has no e2e_field, so the fixture's treasure hunt
		// hides its chests on the start map, as client/scripts does.
		mods = filepath.Join(t.TempDir(), "mods")
		if err := os.CopyFS(mods, os.DirFS(b.mods)); err != nil {
			t.Fatal(err)
		}
		events := filepath.Join(mods, "fixture", "data", "events.yml")
		data, err := os.ReadFile(events)
		if err != nil {
			t.Fatal(err)
		}
		moved := strings.Replace(string(data), "maps: [e2e_field]", "maps: [placeholder/map]", 1)
		if moved == string(data) {
			t.Fatal("the fixture's treasure hunt no longer names e2e_field")
		}
		if err := os.WriteFile(events, []byte(moved), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A server waits for a free place, and frees it when the test ends.
	places := servers
	if race {
		places = raceServers
	}
	places <- struct{}{}
	t.Cleanup(func() { <-places })
	if o.Seed == "" {
		o.Seed = "e2e:" + t.Name()
	}
	if o.StartJob == "" {
		o.StartJob = DefaultStartJob
	}
	if o.StartItems == "" {
		o.StartItems = DefaultStartItems
	}
	dir := t.TempDir()
	config := filepath.Join(dir, "waserver.yml")
	text := configYAML
	if o.Procedural {
		// The Mossy Fields sample's spawn_table hook needs its
		// permission approved.
		text = strings.Replace(text, "mods:\n", "mods:\n  dir: "+mods+"\n", 1)
		text = strings.Replace(text, "    e2e.fixture: [", "    example.mossy-fields: [spawns.modify]\n    e2e.fixture: [", 1)
	}
	if err := os.WriteFile(config, []byte(text+o.Config), 0o600); err != nil {
		t.Fatal(err)
	}
	switch {
	case o.Procedural:
		newWorld(t, wactl, adminURL(t, admin), "procedural", config, dir)
	case o.Classic:
		newWorld(t, wactl, adminURL(t, admin), "classic", "", dir)
	}
	addr, debug := freeAddr(t), freeAddr(t)
	assets, maps := b.assets, []string{"--map", StartMap, "--maps", o.Maps}
	switch {
	case o.Assets != "" && o.Maps == "":
		assets, maps = o.Assets, nil
	case o.Assets != "":
		assets, maps = o.Assets, maps[2:]
	case o.Maps == "":
		maps[3] = StartMap + "," + strings.TrimPrefix(FieldMap, "map/")
	}
	args := append([]string{"--config", config, "--assets", assets}, maps...)
	args = append(args, "--dev", "--mode", "renewal",
		"--addr", addr, "--debug-addr", debug, "--origins", "", "--shutdown-countdown", "0",
		"--seed", o.Seed, "--start-job", o.StartJob, "--start-items", o.StartItems, "--mods", mods)
	if o.Ruleset != "" {
		args = append(args, "--ruleset", o.Ruleset)
	}
	s := &Server{t: t, WS: "ws://" + addr + "/ws", Debug: "http://" + debug, DB: admin, bin: bin, args: args,
		// The server's database is the test's, or none.
		env: append(os.Environ(), "WATHENA_DATABASE_URL="+dbURL), logs: &logBuffer{}}
	t.Cleanup(func() {
		s.Stop()
		if t.Failed() {
			t.Logf("server log:\n%s", s.logs.tail(80))
		}
	})
	return s
}

// launch starts the server's process and waits until it serves.
func (s *Server) launch() {
	t := s.t
	t.Helper()
	cmd := exec.Command(s.bin, s.args...) // #nosec G204 -- a binary built by prepare.
	cmd.Env = s.env
	s.cmd, s.exited, s.stopped = cmd, make(chan struct{}), false
	exited := s.exited
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = s.logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	serving := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		once := sync.Once{}
		for sc.Scan() {
			line := sc.Text()
			s.logs.line(line)
			if strings.Contains(line, "msg=serving") {
				once.Do(func() { close(serving) })
			}
		}
		_, _ = io.Copy(io.Discard, stderr)
	}()
	go func() {
		// The exit status joins the log, so a server that a signal
		// stopped shows why in a failed test's log.
		if err := cmd.Wait(); err != nil {
			s.logs.line("the server exited: " + err.Error())
		}
		close(exited)
	}()
	select {
	case <-serving:
	case <-exited:
		t.Fatalf("the server stopped before it served:\n%s", s.logs.tail(40))
	case <-time.After(5 * time.Minute):
		t.Fatalf("the server did not start in 5 minutes:\n%s", s.logs.tail(40))
	}
}

// Restart stops the server and starts it again with the same options,
// addresses and database, as a restart of a live server. Players must
// join again.
func (s *Server) Restart() {
	s.t.Helper()
	s.Stop()
	s.launch()
}

// configYAML approves the fixture mod's permissions, and turns off the
// lower chat limits of new characters, since every test character is
// new. The chat rate is high, since the tests say the fixture's chat
// commands one after another.
const configYAML = `mods:
  approve:
    e2e.fixture: [player.give_items, player.give_zeny, player.warp, player.sanction, guild.manage, rules.override, mail.send, currencies.modify, shops.modify, events.schedule, events.control, player.expand_limits]
chat:
  new_character_period: 0s
  burst: 100
  refill: 10ms
`

// newWorld makes a world of a mode in the test's database with `wactl
// world new`, with the server's config for a procedural world's layout.
// A classic world takes no config.
func newWorld(t testing.TB, wactl, dbURL, mode, config, dir string) {
	t.Helper()
	pw := filepath.Join(dir, "admin-password")
	if err := os.WriteFile(pw, []byte("e2e layout password 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"world", "new", "--mode", mode, "--name", "E2E", "--admin", "Owner1", "--admin-password-file", pw}
	if config != "" {
		args = append(args, "--config", config)
	}
	cmd := exec.Command(wactl, args...) // #nosec G204 -- a binary built by prepareProcedural.
	// The test's folder is outside the repository, as for a release
	// binary, so wactl reads the reference data built into it.
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "WATHENA_MIGRATE_DATABASE_URL="+dbURL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wactl world new: %v\n%s", err, out)
	}
	t.Logf("wactl world new:\n%s", out)
}

// adminURL is the URL of a test database as the test's admin, which
// may make the world.
func adminURL(t testing.TB, admin *pgxpool.Pool) string {
	t.Helper()
	u, err := url.Parse(os.Getenv(storetest.Env))
	if err != nil {
		t.Fatalf("%s is not a URL", storetest.Env)
	}
	u.Path = "/" + admin.Config().ConnConfig.Database
	return u.String()
}

// gameURL is the URL of a test database for the server, acting as the
// game role, as waserver does in production.
func gameURL(t testing.TB, admin *pgxpool.Pool) string {
	t.Helper()
	u, err := url.Parse(os.Getenv(storetest.Env))
	if err != nil {
		t.Fatalf("%s is not a URL", storetest.Env)
	}
	u.Path = "/" + admin.Config().ConnConfig.Database
	// pgx reads a + in the query as itself, so the space is %20, and it
	// refuses a second = in a value.
	q := u.Query()
	q.Del("options")
	u.RawQuery = strings.TrimPrefix(q.Encode()+"&options=-c%20role%3Dwathena_game", "&")
	return u.String()
}

// Stop stops the server, as Ctrl+C does, and waits for it. With a
// database, the last saves are written when it returns. Stopping twice
// does nothing.
func (s *Server) Stop() {
	if s.stopped || s.cmd == nil || s.cmd.Process == nil {
		return
	}
	s.stopped = true
	_ = s.cmd.Process.Signal(os.Interrupt)
	select {
	case <-s.exited:
	case <-time.After(30 * time.Second):
		_ = s.cmd.Process.Kill()
		<-s.exited
		s.t.Error("the server did not stop in 30 seconds")
	}
	// A server built with -race exits with 66 when it found a race.
	if st := s.cmd.ProcessState; st != nil && st.ExitCode() == 66 {
		s.t.Errorf("the server found data races:\n%s", s.logs.tail(200))
	}
}

// Errors are the server's log lines at level ERROR so far, and the race
// detector's reports.
func (s *Server) Errors() []string {
	var out []string
	for _, l := range s.logs.lines() {
		if strings.Contains(l, "level=ERROR") || strings.Contains(l, "WARNING: DATA RACE") {
			out = append(out, l)
		}
	}
	return out
}

// CheckLogs fails the test for each ERROR line and each data race in
// the server's log.
func (s *Server) CheckLogs() {
	s.t.Helper()
	for _, l := range s.Errors() {
		s.t.Errorf("server: %s", l)
	}
}

// AuditItem, AuditPlayer, AuditGround and AuditZone are the JSON of the
// server's GET /debug/wathena/items.
type AuditItem struct {
	Slot   int    `json:"slot"`
	Item   uint32 `json:"item"`
	Amount int    `json:"amount"`
	Serial string `json:"serial"`
}

// AuditPlayer is what one player holds.
type AuditPlayer struct {
	Name        string      `json:"name"`
	CharacterID int64       `json:"character_id"`
	Zeny        int64       `json:"zeny"`
	Items       []AuditItem `json:"items"`
	// Currencies are a character's own currencies above 0, by full name.
	Currencies map[string]int64 `json:"currencies"`
}

// AuditGround is one ground item.
type AuditGround struct {
	Item   uint32 `json:"item"`
	Amount int    `json:"amount"`
	Origin string `json:"origin"`
}

// AuditZone is what one zone holds.
type AuditZone struct {
	Map     string        `json:"map"`
	Players []AuditPlayer `json:"players"`
	Ground  []AuditGround `json:"ground"`
}

// Audit asks the server for every item and all zeny that its players
// and the ground hold.
func (s *Server) Audit(ctx context.Context) ([]AuditZone, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Debug+"/debug/wathena/items", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G704 -- the test server's loopback debug address.
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("audit: %s: %s", resp.Status, bytes.TrimSpace(body))
	}
	var out []AuditZone
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// freeAddr returns a loopback address with a port that was free.
func freeAddr(t testing.TB) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().String()
}

// logBuffer keeps the server's log lines.
type logBuffer struct {
	mu  sync.Mutex
	all []string
}

func (b *logBuffer) Write(p []byte) (int, error) {
	for l := range strings.SplitSeq(strings.TrimRight(string(p), "\n"), "\n") {
		b.line(l)
	}
	return len(p), nil
}

func (b *logBuffer) line(l string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.all = append(b.all, l)
}

func (b *logBuffer) lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.all...)
}

func (b *logBuffer) tail(n int) string {
	l := b.lines()
	return strings.Join(l[max(len(l)-n, 0):], "\n")
}
