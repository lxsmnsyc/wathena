package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/internal/secretenv"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/worldevent"
)

// options are the settings of `waserver all`. They come from defaults,
// then the --config file, then the flags given on the command line. The
// yaml tags are the config file's keys. Secrets never come from here:
// they come from environment variables (see secrets).
type options struct {
	Listen    string `yaml:"listen"`
	PublicURL string `yaml:"public_url"`
	// Origins is nil when neither the file nor a flag set it.
	Origins        *list      `yaml:"origins"`
	PlayURL        string     `yaml:"play_url"`
	TrustedProxies list       `yaml:"trusted_proxies"`
	Assets         string     `yaml:"assets"`
	ClientDir      string     `yaml:"client_dir"`
	StartMap       string     `yaml:"start_map"`
	Maps           list       `yaml:"maps"`
	Mode           string     `yaml:"mode"`
	Captcha        string     `yaml:"captcha"`
	Dev            bool       `yaml:"dev"`
	Build          string     `yaml:"build"`
	StartItems     string     `yaml:"start_items"`
	StartJob       string     `yaml:"start_job"`
	Seed           string     `yaml:"seed"`
	DebugAddr      string     `yaml:"debug_addr"`
	ProfileDir     string     `yaml:"profile_dir"`
	Mods           modOptions `yaml:"mods"`
	// ShutdownCountdown is how long players are warned before the server
	// stops on SIGTERM or an interrupt. 0 stops at once.
	ShutdownCountdown time.Duration `yaml:"shutdown_countdown"`
	// TimeZone is the server's time zone, as an IANA name such as
	// Asia/Manila. Schedules and world events match their times there.
	TimeZone string `yaml:"time_zone"`
	// Chat are the chat rate limits and the chat log's retention
	// (decision 0018).
	Chat chatOptions `yaml:"chat"`
	// Jail is where jailed players go.
	Jail jailOptions `yaml:"jail"`
	// Staff are the staff owners and roles (decision 0028).
	Staff staff.Config `yaml:"staff"`
	// Privacy is the privacy notice at /account/privacy (decision 0032).
	Privacy privacyOptions `yaml:"privacy"`
	// Procedural are the generated fields of a procedural world
	// (decision 0039).
	Procedural proceduralOptions `yaml:"procedural"`
	// Spawns are a new procedural world's spawn settings (decision
	// 0042). `wactl world new --config` reads them; waserver only checks
	// them, since a made world keeps its spawns.
	Spawns spawns.Settings `yaml:"spawns"`
}

// proceduralOptions are the generated fields' settings.
type proceduralOptions struct {
	// Fields lists the fields to run as x:y or x:y:biome. A procedural
	// world without it runs 0:0. With dev and no world row, it makes the
	// world procedural.
	Fields list `yaml:"fields"`
	// FieldSize is each field's width and height in cells, 300 by
	// default, or when 0.
	FieldSize int `yaml:"field_size"`
	// Tilesets is a YAML tileset table that maps the generator's
	// tilesets to the asset build's textures and models. Empty uses the
	// placeholder pack.
	Tilesets string `yaml:"tilesets"`
	// Towns is how many towns to run, t1 to t<n>, each with the next
	// town template in record order (decision 0040). Dungeons lists the
	// dungeons as style or style:floors. With --dev and no world row, a
	// dev world links them as phase 7 step 3's dev links did, through
	// the world layout's code (decision 0041).
	Towns    int  `yaml:"towns"`
	Dungeons list `yaml:"dungeons"`
	// Layout is a new world's layout: `wactl world new --config` reads
	// it, and so does a dev world with DevLayout (decision 0041).
	Layout layout.Options `yaml:"layout"`
	// DevLayout makes a --dev world without a world row build a full
	// layout from Layout and --seed, as a new world would.
	DevLayout bool `yaml:"dev_layout"`
	// TeleportBase and TeleportStep are the teleport service's price:
	// the base, plus the step for each warp between the towns. 0 takes
	// the defaults, 500 and 100 zeny.
	TeleportBase int `yaml:"teleport_base"`
	TeleportStep int `yaml:"teleport_step"`
}

