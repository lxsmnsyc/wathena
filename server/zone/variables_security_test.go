package zone

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/modscript"
)

// One mod's variables on a player are limited in bytes, and other mods
// keep room for theirs (review decision 52).
func TestModVariableBytesPerPlayer(t *testing.T) {
	z := &Zone{players: map[uint32]*Player{1: {detached: true}}}
	h := scriptHost{z}
	big := `"` + strings.Repeat("x", 60_000) + `"`
	var err error
	n := 0
	for ; n < 40 && err == nil; n++ {
		err = h.SetVariable(1, modscript.VariableKey{Mod: "example.a", Name: fmt.Sprintf("v%d", n), Scope: "character"}, big)
	}
	if err == nil || !strings.Contains(err.Error(), "more than the") {
		t.Fatalf("err = %v, want the byte limit", err)
	}
	if want := modscript.MaxModPlayerBytes/len(big) + 1; n != want {
		t.Errorf("the limit came at value %d, want %d", n, want)
	}
	if err := h.SetVariable(1, modscript.VariableKey{Mod: "example.a", Name: "v0", Scope: "character"}, "1"); err != nil {
		t.Fatalf("a smaller value failed: %v", err)
	}
	if err := h.SetVariable(1, modscript.VariableKey{Mod: "example.b", Name: "v", Scope: "account"}, big); err != nil {
		t.Fatalf("another mod's value failed: %v", err)
	}
}
