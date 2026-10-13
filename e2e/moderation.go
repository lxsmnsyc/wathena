package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// GiveStaffRole gives an online character's account a staff role, as if
// it logged in with a second factor, through --dev's POST
// /debug/wathena/staff (decision 0030).
func (s *Server) GiveStaffRole(ctx context.Context, name, role string) error {
	_, err := s.post(ctx, s.Debug+"/debug/wathena/staff?"+url.Values{"name": {name}, "role": {role}}.Encode(), nil)
	return err
}

// GiveSanction gives a sanction as the admin API's POST /sanctions does,
// through --dev's POST /debug/wathena/sanction. A ban bans the
// character's account, and 0 minutes is permanent. It returns the
// sanction's ID, or the error code of one that was refused.
func (s *Server) GiveSanction(ctx context.Context, kind, name string, minutes int, reason string) (int64, string, error) {
	q := url.Values{"kind": {kind}, "name": {name}, "minutes": {fmt.Sprint(minutes)}, "reason": {reason}}
	body, err := s.post(ctx, s.Debug+"/debug/wathena/sanction?"+q.Encode(), nil)
	if err != nil {
		return 0, "", err
	}
	var out struct {
		ID    int64  `json:"id"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, "", err
	}
	return out.ID, out.Error, nil
}

// GMCommand types a GM command in local chat, and returns the server's
// answer: the next system line.
func (p *Player) GMCommand(line string) string {
	p.t.Helper()
	p.send("command", func(ctx context.Context) error { return p.Chat(ctx, line) })
	return p.Answer()
}

// Answer waits for the next system line.
func (p *Player) Answer() string {
	p.t.Helper()
	return p.Expect("a system line", func(e *wathenav1.Event) bool {
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM
	}).GetChat().GetText()
}

// HeardText reports whether the player got any chat line with a text
// so far.
func (p *Player) HeardText(text string) bool {
	events, _ := p.Events(0)
	for _, e := range events {
		if c := e.GetChat(); c != nil && strings.Contains(c.GetText(), text) {
			return true
		}
	}
	return false
}
