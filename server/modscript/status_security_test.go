package modscript

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/lxsmnsyc/wathena/server/mod"
)

// statusSpy is a host with status effects. It records every start and end.
type statusSpy struct {
	*fakeHost
	started []string
}

func (s *statusSpy) StartStatus(t Entity, st StatusStart) (bool, error) {
	s.started = append(s.started, fmt.Sprintf("start %s %d status %d", t.Kind, t.ID, st.Status))
	return true, nil
}
func (s *statusSpy) PreviewStatus(Entity, StatusStart) (StatusPreview, bool, error) {
	return StatusPreview{}, false, nil
}
func (s *statusSpy) EndStatus(t Entity, status int64) (bool, error) {
	s.started = append(s.started, fmt.Sprintf("end %s %d status %d", t.Kind, t.ID, status))
	return true, nil
}
func (s *statusSpy) Statuses(Entity, int64) ([]StatusInfo, error)      { return nil, nil }
func (s *statusSpy) DefineStatus(StatusDefinition) (int64, error)      { return 900, nil }
func (s *statusSpy) AddTimedBonuses(int64, TimedBonuses) (bool, error) { return true, nil }

// player.status is an approval permission: a mod without it must not start
// or end a status on a player.
func TestStartStatusNeedsPermission(t *testing.T) {
	spy := &statusSpy{fakeHost: newHost()}
	var buf logBuffer
	m := modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    player:start_status(5, { milliseconds = 1000000, cannot_be_avoided = true })
    player:end_status(6)
  end)
end`)
	if m.Grants.Has("player.status") {
		t.Fatal("test mod must not hold player.status")
	}
	r, err := New(Options{Host: spy, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: []string{"example.a"}}, []*mod.Scripts{m})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	r.Fire("login", Player(1))
	if len(spy.started) != 0 {
		t.Fatalf("a mod without player.status changed statuses: %q", spy.started)
	}
}
