package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store/db"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// ServerVariables keeps the server variables of mods in the
// mod_server_variables table. It is a variables.Store.
type ServerVariables struct {
	pool *pgxpool.Pool
}

// NewServerVariables returns the server variable store of a database.
// waserver uses the game role's pool.
func NewServerVariables(pool *pgxpool.Pool) *ServerVariables { return &ServerVariables{pool: pool} }

// ServerVariables returns every saved value.
func (s *ServerVariables) ServerVariables(ctx context.Context) (map[variables.Key]string, error) {
	rows, err := db.New(s.pool).ServerVariables(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[variables.Key]string, len(rows))
	for _, r := range rows {
		out[variables.Key{Mod: r.ModID, Name: r.Name}] = string(r.Value)
	}
	return out, nil
}

// SetServerVariable saves a value. An empty value deletes the row.
func (s *ServerVariables) SetServerVariable(ctx context.Context, k variables.Key, value string) error {
	q := db.New(s.pool)
	if value == "" {
		return q.DeleteServerVariable(ctx, db.DeleteServerVariableParams{ModID: k.Mod, Name: k.Name})
	}
	return q.SetServerVariable(ctx, db.SetServerVariableParams{ModID: k.Mod, Name: k.Name, Value: []byte(value)})
}
