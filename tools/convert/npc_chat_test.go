package convert

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// TestChatCommandsConvert checks the channel commands and message
// (proposal 0002, review decision 26).
func TestChatCommandsConvert(t *testing.T) {
	res := eventResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ channel_chat "#trade","Hello"; end; }`, []string{
			`channel_1 = channels.get("trade")`, "if channel_1 then", `channel_1:send("Hello")`, `local channels = require "wa.channels"`,
		}},
		{`{ channel_chat "#Lounge","Hi " + strcharinfo(0); end; }`, []string{`channels.get("lounge")`, `:send("Hi " .. player.name)`}},
		{`{ channel_join "#trade"; end; }`, []string{`channels.get("trade")`, ":add(player)"}},
		{`{ channel_kick "#trade",getcharid(0); end; }`, []string{`channels.get("trade")`, ":remove(player)"}},
		{`{ channel_kick "#trade",strcharinfo(0); end; }`, []string{":remove(player)"}},
		{`{ channel_create "#lounge","[Lounge]"; end; }`, []string{
			`if not channels.get("lounge") then`, `channels.create { name = "lounge", display_name = "[Lounge]" }`,
		}},
		{`{ message strcharinfo(0),"Hi"; end; }`, []string{`player:send_message("Hi")`}},
		{`{ dispbottom "Hi"; end; }`, []string{`player:send_message("Hi")`}},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		lua, err := n.npcScript()
		if err != nil {
			t.Errorf("%s: %v", c.script, err)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(lua, w) {
				t.Errorf("%s: got\n%s\nwant %q", c.script, lua, w)
			}
		}
	}
	for _, c := range []struct{ script, want string }{
		{`{ channel_chat "#trade","Hello",0xFF0000; end; }`, "channel_chat: with a color"},
		{`{ channel_chat "prontera","Hello"; end; }`, "channel_chat: a map's channel"},
		{`{ channel_chat "#map","Hello"; end; }`, "channel_chat: the map channel"},
		{`{ channel_chat .@c$,"Hello"; end; }`, "channel_chat: a computed channel"},
		{`{ channel_join "#trade",150001; end; }`, "channel_join: for another character"},
		{`{ channel_kick "#trade",150001; end; }`, "channel_kick: another character"},
		{`{ channel_kick "#trade","Bob"; end; }`, "channel_kick: another character"},
		{`{ channel_create "#lounge","[Lounge]","pass"; end; }`, "channel_create: a password"},
		{`{ channel_delete "#lounge"; end; }`, "channel_delete: the channel API cannot delete a channel"},
		{`{ channel_setopt "#lounge",1,0; end; }`, "channel_setopt: the channel API cannot change"},
		{`{ channel_ban "#lounge",150001; end; }`, "channel_ban: the channel API has no bans"},
		{`{ message "Bob","Hi"; end; }`, "message: to another character"},
		{`{ dispbottom "Hi",0xFF0000; end; }`, "dispbottom: with a color"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}

// chatHost is npcHost with chat channels: core's trade, and the
// channels scripts make.
type chatHost struct {
	*npcHost
	channels map[string]int64
	members  map[int64]map[int64]bool
	sent     []string
}

func (h *chatHost) Whisper(int64, string, string) error                 { return nil }
func (h *chatHost) IsBlocking(int64, int64) (bool, error)               { return false, nil }
func (h *chatHost) Sanction(int64, string, int64, string, string) error { return nil }
func (h *chatHost) Unmute(int64) (bool, error)                          { return false, nil }
func (h *chatHost) ReleaseChannels(int64)                               {}

func (h *chatHost) CreateChannel(_ int64, def chat.ChannelDef) (int64, error) {
	if _, ok := h.channels[def.Name]; ok {
		return 0, fmt.Errorf("the name %s is taken", def.Name)
	}
	id := int64(len(h.channels) + 1)
	h.channels[def.Name] = id
	h.sent = append(h.sent, "create "+def.Name+" "+def.DisplayName)
	return id, nil
}

func (h *chatHost) FindChannel(name string) (int64, bool) {
	n, ok := chat.NormalizeChannel(name)
	if !ok {
		return 0, false
	}
	id, ok := h.channels[n]
	return id, ok
}

func (h *chatHost) ChannelSend(channel int64, text string) error {
	h.sent = append(h.sent, fmt.Sprintf("send %d %s", channel, text))
	return nil
}

func (h *chatHost) ChannelAdd(channel, player int64) (bool, error) {
	if h.members[channel] == nil {
		h.members[channel] = map[int64]bool{}
	}
	if h.members[channel][player] {
		return false, nil
	}
	h.members[channel][player] = true
	return true, nil
}

func (h *chatHost) ChannelRemove(channel, player int64) (bool, error) {
	if !h.members[channel][player] {
		return false, nil
	}
	delete(h.members[channel], player)
	return true, nil
}

func (h *chatHost) ChannelMembers(int64) ([]string, error) { return nil, nil }

// TestRunConvertedChatCommands runs converted channel commands: a channel
// that does not exist is skipped without an error, as in rAthena.
func TestRunConvertedChatCommands(t *testing.T) {
	_, base, core := testRuntime(t, reference.Renewal)
	logs := &logRecorder{}
	h := &chatHost{npcHost: &npcHost{testHost: base, npcs: map[int64]*NPC{}, steps: map[int64]modscript.Step{}, ended: map[int64]bool{},
		vars: map[modscript.VariableKey]string{}}, channels: map[string]int64{"trade": 100}, members: map[int64]map[int64]bool{}}
	res := eventResult(t)
	n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "Chat#t", Map: "prontera", Script: `{
		channel_create "#lounge","[Lounge]";
		channel_create "#lounge","[Again]";
		channel_join "#lounge";
		channel_join "#trade";
		channel_chat "#lounge","Hello";
		channel_chat "#none","Nobody hears this";
		channel_join "#none";
		channel_kick "#trade",getcharid(0);
		end;
	}`})
	lua, err := n.npcScript()
	if err != nil {
		t.Fatal(err)
	}
	sc := coreScripts(t, core)
	sc[0].Files["scripts/npcs/chat_test.lua"] = lua
	logger := slog.New(logs)
	rt, err := modscript.New(modscript.Options{Host: h, Mods: []string{"core"}, Constants: core.Constants, Jobs: base.jobs,
		Log: logger, Server: variables.NewServer(nil, nil), Clock: func() time.Time { return testClock }, Location: time.UTC}, sc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	h.npcs[1] = &NPC{Name: "Chat#t", Source: "Chat#t", Map: "prontera"}
	npc, err := rt.StartNPC(1, "core:scripts/npcs/chat_test.lua")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Click(npc, 1); err != nil {
		t.Fatal(err)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
	lounge := h.channels["lounge"]
	if want := []string{"create lounge [Lounge]", fmt.Sprintf("send %d Hello", lounge)}; !slices.Equal(h.sent, want) {
		t.Errorf("got %q, want %q", h.sent, want)
	}
	if !h.members[lounge][1] || h.members[100][1] {
		t.Errorf("members: %v", h.members)
	}
}
