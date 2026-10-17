// Package economy is the economy simulator of procedural worlds
// (strategy section 8.13, decision 0046):
//
//   - A model player of each level: a melee character with the status
//     points of its level, the gear of a town's dealers and a potion,
//     from core/data/economy.yml.
//   - Simulate estimates a field's base and job exp, zeny and supplies
//     per hour for that player, from the field's spawn list, the
//     monsters' stats, exp and drops, the items' prices, and the hunt of
//     the model: the walk to the next monster by the density, the fight
//     by the server's battle formulas, and the loot.
//   - Each level band has target rates, which TestTargets derives from
//     rAthena's classic fields with the same simulator into
//     core/data/economy_targets.yml. A generated field must be within
//     the tolerance of its band's target (the generator checks), and bot
//     runs within theirs of the simulator.
//
// Every number the checks compare is an integer, and every draw comes
// from seeded streams, so a world gives the same results each time.
package economy

import (
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/economy/core"
)

// Config is core/data/economy.yml: the model player, its hunt and the
// tolerances.
type Config struct {
	Player    PlayerConfig `yaml:"player"`
	Hunt      Hunt         `yaml:"hunt"`
	Tolerance Tolerance    `yaml:"tolerance"`
}

// PlayerConfig is the model player of each level.
type PlayerConfig struct {
	Jobs    []JobStep    `yaml:"jobs"`
	Build   Build        `yaml:"build"`
	Gear    []GearStep   `yaml:"gear"`
	Potions []PotionStep `yaml:"potions"`
}

// JobStep is the job from a base level on.
type JobStep struct {
	From        int `yaml:"from"`
	Job         int `yaml:"job"`
	MaxJobLevel int `yaml:"max_job_level"`
}

// Build shares the status points among the stats by weight.
type Build struct {
	Str int `yaml:"str"`
	Agi int `yaml:"agi"`
	Vit int `yaml:"vit"`
	Dex int `yaml:"dex"`
}

// GearStep is the gear worn from a base level on, by AegisName, and
// the refine level of each piece.
type GearStep struct {
	From   int      `yaml:"from"`
	Items  []string `yaml:"items"`
	Refine int      `yaml:"refine"`
}

// PotionStep is the potion of a level and the HP its itemheal draws.
type PotionStep struct {
	From int    `yaml:"from"`
	Item string `yaml:"item"`
	HP   [2]int `yaml:"hp"`
}

// Hunt is how the model player spends its time.
type Hunt struct {
	// ApproachPermille scales the walk to the next monster: thousandths
	// of the side of the square that holds one monster on average.
	ApproachPermille int `yaml:"approach_permille"`
	// OverheadMs is the time of each kill outside the walk and the
	// fight, and LootMs the time to pick up each item that drops.
	OverheadMs int `yaml:"overhead_ms"`
	LootMs     int `yaml:"loot_ms"`
	// Fights is how many fights the simulator plays per monster.
	Fights int `yaml:"fights"`
	// MaxFightMs is the longest fight the player takes on: it leaves
	// alone monsters whose fights last longer on average.
	MaxFightMs int `yaml:"max_fight_ms"`
}

// Tolerance is how far results may be, in percent.
type Tolerance struct {
	// Generator is how far a generated field may be from its band's
	// target, and Bots how far a bot run may be from the simulator.
	Generator int `yaml:"generator"`
	Bots      int `yaml:"bots"`
}

// LoadConfig reads core's economy model.
func LoadConfig() (*Config, error) {
	b, err := core.Files.ReadFile(core.ModelFile)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", core.ModelFile, err)
	}
	if err := c.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", core.ModelFile, err)
	}
	return &c, nil
}

func (c *Config) check() error {
	p := c.Player
	switch {
	case len(p.Jobs) == 0 || p.Jobs[0].From != 1:
		return fmt.Errorf("player.jobs must start at level 1")
	case len(p.Gear) == 0 || p.Gear[0].From != 1:
		return fmt.Errorf("player.gear must start at level 1")
	case len(p.Potions) == 0 || p.Potions[0].From != 1:
		return fmt.Errorf("player.potions must start at level 1")
	case p.Build.Str < 0 || p.Build.Agi < 0 || p.Build.Vit < 0 || p.Build.Dex < 0 || p.Build.Str+p.Build.Agi+p.Build.Vit+p.Build.Dex == 0:
		return fmt.Errorf("player.build needs a positive weight")
	case c.Hunt.ApproachPermille < 0 || c.Hunt.OverheadMs < 0 || c.Hunt.LootMs < 0:
		return fmt.Errorf("hunt values are 0 or more")
	case c.Hunt.Fights < 1 || c.Hunt.Fights > 1000:
		return fmt.Errorf("hunt.fights is 1 to 1000")
	case c.Hunt.MaxFightMs < 1000:
		return fmt.Errorf("hunt.max_fight_ms is 1000 or more")
	case c.Tolerance.Generator < 1 || c.Tolerance.Bots < 1:
		return fmt.Errorf("tolerance values are 1 or more")
	}
	for _, g := range p.Gear {
		if g.Refine < 0 || g.Refine > 10 {
			return fmt.Errorf("player.gear: refine %d is 0 to 10", g.Refine)
		}
	}
	for _, p := range p.Potions {
		if p.HP[0] < 1 || p.HP[1] < p.HP[0] {
			return fmt.Errorf("player.potions: %s heals %v", p.Item, p.HP)
		}
	}
	return nil
}

// stepAt returns the index of the last step whose from is at most level.
func stepAt(level int, from func(i int) int, n int) int {
	at := 0
	for i := range n {
		if from(i) <= level {
			at = i
		}
	}
	return at
}