// privacyOptions are the owner's parts of the privacy notice. The page
// shows them as plain text.
type privacyOptions struct {
	// Contact is how players reach the owner about their data, such as
	// an email address.
	Contact string `yaml:"contact"`
	// Text is the owner's own notice. Blank lines start paragraphs.
	Text string `yaml:"text"`
}

// chatOptions are the chat settings (server/README.md, "Chat and
// moderation").
type chatOptions struct {
	chat.Limits `yaml:",inline"`
	// LogDays is how many days chat lines are kept, except those of open
	// reports. 30 by default, as strategy section 7.10.
	LogDays int `yaml:"log_days"`
}

// jailOptions are the jail map and its cells, rAthena's MAP_JAIL and
// pc_jail cells by default.
type jailOptions struct {
	Map   string   `yaml:"map"`
	Cells [][2]int `yaml:"cells"`
}

// modOptions are the mods settings (server/README.md, "Mods").
type modOptions struct {
	// Dir is the folder of installed mods. Empty loads only the built-in
	// core mods.
	Dir string `yaml:"dir"`
	// Ruleset is the world's ruleset mod. Empty picks the core ruleset of
	// the mode.
	Ruleset string `yaml:"ruleset"`
	// Strict turns data overlay warnings into errors.
	Strict bool `yaml:"strict"`
	// Approve lists the permissions the owner approved, by mod ID.
	Approve map[string][]string `yaml:"approve"`
	// Disable lists installed mods that must not load.
	Disable []string `yaml:"disable"`
	// Events change the trigger or the duration of world events, or
	// turn them off, by the event's full name.
	Events map[string]worldevent.Override `yaml:"events"`
	// MaxAssetMB limits each mod's asset build, in MiB before
	// compression. 0 is the default, 200 (decision 0033).
	MaxAssetMB int `yaml:"max_asset_mb"`
}

// list is a list of strings. In the config file it is a YAML sequence
// or one comma-separated string. On the command line it is the latter.
type list []string

func (l *list) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*l = splitList(n.Value)
		return nil
	case yaml.SequenceNode:
		var s []string
		if err := n.Decode(&s); err != nil {
			return err
		}
		*l = nil
		for _, v := range s {
			*l = append(*l, splitList(v)...)
		}
		return nil
	}
	return fmt.Errorf("line %d: want a list or a comma-separated string", n.Line)
}

func defaultOptions() options {
	return options{
		Listen:   "127.0.0.1:8300",
		StartMap: "prontera",
		Mode:     "renewal",
		Captcha:  "none",
		Build:    "all_1",
		TimeZone: "UTC",
		Chat:     chatOptions{Limits: chat.DefaultLimits(), LogDays: 30},
		Jail:     jailOptions{Map: "sec_pri", Cells: [][2]int{{49, 75}, {24, 75}}},

		ShutdownCountdown: 10 * time.Second,
		Procedural:        proceduralOptions{FieldSize: 300},
	}
}

// devOrigins are the trusted client origins when nothing sets them and
// there is no public URL: the web client's Vite dev server.
var devOrigins = list{"http://localhost:5173", "http://127.0.0.1:5173"}

// secrets come from environment variables, or files named by the same
// names with _FILE, so they never show in process lists or config files.
type secrets struct {
	// Database is the game role's URL, AccountDatabase the account
	// role's.
	Database, AccountDatabase string
	// SecretKey seals TOTP secrets. OldSecretKeys are keys it replaced,
	// comma-separated.
	SecretKey, OldSecretKeys string
}

// Environment variables. Each also has a _FILE form.
const (
	databaseEnv        = "WATHENA_DATABASE_URL"
	accountDatabaseEnv = "WATHENA_ACCOUNT_DATABASE_URL"
	secretKeyEnv       = "WATHENA_ACCOUNT_SECRET_KEY"
	oldSecretKeysEnv   = "WATHENA_ACCOUNT_OLD_SECRET_KEYS"
)

