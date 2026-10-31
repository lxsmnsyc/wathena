package modscript

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/castle"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// Castles (proposal 0004, section 8): castle handles, wa.castles and the
// castle_change event. A castle handle's ID is its record ID. Reads need
// game.read; changes need castles.manage, and npcs.control too on a
// castle another mod added, core's too.

// CastleChange is the castle_change event: a castle's owner changed.
type CastleChange struct {
	Castle   uint32
	Previous int64
}

// CastleChangeArg is castle_change's argument, which becomes a
// wa.CastleChange.
func CastleChangeArg(e CastleChange) Arg {
	return Arg{"castle_change", script.NewTable(
		script.KV("castle", script.Int(int64(e.Castle))), script.KV("previous_owner_id", script.Int(e.Previous)))}
}

func (r *Runtime) castles(what string) (*castle.Registry, error) {
	if r.opts.Castles == nil {
		return nil, errors.New(what + ": castles do not exist here")
	}
	return r.opts.Castles, nil
}

func (r *Runtime) castleRecord(c *script.Call, i int, what string) (*castle.Registry, *castle.Record, error) {
	reg, err := r.castles(what)
	if err != nil {
		return nil, nil, err
	}
	id, ok := c.Arg(i).AsInt()
	if !ok || id < 0 {
		return nil, nil, errors.New(what + ": not a castle handle")
	}
	rec := reg.ByID(uint32(id)) // #nosec G115 -- checked above; record IDs are uint32.
	if rec == nil {
		return nil, nil, fmt.Errorf("%s: no castle %d", what, id)
	}
	return reg, rec, nil
}

// castlesFind answers castles.get, castles.list and castles.of with
// record IDs.
func (r *Runtime) castlesFind(c *script.Call) error {
	by := c.String(0)
	what := "castles." + by
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	reg, err := r.castles(what)
	if err != nil {
		return err
	}
	switch by {
	case "get":
		name, ok := c.Arg(1).AsString()
		if !ok {
			return errors.New(what + ": want a castle's name or map")
		}
		rec := reg.Find(name, r.actor())
		if rec == nil {
			c.Return(script.Nil)
			return nil
		}
		c.Return(script.Int(int64(rec.ID)))
	case "list":
		// edition is "" for every edition; owner is -1 for any owner, 0
		// for castles without one, or a guild ID.
		edition := c.String(1)
		owner := c.Int(2)
		var ids []int64
		for _, rec := range reg.Records() {
			if edition != "" && rec.Edition != edition {
				continue
			}
			if owner >= 0 {
				if s, _ := reg.State(rec.Name); s.Owner != owner {
					continue
				}
			}
			ids = append(ids, int64(rec.ID))
		}
		c.Return(intList(ids))
	case "of":
		var ids []int64
		for _, rec := range reg.Of(c.Int(1)) {
			ids = append(ids, int64(rec.ID))
		}
		c.Return(intList(ids))
	default:
		return fmt.Errorf("castles: no call %s", by)
	}
	return nil
}

// castleField reads a field of a castle handle. owner is the owner's
// guild ID, which the prelude makes a guild handle.
func (r *Runtime) castleField(c *script.Call) error {
	if err := r.need("reading a castle", "game.read"); err != nil {
		return err
	}
	reg, rec, err := r.castleRecord(c, 0, "reading a castle")
	if err != nil {
		return err
	}
	s, _ := reg.State(rec.Name)
	var v script.Value
	switch c.String(1) {
	case "id":
		v = script.Int(int64(rec.ID))
	case "name":
		v = script.String(rec.Name)
	case "display_name":
		v = script.String(rec.DisplayName)
	case "map":
		v = script.String(rec.Map)
	case "edition":
		v = script.String(rec.Edition)
	case "npc":
		v = script.String(rec.NPC)
	case "owner", "owner_id":
		v = script.Int(s.Owner)
	case "economy":
		v = script.Int(int64(s.Economy))
	case "defense":
		v = script.Int(int64(s.Defense))
	case "invested_economy":
		v = script.Int(int64(s.InvestedEconomy))
	case "invested_defense":
		v = script.Int(int64(s.InvestedDefense))
	case "next_time":
		v = script.Int(int64(s.NextTime))
	case "pay_time":
		v = script.Int(int64(s.PayTime))
	case "create_time":
		v = script.Int(int64(s.CreateTime))
	case "kafra":
		v = script.Bool(s.Kafra)
	case "teleport":
		if t := rec.Teleport; t != nil {
			v = script.NewTable(script.KV("x", script.Int(int64(t.X))), script.KV("y", script.Int(int64(t.Y))),
				script.KV("cost", script.Int(t.Cost)), script.KV("siege_cost", script.Int(t.SiegeCost)))
		}
	}
	c.Return(v)
	return nil
}

// needCastleManage checks castles.manage, and npcs.control for a castle
// another mod added. A test case's own code needs no permission, as for
// guilds.
func (r *Runtime) needCastleManage(what string, rec *castle.Record) error {
	if r.opts.Testing && r.current == nil {
		return nil
	}
	if err := r.need(what, "castles.manage"); err != nil {
		return err
	}
	owner := mod.Core
	if p, _, ok := strings.Cut(rec.Name, ":"); ok {
		owner = p
	}
	if owner != r.actor() {
		return r.need(what+" of "+owner+"'s castle", "npcs.control")
	}
	return nil
}

// castleCall runs a castle handle's methods.
func (r *Runtime) castleCall(c *script.Call) error {
	method := c.String(0)
	what := "castle:" + method
	reg, rec, err := r.castleRecord(c, 1, what)
	if err != nil {
		return err
	}
	if method == "get_guardian" {
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
		i := c.Int(2)
		if i < 0 || i >= castle.Guardians {
			return fmt.Errorf("%s: the index is 0 to %d", what, castle.Guardians-1)
		}
		s, _ := reg.State(rec.Name)
		c.Return(script.Bool(s.Installed(int(i))))
		return nil
	}
	if err := r.needCastleManage(what, rec); err != nil {
		return err
	}
	switch method {
	case "set_owner":
		guild := c.Int(2)
		if guild < 0 {
			return errors.New(what + ": want a guild handle or nil")
		}
		c.Return(script.Bool(reg.SetOwner(rec.Name, guild, castle.SourceScript)))
		return nil
	case "set":
		// Arguments 2 to 8 are economy, defense, invested_economy,
		// invested_defense, next_time, pay_time and create_time, nil when
		// not set; 9 is kafra.
		var v castle.Values
		for i, to := range []**int{&v.Economy, &v.Defense, &v.InvestedEconomy, &v.InvestedDefense, &v.NextTime, &v.PayTime, &v.CreateTime} {
			a := c.Arg(2 + i)
			if a.IsNil() {
				continue
			}
			n, ok := a.AsInt()
			if !ok || n > 1<<31-1 {
				return errors.New(what + ": values are whole numbers below 2^31")
			}
			x := int(n)
			*to = &x
		}
		if a := c.Arg(9); !a.IsNil() {
			b := a.Truthy()
			v.Kafra = &b
		}
		if err := reg.Set(rec.Name, v); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		return nil
	case "set_guardian":
		if err := reg.SetGuardian(rec.Name, int(c.Int(2)), c.Bool(3)); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		return nil
	}
	return fmt.Errorf("castle: no method %s", method)
}
