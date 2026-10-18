package zone

import (
	"testing"

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
