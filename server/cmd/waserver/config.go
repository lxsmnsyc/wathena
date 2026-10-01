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
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/internal/secretenv"
)

// options are the settings of `waserver all`. They come from defaults,
// then the --config file, then the flags given on the command line. The
// yaml tags are the config file's keys. Secrets never come from here:
// they come from environment variables (see secrets).
type options struct {
	Listen    string `yaml:"listen"`
	PublicURL string `yaml:"public_url"`
	// Origins is nil when neither the file nor a flag set it.
	Origins        *list  `yaml:"origins"`
	PlayURL        string `yaml:"play_url"`
	TrustedProxies list   `yaml:"trusted_proxies"`
	Assets         string `yaml:"assets"`
	ClientDir      string `yaml:"client_dir"`
	StartMap       string `yaml:"start_map"`
	Maps           list   `yaml:"maps"`
	Mode           string `yaml:"mode"`
	Captcha        string `yaml:"captcha"`
	Dev            bool   `yaml:"dev"`
	Build          string `yaml:"build"`
	StartItems     string `yaml:"start_items"`
	DebugAddr      string `yaml:"debug_addr"`
	ProfileDir     string `yaml:"profile_dir"`
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
	flags.StringVar(&f.DebugAddr, "debug-addr", "", "loopback address to serve measurements and profiles on, such as 127.0.0.1:8301; empty serves none (debug_addr)")
	flags.StringVar(&f.ProfileDir, "profile-dir", "", "folder for the CPU profiles taken when a zone is over the tick budget; empty takes none (profile_dir)")
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
		case "debug-addr":
			o.DebugAddr = f.DebugAddr
		case "profile-dir":
			o.ProfileDir = f.ProfileDir
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
	if _, ok := builds[o.Build]; !ok {
		bad("build", "%q is not one of all_1, melee, critical, magic, ranged or tank", o.Build)
	}
	if _, err := parseStartItems(o.StartItems); err != nil {
		bad("start_items", "%v", err)
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