func loadSecrets() (secrets, error) {
	var s secrets
	var errs []error
	for _, v := range []struct {
		name string
		to   *string
	}{
		{databaseEnv, &s.Database},
		{accountDatabaseEnv, &s.AccountDatabase},
		{secretKeyEnv, &s.SecretKey},
		{oldSecretKeysEnv, &s.OldSecretKeys},
	} {
		value, err := secretenv.Get(v.name)
		errs = append(errs, err)
		*v.to = value
	}
	return s, errors.Join(errs...)
}

// parseOptions reads the flags of `waserver all`, and the config file
// that --config names. Flags given on the command line win over the file.
// database is the legacy --database flag, which holds a secret.
func parseOptions(name string, args []string, stderr io.Writer) (o options, database string, err error) {
	o = defaultOptions()
	f := o
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	config := flags.String("config", "", "YAML config file; flags given on the command line win over it")
	flags.StringVar(&f.Assets, "assets", "", "asset build folder (assets)")
	flags.StringVar(&f.ClientDir, "client-dir", "", "folder with a web client build to serve at /, instead of the one built into waserver (client_dir)")
	flags.StringVar(&f.StartMap, "map", o.StartMap, "map where new characters start, such as prontera or placeholder/map (start_map)")
	maps := flags.String("maps", "", "maps to run, comma-separated, or all for every map in the asset build; empty runs --map only (maps)")
	flags.StringVar(&f.Listen, "addr", o.Listen, "address to listen on (listen)")
	flags.BoolVar(&f.Dev, "dev", false, "accept any name as a session token, for development only; needs a loopback --addr (dev)")
	origins := flags.String("origins", "", "more trusted web client origins, as scheme://host[:port], comma-separated. The public URL's origin is always trusted. Without a public URL, the default is the Vite dev server (origins)")
	flags.StringVar(&f.PublicURL, "public-url", "", "the server's public URL, such as https://game.example.com. The client is at /, the account pages at /account/, the game at /ws. Its scheme decides Secure cookies, and its host is the passkeys' domain (public_url)")
	flags.StringVar(&f.PublicURL, "account-url", "", "the same as --public-url")
	flags.StringVar(&f.PlayURL, "play-url", "", "the web client URL that Play opens when the account page was not opened from a client (play_url)")
	proxies := flags.String("trusted-proxies", "", "proxy addresses or CIDR blocks whose X-Forwarded-For is believed, comma-separated (trusted_proxies)")
	flags.StringVar(&f.Captcha, "captcha", o.Captcha, "captcha after repeated failed logins: none, or dev for a development checkbox (captcha)")
	flags.StringVar(&f.Mode, "mode", o.Mode, "rAthena mode whose recorded spawns to use: renewal or pre-renewal, or none (mode)")
	flags.StringVar(&f.Build, "build", o.Build, "raised stats every novice starts with: all_1, melee, critical, magic, ranged, or tank. Other builds than all_1 are for testing (build)")
	flags.StringVar(&f.StartItems, "start-items", "", "items every new character starts with, as id:amount pairs such as 501:10,512:20, for development (start_items)")
	flags.StringVar(&f.StartJob, "start-job", "", "job and levels every new character starts with, as job:base:job such as 1:50:40 for a Swordman, with the skill points of its job level, for development; needs --dev (start_job)")
	flags.StringVar(&f.Seed, "seed", "", "text that fixes the random streams of the zones and world events, so a test runs the same each time, for development; needs --dev (seed)")
	flags.StringVar(&f.DebugAddr, "debug-addr", "", "loopback address to serve measurements and profiles on, such as 127.0.0.1:8301; empty serves none (debug_addr)")
	flags.StringVar(&f.ProfileDir, "profile-dir", "", "folder for the CPU profiles taken when a zone is over the tick budget; empty takes none (profile_dir)")
	flags.StringVar(&f.Mods.Dir, "mods", "", "folder of installed mods; empty loads only the built-in core mods (mods.dir)")
	flags.StringVar(&f.Mods.Ruleset, "ruleset", "", "the world's ruleset mod, such as core.ruleset.renewal; empty picks the core ruleset of --mode (mods.ruleset)")
	flags.BoolVar(&f.Mods.Strict, "mods-strict", false, "make data overlay warnings errors, such as two mods setting the same field (mods.strict)")
	flags.StringVar(&f.TimeZone, "time-zone", o.TimeZone, "the server's time zone, as an IANA name such as Asia/Manila; schedules and world events use it (time_zone)")
	flags.DurationVar(&f.ShutdownCountdown, "shutdown-countdown", o.ShutdownCountdown, "how long players are warned before the server stops on SIGTERM, such as 10s; 0 stops at once, and a second signal skips the rest (shutdown_countdown)")
	fieldList := flags.String("fields", "", "generated fields to run, as x:y or x:y:biome, comma-separated; with --dev and no world row they make a procedural world, which needs -tags procedural (procedural.fields)")
	flags.IntVar(&f.Procedural.FieldSize, "field-size", o.Procedural.FieldSize, "the width and height of each generated field, 32 to 1024 cells (procedural.field_size)")
	flags.StringVar(&f.Procedural.Tilesets, "tilesets", "", "YAML tileset table that maps the generator's tilesets to the asset build's textures and models; empty uses placeholders (procedural.tilesets)")
	flags.IntVar(&f.Procedural.Towns, "towns", 0, "how many generated towns to run, each with the next town template; with --dev and no world row they make a procedural world (procedural.towns)")
	flags.BoolVar(&f.Procedural.DevLayout, "layout", false, "with --dev and no world row, build a full world layout from procedural.layout and --seed, as wactl world new does; needs -tags procedural (procedural.dev_layout)")
	dungeonList := flags.String("dungeons", "", "generated dungeons to run, as style or style:floors, comma-separated, such as crypt:3; with --dev and no world row they make a procedural world (procedural.dungeons)")
	flags.StringVar(&database, "database", "", "PostgreSQL URL of the game role; prefer the "+databaseEnv+" environment variable, which keeps the password out of process lists")
	if err := flags.Parse(args); err != nil {
		return o, "", err
	}
	if flags.NArg() > 0 {
		return o, "", fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if *config != "" {
		if err := loadConfig(*config, &o); err != nil {
			return o, "", err
		}
	}
	flags.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "assets":
			o.Assets = f.Assets
		case "client-dir":
			o.ClientDir = f.ClientDir
		case "map":
			o.StartMap = f.StartMap
		case "maps":
			o.Maps = splitList(*maps)
		case "addr":
			o.Listen = f.Listen
		case "dev":
			o.Dev = f.Dev
		case "origins":
			l := list(splitList(*origins))
			o.Origins = &l
		case "public-url", "account-url":
			o.PublicURL = f.PublicURL
		case "play-url":
			o.PlayURL = f.PlayURL
		case "trusted-proxies":
			o.TrustedProxies = splitList(*proxies)
		case "captcha":
			o.Captcha = f.Captcha
		case "mode":
			o.Mode = f.Mode
		case "build":
			o.Build = f.Build
		case "start-items":
			o.StartItems = f.StartItems
		case "start-job":
			o.StartJob = f.StartJob
		case "seed":
			o.Seed = f.Seed
		case "debug-addr":
			o.DebugAddr = f.DebugAddr
		case "profile-dir":
			o.ProfileDir = f.ProfileDir
		case "mods":
			o.Mods.Dir = f.Mods.Dir
		case "ruleset":
			o.Mods.Ruleset = f.Mods.Ruleset
		case "mods-strict":
			o.Mods.Strict = f.Mods.Strict
		case "shutdown-countdown":
			o.ShutdownCountdown = f.ShutdownCountdown
		case "time-zone":
			o.TimeZone = f.TimeZone
		case "fields":
			o.Procedural.Fields = splitList(*fieldList)
		case "field-size":
			o.Procedural.FieldSize = f.Procedural.FieldSize
		case "tilesets":
			o.Procedural.Tilesets = f.Procedural.Tilesets
		case "towns":
			o.Procedural.Towns = f.Procedural.Towns
		case "dungeons":
			o.Procedural.Dungeons = splitList(*dungeonList)
		case "layout":
			o.Procedural.DevLayout = f.Procedural.DevLayout
		}
	})
	return o, database, nil
}

