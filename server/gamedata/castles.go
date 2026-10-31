package gamedata

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/castle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// Castles (proposal 0004, section 1): records of data/castles.yml. A
// record has:
//   - id: rAthena's castle ID for core's records. Mods' records get IDs
//     from 10,000.
//   - map: the castle's map. A map has at most one castle.
//   - display_name: what getcastlename returns, 1 to 24 characters. It
//     is the record's name without the mod prefix when left out.
//   - edition: fe, se or te. te needs the renewal ruleset.
//   - npc: the record name of the NPC that hears OnGuildBreak.
//   - client_id: the client's castle number, 0 for none.
//   - teleport: {x, y, cost, siege_cost}, or none.
//
// A record that breaks a rule of the world (an edition the ruleset does
// not have, a second castle on a map) is left out with a note, as the
// proposal asks. waserver leaves out records whose map does not run or
// whose teleport cell is not walkable.

// modCastles merges the castle records.
func modCastles(ctx context.Context, set *mod.Set, ids *mod.IDs, mode reference.Mode) ([]*castle.Record, *mod.Overlay, error) {
	o, err := set.Overlay(mod.CastleKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	var out []*castle.Record
	var errs []error
	castleMaps := map[string]string{}
	for _, r := range o.Records {
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("%s: castles %s: %s", r.Where, r.Name, fmt.Sprintf(format, args...)))
		}
		for _, k := range slices.Sorted(maps.Keys(r.Fields)) {
			switch k {
			case "id", "map", "display_name", "edition", "npc", "client_id", "teleport":
			default:
				bad("%q is not a field; the fields are map, display_name, edition, npc, client_id and teleport", k)
			}
		}
		var id uint32
		if r.Mod == mod.Core {
			n, ok := r.Fields["id"].(int)
			if !ok || n < 0 || n >= int(mod.CastleIDs.Start) {
				bad("a core record needs an id from 0 to %d", mod.CastleIDs.Start-1)
				continue
			}
			id = uint32(n) // #nosec G115 -- checked above.
		} else {
			if _, ok := r.Fields["id"]; ok && r.SetBy["id"] == r.Mod {
				bad("id is given by the server; leave it out")
				continue
			}
			n, err := ids.ID(ctx, mod.CastleIDs, r.Name, r.Mod)
			if err != nil {
				return nil, nil, err
			}
			id = n
		}
		rec := &castle.Record{ID: id, Name: r.Name, DisplayName: mod.Local(r.Name)}
		if rec.DisplayName == "" {
			rec.DisplayName = r.Name
		}
		if v, ok := r.Fields["display_name"]; ok {
			s, isText := v.(string)
			if !isText || s == "" || utf8.RuneCountInString(s) > 24 {
				bad("display_name is 1 to 24 characters")
				continue
			}
			rec.DisplayName = s
		}
		var ok bool
		if rec.Map, ok = r.Fields["map"].(string); !ok || rec.Map == "" {
			bad("map is the castle's map")
			continue
		}
		if rec.NPC, ok = r.Fields["npc"].(string); !ok || rec.NPC == "" {
			bad("npc is the record name of the NPC that hears OnGuildBreak")
			continue
		}
		rec.Edition, _ = r.Fields["edition"].(string)
		if !slices.Contains(castle.Editions, rec.Edition) {
			bad("edition is fe, se or te")
			continue
		}
		if v, ok := r.Fields["client_id"]; ok {
			n, isInt := v.(int)
			if !isInt || n < 0 || n > 65535 {
				bad("client_id is 0 to 65535")
				continue
			}
			rec.ClientID = uint32(n) // #nosec G115 -- checked above.
		}
		if v, ok := r.Fields["teleport"]; ok && v != nil {
			t, err := castleTeleport(v)
			if err != nil {
				bad("%v", err)
				continue
			}
			rec.Teleport = t
		}
		if rec.Edition == "te" && mode != reference.Renewal {
			o.Notes = append(o.Notes, fmt.Sprintf("castles %s: left out: edition te needs the renewal ruleset", r.Name))
			continue
		}
		if other, taken := castleMaps[rec.Map]; taken {
			o.Notes = append(o.Notes, fmt.Sprintf("castles %s: left out: %s already has the castle %s", r.Name, rec.Map, other))
			continue
		}
		castleMaps[rec.Map] = r.Name
		out = append(out, rec)
	}
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}
	slices.SortFunc(out, func(a, b *castle.Record) int { return int(a.ID) - int(b.ID) })
	return out, o, nil
}

// castleTeleport reads a teleport field: {x, y, cost, siege_cost}, with
// rAthena's costs of 100 and 100,000 when left out.
func castleTeleport(v any) (*castle.Teleport, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("teleport is {x, y, cost, siege_cost}")
	}
	t := &castle.Teleport{Cost: 100, SiegeCost: 100000}
	for k, f := range m {
		n, isInt := f.(int)
		switch k {
		case "x", "y":
			if !isInt || n <= 0 || n >= 1024 {
				return nil, fmt.Errorf("teleport.%s is a cell from 1 to 1023", k)
			}
			if k == "x" {
				t.X = n
			} else {
				t.Y = n
			}
		case "cost", "siege_cost":
			if !isInt || n < 0 || n > character.MaxZeny {
				return nil, fmt.Errorf("teleport.%s is 0 to %d zeny", k, character.MaxZeny)
			}
			if k == "cost" {
				t.Cost = int64(n)
			} else {
				t.SiegeCost = int64(n)
			}
		default:
			return nil, fmt.Errorf("teleport.%s is not a field; the fields are x, y, cost and siege_cost", k)
		}
	}
	if t.X == 0 || t.Y == 0 {
		return nil, errors.New("teleport needs x and y")
	}
	return t, nil
}
