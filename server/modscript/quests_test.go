package modscript

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/quest"
)

// questHost keeps player 1's quest log with the quest package's rules,
// at a fixed time.
type questHost struct {
	*fakeHost
	log   []character.Quest
	now   time.Time
	icons []string
}

func (h *questHost) AddQuest(_ int64, r *quest.Record) (bool, error) {
	return quest.Add(&h.log, r, h.now), nil
}

func (h *questHost) CompleteQuest(_ int64, r *quest.Record) (bool, error) {
	return quest.SetState(h.log, r.ID, character.QuestComplete), nil
}

func (h *questHost) RemoveQuest(_ int64, r *quest.Record) (bool, error) {
	return quest.Remove(&h.log, r.ID), nil
}

func (h *questHost) ChangeQuest(_ int64, from, to *quest.Record) (bool, error) {
	return quest.Change(&h.log, from.ID, to, h.now), nil
}

func (h *questHost) QuestState(_ int64, r *quest.Record) (string, error) {
	i := quest.Find(h.log, r.ID)
	if i < 0 {
		return quest.StateNone, nil
	}
	return quest.StateName(h.log[i].State), nil
}

func (h *questHost) QuestProgress(_ int64, r *quest.Record, j int) (int, bool, error) {
	i := quest.Find(h.log, r.ID)
	if i < 0 {
		return 0, false, nil
	}
	return h.log[i].Counts[j], true, nil
}

func (h *questHost) QuestTimedOut(_ int64, r *quest.Record) (bool, error) {
	i := quest.Find(h.log, r.ID)
	return i >= 0 && quest.TimedOut(&h.log[i], h.now), nil
}

func (h *questHost) ShowQuestIcon(npc, player int64, icon, mark string) error {
	if mark != "" {
		icon += "/" + mark
	}
	h.icons = append(h.icons, icon)
	return nil
}

// testQuests are core's 1100, with a Poring objective of 20 and an hour
// limit, core's 7, without objectives, and example.a:wolves.
func testQuests() *quest.Table {
	return quest.NewTable([]*quest.Record{
		{ID: 1100, Name: "1100", Mod: "core", Title: "Solo", Limit: quest.Limit{Seconds: 3600},
			Targets: []quest.Target{{Monster: 1002, Count: 20, Race: -1, Size: -1, Element: -1}}},
		{ID: 7, Name: "7", Mod: "core", Title: "Talk"},
		{ID: 1_000_000, Name: "example.a:wolves", Mod: "example.a", Title: "Wolves",
			Targets: []quest.Target{{Monster: 1013, Count: 3}, {Count: 2, Race: 2, Size: -1, Element: -1}}},
	})
}

// runQuests runs a mod's login handler against a quest host.
func runQuests(t *testing.T, read bool, src string) (*questHost, *Runtime, string, string) {
	t.Helper()
	var buf logBuffer
	h := &questHost{fakeHost: newHost(), now: time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)}
	sc := modScripts("example.a", nil, nil, "scripts/main.lua", src)
	if !read {
		delete(sc.Grants, "game.read")
	}
	r, err := New(Options{Host: h, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: []string{"example.a"}, Quests: testQuests()}, []*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	r.Fire("login", Player(1))
	return h, r, strings.ReplaceAll(strings.Join(h.messages, "|"), "1:", ""), buf.String()
}

// TestQuestCalls checks the player's quest calls (review decision 45).
func TestQuestCalls(t *testing.T) {
	h, _, got, logs := runQuests(t, true, `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    local function say(...)
      local out = {}
      for i = 1, select("#", ...) do out[i] = tostring((select(i, ...))) end
      player:send_message(table.concat(out, " "))
    end
    say(player:get_quest_state(1100), player:add_quest(1100), player:add_quest("1100"), player:get_quest_state(1100))
    say(player:get_quest_progress(1100, 1))
    say(player:get_quest_progress(1100, 2))
    say(player:quest_timed_out(1100), player:quest_timed_out(7))
    say(player:add_quest("wolves"), player:get_quest_state("example.a:wolves"))
    say(player:get_quest_progress("wolves", 2))
    say(player:change_quest(1100, 7), player:get_quest_state(1100), player:get_quest_state(7))
    say(player:change_quest(1100, 7), player:complete_quest(7), player:complete_quest(7), player:get_quest_state(7))
    say(player:remove_quest(7), player:remove_quest(7), player:get_quest_state(7))
    for _, bad in ipairs({
      function() player:add_quest(999) end,
      function() player:add_quest("nope") end,
      function() player:add_quest({}) end,
      function() player:get_quest_progress(1100, 0) end,
      function() player:get_quest_progress(1100, 1.5) end,
    }) do
      local ok, err = pcall(bad)
      say(ok, err)
    end
  end)
end
`)
	lines := strings.Split(got, "|")
	want := []string{
		"none true false active",
		"0 20",
		"nil nil",
		"false false",
		"true active",
		"0 2",
		"true none active",
		"false true false complete",
		"true false none",
	}
	if len(lines) != len(want)+5 {
		t.Fatalf("messages %q, log %s", got, logs)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d: %q, want %q", i, lines[i], w)
		}
	}
	for i, frag := range []string{"no quest 999", `no quest "nope"`, "want a quest", "objective is a number from 1", "objective must be an integer"} {
		if l := lines[len(want)+i]; !strings.HasPrefix(l, "false") || !strings.Contains(l, frag) {
			t.Errorf("error %d: %q, want %q", i, l, frag)
		}
	}
	if len(h.log) != 1 || h.log[0].ID != 1_000_000 {
		t.Errorf("the log %+v", h.log)
	}
}

// TestQuestReadsNeedGameRead: the reads need game.read, the changes do
// not.
func TestQuestReadsNeedGameRead(t *testing.T) {
	_, _, got, logs := runQuests(t, false, `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    player:send_message(tostring(player:add_quest(1100)))
    player:get_quest_state(1100)
  end)
end
`)
	if got != "true" || !strings.Contains(logs, "player:get_quest_state needs the permission game.read") {
		t.Errorf("messages %q, log %s", got, logs)
	}
}

// TestQuestEvents checks the objects of quest_add, quest_progress and
// quest_complete.
func TestQuestEvents(t *testing.T) {
	h, r, _, logs := runQuests(t, true, `
local events = require "wa.events"
return function()
  for _, name in ipairs({ "quest_add", "quest_progress", "quest_complete" }) do
    events.on(name, function(e)
      local parts = { name, e.player.name, e.quest, tostring(e.id) }
      if e.objective ~= nil then
        parts[#parts + 1] = e.objective .. ":" .. e.count .. "/" .. e.goal
      end
      e.player:send_message(table.concat(parts, " "))
      local ok = pcall(function() e.quest = "x" end)
      if ok then e.player:send_message("writable") end
    end)
  end
end
`)
	q := testQuests()
	r.FireQuest(QuestEvent{Kind: "quest_add", Player: 1, Quest: q.ByID(1100)})
	r.FireQuest(QuestEvent{Kind: "quest_progress", Player: 1, Quest: q.ByID(1_000_000), Objective: 2, Count: 1, Goal: 2})
	r.FireQuest(QuestEvent{Kind: "quest_complete", Player: 1, Quest: q.ByID(7)})
	got := strings.ReplaceAll(strings.Join(h.messages, "|"), "1:", "")
	want := "quest_add Alice 1100 1100|quest_progress Alice example.a:wolves 1000000 2:1/2|quest_complete Alice 7 7"
	if got != want {
		t.Errorf("events %q, want %q, log %s", got, want, logs)
	}
}
