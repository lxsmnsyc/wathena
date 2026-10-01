package zone

import (
	"context"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// saves records what a player's saver gets.
type saves struct {
	got   []*character.Character
	final []bool
}

func (s *saves) Save(c *character.Character, final bool) {
	s.got = append(s.got, c)
	s.final = append(s.final, final)
}

// addCharacter joins a loaded character directly, as Run would.
func addCharacter(z *Zone, c *character.Character, s Saver) *Player {
	sess := newSession(s)
	p := newPlayer(sess, c)
	sess.zone, sess.player = z, p
	z.join(p)
	return p
}

func saveZone() *Zone {
	z := New("map/test", open(40), rng.FromKey([32]byte{8}).Stream(), nil)
	z.Items = testItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	return z
}

func savedCharacter() *character.Character {
	return &character.Character{
		Info: character.Info{
			ID: 7, AccountID: 3, Slot: 1, Name: "Saved", Job: 0, Male: true,
			Hair: 4, HairColor: 2, ClothColor: 1,
			BaseLevel: 3, JobLevel: 2, BaseExp: 15, JobExp: 4, StatusPoints: 9, SkillPoints: 1,
			Stats: status.Stats{Str: 5, Agi: 4, Vit: 3, Int: 2, Dex: 6, Luk: 7},
			HP:    120, SP: 30, Map: "map/test", X: 11, Y: 12, Zeny: 500, InventorySize: 110,
		},
		Items: []character.Item{
			{Slot: 0, ID: 501, Amount: 3, Serial: item.NewSerial()},
			{Slot: 5, ID: 1201, Amount: 1, Serial: item.NewSerial()},
		},
	}
}

func TestLoadedCharacterEntersAsSaved(t *testing.T) {
	z := saveZone()
	c := savedCharacter()
	p := addCharacter(z, c, &saves{})
	e := p.entity
	if e.x != 11 || e.y != 12 || e.hp != 120 || p.sp != 30 {
		t.Errorf("at %d,%d with %d HP, %d SP", e.x, e.y, e.hp, p.sp)
	}
	if p.progress.BaseLevel != 3 || p.progress.JobExp != 4 || p.progress.StatusPoints != 9 || p.char.Stats != c.Stats {
		t.Errorf("progress %+v, stats %+v", p.progress, p.char.Stats)
	}
	if p.Look != (Look{Job: 0, Sex: 1, Hair: 4, HairColor: 2}) {
		t.Errorf("look %+v", p.Look)
	}
	inv := p.inventory
	if inv.Size != 110 || inv.Slots[0].Amount != 3 || inv.Slots[5].ID != 1201 || inv.Slots[5].Serial != c.Items[1].Serial {
		t.Errorf("inventory %+v", inv.Slots[:6])
	}
	if want := 3*z.Items.Get(501).Weight + z.Items.Get(1201).Weight; inv.Weight != want {
		t.Errorf("weight %d, want %d", inv.Weight, want)
	}
	// The snapshot of an unchanged player is the loaded character.
	if got := z.snapshot(p); !got.Equal(c) {
		t.Errorf("snapshot %+v\nwant %+v", got, c)
	}
}

func TestDeadOrMovedCharacterStartsAtStart(t *testing.T) {
	z := saveZone()
	x, y := z.startCell()
	other := savedCharacter()
	other.Name, other.Map = "Other", "map/elsewhere"
	p := addCharacter(z, other, &saves{})
	if p.entity.x != x || p.entity.y != y {
		t.Errorf("%s at %d,%d, want the start %d,%d", other.Name, p.entity.x, p.entity.y, x, y)
	}
	// A character that logged out dead was saved at its save point
	// (pc_makesavestatus), and comes back there with full HP.
	dead := savedCharacter()
	dead.HP = 0
	p = addCharacter(z, dead, &saves{})
	if p.entity.x != dead.X || p.entity.y != dead.Y || p.entity.hp != 200 {
		t.Errorf("a character that logged out dead is at %d,%d with %d HP, want %d,%d with full HP",
			p.entity.x, p.entity.y, p.entity.hp, dead.X, dead.Y)
	}
}

func TestAutosaveOnlyWhenChanged(t *testing.T) {
	z := saveZone()
	s := &saves{}
	// Full HP and SP, so natural regen changes nothing.
	c := savedCharacter()
	c.HP, c.SP = 200, 50
	p := addCharacter(z, c, s)
	z.stepAt(50)
	// Nothing is saved before the interval, and an unchanged character
	// is not saved at all.
	z.stepAt(DefaultSaveInterval.Milliseconds() + 50)
	if len(s.got) != 0 {
		t.Fatalf("%d saves of an unchanged character", len(s.got))
	}
	// Without job tables, the maximum weight is set by hand.
	p.maxWeight = maxWeight
	if r := z.addItem(p, 909, 4); r != item.AddSuccess {
		t.Fatalf("add: %v", r)
	}
	z.stepAt(DefaultSaveInterval.Milliseconds() + 100)
	if len(s.got) != 0 {
		t.Fatal("saved before the interval passed")
	}
	z.stepAt(2*DefaultSaveInterval.Milliseconds() + 50)
	if len(s.got) != 1 || s.final[0] {
		t.Fatalf("saves %v, want one autosave", s.final)
	}
	got := s.got[0]
	if n := len(got.Items); n != 3 || got.Items[1].ID != 909 || got.Items[1].Amount != 4 || got.Items[1].Serial.IsZero() {
		t.Errorf("saved items %+v", got.Items)
	}
	z.stepAt(3*DefaultSaveInterval.Milliseconds() + 50)
	if len(s.got) != 1 {
		t.Errorf("%d saves, want no new save without changes", len(s.got))
	}
	// Leaving always saves, as the session's last save.
	z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	if len(s.got) != 2 || !s.final[1] {
		t.Fatalf("saves %v, want a final save at logout", s.final)
	}
	select {
	case <-p.sess.left:
	default:
		t.Error("left is still open after the last save")
	}
}

func TestSaveIntervalSetting(t *testing.T) {
	z := saveZone()
	z.SaveInterval = 1000
	s := &saves{}
	p := addCharacter(z, savedCharacter(), s)
	p.entity.hp = 50
	z.stepAt(1000)
	if len(s.got) != 1 || s.got[0].HP != 50 {
		t.Fatalf("saves %d, want one with the new HP", len(s.got))
	}
}

func TestShutdownSavesEveryone(t *testing.T) {
	z := saveZone()
	a, b := &saves{}, &saves{}
	addCharacter(z, savedCharacter(), a)
	c := savedCharacter()
	c.ID, c.Name = 8, "Other"
	addCharacter(z, c, b)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	z.Run(ctx)
	if len(a.got) != 1 || !a.final[0] || len(b.got) != 1 || !b.final[0] {
		t.Errorf("final saves %v and %v", a.final, b.final)
	}
	select {
	case <-z.Stopped():
	default:
		t.Error("Stopped is open after Run returned")
	}
}

func TestNewCharacterHasSerials(t *testing.T) {
	z := saveZone()
	z.StartItems = []StartItem{{501, 2}, {1201, 1}, {1201, 1}}
	c := z.NewCharacter("New", Look{Sex: 1, Hair: 3})
	if len(c.Items) != 3 || !c.Male || c.Hair != 3 || c.BaseLevel != 1 || c.HP != 200 || c.SP != 50 || c.Map != "map/test" {
		t.Fatalf("new character %+v", c)
	}
	seen := map[item.Serial]bool{}
	for _, it := range c.Items {
		if it.Serial.IsZero() || seen[it.Serial] {
			t.Errorf("item %+v has a missing or repeated serial", it)
		}
		seen[it.Serial] = true
	}
}

// staleSaver is a saver whose session another one took over. It records
// the function the zone gives OnStale.
type staleSaver struct {
	saves
	onStale chan func()
}

func (s *staleSaver) OnStale(f func()) { s.onStale <- f }

func TestStaleSaveDisconnects(t *testing.T) {
	z := saveZone()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go z.Run(ctx)
	s := &staleSaver{onStale: make(chan func(), 1)}
	p, err := z.JoinCharacter(ctx, savedCharacter(), s)
	if err != nil {
		t.Fatal(err)
	}
	// The store calls the function on its own goroutine when a save is
	// rejected as stale.
	go (<-s.onStale)()
	var reason wathenav1.DisconnectReason
	timeout := time.After(5 * time.Second)
	for done := false; !done; {
		select {
		case m, ok := <-p.Out:
			if !ok {
				done = true
				break
			}
			if m.HasDisconnect() {
				reason = m.GetDisconnect().GetReason()
			}
		case <-timeout:
			t.Fatal("the player was not dropped")
		}
	}
	if reason != wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_IN_ELSEWHERE {
		t.Errorf("disconnect reason %v", reason)
	}
	// The player left the zone. Leave returns at once.
	p.Leave()
}
