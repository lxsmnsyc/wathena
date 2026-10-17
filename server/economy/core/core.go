// Package core holds core's economy data (strategy section 8.13,
// decision 0046): data/economy.yml, the model player and the hunt, and
// data/economy_targets.yml, the target rates of each level band, which
// the economy package's TestTargets derives from rAthena's classic
// fields.
package core

import "embed"

// Files are core's data/economy.yml and data/economy_targets.yml, under
// data/.
//
//go:embed data
var Files embed.FS

// ModelFile is the model's file, and TargetsFile the targets'.
const (
	ModelFile   = "data/economy.yml"
	TargetsFile = "data/economy_targets.yml"
)