// secretKeys are config keys that people may try, with where the value
// goes instead.
var secretKeys = map[string]string{
	"database":         databaseEnv,
	"database_url":     databaseEnv,
	"account_database": accountDatabaseEnv,
	"secret_key":       secretKeyEnv,
}

// loadConfig reads a YAML config file over o. Unknown keys are errors,
// so a typo does not pass silently.
func loadConfig(path string, o *options) error {
	data, err := os.ReadFile(path) // #nosec G304 -- the file the operator named.
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	var keys map[string]yaml.Node
	if err := yaml.Unmarshal(data, &keys); err != nil {
		return fmt.Errorf("config %s: %w", path, err)
	}
	for k, env := range secretKeys {
		if _, ok := keys[k]; ok {
			return fmt.Errorf("config %s: %s: secrets do not go in the config file; set %s or %s_FILE", path, k, env, env)
		}
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(o); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("config %s: %w", path, err)
	}
	return nil
}

// settings are the checked options.
type settings struct {
	options
	// Public is the public URL's origin, or "" without one.
	Public string
	// Origins are the trusted client origins: the public origin and the
	// listed ones.
	Origins        []string
	TrustedProxies []netip.Prefix
	Secrets        secrets
	// Location is the time zone of TimeZone.
	Location *time.Location
	// Roles are the checked staff roles.
	Roles *staff.Roles
}

