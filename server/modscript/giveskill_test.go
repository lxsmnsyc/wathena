package modscript

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/mod"
)

// giveSkillHost is fakeHost with the job calls, recording player:give_skill.
type giveSkillHost struct {
	*fakeHost
	given []string
}

func (h *giveSkillHost) NextJobs(int) []int                             { return nil }
func (h *giveSkillHost) ChangeJob(int64, int, bool) (bool, error)       { return false, nil }
func (h *giveSkillHost) Rebirth(int64) (bool, error)                    { return false, nil }
func (h *giveSkillHost) ResetSkills(int64) error                        { return nil }
func (h *giveSkillHost) ResetStats(int64) error                         { return nil }
func (h *giveSkillHost) ResetSkill(int64, int, int, bool) (bool, error) { return false, nil }
func (h *giveSkillHost) ResetStat(int64, int, int) (bool, error)        { return false, nil }
func (h *giveSkillHost) GiveSkill(p int64, skill, level int, kind SkillKind) (bool, error) {
	h.given = append(h.given, fmt.Sprintf("%d:%d:%d:%d", p, skill, level, kind))
	return skill != 9999, nil
}

// TestGiveSkill checks player:give_skill (review decision 57): its
// permission, its kinds and its arguments.
func TestGiveSkill(t *testing.T) {
	src := `
local events = require "wa.events"
return function()
  events.on("login", function(p)
    local ok, err = pcall(function()
      p:send_message(tostring(p:give_skill(144, 1, "permanent")))
      p:send_message(tostring(p:give_skill(143, 0, "permanent")))
      p:send_message(tostring(p:give_skill(19, 3, "temporary")))
      p:send_message(tostring(p:give_skill(28, 2, "temporary_add")))
      p:send_message(tostring(p:give_skill(28, 14, "temporary")))
      p:send_message(tostring(p:give_skill(9999, 1, "temporary")))
    end)
    if not ok then p:send_message(tostring(err)) end
    for _, bad in ipairs({ { 28, 1, "granted" }, { 28, -1, "temporary" }, { 0, 1, "temporary" }, { 28, 1 } }) do
      local ok, err = pcall(p.give_skill, p, bad[1], bad[2], bad[3])
      p:send_message(ok and "ok" or tostring(err))
    end
  end)
end`
	run := func(grants []string) (*giveSkillHost, *logBuffer) {
		h := &giveSkillHost{fakeHost: newHost()}
		var buf logBuffer
		r, err := New(Options{Host: h, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: []string{"example.a"}},
			[]*mod.Scripts{modScripts("example.a", grants, nil, "scripts/main.lua", src)})
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		r.Fire("login", Player(1))
		return h, &buf
	}
	h, _ := run(nil)
	if len(h.given) != 0 || len(h.messages) == 0 || !strings.Contains(h.messages[0], "player:give_skill needs the permission player.skills") {
		t.Fatalf("without the permission: given %v, messages %q", h.given, h.messages)
	}
	h, logs := run([]string{"player.skills"})
	want := []string{"1:144:1:0", "1:143:0:0", "1:19:3:1", "1:28:2:2", "1:9999:1:1"}
	if strings.Join(h.given, " ") != strings.Join(want, " ") {
		t.Errorf("given %v, want %v; logs: %s", h.given, want, logs)
	}
	msgs := strings.Join(h.messages, "\n")
	for _, w := range []string{"1:true\n1:true\n1:true\n1:true\n1:false\n1:false\n",
		`want the kind "permanent", "temporary" or "temporary_add"`, "want a level from 0", "want a skill from skills.id"} {
		if !strings.Contains(msgs, w) {
			t.Errorf("no %q in\n%s", w, msgs)
		}
	}
	if n := strings.Count(msgs, "want the kind"); n != 2 {
		t.Errorf("%d kind errors, want 2 (an unknown kind and none):\n%s", n, msgs)
	}
}
