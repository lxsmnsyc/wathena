package zone

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
)

// guildSkillRef is the skill part of guild.json (rAthena fork commit
// f30328d04): guild skills cast by a master among players of its guild,
// another guild and an allied guild.
type guildSkillRef struct {
	SkillCases []guildSkillCase `json:"skill_cases"`
}

// guildSkillCase is one cast of guild.json.
type guildSkillCase struct {
	Name          string `json:"name"`
	Skill         int    `json:"skill"`
	Level         int    `json:"level"`
	GvG           bool   `json:"gvg"`
	EmergencyCall int    `json:"emergency_call"`
	Caster        int    `json:"caster"`
	Cast          struct {
		CheckFails []struct{ Skill int } `json:"check_fails"`
		Fails      []struct{ Skill int } `json:"fails"`
	} `json:"cast"`
	Blocks map[string]int64 `json:"blocks"`
	Second *struct {
		Skill int `json:"skill"`
		Used  int `json:"used"`
	} `json:"second"`
	BlocksAfterSecond map[string]int64 `json:"blocks_after_second"`
	Players           []struct {
		Role     string `json:"role"`
		Guild    int    `json:"guild"`
		Map      string `json:"map"`
		DX       *int   `json:"dx"`
		DY       *int   `json:"dy"`
		Dead     bool   `json:"dead"`
		NoCall   bool   `json:"disable_call"`
		HP       int64  `json:"hp"`
		HPAfter  int64  `json:"hp_after"`
		SP       int64  `json:"sp"`
		SPAfter  int64  `json:"sp_after"`
		MapAfter string `json:"map_after"`
		DXAfter  *int   `json:"dx_after"`
		DYAfter  *int   `json:"dy_after"`
		Stats    struct {
			Str, Int, Dex int
		} `json:"stats"`
		StatsAfter struct {
			Str, Int, Dex int
		} `json:"stats_after"`
		Statuses []struct {
			Type  int   `json:"type"`
			Val1  int   `json:"val1"`
			Val2  int   `json:"val2"`
			Val3  int   `json:"val3"`
			Timer int64 `json:"timer"`
		} `json:"statuses"`
	} `json:"players"`
}

// TestGuildSkillsMatchReference replays every recorded guild skill cast:
// who gets which status, Restore's heal, the stats Battle Orders adds,
// where Emergency Call puts each member, the failures, and the skill
// blocks. rAthena's disable_call, a player setting wAthena does not have
// yet, is the one difference: that player is left out of the compare.
func TestGuildSkillsMatchReference(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			var ref guildSkillRef
			if err := reference.Load(mode, "guild", &ref); err != nil {
				t.Fatal(err)
			}
			passed, players := 0, 0
			for _, c := range ref.SkillCases {
				n, ok := replayGuildSkill(t, mode, c)
				players += n
				if ok {
					passed++
				}
			}
			t.Logf("%s: %d/%d skill cases match, %d player results compared", mode, passed, len(ref.SkillCases), players)
		})
	}
}

