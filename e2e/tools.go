package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Announce shows a line as the admin API's POST /announce does, to the
// world or to one map, through --dev's POST /debug/wathena/announce
// (decision 0031).
func (s *Server) Announce(ctx context.Context, text, mapName string) error {
	q := url.Values{"text": {text}}
	if mapName != "" {
		q.Set("map", mapName)
	}
	_, err := s.post(ctx, s.Debug+"/debug/wathena/announce?"+q.Encode(), nil)
	return err
}

// RestartPlan is the JSON of a planned restart, or of a refused one.
type RestartPlan struct {
	At      time.Time `json:"at"`
	Reason  string    `json:"reason"`
	Message string    `json:"message"`
	// Error is the code of a refused plan or cancel.
	Error string `json:"error"`
}

// PlanRestart plans a restart that long from the server clock, through
// --dev's POST /debug/wathena/restart.
func (s *Server) PlanRestart(ctx context.Context, in time.Duration, reason, message string) (RestartPlan, error) {
	q := url.Values{"in": {in.String()}, "reason": {reason}, "message": {message}}
	body, err := s.post(ctx, s.Debug+"/debug/wathena/restart?"+q.Encode(), nil)
	if err != nil {
		return RestartPlan{}, err
	}
	var p RestartPlan
	return p, json.Unmarshal(body, &p)
}

// CancelRestart cancels the planned restart, through --dev's DELETE
// /debug/wathena/restart.
func (s *Server) CancelRestart(ctx context.Context) (RestartPlan, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.Debug+"/debug/wathena/restart", nil)
	if err != nil {
		return RestartPlan{}, err
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G704 -- the test server's loopback debug address.
	if err != nil {
		return RestartPlan{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return RestartPlan{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return RestartPlan{}, &HTTPError{Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	var p RestartPlan
	return p, json.Unmarshal(body, &p)
}

// WaitExit waits until the server's process ends by itself, and returns
// its exit status. ok is false when it still runs after d.
func (s *Server) WaitExit(d time.Duration) (code int, ok bool) {
	select {
	case <-s.exited:
		return s.cmd.ProcessState.ExitCode(), true
	case <-time.After(d):
		return 0, false
	}
}

// Announcement waits for an announcement that contains text.
func (p *Player) Announcement(text string) *wathenav1.ChatMessage {
	p.t.Helper()
	return p.Expect("an announcement with "+text, func(e *wathenav1.Event) bool {
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_ANNOUNCEMENT && strings.Contains(c.GetText(), text)
	}).GetChat()
}

// SystemLine waits for a system line that starts with prefix.
func (p *Player) SystemLine(prefix string) string {
	p.t.Helper()
	return p.Expect("a system line with "+prefix, func(e *wathenav1.Event) bool {
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM && strings.HasPrefix(c.GetText(), prefix)
	}).GetChat().GetText()
}
