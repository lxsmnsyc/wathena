package modscript

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/staff"
)

// staffHost is a fake zone whose players have staff roles, by player
// ID, already filtered by the two-factor rule.
type staffHost struct {
	*fakeHost
	roles map[int64]string
}

func (h *staffHost) StaffRole(id int64) (string, error) {
	if _, ok := h.players[id]; !ok {
		return "", fmt.Errorf("the player has left")
	}
	return h.roles[id], nil
}

func (h *staffHost) HasStaffPermission(id int64, permission string) (bool, error) {
	role, err := h.StaffRole(id)
	return role != "" && staff.MustDefaultRoles().Has(role, permission), err
}

// TestStaffCalls checks player:get_staff_role and
// player:has_staff_permission (review decision 33) for a player with a
// role, the owner, a player without one, and the errors.
func TestStaffCalls(t *testing.T) {
	for _, c := range []struct {
		role, want string
	}{
		{"game_master", "game_master true true false"},
		{staff.Owner, "owner true true true"},
		{"", "nil false false false"},
	} {
		host := &staffHost{fakeHost: newHost(), roles: map[int64]string{1: c.role}}
		var buf logBuffer
		sc := modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    player:send_message(table.concat({
      tostring(player:get_staff_role()),
      tostring(player:has_staff_permission("moderation.mute")),
      tostring(player:has_staff_permission("audit.read")),
      tostring(player:has_staff_permission("staff.assign")),
    }, " "))
    for _, f in ipairs({
      function() return player:has_staff_permission("moderation.*") end,
      function() return player:has_staff_permission(1) end,
    }) do
      local ok, e = pcall(f)
      player:send_message((tostring(ok) .. " " .. tostring(e)):sub(1, 200))
    end
  end)
end`)
		r, err := New(Options{Host: host, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: []string{"example.a"}}, []*mod.Scripts{sc})
		if err != nil {
			t.Fatal(err)
		}
		r.Fire("login", Player(1))
		r.Close()
		lines := host.messages
		if len(lines) != 3 {
			t.Fatalf("%s: messages %q, logs %s", c.role, lines, buf.String())
		}
		if want := "1:" + c.want; lines[0] != want {
			t.Errorf("%s: %q, want %q", c.role, lines[0], want)
		}
		if !strings.Contains(lines[1], `there is no staff permission "moderation.*"; the permissions are staff.assign, audit.read`) {
			t.Errorf("unknown name: %q", lines[1])
		}
		if !strings.Contains(lines[2], "the permission must be a string") {
			t.Errorf("not a string: %q", lines[2])
		}
	}
}

// TestStaffCallsWithoutHost checks that a runtime whose host has no
// staff roles raises an error, not a wrong answer.
func TestStaffCallsWithoutHost(t *testing.T) {
	host := newHost()
	sc := modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    local ok, e = pcall(function() return player:get_staff_role() end)
    player:send_message(tostring(ok) .. " " .. tostring(e))
  end)
end`)
	r, err := New(Options{Host: host, Log: slog.New(slog.DiscardHandler), Mods: []string{"example.a"}}, []*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Fire("login", Player(1))
	if len(host.messages) != 1 || !strings.Contains(host.messages[0], "staff roles do not exist here") {
		t.Errorf("messages %q", host.messages)
	}
}
