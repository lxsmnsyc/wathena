package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// worldNew makes a world in a database that roles.sql prepared:
//  1. It applies pending migrations.
//  2. It makes the world row with a new secret root seed, the key of its
//     public branch, and the world mode, with the newest generator for a
//     procedural world. A procedural world gets its layout too: the map
//     graph, town names and level bands (decision 0041), from the mods
//     and procedural.layout of --config, and each field's and dungeon
//     floor's spawns from the seed's secret branch, with the mods'
//     spawn_table hooks (decision 0042).
//  3. It makes the first account with a password, and records its ID
//     as the world's owner (review decision 54).
//  4. It prints how the owner role starts to count.
//
// Steps 2 and 3 run in one transaction, so a failure leaves no world.
func worldNew(args []string) error {
	fs := newFlags("world new")
	database := fs.String("database", "", "PostgreSQL URL of the migrator role; prefer the "+migrateEnv+" environment variable")
	name := fs.String("name", "wAthena", "the world's name, 1 to 64 characters")
	ruleset := fs.String("ruleset", "renewal", "the base ruleset: renewal or pre-renewal. waserver's mode must match it")
	mode := fs.String("mode", gen.DefaultMode(), "the world mode: "+strings.Join(gen.Modes(), " or ")+". It cannot change later")
	admin := fs.String("admin", "", "name of the first account, the world's owner: 4 to 24 ASCII letters, digits and underscores, starting with a letter")
	passwordFile := fs.String("admin-password-file", "", "file with the account's password on its first line, or - for standard input")
	config := fs.String("config", "", "waserver.yml, for a procedural world's layout and spawns: its mods section, procedural.layout and spawns")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if n := len([]rune(*name)); n < 1 || n > 64 {
		return errors.New("--name must be 1 to 64 characters")
	}
	if *ruleset != "renewal" && *ruleset != "pre-renewal" {
		return fmt.Errorf("--ruleset %q is not renewal or pre-renewal", *ruleset)
	}
	generator, rngVersion, err := worldVersions(*mode)
	if err != nil {
		return err
	}
	if !account.ValidAccountName(*admin) {
		return errors.New("--admin must be 4 to 24 ASCII letters, digits and underscores, starting with a letter")
	}
	if *passwordFile == "" {
		return errors.New("--admin-password-file is required: a file, or - for standard input")
	}
	password, err := readPassword(*passwordFile)
	if err != nil {
		return err
	}
	if p := account.PasswordProblem(*admin, password); p != "" {
		return errors.New("the admin password is refused: " + p)
	}
	dsn, err := migratorURL(*database)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext(5 * time.Minute)
	defer cancel()
	pool, err := connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrateUp(ctx, pool); err != nil {
		return err
	}
	if _, err := store.LoadWorld(ctx, pool); err == nil {
		return store.ErrWorldExists
	} else if !errors.Is(err, store.ErrNoWorld) {
		return err
	}
	hash, err := account.NewHasher(account.DefaultParams, 1).Hash(ctx, password)
	if err != nil {
		return err
	}
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return err
	}
	w := store.World{Name: *name, Ruleset: *ruleset, Seed: seed, Mode: *mode, GeneratorVersion: generator, RNGVersion: rngVersion}
	if w.PublicNode, err = publicNode(seed); err != nil {
		return err
	}
	if w.Mode == gen.ModeProcedural {
		pw, err := proceduralWorld(*config, *ruleset, seed, generator)
		if err != nil {
			return err
		}
		w.Layout, w.Spawns, w.Content, w.Rotations, w.SeasonKey = pw.layout, pw.spawns, pw.content, pw.rotations, pw.seasonKey
	} else if *config != "" {
		return errors.New("--config is for a procedural world's layout; a classic world has none")
	}
	id, err := store.NewWorld(ctx, pool, w, store.Admin{Name: *admin, Hash: hash})
	if err != nil {
		return err
	}
	fmt.Printf("made the %s world %q (%s) and the account %s (id %d)\n", worldModeText(w), *name, *ruleset, *admin, id)
	if w.Layout != nil {
		fmt.Print(layoutText(w.Layout))
		fmt.Print(spawnText(w.Spawns))
		fmt.Print(contentText(w.Content))
		fmt.Print(rotationText(w.Layout, w.Rotations))
	}
	fmt.Print(ownersHint(*admin, id))
	return nil
}

