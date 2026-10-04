package zone

import (
	"log/slog"
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// warehouseZone is prontera with core's converted Guild Warehouse
// Manager (npc/re/merchants/guild_warehouse.txt, a duplicate of the
// floating GuildWarehouse), and a player next to it with 5000 zeny.
func warehouseZone(t *testing.T) (*Zone, *Player, *entity, *syncBuffer) {
	t.Helper()
	z := New("map/prontera", open(400), rng.FromKey([32]byte{9}).Stream(), nil)
	z.Items = testItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	coreMods(t, z)
	logs := &syncBuffer{}
	z.Mods.Log = slog.New(slog.NewTextHandler(logs, nil))
	z.Mods.Server = variables.NewServer(nil, nil)
	core, err := testCore()
	if err != nil {
		t.Fatal(err)
	}
	var npc NPC
	for _, n := range core.NPCs.NPCs {
		if n.Placed && n.DuplicateOf == "GuildWarehouse" {
			npc = NPC{Name: "core:" + n.Name, DisplayName: "Guild Warehouse Manager", X: 152, Y: 326, Sprite: 896, Script: "core:" + n.Script}
			break
		}
	}
	if npc.Script == "" || npc.Script == "core:" {
		t.Fatal("the Guild Warehouse Manager did not convert")
	}
	z.SetNPCs([]NPC{npc})
	t.Cleanup(z.stopScripts)
	c := savedCharacter()
	c.Map, c.X, c.Y, c.Zeny = "map/prontera", 153, 326, 5000
	p := addCharacter(z, c, &saves{})
	z.stepAt(50)
	events(p)
	var n *entity
	for e := range z.entities.all() {
		if e.npc != nil {
			n = e
		}
	}
	return z, p, n, logs
}

// openWarehouse talks to the manager and chooses to open the guild
// warehouse.
func openWarehouse(t *testing.T, z *Zone, p *Player, n *entity, logs *syncBuffer) []*wathenav1.Event {
	t.Helper()
	ev := click(z, p, n)
	if !strings.HasPrefix(lines(ev), "[Warehouse Manager]|How are you?") {
		t.Fatalf("lines %q, logs %s", lines(ev), logs)
	}
	ev = next(z, p, next(z, p, ev))
	return choose(z, p, ev, "Open guild warehouse")
}

func storageOpened(ev []*wathenav1.Event) *wathenav1.StorageOpen {
	for _, e := range ev {
		if s := e.GetStorageOpen(); s != nil {
			return s
		}
	}
	return nil
}

// learnGuildStorage raises the guild skills that Guild Storage needs in
// renewal, then Guild Storage, as the master does.
func learnGuildStorage(t *testing.T, reg *social.Registry, master *Player, id int64) {
	t.Helper()
	// Guild levels 2 to 21 give the 20 skill points.
	if _, err := reg.GiveExp(id, 300_000_000); err != nil {
		t.Fatal(err)
	}
	ups := []int{social.SkillApproval, social.SkillGuardResearch}
	for range 10 {
		ups = append(ups, social.SkillExtension)
	}
	ups = append(ups, social.SkillBattleOrder, social.SkillRegeneration, social.SkillEmergencyCall,
		social.SkillGloryWounds, social.SkillSoulCold, social.SkillLeadership, social.SkillHawkEyes, social.SkillGuildStorage)
	for _, s := range ups {
		if o := reg.GuildSkillUp(master.soc, s); o != social.GuildSkillRaised {
			t.Fatalf("skill %d: %v", s, o)
		}
	}
	if lv := reg.GuildSkillLevel(id, social.SkillGuildStorage); lv != 1 {
		t.Fatalf("Guild Storage level %d", lv)
	}
}

func TestGuildWarehouseOpensStorage(t *testing.T) {
	z, p, n, logs := warehouseZone(t)
	reg := z.socialHub()
	id, o := reg.GuildCreate(p.soc, "Wardens")
	if o != social.GuildCreated {
		t.Fatalf("create: %v", o)
	}
	learnGuildStorage(t, reg, p, id)
	events(p)
	ev := openWarehouse(t, z, p, n, logs)
	if got := lines(ev); !strings.Contains(got, "I will open the guild storage for you then.") {
		t.Fatalf("lines %q, logs %s", got, logs)
	}
	d := lastDialog(ev)
	if d == nil || !d.HasClose() {
		t.Fatalf("no Close button: %v", d)
	}
	ev = answer(z, p, reply(d.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Close = new(true) }))
	s := storageOpened(ev)
	if s == nil || s.GetKind() != wathenav1.StorageKind_STORAGE_KIND_GUILD {
		t.Fatalf("no guild storage opened: %v, logs %s", ev, logs)
	}
	if p.record.Zeny != 4000 {
		t.Errorf("zeny %d, want 4000", p.record.Zeny)
	}
}

func TestGuildWarehouseNeedsTheSkill(t *testing.T) {
	z, p, n, logs := warehouseZone(t)
	if _, o := z.socialHub().GuildCreate(p.soc, "Wardens"); o != social.GuildCreated {
		t.Fatalf("create: %v", o)
	}
	events(p)
	ev := openWarehouse(t, z, p, n, logs)
	if got := lines(ev); !strings.Contains(got, "It seems that it is not yet possible for your guild to use the guild warehouse") {
		t.Fatalf("lines %q, logs %s", got, logs)
	}
	if storageOpened(ev) != nil || p.record.Zeny != 5000 {
		t.Errorf("storage opened or zeny taken: zeny %d", p.record.Zeny)
	}
}

func TestGuildWarehouseRefusesWithoutGuild(t *testing.T) {
	z, p, n, logs := warehouseZone(t)
	ev := openWarehouse(t, z, p, n, logs)
	if got := lines(ev); got != "[Warehouse Manager]|The guild storage is only available for guild members." {
		t.Fatalf("lines %q, logs %s", got, logs)
	}
	if storageOpened(ev) != nil || p.record.Zeny != 5000 {
		t.Errorf("storage opened or zeny taken: zeny %d", p.record.Zeny)
	}
}
