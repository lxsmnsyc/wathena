package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Chat and moderation steps (decision 0018).

// Whisper whispers a line to a name and returns the server's answer.
func (p *Player) Whisper(to, text string) *wathenav1.WhisperResult {
	p.t.Helper()
	p.send("whisper", func(ctx context.Context) error { return p.Bot.Whisper(ctx, to, text) })
	return p.Expect("the answer to a whisper to "+to, (*wathenav1.Event).HasWhisperResult).GetWhisperResult()
}

// HearsWhisper waits for a whisper from a name.
func (p *Player) HearsWhisper(from, text string) *wathenav1.ChatMessage {
	p.t.Helper()
	return p.Expect(fmt.Sprintf("a whisper from %s: %q", from, text), func(e *wathenav1.Event) bool {
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_WHISPER && c.GetFromName() == from && c.GetText() == text
	}).GetChat()
}

// JoinChannel joins a channel and waits until the player is in it.
func (p *Player) JoinChannel(name string) *wathenav1.ChannelJoined {
	p.t.Helper()
	p.send("join "+name, func(ctx context.Context) error {
		return p.Channel(ctx, wathenav1.ChannelAction_CHANNEL_ACTION_JOIN, name)
	})
	return p.Expect("joining "+name, func(e *wathenav1.Event) bool {
		if f := e.GetChannelFailed(); f != nil {
			p.t.Fatalf("%s: joining %s failed: %s", p.Name, name, f.GetReason())
		}
		return e.GetChannelJoined().GetName() == name
	}).GetChannelJoined()
}

// SayIn says a line in a channel and waits until the player hears it
// back.
func (p *Player) SayIn(channel, text string) *wathenav1.ChatMessage {
	p.t.Helper()
	p.send("say in "+channel, func(ctx context.Context) error { return p.ChannelChat(ctx, channel, text) })
	return p.HearsIn(channel, p.Name, text)
}

// HearsIn waits for a channel's line from a speaker.
func (p *Player) HearsIn(channel, from, text string) *wathenav1.ChatMessage {
	p.t.Helper()
	return p.Expect(fmt.Sprintf("%s to say %q in %s", from, text, channel), func(e *wathenav1.Event) bool {
		if r := e.GetChatRefused(); r != nil {
			p.t.Fatalf("%s: a line was refused: %s", p.Name, r.GetReason())
		}
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_NAMED && c.GetChannelName() == channel &&
			c.GetFromName() == from && c.GetText() == text
	}).GetChat()
}

// BlockName blocks a name, or unblocks it, and returns the new block
// list.
func (p *Player) BlockName(name string, unblock bool) *wathenav1.BlockList {
	p.t.Helper()
	p.send("block "+name, func(ctx context.Context) error { return p.Block(ctx, name, unblock) })
	return p.Expect("the block list", func(e *wathenav1.Event) bool {
		bl := e.GetBlockList()
		if bl == nil {
			return false
		}
		has := slices.ContainsFunc(bl.GetNames(), func(n string) bool { return strings.EqualFold(n, name) })
		return has != unblock || bl.GetFull()
	}).GetBlockList()
}

// SayRefused says a line of local chat and waits for the refusal.
func (p *Player) SayRefused(text string) *wathenav1.ChatRefused {
	p.t.Helper()
	p.send("chat", func(ctx context.Context) error { return p.Chat(ctx, text) })
	return p.Expect("a refused line", (*wathenav1.Event).HasChatRefused).GetChatRefused()
}

// ReportLine reports a line, or a player when line is empty, and returns
// the outcome.
func (p *Player) ReportLine(name string, line []byte, reason string) wathenav1.ReportOutcome {
	p.t.Helper()
	p.send("report", func(ctx context.Context) error { return p.Report(ctx, name, line, reason) })
	return p.Expect("the report's result", (*wathenav1.Event).HasReportResult).GetReportResult().GetOutcome()
}

// Sanction waits for a sanction notice.
func (p *Player) Sanction(ended bool) *wathenav1.SanctionNotice {
	p.t.Helper()
	return p.Expect(fmt.Sprintf("a sanction notice, ended %v", ended), func(e *wathenav1.Event) bool {
		s := e.GetSanction()
		return s != nil && s.GetEnded() == ended
	}).GetSanction()
}

// AdvanceClock moves the server clock forward, with --dev's POST
// /debug/wathena/clock.
func (s *Server) AdvanceClock(ctx context.Context, d time.Duration) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Debug+"/debug/wathena/clock?advance="+url.QueryEscape(d.String()), nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G704 -- the test server's loopback debug address.
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("clock: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// ReportLineJSON and ReportJSON are the JSON of the server's GET
// /debug/wathena/reports.
type ReportLineJSON struct {
	ID   string    `json:"id"`
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
	Name string    `json:"name"`
	To   string    `json:"to"`
	Map  string    `json:"map"`
	Text string    `json:"text"`
}

// ReportJSON is one report, newest first.
type ReportJSON struct {
	ID       int64            `json:"id"`
	Reporter string           `json:"reporter"`
	Target   string           `json:"target"`
	Line     string           `json:"line"`
	Reason   string           `json:"reason"`
	Status   string           `json:"status"`
	Lines    []ReportLineJSON `json:"lines"`
}

// Reports asks the server for the newest reports.
func (s *Server) Reports(ctx context.Context) ([]ReportJSON, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Debug+"/debug/wathena/reports", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G704 -- the test server's loopback debug address.
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("reports: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out []ReportJSON
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
