package gateway

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

func TestCleanChat(t *testing.T) {
	for in, want := range map[string]string{
		"hello":                           "hello",
		"  hi  ":                          "hi",
		"a\tb\nc":                         "a b c",
		"<b>x</b>":                        "<b>x</b>",
		"포링 hi":                           "포링 hi",
		"abc\u202Egpj.exe":                "abcgpj.exe",
		"\u2066x\u2069\u200E\u200F\u061C": "x",
		strings.Repeat("가", MaxChat):      strings.Repeat("가", MaxChat),
		" " + strings.Repeat("a", MaxChat) + "\n": strings.Repeat("a", MaxChat),
	} {
		got, ok := CleanChat(in)
		if !ok || got != want {
			t.Errorf("CleanChat(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "   ", "\n\t", "\u202E", "\xff", strings.Repeat("a", MaxChat+1)} {
		if got, ok := CleanChat(in); ok {
			t.Errorf("CleanChat(%q) = %q, want rejected", in, got)
		}
	}
}

func TestChatLimit(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newChatLimit(now)
	for i := range ChatBurst {
		if ok, _ := l.allow(now); !ok {
			t.Fatalf("line %d of the burst dropped", i+1)
		}
	}
	if ok, over := l.allow(now); ok || over {
		t.Fatalf("line past the burst: %v, %v", ok, over)
	}
	// One line comes back per ChatRefill.
	now = now.Add(ChatRefill)
	if ok, _ := l.allow(now); !ok {
		t.Fatal("refilled line dropped")
	}
	if ok, _ := l.allow(now); ok {
		t.Fatal("second line after one refill allowed")
	}
	// The bucket holds at most ChatBurst.
	now = now.Add(time.Hour)
	for range ChatBurst {
		l.allow(now)
	}
	if ok, _ := l.allow(now); ok {
		t.Fatal("bucket held more than the burst")
	}

	// Flooding ends the connection after ChatStrikes dropped lines.
	l = newChatLimit(now)
	for range ChatBurst {
		l.allow(now)
	}
	for i := range ChatStrikes {
		if _, over := l.allow(now); over {
			t.Fatalf("over after %d dropped lines", i+1)
		}
	}
	if _, over := l.allow(now); !over {
		t.Fatal("not over after ChatStrikes+1 dropped lines")
	}

	// Two lines a second, forever, never ends the connection.
	l = newChatLimit(now)
	for range 1000 {
		now = now.Add(ChatRefill / 2)
		if _, over := l.allow(now); over {
			t.Fatal("two lines a second ended the connection")
		}
	}
}

func chat(channel wathenav1.ChatChannel, text string) *wathenav1.ClientMessage {
	return wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
		Chat: wathenav1.Chat_builder{Channel: channel, Text: text}.Build(),
	}.Build()}.Build()
}

func TestChatDelivery(t *testing.T) {
	url := server(t)
	a := dial(t, url)
	a.hello("Alice", schema.Version)
	a.next((*wathenav1.ServerMessage).HasWelcome)
	b := dial(t, url)
	b.hello("Bob", schema.Version)
	b.next((*wathenav1.ServerMessage).HasWelcome)
	aliceOnB := b.next(hasEvent(func(e *wathenav1.Event) bool { return e.GetAppear().GetName() == "Alice" }))
	var alice uint32
	for _, e := range aliceOnB.GetFrame().GetEvents() {
		if e.GetAppear().GetName() == "Alice" {
			alice = e.GetAppear().GetId()
		}
	}

	// Lines that break the rules, and other channels, go nowhere. The
	// last line shows that they were skipped.
	a.send(chat(wathenav1.ChatChannel_CHAT_CHANNEL_WORLD, "world"))
	a.send(chat(wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL, " \n "))
	a.send(chat(wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL, strings.Repeat("a", MaxChat+1)))
	a.send(chat(wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL, " <b>x</b>\u202E "))
	m := b.next(hasEvent((*wathenav1.Event).HasChat))
	var got []*wathenav1.ChatMessage
	for _, e := range m.GetFrame().GetEvents() {
		if e.HasChat() {
			got = append(got, e.GetChat())
		}
	}
	if len(got) != 1 || got[0].GetFrom() != alice || got[0].GetFromName() != "Alice" || got[0].GetText() != "<b>x</b>" {
		t.Fatalf("Bob hears %v", got)
	}
	// Alice hears herself.
	a.next(hasEvent(func(e *wathenav1.Event) bool { return e.GetChat().GetText() == "<b>x</b>" }))
}

func TestChatFlood(t *testing.T) {
	url := server(t)
	a := dial(t, url)
	a.hello("Alice", schema.Version)
	a.next((*wathenav1.ServerMessage).HasWelcome)
	// The burst goes out.
	for i := range ChatBurst {
		a.send(chat(wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL, fmt.Sprint(i)))
	}
	a.next(hasEvent(func(e *wathenav1.Event) bool { return e.GetChat().GetText() == fmt.Sprint(ChatBurst-1) }))
	// A flood past ChatStrikes dropped lines ends the connection. A
	// refill or two may land while the test runs.
	for range ChatStrikes + 3 {
		a.send(chat(wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL, "spam"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var last *wathenav1.ServerMessage
	for {
		_, data, err := a.c.Read(ctx)
		if err != nil {
			break
		}
		last = &wathenav1.ServerMessage{}
		_ = proto.Unmarshal(data, last)
	}
	if last.GetDisconnect().GetReason() != wathenav1.DisconnectReason_DISCONNECT_REASON_RATE_LIMITED {
		t.Errorf("last message %v, want a rate limit disconnect", last)
	}
}
