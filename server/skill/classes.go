package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The first class skills, by class. Each type embeds a shape, and the
// methods of the parts it changes are in ratios.go and in the class files.

type (
	bash           struct{ WeaponSkill }
	magnumBreak    struct{ Base }
	napalmBeat     struct{ Base }
	soulStrike     struct{ Base }
	coldBolt       struct{ Base }
	frostDiver     struct{ Base }
	fireBall       struct{ Base }
	fireWall       struct{ Base }
	fireBolt       struct{ Base }
	lightningBolt  struct{ Base }
	thunderStorm   struct{ Base }
	doubleStrafe   struct{ WeaponSkill }
	arrowShower    struct{ Base }
	chargeArrow    struct{ WeaponSkill }
	holyLight      struct{ Base }
	ruwach         struct{ Base }
	heal           struct{ Base }
	mammonite      struct{ WeaponSkill }
	cartRevolution struct{ Base }
	envenom        struct{ WeaponSkill }
	sandAttack     struct{ WeaponSkill }
	throwStone     struct{ Base }
)

// WeaponSkill is rAthena's WeaponSkillImpl: a single target weapon
// attack.
type WeaponSkill struct{ Base }

func init() {
	register(map[int]battle.SkillImpl{
		IDBash:           bash{},
		IDMagnumBreak:    magnumBreak{},
		IDNapalmBeat:     napalmBeat{},
		IDSoulStrike:     soulStrike{},
		IDColdBolt:       coldBolt{},
		IDFrostDiver:     frostDiver{},
		IDFireBall:       fireBall{},
		IDFireWall:       fireWall{},
		IDFireBolt:       fireBolt{},
		IDLightningBolt:  lightningBolt{},
		IDThunderStorm:   thunderStorm{},
		IDDoubleStrafe:   doubleStrafe{},
		IDArrowShower:    arrowShower{},
		IDChargeArrow:    chargeArrow{},
		IDHolyLight:      holyLight{},
		IDRuwach:         ruwach{},
		IDHeal:           heal{},
		IDMammonite:      mammonite{},
		IDCartRevolution: cartRevolution{},
		IDEnvenom:        envenom{},
		IDSandAttack:     sandAttack{},
		IDThrowStone:     throwStone{},
	})
}

// MaxCartWeight is battle_config.max_cart_weight: a pushcart's maximum
// weight, which status_calc_pc gives every player. It is 8000 in
// conf/battle/player.conf, which battle_set_defaults multiplies by 10,
// as weights are in tenths.
const MaxCartWeight = 80000
