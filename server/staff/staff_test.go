package staff

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestCatalogNames(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Catalog {
		group, name, ok := strings.Cut(p.Name, ".")
		if !ok || group == "" || name == "" || strings.Contains(name, ".") {
			t.Errorf("%q is not group.name", p.Name)
		}
		if seen[p.Name] {
			t.Errorf("%q is listed twice", p.Name)
		}
		seen[p.Name] = true
		if p.Step < 1 || p.Step > 8 || p.Allows == "" {
			t.Errorf("%q: step %d, allows %q", p.Name, p.Step, p.Allows)
		}
	}
	for _, n := range []string{"staff.assign", "audit.read"} {
		if !Known(n) {
			t.Errorf("step 1 needs %s", n)
		}
	}
}

func TestMatch(t *testing.T) {
	for _, c := range []struct {
		pattern string
		want    []string
	}{
		{"staff.assign", []string{"staff.assign"}},
		{"audit.*", []string{"audit.read", "audit.review"}},
		{"players.*", []string{"players.read", "players.read_ip"}},
		{"items.*", []string{"items.history", "items.restore", "items.create"}},
		{"moderation.*", []string{"moderation.reports", "moderation.mute", "moderation.kick", "moderation.jail", "moderation.ban"}},
		{"staff.give", nil},
		{"nothing.*", nil},
		{"*", nil},
		{"staff", nil},
		{"staff.", nil},
		{"aud*", nil},
	} {
		if got := Match(c.pattern); !slices.Equal(got, c.want) {
			t.Errorf("Match(%q) = %v, want %v", c.pattern, got, c.want)
		}
	}
}

func TestDefaultRoles(t *testing.T) {
	r := MustDefaultRoles()
	if got := r.Names(); !slices.Equal(got, []string{"admin", "game_master", "moderator"}) {
		t.Errorf("roles %v", got)
	}
	// An admin has every permission, as the owner.
	if got := r.Permissions("admin"); !slices.Equal(got, Names()) {
		t.Errorf("admin has %v", got)
	}
	for _, c := range []struct {
		role, perm string
		want       bool
	}{
		{Owner, "staff.assign", true},
		{Owner, "mods.disable", true},
		{Owner, "staff.give", false},
		{"admin", "staff.assign", true},
		{"game_master", "staff.assign", false},
		{"game_master", "audit.read", true},
		{"game_master", "moderation.ban", true},
		{"moderator", "moderation.ban", false},
		{"moderator", "moderation.mute", true},
		{"moderator", "audit.read", false},
		// Review decision 34's step 2 permissions.
		{"admin", "players.read_ip", true},
		{"admin", "items.create", true},
		{"admin", "audit.review", true},
		{"game_master", "players.read", true},
		{"game_master", "items.history", true},
		{"game_master", "items.restore", false},
		{"game_master", "items.create", false},
		{"game_master", "players.read_ip", false},
		{"game_master", "audit.review", false},
		{"moderator", "players.read", true},
		{"moderator", "items.history", false},
		{"", "audit.read", false},
		{"missing", "audit.read", false},
	} {
		if got := r.Has(c.role, c.perm); got != c.want {
			t.Errorf("%s has %s: %v, want %v", c.role, c.perm, got, c.want)
		}
	}
	if !r.Covers("admin", "game_master") || !r.Covers("admin", "admin") || r.Covers("game_master", "admin") ||
		!r.Covers(Owner, "admin") || !r.Covers("game_master", "moderator") {
		t.Error("Covers is wrong")
	}
}

func TestParseRolesErrors(t *testing.T) {
	_, err := ParseRoles(map[string][]string{
		"owner":     {"audit.read"},
		"Bad Name":  {"audit.read"},
		"helper":    {"audit.read", "audit.write", "nope.*"},
		"fine_role": {"audit.*"},
	})
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"owner comes only from staff.owners", `"Bad Name" is not a role name`,
		`staff.roles.helper: "audit.write" is not a permission`, `staff.roles.helper: "nope.*" is not a permission`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "fine_role") {
		t.Errorf("fine_role is fine: %v", err)
	}
	r, err := ParseRoles(map[string][]string{"helper": {"audit.read"}, "empty": nil})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Exists("empty") || r.Has("empty", "audit.read") || !r.Has("helper", "audit.read") || r.Exists("admin") {
		t.Error("a configured list replaces the defaults")
	}
}

func TestConfigBuild(t *testing.T) {
	if _, err := (Config{Owners: []string{"Alice", "alice", "x y"}}).Build(); err == nil ||
		!strings.Contains(err.Error(), `"alice" is listed twice`) || !strings.Contains(err.Error(), `"x y" is not an account name`) {
		t.Errorf("owners: %v", err)
	}
	r, err := (Config{Owners: []string{"Alice"}}).Build()
	if err != nil || !r.Exists("moderator") {
		t.Errorf("defaults: %v", err)
	}
}

func TestRegistry(t *testing.T) {
	reg := NewRegistry(MustDefaultRoles(), []string{"Alice"})
	reg.Load(map[int64]string{2: "moderator", 3: "retired"})
	two := func(name string) Login { return Login{AccountName: name, TwoFactor: true} }
	for _, c := range []struct {
		account int64
		login   Login
		want    string
	}{
		{1, two("ALICE"), Owner},
		{1, Login{AccountName: "Alice"}, ""},
		{2, two("Bob"), "moderator"},
		{2, Login{AccountName: "Bob"}, ""},
		// A role the config no longer has counts as none.
		{3, two("Carol"), ""},
		{4, two("Dave"), ""},
	} {
		if got := reg.RoleOfLogin(c.account, c.login); got != c.want {
			t.Errorf("account %d %+v: %q, want %q", c.account, c.login, got, c.want)
		}
	}
	ctx := context.Background()
	_ = reg.StaffChanged(ctx, 4, "game_master")
	_ = reg.StaffChanged(ctx, 2, "")
	if reg.RoleOf(4, "Dave") != "game_master" || reg.RoleOf(2, "Bob") != "" {
		t.Error("StaffChanged did not apply")
	}
	var nilReg *Registry
	if nilReg.RoleOf(1, "Alice") != "" {
		t.Error("a nil registry has roles")
	}
}
