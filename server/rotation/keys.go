package rotation

import (
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Keys of the secret branch (proposal 0002, review decision 47): `wactl
// world new` gives the game role one key per rotating dungeon and one
// for seasons, which reveal only those maps' future spawns. The game
// server never holds the world's seed or its secret branch.
//
//	secret
//	├── rotation
//	│   └── <dungeon>          RotationKey, in world_rotations.key
//	│       └── date(yyyymmdd) RotationSecret: the secret of a date's spawns
//	└── season                 SeasonKey, in world.season_key
//	    └── <town ID>
//	        └── season(n)      SeasonSecret: the secret of a region's season
//
// spawns.Plan takes RotationSecret or SeasonSecret as its secret, so the
// floors' and fields' lists come from child(child(that, "spawns"), <map
// ID>), with the same selection code as at world creation.

// RotationKey returns child(child(secret, "rotation"), dungeon).
func RotationKey(secret rng.Node, dungeon string) rng.Node {
	return secret.Child("rotation").Child(dungeon)
}

// SeasonKey returns child(secret, "season").
func SeasonKey(secret rng.Node) rng.Node { return secret.Child("season") }

// RotationSecret returns the secret of a rotation date's spawns:
// child(key, "date", yyyymmdd).
func RotationSecret(key rng.Node, date string) rng.Node {
	return key.Child("date", layout.DateNumber(date))
}

// SeasonSecret returns the secret of a region's season's spawns:
// child(child(key, town), "season", n).
func SeasonSecret(key rng.Node, town string, season int) rng.Node {
	return key.Child(town).Child("season", int64(season))
}