func replayGuildSkill(t *testing.T, mode reference.Mode, c guildSkillCase) (int, bool) {
	t.Helper()
	renewal := mode == reference.Renewal
	z, _, _ := statusZone(t)
	z.Skills, z.SkillTrees = testSkills(t, mode)
	g, err := rules.GuildOf(map[string]int{"emergency_call": c.EmergencyCall})
	if err != nil {
		t.Fatal(err)
	}
	world := *rules.Of(renewal)
	world.Guild = g
	z.Mods.Rules = &world
	z.GvG = c.GvG
	reg := z.socialHub()
	reg.SetGuildData(social.CoreGuildTables(renewal), g)
	const cx, cy = 12, 12
	ps := make([]*Player, len(c.Players))
	var away *social.Player
	awayBox := &recordInbox{}
	for i, rp := range c.Players {
		if rp.Map != "same" {
			away = reg.Online(social.Login{Name: rp.Role, CharID: int64(100 + i), Inbox: awayBox, Map: "map/other"})
			continue
		}
		ch := savedCharacter()
		ch.ID, ch.Name, ch.Map, ch.X, ch.Y = int64(100+i), rp.Role, z.Map, cx+*rp.DX, cy+*rp.DY
		ch.Items = []character.Item{}
		ps[i] = addCharacter(z, ch, &saves{})
	}
	z.stepAt(100)
	socOf := func(i int) *social.Player {
		if ps[i] == nil {
			return away
		}
		return ps[i].soc
	}
	// Guild 1 in the order of the players, guild 2, and guild 3 allied
	// with guild 1.
	masters := map[int]int{}
	for i, rp := range c.Players {
		if _, ok := masters[rp.Guild]; !ok {
			masters[rp.Guild] = i
			if id, o := reg.GuildCreate(socOf(i), "Guild "+strconv.Itoa(rp.Guild)); id == 0 {
				t.Fatalf("%s: guild %d: %v", c.Name, rp.Guild, o)
			}
			continue
		}
		g1 := reg.GuildIDOf(socOf(masters[rp.Guild]))
		if ok, err := reg.GuildAddMember(g1, socOf(i)); !ok || err != nil {
			t.Fatalf("%s: %s did not join: %v", c.Name, rp.Role, err)
		}
	}
	m1, m3 := socOf(masters[1]), socOf(masters[3])
	reg.GuildAlly(m1, c.Players[masters[3]].Role)
	reg.GuildAllyReply(m3, reg.GuildIDOf(m1), true)
	id1 := reg.GuildIDOf(m1)
	if _, err := reg.GiveExp(id1, social.MaxGuildExp); err != nil {
		t.Fatal(err)
	}
	regen := 3
	if c.Skill == social.SkillRegeneration {
		regen = c.Level
	}
	ups := []int{social.SkillApproval, social.SkillGuardResearch}
	for range 5 {
		ups = append(ups, social.SkillExtension)
	}
	ups = append(ups, social.SkillBattleOrder)
	for range regen {
		ups = append(ups, social.SkillRegeneration)
	}
	ups = append(ups, social.SkillRestore, social.SkillEmergencyCall)
	for _, s := range ups {
		if o := reg.GuildSkillUp(m1, s); o != social.GuildSkillRaised {
			t.Fatalf("%s: skill %d: %v", c.Name, s, o)
		}
	}
	z.drainSocial()
	for _, id := range []int64{id1, reg.GuildIDOf(socOf(masters[2])), reg.GuildIDOf(m3)} {
		refreshGuild(z, id)
	}
	for i, rp := range c.Players {
		p := ps[i]
		if p == nil {
			continue
		}
		e := p.entity
		e.hp, p.sp = int64(e.status.MaxHP)/4, int64(e.status.MaxSP)/4
		if rp.Dead {
			// Dead the whole case: the zone revives at the save point
			// after 3 seconds, which rAthena's recorder does not.
			e.dead, e.hp, e.diedAt = true, 0, 1<<40
		}
		p.cooldowns = nil
		events(p)
	}
	caster := ps[c.Caster]
	before := map[int][3]int{}
	for i, p := range ps {
		if p != nil {
			st := p.entity.status
			before[i] = [3]int{st.Str, st.Int, st.Dex}
		}
	}
	hpBefore := map[int][2]int64{}
	for i, p := range ps {
		if p != nil {
			hpBefore[i] = [2]int64{p.entity.hp, p.sp}
		}
	}
	// The cast runs until it ends, so natural regen adds little.
	cast(z, caster, c.Skill, c.Level, caster.entity)
	var ev []*wathenav1.Event
	now := int64(150)
	for ; now <= 12000; now += 50 {
		z.stepAt(now)
		ev = append(ev, events(caster)...)
		if caster.entity.cast == nil {
			break
		}
	}
	z.drainSocial()
	z.stepAt(now + 50)
	ev = append(ev, events(caster)...)
	ok := true
	fail := func(format string, a ...any) {
		ok = false
		t.Errorf("%s %s: %s", mode, c.Name, fmt.Sprintf(format, a...))
	}
	failed := len(guildFailures(ev)) > 0
	wantFailed := len(c.Cast.Fails) > 0 || len(c.Cast.CheckFails) > 0
	if failed != wantFailed {
		fail("failed %v, want %v", failed, wantFailed)
	}
	// A second cast comes before the results, which rAthena records after
	// both.
	if c.Second != nil {
		caster.entity.canAct = 0
		cast(z, caster, c.Second.Skill, 1, caster.entity)
		var ev2 []*wathenav1.Event
		for now += 100; now <= 30000; now += 50 {
			z.stepAt(now)
			ev2 = append(ev2, events(caster)...)
			if caster.entity.cast == nil {
				break
			}
		}
		z.drainSocial()
		z.stepAt(now + 50)
		refused := slices.ContainsFunc(guildFailures(ev2), func(f *wathenav1.SkillFailed) bool {
			return f.GetReason() == wathenav1.SkillFailure_SKILL_FAILURE_SKILLINTERVAL
		})
		if refused != (c.Second.Used == 0) {
			fail("second cast refused %v, want used %d", refused, c.Second.Used)
		}
	}
	compared := 0
	for i, rp := range c.Players {
		if rp.NoCall {
			continue
		}
		p := ps[i]
		if p == nil {
			// The member on the other map gets Emergency Call's notice.
			var call *social.Call
			for _, n := range awayBox.got {
				if n.Kind == social.NoticeEmergencyCall {
					call = &n.Call
				}
			}
			moved := rp.MapAfter == "same"
			if (call != nil) != moved {
				fail("%s: called %v, want %v", rp.Role, call != nil, moved)
			} else if moved && (call.Map != z.Map || call.X != caster.entity.x+*rp.DXAfter || call.Y != caster.entity.y+*rp.DYAfter) {
				fail("%s: called to %+v, want %d,%d from the caster", rp.Role, call, *rp.DXAfter, *rp.DYAfter)
			}
			compared++
			continue
		}
		e := p.entity
		// Statuses: Battle Orders and Regeneration, with their values.
		for _, typ := range []int{152, 153} {
			var want *struct {
				Type  int   `json:"type"`
				Val1  int   `json:"val1"`
				Val2  int   `json:"val2"`
				Val3  int   `json:"val3"`
				Timer int64 `json:"timer"`
			}
			for j := range rp.Statuses {
				if rp.Statuses[j].Type == typ {
					want = &rp.Statuses[j]
				}
			}
			got := e.sc.Get(typ)
			switch {
			case (got != nil) != (want != nil):
				fail("%s: status %d %v, want %v", rp.Role, typ, got != nil, want != nil)
			case got != nil && (got.Val1 != want.Val1 || got.Val2 != want.Val2 || got.Val3 != want.Val3):
				fail("%s: status %d values %d %d %d, want %d %d %d", rp.Role, typ, got.Val1, got.Val2, got.Val3, want.Val1, want.Val2, want.Val3)
			}
		}
		// Battle Orders' stats.
		st := e.status
		gotD := [3]int{st.Str - before[i][0], st.Int - before[i][1], st.Dex - before[i][2]}
		wantD := [3]int{rp.StatsAfter.Str - rp.Stats.Str, rp.StatsAfter.Int - rp.Stats.Int, rp.StatsAfter.Dex - rp.Stats.Dex}
		if gotD != wantD {
			fail("%s: stats change %v, want %v", rp.Role, gotD, wantD)
		}
		// Restore heals 90% of max HP and SP, capped.
		healed := rp.HPAfter > rp.HP
		gotHealed := e.hp-hpBefore[i][0] > int64(e.status.MaxHP)/2
		if healed != gotHealed {
			fail("%s: healed %v, want %v", rp.Role, gotHealed, healed)
		} else if healed {
			maxHP, maxSP := int64(st.MaxHP), int64(st.MaxSP)
			// Natural regen may add a little after the heal.
			if e.hp < min(maxHP, hpBefore[i][0]+maxHP*90/100) || p.sp < min(maxSP, hpBefore[i][1]+maxSP*90/100) {
				fail("%s: hp %d sp %d after Restore", rp.Role, e.hp, p.sp)
			}
		}
		// Emergency Call's cells.
		if c.Skill == social.SkillEmergencyCall && i != c.Caster {
			gx, gy := e.x-caster.entity.x, e.y-caster.entity.y
			if rp.DXAfter == nil || gx != *rp.DXAfter || gy != *rp.DYAfter {
				fail("%s: at %d,%d from the caster, want %v,%v", rp.Role, gx, gy, rp.DXAfter, rp.DYAfter)
			}
		}
		compared++
	}
	// The skill blocks: the cooldown events of the cast.
	got := map[string]int64{}
	for _, e := range ev {
		if cd := e.GetSkillCooldown(); cd != nil && cd.GetSkillId() != 0 {
			got[strconv.Itoa(int(cd.GetSkillId()))] = int64(cd.GetMs())
		}
	}
	if !sameBlocks(got, c.Blocks) {
		fail("blocks %v, want %v", got, c.Blocks)
	}
	return compared, ok
}

// sameBlocks compares skill blocks, allowing rAthena's real clock 3 ms.
func sameBlocks(got, want map[string]int64) bool {
	if len(got) != len(want) {
		return false
	}
	for k, w := range want {
		g, ok := got[k]
		if !ok || g > w || w-g > 3 {
			return false
		}
	}
	return true
}

// recordInbox keeps the notices of a player on another map.
type recordInbox struct{ got []social.Notice }

func (b *recordInbox) DeliverSocial(n social.Notice) { b.got = append(b.got, n) }

func guildFailures(ev []*wathenav1.Event) []*wathenav1.SkillFailed {
	return eventsOf2(ev, (*wathenav1.Event).GetSkillFailed, (*wathenav1.Event).HasSkillFailed)
}
