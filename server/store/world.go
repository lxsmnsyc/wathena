package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Errors of the world functions.
var (
	ErrNoWorld       = errors.New("store: the database has no world; run wactl world new")
	ErrWorldExists   = errors.New("store: the database already has a world")
	ErrAccountExists = errors.New("store: the account name is taken")
)

// World describes the world of a database.
type World struct {
	Name    string
	Ruleset string
	// Seed is the secret root of the seed tree. LoadWorld leaves it nil.
	Seed []byte
	// Mode is classic or procedural (strategy section 8.1). An empty mode
	// makes a classic world.
	Mode string
	// GeneratorVersion and RNGVersion are the map generator version and
	// the seed tree spec, such as "rng-v1", of a procedural world. A
	// classic world has 0 and "".
	GeneratorVersion int
	RNGVersion       string
	// PublicNode is the key of the seed tree's public branch, which the
	// game role may read instead of Seed (decision 0039). It is nil for
	// worlds made before migration 00030.
	PublicNode []byte
	// Layout is a procedural world's layout, which NewWorld writes with
	// the row (decision 0041). LoadWorld leaves it nil: LoadLayout reads
	// it.
	Layout *layout.Layout
	// Spawns are the spawn lists of the layout's fields and dungeon
	// floors, which NewWorld writes with the layout (decision 0042).
	// LoadWorld leaves them nil: LoadSpawns reads them.
	Spawns []spawns.Planned
}

// Admin is the first account that NewWorld makes. Hash is the
// password's PHC string, from account.Hasher. The account has no role
// until the config's staff.owners names it (decision 0028).
type Admin struct {
	Name, Hash string
}

// NewWorld makes the world row and its first account in one
// transaction. It needs the migrator role, which alone may make the
// world. It returns the account's ID.
func NewWorld(ctx context.Context, pool *pgxpool.Pool, w World, admin Admin) (int64, error) {
	var id int64
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		mode := w.Mode
		if mode == "" {
			mode = "classic"
		}
		n, err := q.CreateWorld(ctx, db.CreateWorldParams{
			Name: w.Name, Ruleset: w.Ruleset, Seed: w.Seed, Mode: mode,
			GeneratorVersion: w.GeneratorVersion, RngVersion: w.RNGVersion,
			PublicNode: w.PublicNode,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrWorldExists
		}
		if w.Layout != nil {
			if err := insertLayout(ctx, q, w.Layout); err != nil {
				return fmt.Errorf("the world layout: %w", err)
			}
		}
		if err := insertSpawns(ctx, q, w.Spawns); err != nil {
			return fmt.Errorf("the world's spawns: %w", err)
		}
		id, err = q.CreateAdminAccount(ctx, db.CreateAdminAccountParams{Name: admin.Name, Hash: admin.Hash})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccountExists
		}
		return err
	})
	return id, err
}

// LoadWorld reads the world without its seed, with its public branch. The game role runs it. It
// returns ErrNoWorld when `wactl world new` has not run.
func LoadWorld(ctx context.Context, pool *pgxpool.Pool) (World, error) {
	row, err := db.New(pool).World(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return World{}, ErrNoWorld
	}
	if err != nil {
		return World{}, err
	}
	return World{
		Name: row.Name, Ruleset: row.Ruleset, Mode: row.Mode,
		GeneratorVersion: row.GeneratorVersion, RNGVersion: row.RngVersion,
		PublicNode: row.PublicNode,
	}, nil
}
