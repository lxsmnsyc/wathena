package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

//go:embed migrations/*.sql
var migrations embed.FS

// ErrPending is returned when the database has migrations that are not
// applied yet. `wactl migrate up` applies them.
var ErrPending = errors.New("store: the database has pending migrations; run wactl migrate up")

// Migrator applies the schema migrations in server/store/migrations with
// goose. It needs a role that may change the schema.
type Migrator struct {
	db       *sql.DB
	provider *goose.Provider
}

// NewMigrator opens a migrator on a pool. Close it before closing the
// pool.
func NewMigrator(pool *pgxpool.Pool) (*Migrator, error) {
	db := stdlib.OpenDBFromPool(pool)
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	// The session lock stops two migrators from running at once.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Migrator{db: db, provider: p}, nil
}

// Close closes the migrator's database handle.
func (m *Migrator) Close() error { return m.db.Close() }

// Up applies all pending migrations, each in its own transaction, and
// returns the versions it applied.
func (m *Migrator) Up(ctx context.Context) ([]int64, error) {
	res, err := m.provider.Up(ctx)
	var applied []int64
	for _, r := range res {
		applied = append(applied, r.Source.Version)
	}
	return applied, err
}

// Migration is one migration and whether it is applied.
type Migration struct {
	Version int64
	Name    string
	Applied bool
}

// Status lists every migration and whether it is applied.
func (m *Migrator) Status(ctx context.Context) ([]Migration, error) {
	st, err := m.provider.Status(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Migration, 0, len(st))
	for _, s := range st {
		out = append(out, Migration{Version: s.Source.Version, Name: s.Source.Path, Applied: s.State == goose.StateApplied})
	}
	return out, nil
}

// CheckMigrations returns ErrPending when the database misses
// migrations. It only reads, so the game role may run it.
func CheckMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	m, err := NewMigrator(pool)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()
	exists, err := db.New(pool).HasMigrationTable(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return ErrPending
	}
	pending, err := m.provider.HasPending(ctx)
	if err != nil {
		return fmt.Errorf("store: check migrations: %w", err)
	}
	if pending {
		return ErrPending
	}
	return nil
}

// HasSchema reports whether any migration ran on the database, which
// makes goose's table. `wactl world restore` only restores into a
// database without it.
func HasSchema(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	return db.New(pool).HasMigrationTable(ctx)
}