// publicNode returns the key of the public branch of the seed tree that
// seed roots (decision 0039). The game role reads it instead of the seed.
func publicNode(seed []byte) ([]byte, error) {
	root, err := rng.NewRoot(seed)
	if err != nil {
		return nil, err
	}
	key := gen.PublicBranch(root).Key()
	return key[:], nil
}

// worldVersions returns the generator and rng versions a new world of
// the mode records: none for classic, the newest generator for
// procedural. A build without the procedural tag has only classic.
func worldVersions(mode string) (generator int, rngVersion string, err error) {
	switch {
	case mode == gen.ModeProcedural && !gen.HasMode(mode):
		return 0, "", errors.New("--mode procedural needs a wactl built with -tags procedural")
	case !gen.HasMode(mode):
		return 0, "", fmt.Errorf("--mode %q is not %s", mode, strings.Join(gen.Modes(), " or "))
	case mode == gen.ModeClassic:
		return 0, "", nil
	}
	g, ok := gen.Latest()
	if !ok {
		return 0, "", errors.New("this build has no map generator")
	}
	return g.Version, g.RNG, nil
}

// worldModeText names a world's mode, with its versions when procedural.
func worldModeText(w store.World) string {
	if w.Mode == gen.ModeProcedural {
		return fmt.Sprintf("procedural (generator %d, %s)", w.GeneratorVersion, w.RNGVersion)
	}
	return w.Mode
}

// ownersHint tells the operator that the account is the world's owner
// by its ID (review decision 54), and when the role counts. staff.owners
// is not needed for it.
func ownersHint(admin string, id int64) string {
	return fmt.Sprintf("The account %s (id %d) is the world's owner. It needs no staff.owners line in waserver.yml.\n"+
		"The owner role counts once the account turns on TOTP or a passkey, and logs in with a TOTP code, a recovery code, or a passkey that checks a PIN or fingerprint.\n", admin, id)
}

// readPassword reads the first line of a file, or of standard input for
// "-". A password file from Docker secrets ends with a line break.
func readPassword(path string) (string, error) {
	var r io.Reader = os.Stdin
	if path != "-" {
		f, err := os.Open(path) // #nosec G304 -- the file the operator named.
		if err != nil {
			return "", fmt.Errorf("--admin-password-file: %w", err)
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	line, err := bufio.NewReader(io.LimitReader(r, account.MaxPassword+2)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("--admin-password-file: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// pgCommand makes a command of the PostgreSQL client tools for a
// database URL, which it passes with urlFlag: --dbname= for pg_dump and
// pg_restore, --maintenance-db= for createdb and dropdb. The password
// moves from the URL to PGPASSWORD, so it never shows in process lists.
// The context's end kills the tool.
func pgCommand(ctx context.Context, tool, urlFlag, raw string, args ...string) (*exec.Cmd, error) {
	path, err := exec.LookPath(tool)
	if err != nil {
		return nil, fmt.Errorf("%s is not installed; install the PostgreSQL 18 client tools, or run wactl from the tools image (deploy/README.md)", tool)
	}
	dbname, password, err := splitPassword(raw)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, path, append([]string{urlFlag + dbname}, args...)...) // #nosec G204 -- a PostgreSQL tool with fixed flags.
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password, "PGAPPNAME=wactl")
	cmd.Stderr = os.Stderr
	return cmd, nil
}

// splitPassword takes the password out of a postgres:// URL, from its
// user part or its password parameter. Other parts, such as sslmode,
// stay.
func splitPassword(raw string) (dbname, password string, err error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil {
		return "", "", errors.New("the database URL must be a postgres:// URL with a user name")
	}
	password, _ = u.User.Password()
	q := u.Query()
	if p := q.Get("password"); p != "" {
		password = p
		q.Del("password")
		u.RawQuery = q.Encode()
	}
	u.User = url.User(u.User.Username())
	return u.String(), password, nil
}

// runPG runs a PostgreSQL tool and names it in errors.
func runPG(cmd *exec.Cmd) error {
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", cmd.Args[0], err)
	}
	return nil
}
