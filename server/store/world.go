package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
	Seed             []byte
	GeneratorVersion int
}

// Admin is the first staff account that NewWorld makes. Hash is the
// password's PHC string, from account.Hasher.
type Admin struct {
	Name, Hash string
}

// NewWorld makes the world row and its first staff account in one
// transaction. It needs the migrator role, which alone may make staff
// accounts. It returns the account's ID.
func NewWorld(ctx context.Context, pool *pgxpool.Pool, w World, admin Admin) (int64, error) {
	var id int64
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		n, err := q.CreateWorld(ctx, db.CreateWorldParams{
			Name: w.Name, Ruleset: w.Ruleset, Seed: w.Seed, GeneratorVersion: w.GeneratorVersion,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrWorldExists
		}
		id, err = q.CreateStaffAccount(ctx, db.CreateStaffAccountParams{Name: admin.Name, Hash: admin.Hash})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccountExists
		}
		return err
	})
	return id, err
}

// LoadWorld reads the world without its seed. The game role runs it. It
// returns ErrNoWorld when `wactl world new` has not run.
func LoadWorld(ctx context.Context, pool *pgxpool.Pool) (World, error) {
	row, err := db.New(pool).World(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return World{}, ErrNoWorld
	}
	if err != nil {
		return World{}, err
	}
	return World{Name: row.Name, Ruleset: row.Ruleset, GeneratorVersion: row.GeneratorVersion}, nil
}
