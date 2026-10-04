package battle

import (
	"math/rand/v2"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
)

func TestPreRenewalAttributeMatchesReference(t *testing.T) {
	var ref reference.Elements
	if err := reference.Load(reference.PreRenewal, "elements", &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Levels) != maxElementLevel {
		t.Fatalf("got %d levels, want %d", len(ref.Levels), maxElementLevel)
	}
	for _, lv := range ref.Levels {
		for atk, row := range lv.Table {
			for def, want := range row {
				if got := PreRenewalAttribute(lv.Level, atk, def); got != want {
					t.Errorf("level %d, %d against %d: got %d, want %d", lv.Level, atk, def, got, want)
				}
			}
		}
	}
}

func TestAttrFixPreRenewal(t *testing.T) {
	tests := []struct {
		damage             int64
		atk, def, lv, flag int
		want               int64
	}{
		{100, Neutral, Neutral, 1, 0, 100},
		{100, Neutral, Ghost, 1, 0, 25},
		{101, Neutral, Ghost, 1, 0, 25},
		{99, Water, Fire, 1, 0, 148},
		// Negative percents count as 0 without flag 1, and stay negative
		// with it.
		{100, Poison, Undead, 1, 0, 0},
		{100, Poison, Undead, 1, 1, -25},
		{-101, Neutral, Neutral, 1, 1, -101},
		{100, -1, Neutral, 1, 0, 100},
		{100, Neutral, Neutral, 5, 0, 100},
	}
	for _, tt := range tests {
		if got := AttrFixPreRenewal(tt.damage, tt.atk, tt.def, tt.lv, tt.flag); got != tt.want {
			t.Errorf("AttrFixPreRenewal(%d, %d, %d, %d, %d) = %d, want %d", tt.damage, tt.atk, tt.def, tt.lv, tt.flag, got, tt.want)
		}
	}
}

