package zone

import (
	"context"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// commits records what a Committer gets.
type commits struct {
	got []commitCall
}

type commitCall struct {
	saves []CommitSave
	logs  transfer.Logs
}

func (c *commits) Commit(saves []CommitSave, logs transfer.Logs) {
	c.got = append(c.got, commitCall{saves, logs})
}

// TestTransferCommits checks that a drop and a pick-up by another player
// each give one commit, with the characters they changed and their log
// lines, in order, and that a later save comes after the commit.
func TestTransferCommits(t *testing.T) {
	z := saveZone()
	cm := &commits{}
	z.Committer = cm
	a := savedCharacter()
	b := savedCharacter()
	b.ID, b.Name, b.Items = 8, "Other", nil
	sa, sb := &saves{}, &saves{}
	pa, pb := addCharacter(z, a, sa), addCharacter(z, b, sb)
	pb.maxWeight = 1 << 20
	stack := pa.inventory.Slots[0].Serial

	z.dropItem(pa, 0, 2)
	z.commitTransfers()
	if len(cm.got) != 1 || len(cm.got[0].saves) != 1 || cm.got[0].saves[0].Saver != Saver(sa) {
		t.Fatalf("after the drop: %+v", cm.got)
	}
	drop := cm.got[0].logs.Items
	if len(drop) != 1 || drop[0].Action != transfer.ActionDrop || drop[0].From != transfer.Inventory(7) ||
		drop[0].To != transfer.End(transfer.KindGround) || drop[0].Serial != stack || drop[0].Amount != 2 {
		t.Errorf("drop log %+v", drop)
	}
	if got := cm.got[0].saves[0].Character; got.Items[0].Amount != 1 || pa.saved != got {
		t.Errorf("the commit's copy %+v", got.Items)
	}

	var ground *entity
	for e := range z.entities.all() {
		if e.item != nil {
			ground = e
		}
	}
	if ground == nil || ground.item.origin != stack {
		t.Fatalf("ground item %+v", ground)
	}
	z.take(pb, ground, 0)
	z.commitTransfers()
	if len(cm.got) != 2 || cm.got[1].saves[0].Saver != Saver(sb) {
		t.Fatalf("after the pick-up: %+v", cm.got)
	}
	pick := cm.got[1].logs.Items
	if len(pick) != 1 || pick[0].Action != transfer.ActionPickUp || pick[0].Source != stack ||
		pick[0].Serial != pb.inventory.Slots[0].Serial || pick[0].Serial == stack || pick[0].To != transfer.Inventory(8) {
		t.Errorf("pick-up log %+v", pick)
	}
	// Nothing new: no commit.
	z.commitTransfers()
	if len(cm.got) != 2 {
		t.Errorf("%d commits", len(cm.got))
	}

	// A transfer just before a save is committed first.
	z.earnZeny(pb, 50, transfer.ActionGive, transfer.End(transfer.KindScript))
	z.save(pb, true)
	if len(cm.got) != 3 || len(sb.got) != 1 || cm.got[2].logs.Zeny[0].Amount != 50 {
		t.Errorf("%d commits and %d saves", len(cm.got), len(sb.got))
	}
}

// TestTransfersWithoutCommitter keeps saving as before: no save at a
// transfer, and the lines go to the log.
func TestTransfersWithoutCommitter(t *testing.T) {
	z := saveZone()
	s := &saves{}
	p := addCharacter(z, savedCharacter(), s)
	z.dropItem(p, 0, 1)
	z.commitTransfers()
	if len(s.got) != 0 || len(z.txPlayers) != 0 || !z.txLogs.Empty() {
		t.Errorf("%d saves, %d players and %v waiting", len(s.got), len(z.txPlayers), z.txLogs)
	}
}

// TestShopTransferIsAtomic checks that a purchase that does not fit
// changes nothing, and one that fits logs its zeny and items together.
func TestShopTransferIsAtomic(t *testing.T) {
	z := saveZone()
	p := addCharacter(z, savedCharacter(), &saves{})
	box := z.box(p)
	tx := transfer.New(transfer.ActionBuy).Pay(z.wallet(p), 100, transfer.End(transfer.KindShop)).
		Create(transfer.End(transfer.KindShop), item.Slot{ID: 909}, 2, box, item.Serial{}).
		Create(transfer.End(transfer.KindShop), item.Slot{ID: 1201}, 1, box, item.Serial{})
	p.maxWeight = p.inventory.Weight + 30
	if err := z.apply(p, tx, 0); err == nil {
		t.Fatal("a purchase over the weight limit passed")
	}
	if p.record.Zeny != 500 || p.inventory.Count(909) != 0 || !z.txLogs.Empty() {
		t.Errorf("the failed purchase changed zeny %d, items %d, logs %v", p.record.Zeny, p.inventory.Count(909), z.txLogs)
	}
	p.maxWeight = 1 << 20
	tx = transfer.New(transfer.ActionBuy).Pay(z.wallet(p), 100, transfer.End(transfer.KindShop)).
		Create(transfer.End(transfer.KindShop), item.Slot{ID: 909}, 2, z.box(p), item.Serial{})
	if err := z.apply(p, tx, 0); err != nil {
		t.Fatal(err)
	}
	if p.record.Zeny != 400 || len(z.txLogs.Items) != 1 || len(z.txLogs.Zeny) != 1 {
		t.Errorf("zeny %d, logs %+v", p.record.Zeny, z.txLogs)
	}
}

// TestAudit lists what players and the ground hold, on the zone's
// goroutine.
func TestAudit(t *testing.T) {
	z := saveZone()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go z.Run(ctx)
	c := savedCharacter()
	sess, err := z.JoinCharacter(ctx, c, &saves{})
	if err != nil {
		t.Fatal(err)
	}
	_ = sess
	deadline, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	a, err := z.Audit(deadline)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Players) != 1 || a.Players[0].Zeny != 500 || len(a.Players[0].Items) != 2 || a.Players[0].Owner != transfer.Inventory(7) {
		t.Errorf("audit %+v", a)
	}
	want := []character.Item{c.Items[0], c.Items[1]}
	for i, it := range a.Players[0].Items {
		if it.Serial != want[i].Serial || it.Amount != want[i].Amount {
			t.Errorf("item %d: %+v, want %+v", i, it, want[i])
		}
	}
}
