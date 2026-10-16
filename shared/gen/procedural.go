//go:build procedural

package gen

import "github.com/lxsmnsyc/wathena/shared/rng"

// Procedural is true: this build contains the procedural world mode.
const Procedural = true

// registry lists every generator version, oldest first. A version stays
// until no world uses it (strategy section 8.4).
var registry = []Generator{
	{Version: 1, RNG: rng.Version, Grid: gridV1},
}