// validate checks the options and secrets together and lists every
// problem it finds, by config key.
func validate(o options, s secrets) (settings, error) {
	st := settings{options: o, Secrets: s}
	var errs []error
	bad := func(key, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", key, fmt.Sprintf(format, args...)))
	}

	if o.Assets == "" {
		bad("assets", "is required: the folder that `wactl assets build` made")
	} else if f, err := os.Open(filepath.Join(o.Assets, "manifest.json")); err != nil { // #nosec G304 -- the folder the operator named.
		if errors.Is(err, fs.ErrNotExist) {
			bad("assets", "%s has no manifest.json; build assets with `wactl assets build`", o.Assets)
		} else {
			bad("assets", "%v; the build must be readable by the user waserver runs as", err)
		}
	} else {
		_ = f.Close()
	}
	if o.ClientDir != "" {
		if _, err := os.Stat(filepath.Join(o.ClientDir, "index.html")); err != nil {
			bad("client_dir", "%s has no index.html; build the client with `pnpm build`", o.ClientDir)
		}
	}
	if _, _, err := net.SplitHostPort(o.Listen); err != nil {
		bad("listen", "%q is not host:port", o.Listen)
	}
	if o.Dev && !loopback(o.Listen) {
		bad("dev", "lets anyone play any character, so listen must be a loopback address such as 127.0.0.1:8300")
	}
	if o.ShutdownCountdown < 0 || o.ShutdownCountdown > 10*time.Minute {
		bad("shutdown_countdown", "is 0 to 10m")
	}
	if o.TimeZone == "" {
		st.Location = time.UTC
	} else if loc, err := time.LoadLocation(o.TimeZone); err != nil || o.TimeZone == "Local" {
		bad("time_zone", "%q is not an IANA time zone name, such as UTC or Asia/Manila", o.TimeZone)
	} else {
		st.Location = loc
	}
	// Options made without defaultOptions get the defaults here.
	def := defaultOptions()
	if st.Chat.Limits == (chat.Limits{}) {
		st.Chat.Limits = def.Chat.Limits
	}
	if st.Chat.LogDays == 0 {
		st.Chat.LogDays = def.Chat.LogDays
	}
	if st.Jail.Map == "" {
		st.Jail = def.Jail
	}
	if err := st.Chat.Check(); err != nil {
		bad("chat", "%v", err)
	}
	if st.Chat.LogDays < 1 || st.Chat.LogDays > 3650 {
		bad("chat.log_days", "is 1 to 3650")
	}
	if len(o.Privacy.Contact) > 1000 {
		bad("privacy.contact", "is at most 1,000 bytes")
	}
	if len(o.Privacy.Text) > 20000 {
		bad("privacy.text", "is at most 20,000 bytes")
	}
	for _, c := range st.Jail.Cells {
		if c[0] < 0 || c[1] < 0 || c[0] > 1023 || c[1] > 1023 {
			bad("jail.cells", "%v is not a cell", c)
		}
	}
	if o.DebugAddr != "" && !loopback(o.DebugAddr) {
		bad("debug_addr", "has no access control, so it must be a loopback address such as 127.0.0.1:8301")
	}

	if o.PublicURL != "" {
		if err := checkPublicURL(o.PublicURL); err != nil {
			bad("public_url", "%v", err)
		} else {
			st.Public = canonicalOrigin(o.PublicURL)
		}
	} else if !o.Dev && !loopback(o.Listen) {
		bad("public_url", "is required when listen is not a loopback address: cookies, passkeys and origin checks need it")
	}
	listed := devOrigins
	if o.Origins != nil {
		listed = *o.Origins
	} else if o.PublicURL != "" {
		listed = nil
	}
	if st.Public != "" {
		st.Origins = append(st.Origins, st.Public)
	}
	for _, v := range listed {
		if originOf(v) != v {
			bad("origins", "%q is not scheme://host[:port]", v)
			continue
		}
		if v != st.Public {
			st.Origins = append(st.Origins, v)
		}
	}
	proxies, err := parseProxies(o.TrustedProxies)
	if err != nil {
		bad("trusted_proxies", "%v", err)
	}
	st.TrustedProxies = proxies

	switch o.Captcha {
	case "none", "dev":
	default:
		bad("captcha", "%q is not none or dev", o.Captcha)
	}
	switch o.Mode {
	case "renewal", "pre-renewal", "none":
	default:
		bad("mode", "%q is not renewal, pre-renewal or none", o.Mode)
	}
	if _, ok := gamedata.Builds[o.Build]; !ok {
		bad("build", "%q is not one of all_1, melee, critical, magic, ranged or tank", o.Build)
	}
	if _, err := parseStartItems(o.StartItems); err != nil {
		bad("start_items", "%v", err)
	}
	if o.StartJob != "" {
		if _, err := parseStartJob(o.StartJob); err != nil {
			bad("start_job", "%v", err)
		} else if !o.Dev {
			bad("start_job", "is for development and needs dev")
		}
	}
	if o.Seed != "" && !o.Dev {
		bad("seed", "is for development and needs dev: a known seed lets players predict drops")
	}
	if roles, err := o.Staff.Build(); err != nil {
		for _, e := range strings.Split(err.Error(), "\n") {
			errs = append(errs, errors.New(e))
		}
	} else {
		st.Roles = roles
	}
	if o.Mods.Dir != "" {
		if fi, err := os.Stat(o.Mods.Dir); err != nil || !fi.IsDir() {
			bad("mods.dir", "%s is not a folder", o.Mods.Dir)
		}
	}
	if o.Procedural.FieldSize == 0 {
		st.Procedural.FieldSize = defaultOptions().Procedural.FieldSize
	} else if o.Procedural.FieldSize < 32 || o.Procedural.FieldSize > 1024 {
		bad("procedural.field_size", "must be 32 to 1024 cells")
	}
	for _, f := range o.Procedural.Fields {
		if _, err := fields.ParseSpec(f); err != nil {
			bad("procedural.fields", "%v", err)
		}
	}
	if o.Procedural.Tilesets != "" {
		if _, err := os.Stat(o.Procedural.Tilesets); err != nil {
			bad("procedural.tilesets", "%v", err)
		}
	}
	if o.Procedural.Towns < 0 || o.Procedural.Towns > maxTowns {
		bad("procedural.towns", "must be 0 to %d", maxTowns)
	}
	for _, d := range o.Procedural.Dungeons {
		if _, _, err := fields.ParseDungeonSpec(d); err != nil {
			bad("procedural.dungeons", "%v", err)
		}
	}
	if _, err := o.Procedural.Layout.Config(st.Procedural.FieldSize); err != nil {
		bad("procedural.layout", "%v", err)
	}
	if core, err := spawns.LoadCore(); err != nil {
		bad("spawns", "%v", err)
	} else if _, err := o.Spawns.Resolve(core.Densities); err != nil {
		bad("spawns", "%v", err)
	}
	if o.Procedural.DevLayout && wantsDevMaps(o.Procedural) {
		bad("procedural.dev_layout", "a dev layout places its own maps; leave out --fields, --towns and --dungeons")
	}
	if o.Procedural.TeleportBase < 0 || o.Procedural.TeleportStep < 0 || o.Procedural.TeleportBase > 1_000_000 || o.Procedural.TeleportStep > 1_000_000 {
		bad("procedural.teleport_base", "the teleport prices are 0 to 1000000 zeny")
	}
	if o.Mods.MaxAssetMB < 0 || o.Mods.MaxAssetMB > 64<<10 {
		bad("mods.max_asset_mb", "must be 0 for the default of 200, or up to 65536")
	}

	if !o.Dev && (s.Database == "" || s.AccountDatabase == "") {
		errs = append(errs, errors.New("accounts need "+databaseEnv+" (the game role) and "+accountDatabaseEnv+" (the account role), or their _FILE forms; use dev for development without accounts"))
	}
	if s.SecretKey == "" && s.OldSecretKeys != "" {
		errs = append(errs, errors.New(oldSecretKeysEnv+" is set without "+secretKeyEnv))
	}
	return st, errors.Join(errs...)
}

