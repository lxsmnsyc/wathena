package modscript

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// chatSpy is a host with chat. It records the whispers scripts send.
type chatSpy struct {
	*fakeHost
	whispers []string
}

func (c *chatSpy) Whisper(player int64, mod, from, text string) error {
	c.whispers = append(c.whispers, mod+"/"+from+"|"+text)
	return nil
}
func (c *chatSpy) IsBlocking(int64, int64) (bool, error)               { return false, nil }
func (c *chatSpy) Sanction(int64, string, int64, string, string) error { return nil }
func (c *chatSpy) Unmute(int64) (bool, error)                          { return false, nil }
func (c *chatSpy) CreateChannel(int64, chat.ChannelDef) (int64, error) { return 0, nil }
func (c *chatSpy) FindChannel(string) (int64, bool)                    { return 0, false }
func (c *chatSpy) ChannelSend(int64, string) error                     { return nil }
func (c *chatSpy) ChannelAdd(int64, int64) (bool, error)               { return false, nil }
func (c *chatSpy) ChannelRemove(int64, int64) (bool, error)            { return false, nil }
func (c *chatSpy) ChannelMembers(int64) ([]string, error)              { return nil, nil }
func (c *chatSpy) ReleaseChannels(int64)                               {}

func newChatRuntime(t *testing.T, m *mod.Scripts) (*Runtime, *chatSpy) {
	t.Helper()
	spy := &chatSpy{fakeHost: newHost()}
	var buf logBuffer
	r, err := New(Options{Host: spy, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: []string{m.Mod}}, []*mod.Scripts{m})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, spy
}

// A mod with no permission must not whisper under the name of a player.
// Here the name is Bob's, who is online. The whisper reaches the player
// marked with the mod's ID, so it cannot pass for Bob's (review decision
// 52).
func TestWhisperCannotImpersonatePlayer(t *testing.T) {
	m := modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    player:whisper("your account has a problem, tell me your password", { from = "Bob" })
  end)
end`)
	r, spy := newChatRuntime(t, m)
	r.Fire("login", Player(1))
	if len(spy.whispers) != 1 {
		t.Fatalf("whispers = %q, want one", spy.whispers)
	}
	for _, w := range spy.whispers {
		if !strings.HasPrefix(w, "example.a/") {
			t.Fatalf("a mod without permissions whispered as player Bob: %q", spy.whispers)
		}
	}
}

// A mod that only holds the default event.listen must not read the text of
// private whispers between two players.
func TestChatEventHidesWhispers(t *testing.T) {
	m := modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  local spy
  events.on("login", function(player) spy = player end)
  events.on("chat", function(msg)
    if msg.channel == "whisper" then spy:send_message("heard " .. msg.text) end
  end)
end`)
	r, spy := newChatRuntime(t, m)
	r.Fire("login", Player(1))
	r.FireChat(ChatMessage{Player: 2, Channel: "whisper", To: "Alice", Text: "my secret"})
	for _, msg := range spy.messages {
		if strings.Contains(msg, "my secret") {
			t.Fatalf("a mod with default permissions read a private whisper: %q", spy.messages)
		}
	}
}

// A mod that holds rules.override, which the chat hook needs and which
// sees every line, hears whispers in the chat event too. Public lines
// reach every mod.
func TestChatEventPrivateLinesNeedPermission(t *testing.T) {
	m := modScripts("example.a", []string{"rules.override"}, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  local spy
  events.on("login", function(player) spy = player end)
  events.on("chat", function(msg) spy:send_message(msg.channel .. " " .. msg.text) end)
end`)
	r, spy := newChatRuntime(t, m)
	r.Fire("login", Player(1))
	r.FireChat(ChatMessage{Player: 2, Channel: "whisper", To: "Alice", Text: "my secret"})
	r.FireChat(ChatMessage{Player: 2, Channel: "party", Text: "party line"})
	if len(spy.messages) != 2 {
		t.Fatalf("messages = %q, want the whisper and the party line", spy.messages)
	}
	n := modScripts("example.b", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  local spy
  events.on("login", function(player) spy = player end)
  events.on("chat", function(msg) spy:send_message(msg.channel .. " " .. msg.text) end)
end`)
	r2, spy2 := newChatRuntime(t, n)
	r2.Fire("login", Player(1))
	for _, ch := range []string{"whisper", "party", "guild", "public", "trade"} {
		r2.FireChat(ChatMessage{Player: 2, Channel: ch, Text: "line"})
	}
	if strings.Join(spy2.messages, ",") != "1:public line,1:trade line" {
		t.Fatalf("messages = %q, want only the public and channel lines", spy2.messages)
	}
}

// Core's whispers are the server's own, so they carry no mod.
func TestCoreWhisperIsNotMarked(t *testing.T) {
	m := modScripts("core", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(player) player:whisper("hello", { from = "Kafra" }) end)
end`)
	r, spy := newChatRuntime(t, m)
	r.Fire("login", Player(1))
	if len(spy.whispers) != 1 || spy.whispers[0] != "/Kafra|hello" {
		t.Fatalf("whispers = %q", spy.whispers)
	}
}
