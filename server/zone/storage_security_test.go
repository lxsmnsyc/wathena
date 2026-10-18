package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/storage"
)

// A mod can reach only its own storage records. Mod example.b names the
// full name of a record of mod example.bank, with no dependency on it.
func TestStorageRecordOfOtherModIsHidden(t *testing.T) {
	z := &Zone{Mods: &Mods{Storages: []*storage.Record{{ID: 10000, Name: "example.bank:Vault", Slots: 300}}}}
	if rec := z.storageRecord("example.bank:Vault", "example.b"); rec != nil {
		t.Fatalf("mod example.b reached the storage record %q of mod example.bank", rec.Name)
	}
}

// A mod finds its own records with or without its prefix, and the
// server's own calls, with no mod, find every record.
func TestStorageRecordOwnAndServer(t *testing.T) {
	z := &Zone{Mods: &Mods{Storages: []*storage.Record{{ID: 10000, Name: "example.bank:Vault", Slots: 300}, {ID: 10001, Name: "example.b:Vault", Slots: 10}}}}
	for _, c := range []struct {
		name, mod string
		want      int
	}{
		{"Vault", "example.bank", 10000},
		{"example.bank:Vault", "example.bank", 10000},
		{"Vault", "example.b", 10001},
		{"example.bank:Vault", "", 10000},
		{"Vault", "example.c", 0},
	} {
		got := 0
		if rec := z.storageRecord(c.name, c.mod); rec != nil {
			got = int(rec.ID)
		}
		if got != c.want {
			t.Errorf("storageRecord(%q, %q) = %d, want %d", c.name, c.mod, got, c.want)
		}
	}
}

// TestStorageRefusesDeadPlayer checks that a dead player moves nothing
// in or out of its open storage.
func TestStorageRefusesDeadPlayer(t *testing.T) {
	z, a, _, _ := storageZone(t, stack(0, 909, 10))
	if !z.openPersonalStorage(a, nil) {
		t.Fatal("the storage did not open")
	}
	z.stepAt(z.now + 50)
	events(a)
	a.entity.dead = true
	ev := move(z, a, MovePut, 0, 4)
	if a.inventory.Count(909) != 10 {
		t.Errorf("a dead player put items in its storage: %d left", a.inventory.Count(909))
	}
	if got := storageOutcomes(ev); len(got) != 1 || got[0] != wathenav1.StorageOutcome_STORAGE_OUTCOME_BUSY {
		t.Errorf("outcomes %v, want busy", got)
	}
}

// TestWalkClosesStorage checks that a walk closes an open storage, as
// rAthena lets no player walk with its storage open (pc_cant_act).
func TestWalkClosesStorage(t *testing.T) {
	z, a, _, _ := storageZone(t, stack(0, 909, 10))
	if !z.openPersonalStorage(a, nil) {
		t.Fatal("the storage did not open")
	}
	z.stepAt(z.now + 50)
	events(a)
	z.handle(command{player: a, seq: 2, walkTo: &[2]int{a.entity.x + 5, a.entity.y}})
	if a.storage != nil {
		t.Fatal("the storage is still open after a walk")
	}
	z.stepAt(z.now + 50)
	closed := eventsOf2(events(a), (*wathenav1.Event).GetStorageClose, (*wathenav1.Event).HasStorageClose)
	if len(closed) != 1 {
		t.Errorf("the client was told %d closes, want 1", len(closed))
	}
	if ev := move(z, a, MovePut, 0, 1); a.inventory.Count(909) != 10 {
		t.Errorf("a put after the walk moved items: %v", ev)
	}
}
