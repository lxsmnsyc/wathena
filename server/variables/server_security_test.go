package variables

import (
	"fmt"
	"strings"
	"testing"
)

// One mod's server variables are limited in count and bytes, and other
// mods keep theirs (review decision 52).
func TestModLimits(t *testing.T) {
	s := NewServer(nil, nil)
	for i := range MaxModValues {
		if err := s.Set(Key{Mod: "example.a", Name: fmt.Sprintf("v%d", i)}, "1"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Set(Key{Mod: "example.a", Name: "more"}, "1"); err == nil || !strings.Contains(err.Error(), "the most a mod may") {
		t.Fatalf("err = %v, want the count limit", err)
	}
	if err := s.Set(Key{Mod: "example.a", Name: "v0"}, "2"); err != nil {
		t.Fatalf("changing a value at the limit failed: %v", err)
	}
	if err := s.Set(Key{Mod: "example.a", Name: "v1"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(Key{Mod: "example.a", Name: "more"}, "1"); err != nil {
		t.Fatalf("a value after one was cleared failed: %v", err)
	}
	big := `"` + strings.Repeat("x", 60_000) + `"`
	var err error
	for i := 2; i < 40 && err == nil; i++ {
		err = s.Set(Key{Mod: "example.a", Name: fmt.Sprintf("v%d", i)}, big)
	}
	if err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("err = %v, want the byte limit", err)
	}
	if _, err := s.Update(Key{Mod: "example.a", Name: "v100"}, func(string) (string, error) { return big, nil }); err == nil {
		t.Fatal("Update went over the byte limit")
	}
	if err := s.Set(Key{Mod: "example.b", Name: "v"}, big); err != nil {
		t.Fatalf("another mod's value failed: %v", err)
	}
}
