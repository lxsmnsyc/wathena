package botclient

import "time"

// Scenarios are the load tests in server/README.md, "Load tests and
// metrics". Each needs a server started as its comment says. The rates
// follow the busy town estimate in schema/protocol_test.go: a walk every
// 3 seconds per player and 2 chat lines a second in the whole town.
var Scenarios = map[string]Scenario{
	// 500 players stroll around Prontera and chat.
	// Server: --map prontera.
	"town": {
		Bots: 500, Ramp: 50, Warmup: 15 * time.Second, Duration: 3 * time.Minute, ChatRate: 2,
		Behaviour: Behaviour{WalkEvery: 3 * time.Second, WalkRadius: 8},
	},
	// 500 players crowd around the start cell, all in view of each
	// other, as at a town event, at the rates of the town peak estimate:
	// a walk every 2 seconds and 10 chat lines a second.
	// Server: --map prontera.
	"crowd": {
		Bots: 500, Ramp: 50, Warmup: 15 * time.Second, Duration: 3 * time.Minute, ChatRate: 10,
		Behaviour: Behaviour{WalkEvery: 2 * time.Second, WalkRadius: 10, Stay: true},
	},
	// 500 players start in Prontera and spread to the maps around it
	// through warps: a third of their walks go to a warp portal in view.
	// Server: --map prontera --maps prontera,prt_fild08,prt_in,
	// prt_fild05,prt_fild06,prt_fild07,prt_church,izlude.
	"spread": {
		Bots: 500, Ramp: 50, Warmup: 60 * time.Second, Duration: 3 * time.Minute, ChatRate: 2,
		Behaviour: Behaviour{WalkEvery: 3 * time.Second, WalkRadius: 12, WarpChance: 0.33},
	},
	// 500 players fight the monsters of a field. A bot attacks the
	// nearest monster in view until it dies, and walks to find more.
	// Server: --map prt_fild08 --build melee.
	"fight": {
		Bots: 500, Ramp: 50, Warmup: 15 * time.Second, Duration: 3 * time.Minute, ChatRate: 0.5,
		Behaviour: Behaviour{WalkEvery: 3 * time.Second, WalkRadius: 12, Fight: true},
	},
	// Players fight on generated fields of several bands of a procedural
	// world. wabots spreads the bots over 8 fields from the lowest band
	// to the highest, each as the model player of its field's band, with
	// its save point on its field, so a bot that dies comes back there.
	// Server: a world that wactl world new made; wabots --database.
	"fields": {
		Bots: 500, Ramp: 50, Warmup: 15 * time.Second, Duration: 3 * time.Minute, ChatRate: 0.5, Place: "fields",
		Behaviour: Behaviour{WalkEvery: 3 * time.Second, WalkRadius: 12, Fight: true},
	},
	// Players walk the generated map graph of a procedural world: each
	// walks a route to a portal of its map, other than the one it came
	// in at, and on through the next map. They start spread over the
	// towns as the model player of the highest band, so the fields they
	// cross do not kill them, and do not fight.
	// Server: a world that wactl world new made; wabots --database.
	"graph": {
		Bots: 500, Ramp: 50, Warmup: 60 * time.Second, Duration: 3 * time.Minute, ChatRate: 2, Place: "towns",
		Behaviour: Behaviour{WalkEvery: 3 * time.Second, WalkRadius: 12, Travel: true},
	},
	// A short run for CI against the placeholder map, where every bot
	// sees every other. Rates are higher than in a town.
	// Server: --map placeholder/map.
	"smoke": {
		Bots: 50, Ramp: 25, Warmup: 3 * time.Second, Duration: 20 * time.Second, ChatRate: 5,
		Behaviour: Behaviour{WalkEvery: time.Second, WalkRadius: 8},
	},
}
