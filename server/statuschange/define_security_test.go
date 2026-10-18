package statuschange

import (
	"fmt"
	"strings"
	"testing"
)

// A mod defines at most MaxModStatuses statuses. Defining one again, as
// another zone does, still works at the limit (review decision 52).
func TestDefineLimit(t *testing.T) {
	db, err := Load(&Data{Mode: "renewal"})
	if err != nil {
		t.Fatal(err)
	}
	for i := range MaxModStatuses {
		if _, err := db.Define(ModSpec{Mod: "example.a", Name: fmt.Sprintf("s%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Define(ModSpec{Mod: "example.a", Name: "s0"}); err != nil {
		t.Fatalf("defining a status again failed: %v", err)
	}
	if _, err := db.Define(ModSpec{Mod: "example.a", Name: "one_more"}); err == nil || !strings.Contains(err.Error(), "the most a mod may") {
		t.Fatalf("err = %v, want the limit", err)
	}
	if _, err := db.Define(ModSpec{Mod: "example.b", Name: "other"}); err != nil {
		t.Fatalf("another mod's status failed: %v", err)
	}
}