// checkPublicURL checks that the public URL is the root of an origin,
// and https unless it is on this machine.
func checkPublicURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%q is not an http or https URL", raw)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("%q must be scheme://host[:port] alone: the client is served at its root", raw)
	}
	if u.Scheme == "http" && !loopbackHost(u.Hostname()) {
		return fmt.Errorf("%q must use https; plain http is only for localhost", raw)
	}
	return nil
}

// canonicalOrigin returns a URL's origin as browsers send it in the
// Origin header: the host in lower case, and no port when it is the
// scheme's default. https://Game.example.com:443/ gives
// https://game.example.com.
func canonicalOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	defaultPort := map[string]string{"https": "443", "http": "80"}[u.Scheme]
	if p := u.Port(); p != "" && p != defaultPort {
		host += ":" + p
	}
	return u.Scheme + "://" + host
}

// originOf returns a URL's scheme://host[:port], or "" when it is not an
// absolute http or https URL.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// loopback reports whether a listen address only accepts connections
// from this machine.
func loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	return err == nil && loopbackHost(host)
}

func loopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}

func splitList(s string) []string {
	var out []string
	for v := range strings.SplitSeq(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// parseProxies reads trusted proxies: addresses or CIDR blocks.
func parseProxies(l []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, v := range l {
		if p, err := netip.ParsePrefix(v); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(v)
		if err != nil {
			return nil, fmt.Errorf("%q is not an address or CIDR block", v)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// startJob is --start-job: the job and levels of new characters.
type startJob struct{ job, base, jobLevel int }

// parseStartJob reads --start-job: job:base:job, such as 1:50:40.
func parseStartJob(v string) (startJob, error) {
	parts := strings.Split(v, ":")
	if len(parts) != 3 {
		return startJob{}, fmt.Errorf("%q is not job:base:job", v)
	}
	var n [3]int
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 || x > 1000 || i > 0 && x < 1 {
			return startJob{}, fmt.Errorf("%q is not job:base:job", v)
		}
		n[i] = x
	}
	return startJob{job: n[0], base: n[1], jobLevel: n[2]}, nil
}
