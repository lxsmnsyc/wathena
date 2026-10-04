package currency

import (
	"strings"
	"testing"
)

// TestCore checks core's records: rAthena's cash and Kafra points, on
// the account, up to MAX_ZENY.
func TestCore(t *testing.T) {
	c := Core()
	for _, want := range []Record{{ID: Cash, Name: "cash"}, {ID: Kafra, Name: "kafra"}} {
		r := c.ByID(want.ID)
		if r == nil || r.Name != want.Name || !r.Account || r.Max != MaxAmount {
			t.Errorf("%s: %+v", want.Name, r)
		}
	}
}

// TestBuildAndResolve reads a mod's record and finds names as a mod gave
// them: its own record first, then core's.
func TestBuildAndResolve(t *testing.T) {
	r, err := Build("example.arena:tokens", "example.arena", 10000, map[string]any{"owner": "character", "max": 1000})
	if err != nil || r.Account || r.Max != 1000 || r.DisplayName != "tokens" {
		t.Fatalf("%+v %v", r, err)
	}
	cash, _ := Build("cash", "core", Cash, nil)
	tab := NewTable([]*Record{cash, r})
	if tab.Resolve("tokens", "example.arena") != r || tab.Resolve("cash", "example.arena") != cash ||
		tab.Resolve("example.arena:tokens", "example.other") != r || tab.Resolve("tokens", "example.other") != nil {
		t.Error("resolve")
	}
	for fields, want := range map[*map[string]any]string{
		{"owner": "guild"}:   "owner is account or character",
		{"max": 0}:           "max is a whole number",
		{"display_name": ""}: "display_name is 1 to 24",
		{"colour": "red"}:    `"colour" is not a field`,
	} {
		if _, err := Build("x", "core", 3, *fields); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: %v", *fields, err)
		}
	}
}
