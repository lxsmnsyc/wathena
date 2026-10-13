package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// The world, events and mods tools on the game role (decision 0031):
//   - event_overrides keeps staff changes of world events over the
//     config. A reset deletes the row.
//   - mod_switches keeps the mods staff turned off, by map, where ''
//     is every zone.
//   - Each change writes its audit row in the same transaction.

// EventOverride is a staff change of a world event.
type EventOverride struct {
	Event string
	// On is whether the event is on, or nil for the config's.
	On *bool
	// Trigger is the trigger in the form of data/events.yml, or nil
	// for the config's.
	Trigger map[string]any
	// DurationMinutes is the duration, or 0 for the config's.
	DurationMinutes int
	By              string
	ByID            int64
	At              time.Time
}

// LoadEventOverrides reads the staff changes of world events, for the
// scheduler at start.
func LoadEventOverrides(ctx context.Context, pool *pgxpool.Pool) ([]EventOverride, error) {
	rows, err := db.New(pool).EventOverrides(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]EventOverride, 0, len(rows))
	for _, r := range rows {
		o := EventOverride{Event: r.Event, By: r.ChangedBy, ByID: r.ChangedByID, At: r.ChangedAt.Time}
		if r.Enabled.Valid {
			on := r.Enabled.Bool
			o.On = &on
		}
		if r.DurationMinutes.Valid {
			o.DurationMinutes = int(r.DurationMinutes.Int32)
		}
		if len(r.Trigger) > 0 {
			if err := json.Unmarshal(r.Trigger, &o.Trigger); err != nil {
				return nil, err
			}
		}
		out = append(out, o)
	}
	return out, nil
}

// SetEventOverride saves a staff change of a world event with its audit
// row.
func (s *Store) SetEventOverride(ctx context.Context, o EventOverride, audit AuditRow) error {
	p := db.SetEventOverrideParams{Event: o.Event, ChangedBy: o.By, ChangedByID: o.ByID}
	if o.On != nil {
		p.Enabled = pgtype.Bool{Bool: *o.On, Valid: true}
	}
	if o.DurationMinutes > 0 {
		p.DurationMinutes = pgtype.Int4{Int32: int32(o.DurationMinutes), Valid: true} // #nosec G115 -- at most a week of minutes.
	}
	if o.Trigger != nil {
		b, err := json.Marshal(o.Trigger)
		if err != nil {
			return err
		}
		p.Trigger = b
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.SetEventOverride(ctx, p); err != nil {
			return err
		}
		return writeAudit(ctx, q, audit)
	})
}

// ResetEventOverride deletes a staff change of a world event with its
// audit row. It reports false, and writes nothing, when the event had
// none.
func (s *Store) ResetEventOverride(ctx context.Context, event string, audit AuditRow) (bool, error) {
	found := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.DeleteEventOverride(ctx, event)
		if err != nil || n == 0 {
			return err
		}
		found = true
		return writeAudit(ctx, q, audit)
	})
	return found, err
}

// ModSwitch is a mod that staff turned off in a map's zone, or in every
// zone when Map is "".
type ModSwitch struct {
	Mod, Map, Reason string
	By               string
	ByID             int64
	At               time.Time
}

// LoadModSwitches reads the mods that staff turned off, for the zones at
// start.
func LoadModSwitches(ctx context.Context, pool *pgxpool.Pool) ([]ModSwitch, error) {
	rows, err := db.New(pool).ModSwitches(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ModSwitch, 0, len(rows))
	for _, r := range rows {
		out = append(out, ModSwitch{Mod: r.Mod, Map: r.Map, Reason: r.Reason, By: r.ChangedBy, ByID: r.ChangedByID, At: r.ChangedAt.Time})
	}
	return out, nil
}

// SwitchMod turns a mod off or on, in a map's zone or in every zone for
// map "", with its audit row:
//   - Off in every zone replaces the mod's rows with one for every zone.
//   - Off in a zone adds that zone's row.
//   - On in every zone deletes every row of the mod.
//   - On in a zone deletes that zone's row.
func (s *Store) SwitchMod(ctx context.Context, sw ModSwitch, off bool, audit AuditRow) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var err error
		switch {
		case off && sw.Map == "":
			if _, err = q.DeleteModSwitches(ctx, sw.Mod); err == nil {
				err = q.AddModSwitch(ctx, db.AddModSwitchParams{Mod: sw.Mod, Map: "", Reason: sw.Reason, ChangedBy: sw.By, ChangedByID: sw.ByID})
			}
		case off:
			err = q.AddModSwitch(ctx, db.AddModSwitchParams{Mod: sw.Mod, Map: sw.Map, Reason: sw.Reason, ChangedBy: sw.By, ChangedByID: sw.ByID})
		case sw.Map == "":
			_, err = q.DeleteModSwitches(ctx, sw.Mod)
		default:
			_, err = q.DeleteModSwitch(ctx, db.DeleteModSwitchParams{Mod: sw.Mod, Map: sw.Map})
		}
		if err != nil {
			return err
		}
		return writeAudit(ctx, q, audit)
	})
}

// WriteAudit adds a row to the GM audit log on the game role.
func (s *Store) WriteAudit(ctx context.Context, row AuditRow) error { return writeAudit(ctx, s.q, row) }