// TestPreRenewalPlayerAttackTable checks hand-made cases for the
// pre-renewal player rules, including rules the recorded cases do not
// reach.
func TestPreRenewalPlayerAttackTable(t *testing.T) {
	const (
		short = bfWeapon | bfNormal | bfShort
		long  = bfWeapon | bfNormal | bfLong
	)
	// Weapon ATK 100, level 1, DEX 50: the roll is 50..99, plus base ATK
	// 50. Forced rolls give 100.
	base := func() Status {
		return Status{
			Level: 99, Hit: 300, Str: 100, Dex: 50, BAtk: 50,
			RHW:    Weapon{Atk: 100, Wlv: 1, Range: 1},
			Player: &Player{Weapon: WeaponMace, RightWeapon: WeaponMace},
		}
	}
	hit := func(d int64) Result { return Result{Kind: Hit, Damage: d, Hits: 1, Flag: short} }
	crit := func(d int64) Result { return Result{Kind: Critical, Damage: d, Hits: 1, Flag: short} }
	dagger := func(s *Status) { s.Player.Weapon, s.Player.RightWeapon = WeaponDagger, WeaponDagger }
	bow := func(s *Status) {
		s.Player.Weapon, s.Player.RightWeapon, s.Player.LeftWeapon = WeaponBow, WeaponBow, WeaponBow
		s.Player.ArrowAttack, s.Player.AmmoAtk = true, 30
	}
	defs := func(def, def2 int) func(*Status) { return func(d *Status) { d.Def, d.Def2 = def, def2 } }
	tests := []struct {
		name string
		src  func(*Status)
		dst  func(*Status)
		r    Rand
		want Result
	}{
		{"plain hit", nil, nil, forcedMin{}, hit(100)},
		// The weapon's ATK, plus base ATK. DEF does not apply.
		{"critical", func(s *Status) { s.Cri = 1000 }, defs(50, 40), forcedMin{}, crit(150)},
		// 100 * 50%, minus soft DEF 40 + rnd() % 4.
		{"DEF", nil, defs(50, 40), forcedMin{}, hit(10)},
		{"DEF over 100", nil, defs(120, 0), forcedMin{}, hit(1)},
		// The refine bonus is added after DEF.
		{"refine", func(s *Status) { s.RHW.Atk2 = 30 }, defs(50, 40), forcedMin{}, hit(40)},
		// 50 - 100 + 30 is negative, and is raised to 1.
		{"refine below 0", func(s *Status) { s.RHW.Atk2 = 30 }, defs(50, 100), forcedMin{}, hit(1)},
		{"mastery after DEF", func(s *Status) { s.Player.Skills = map[int]int{skillMaceMastery: 10} }, defs(50, 40), forcedMin{}, hit(40)},
		{"Weapon Research", func(s *Status) { s.Player.Skills = map[int]int{skillWeaponResearch: 10} }, nil, forcedMin{}, hit(120)},
		// A miss still deals the ATK of Weapon Research.
		{"Weapon Research on a miss", func(s *Status) {
			s.Hit = 0
			s.Player.Skills = map[int]int{skillWeaponResearch: 10}
		}, func(d *Status) { d.Flee = 1000 }, fixedRand(99), hit(20)},
		// DEX 50 times weapon ATK 100%, then Fire against Water 1 (50%).
		{"ammunition", func(s *Status) { bow(s); s.Player.AmmoEle = Fire }, func(d *Status) { d.DefEle = Water }, forcedMin{},
			Result{Kind: Hit, Damage: 50, Hits: 1, Flag: long}},
		// A bow's critical hit adds all of the ammunition's ATK.
		{"bow critical", func(s *Status) { bow(s); s.Cri = 1000 }, nil, forcedMin{}, Result{Kind: Critical, Damage: 180, Hits: 1, Flag: long}},
		// A gun's critical hit works like a melee one.
		{"gun critical", func(s *Status) {
			bow(s)
			s.Player.Weapon, s.Player.RightWeapon, s.Player.LeftWeapon = WeaponRevolver, WeaponRevolver, WeaponRevolver
			s.Cri = 1000
		}, nil, forcedMin{}, Result{Kind: Critical, Damage: 150, Hits: 1, Flag: long}},
		// A dagger deals 75% to medium targets: 37 + 50, twice. Double
		// Attack cannot be critical in pre-renewal.
		{"double attack", func(s *Status) {
			dagger(s)
			s.Cri = 1000
			s.Player.Skills = map[int]int{skillDoubleAttack: 10}
		}, nil, forcedMin{}, Result{Kind: Hit, Damage: 174, Hits: 2, Flag: short}},
		{"double attack rate", func(s *Status) {
			dagger(s)
			s.Player.Skills = map[int]int{skillDoubleAttack: 10}
		}, nil, &seqRand{rolls: []int{50}}, hit(87)},
		// Right: 100 + 30 mastery, then 20% from Advanced Katar Mastery.
		// Left: base ATK 50 + 30 mastery. The second hit is 21% of the
		// right hand.
		{"katar", func(s *Status) {
			s.Player.Weapon, s.Player.RightWeapon, s.Player.LeftWeapon = WeaponKatar, WeaponKatar, WeaponKatar
			s.LHW.Wlv = 1
			s.Player.Skills = map[int]int{skillDoubleAttack: 10, skillKatarMastery: 10, skillAdvKatarMastery: 5}
		}, nil, forcedMin{}, Result{Kind: Hit, Damage: 188, Left: 32, Hits: 1, Flag: short}},
		// Two daggers: 87 at 100% on the right and 87 at 80% on the left.
		{"dual wield", func(s *Status) {
			s.Player.Weapon, s.Player.RightWeapon, s.Player.LeftWeapon = 25, WeaponDagger, WeaponDagger // W_DOUBLE_DD
			s.Player.DualWield = DualWieldThief
			s.Player.Skills = map[int]int{skillRightHandMastery: 5, skillLeftHandMastery: 5}
			s.LHW = Weapon{Atk: 100, Wlv: 1, Range: 1}
		}, nil, forcedMin{}, Result{Kind: Hit, Damage: 156, Left: 69, Hits: 1, Flag: short}},
		// Piercing deals DEF + soft DEF percent of the damage.
		{"pierce DEF", func(s *Status) { s.Player.Right.DefRatioRace = 1 << RaceAll }, defs(50, 40), forcedMin{}, hit(90)},
		{"pierce DEF critical", func(s *Status) { s.Player.Right.DefRatioRace = 1 << RaceAll; s.Cri = 1000 }, defs(50, 40), forcedMin{}, crit(150)},
		{"ignore DEF", func(s *Status) { s.Player.Right.IgnoreDefRace = 1 << RaceAll }, defs(50, 40), forcedMin{}, hit(100)},
		// DEF 25 and soft DEF 20: 75 - 20.
		{"ignore DEF rate", func(s *Status) { s.Player.IgnoreDefRace = map[int]int{RaceAll: 50} }, defs(50, 40), forcedMin{}, hit(55)},
		// Cards apply to the whole damage, with the melee rate: 120% * 110%.
		{"cards", func(s *Status) {
			s.Player.Right.AddRace = map[int]int{RaceDemiHuman: 20}
			s.Player.ShortAtkRate = 10
		}, func(d *Status) { d.Race = RaceDemiHuman }, forcedMin{}, hit(132)},
		{"critical damage", func(s *Status) { s.Cri, s.Player.CritAtkRate = 1000, 20 }, nil, forcedMin{}, crit(180)},
		{"non-critical damage", func(s *Status) { s.Player.NonCritAtkRate = 10 }, nil, forcedMin{}, hit(110)},
		// Taekwon Mission adds 2% per level for each other party member
		// on the map: 5 * 2 * 2.
		{"Taekwon Mission in a party", func(s *Status) {
			s.Player.Skills = map[int]int{skillTKPower: 5}
			s.Player.PartyOnMap = 3
		}, nil, forcedMin{}, hit(120)},
		{"Taekwon Mission alone in a party", func(s *Status) {
			s.Player.Skills = map[int]int{skillTKPower: 5}
			s.Player.PartyOnMap = 1
		}, nil, forcedMin{}, hit(100)},
		{"Taekwon Mission without a party", func(s *Status) { s.Player.Skills = map[int]int{skillTKPower: 5} }, nil, forcedMin{}, hit(100)},
		// Poison against Undead 1 is -25%, and the damage is raised to 0.
		{"negative element", func(s *Status) { s.RHW.Ele = Poison }, func(d *Status) { d.DefEle = Undead }, forcedMin{},
			Result{Kind: Miss, Hits: 1, Flag: short}},
	}
	for _, tt := range tests {
		src, dst := base(), Status{Size: SizeMedium, EleLv: 1}
		if tt.src != nil {
			tt.src(&src)
		}
		if tt.dst != nil {
			tt.dst(&dst)
		}
		if got := AttackPreRenewal(&src, &dst, tt.r); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}

	// A monster's normal attack of 100 on a player with DEF 10.
	mob := func() Status {
		return Status{Hit: 300, RHW: Weapon{Atk: 100, Atk2: 100, Range: 1}, Race: RaceDemiHuman, Size: SizeMedium}
	}
	taken := []struct {
		name string
		src  func(*Status)
		dst  func(*Status)
		want Result
	}{
		// 90 minus soft DEF 9 + rnd(0, 0) + 15.
		{"soft DEF", nil, nil, hit(66)},
		// 90 minus 30 + rnd(0, 35) + 50.
		{"high soft DEF", nil, func(d *Status) { d.Def2 = 100 }, hit(10)},
		{"cards", nil, func(d *Status) { d.Player.SubRace = map[int]int{RaceDemiHuman: 20} }, hit(53)},
		{"critical", func(s *Status) { s.Cri = 1000 }, func(d *Status) { d.Player.CritDefRate = 50 }, crit(50)},
	}
	for _, tt := range taken {
		src, dst := mob(), Status{Level: 50, MaxHP: 200, Def: 10, Def2: 30, Size: SizeMedium, EleLv: 1, Player: &Player{}}
		if tt.src != nil {
			tt.src(&src)
		}
		if tt.dst != nil {
			tt.dst(&dst)
		}
		if got := AttackPreRenewal(&src, &dst, forcedMin{}); got != tt.want {
			t.Errorf("taken, %s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// TestPreRenewalAttackTable checks hand-made monster cases.
func TestPreRenewalAttackTable(t *testing.T) {
	const short = bfWeapon | bfNormal | bfShort
	base := Status{Hit: 300, RHW: Weapon{Atk: 100, Atk2: 100, Range: 1}, BAtk: 50}
	tests := []struct {
		name string
		src  func(*Status)
		dst  Status
		r    Rand
		want Result
	}{
		{"plain hit", nil, Status{}, forcedMin{}, Result{Kind: Hit, Damage: 150, Hits: 1, Flag: short}},
		// 150 * 50%, minus 40 + rnd() % 4. The roll of 99 is capped to 3.
		{"DEF", nil, Status{Def: 50, Def2: 40}, fixedRand(99), Result{Kind: Hit, Damage: 32, Hits: 1, Flag: short}},
		// No 40% bonus in pre-renewal, and DEF does not apply.
		{"critical", func(s *Status) { s.Cri = 1000 }, Status{Def: 50, Def2: 40}, forcedMin{}, Result{Kind: Critical, Damage: 150, Hits: 1, Flag: short}},
		// The base hit rate of 80 makes this hit: 80 + 300 - 370 = 10.
		{"hit rate", nil, Status{Flee: 370}, fixedRand(9), Result{Kind: Hit, Damage: 150, Hits: 1, Flag: short}},
		{"miss", nil, Status{Flee: 370}, fixedRand(10), Result{Kind: Miss, Hits: 1, Flag: short}},
		// Monsters ignore the element table on normal attacks.
		{"element", func(s *Status) { s.RHW.Ele = Fire }, Status{DefEle: Fire, EleLv: 4}, forcedMin{}, Result{Kind: Hit, Damage: 150, Hits: 1, Flag: short}},
	}
	for _, tt := range tests {
		src := base
		if tt.src != nil {
			tt.src(&src)
		}
		if got := AttackPreRenewal(&src, &tt.dst, tt.r); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// FuzzAttackPreRenewal checks that AttackPreRenewal never panics and never
// returns negative damage, for players and monsters on both sides.
func FuzzAttackPreRenewal(f *testing.F) {
	f.Add(uint64(1), uint8(WeaponDagger), uint8(WeaponFist), int16(100), int16(10), uint16(120), uint8(4), int16(50), int16(0), uint8(10), false, true, false)
	f.Add(uint64(2), uint8(WeaponKatar), uint8(WeaponKatar), int16(-128), int16(-50), uint16(65535), uint8(0), int16(-100), int16(1000), uint8(0), true, true, true)
	f.Add(uint64(3), uint8(WeaponFist), uint8(WeaponDagger), int16(32767), int16(32767), uint16(0), uint8(255), int16(32767), int16(-32768), uint8(255), false, false, true)
	f.Fuzz(func(t *testing.T, seed uint64, right, left uint8, def, def2 int16, atk uint16, wlv uint8, stat, rate int16, lv uint8, arrow, player, playerTarget bool) {
		// rAthena holds base ATK and weapon ATK in a uint16, so they are
		// never negative.
		src := &Status{
			Level: 99, Hit: int(def2), Cri: int(rate), Str: int(stat), Dex: int(stat), BAtk: max(int(stat), 0),
			RHW: Weapon{Atk: int(atk), Atk2: int(atk) / 2, Wlv: int(wlv), Ele: int(lv) % elementCount},
			LHW: Weapon{Atk: int(atk) % 2, Atk2: max(int(stat), 0), Wlv: int(wlv)},
		}
		if player {
			src.Player = &Player{
				Weapon: int(right), RightWeapon: int(right), LeftWeapon: int(left),
				Skills: map[int]int{
					skillDoubleAttack: int(lv), skillChainAction: int(lv), skillKatarMastery: int(lv),
					skillAdvKatarMastery: int(lv), skillWeaponResearch: int(lv), skillHiltBinding: int(lv),
				},
				ArrowAttack: arrow, AmmoAtk: int(stat), AmmoEle: int(lv) % elementCount,
				Right:         Hand{AddRace: map[int]int{RaceAll: int(rate)}, DefRatioRace: uint32(lv)},
				IgnoreDefRace: map[int]int{RaceAll: int(rate)},
				CritAtkRate:   int(rate), ShortAtkRate: int(rate), LongAtkRate: int(rate),
				DualWield: DualWield(lv % 3),
			}
		}
		dst := &Status{
			Level: 99, MaxHP: int(atk), Def: int(def), Def2: int(def2), Flee: int(def), Size: int(wlv) % 4,
			Race: int(lv) % 13, DefEle: int(lv) % 11, EleLv: int(wlv)%4 + 1,
		}
		if playerTarget {
			dst.Player = &Player{
				Skills:  map[int]int{skillDivineProtection: int(lv)},
				SubRace: map[int]int{RaceAll: int(rate)}, CritDefRate: int(rate), DamageTakenCap: int(lv),
			}
		}
		r := rand.New(rand.NewPCG(seed, seed)) // #nosec G404 -- test input, not security.
		for range 20 {
			if res := AttackPreRenewal(src, dst, r); res.Damage < 0 || res.Left < 0 {
				t.Fatalf("negative damage: %+v", res)
			}
		}
		if res := AttackPreRenewal(src, dst, forcedMin{}); res.Damage < 0 || res.Left < 0 {
			t.Fatalf("negative damage with forced minimum: %+v", res)
		}
	})
}

// TestPreRenewalMobDamageForcedMin checks every pre-renewal monster pair
// against rAthena's result with rnd_force_min.
func TestPreRenewalMobDamageForcedMin(t *testing.T) {
	checkMobDamageForcedMin(t, reference.PreRenewal, AttackPreRenewal)
}

// TestPreRenewalMobDamageRolls runs the checks of TestMobDamageRolls on
// the pre-renewal monster pairs.
func TestPreRenewalMobDamageRolls(t *testing.T) {
	testrace.SkipUnderRace(t, "it compares recorded reference cases on one goroutine")
	checkMobDamageRolls(t, reference.PreRenewal, AttackPreRenewal)
}

// TestPreRenewalPCDamageForcedMin checks every pre-renewal player case
// against rAthena's result with rnd_force_min.
func TestPreRenewalPCDamageForcedMin(t *testing.T) {
	checkPCDamageForcedMin(t, loadModePCFixture(t, reference.PreRenewal))
}

// TestPreRenewalPCDamageRolls runs the checks of TestPCDamageRolls on the
// pre-renewal player cases.
func TestPreRenewalPCDamageRolls(t *testing.T) {
	testrace.SkipUnderRace(t, "it compares recorded reference cases on one goroutine per job")
	checkPCDamageRolls(t, loadModePCFixture(t, reference.PreRenewal))
}
