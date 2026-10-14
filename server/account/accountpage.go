package account

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// The account page (proposal 0002 review decision 37, decision 0032):
// the block lists of the account's characters, the reports they filed,
// the sanctions on the account and its characters, the data export,
// account deletion and the privacy notice.
//
// Block lists, reports, sanctions and the characters' data live in the
// game's tables, which the account role cannot read. The service asks
// the game side through Game. waserver passes the admin world's, which
// reads offline characters from the database and online ones from their
// zones.

// Game is what the account page asks the game side.
type Game interface {
	// Blocks lists the block list of each of the account's characters,
	// in slot order.
	Blocks(ctx context.Context, account int64) ([]CharacterBlocks, error)
	// Unblock takes a name off a character's block list, whether the
	// character is online or not. It returns false when the character is
	// not the account's or does not block the name.
	Unblock(ctx context.Context, account, character int64, name string) (bool, error)
	// Reports lists the reports the account's characters filed, newest
	// first.
	Reports(ctx context.Context, account int64) ([]FiledReport, error)
	// Sanctions lists the sanctions on the account and its characters,
	// active and past, newest first.
	Sanctions(ctx context.Context, account int64) ([]AccountSanction, error)
	// Export reads the game's part of the account's data export. It
	// returns ErrCharacterOnline while a character of the account plays
	// or keeps a shop open on autotrade.
	Export(ctx context.Context, account int64) (*GameExport, error)
	// LeadsGuild returns the name of a character of the account that
	// leads a guild with other members, and the guild's name, or "".
	LeadsGuild(ctx context.Context, account int64) (character, guild string, err error)
	// Deleting closes the game connections and autotrade shops of the
	// account's characters, with text as the reason, after a deletion
	// request.
	Deleting(ctx context.Context, account int64, text string)
}

// ErrCharacterOnline is an export asked for while a character plays.
var ErrCharacterOnline = errors.New("account: a character of the account is online")

// CharacterBlocks is one character's block list.
type CharacterBlocks struct {
	CharacterID int64
	Character   string
	// Online is set when the character plays now. Unblocking works
	// either way.
	Online bool
	Names  []string
}

// Report kinds.
const (
	ReportPlayer      = "player"
	ReportLine        = "line"
	ReportGuildName   = "guild_name"
	ReportGuildEmblem = "guild_emblem"
)

// FiledReport is a report a character of the account filed.
type FiledReport struct {
	ID        int64
	At        time.Time
	Character string
	// Target is the player or guild named.
	Target string
	// Kind is ReportPlayer, ReportLine, ReportGuildName or
	// ReportGuildEmblem.
	Kind   string
	Reason string
	// Closed is set once staff closed the report. ActionTaken says
	// whether staff warned or sanctioned someone. Players see nothing
	// more of the outcome.
	Closed, ActionTaken bool
}

// AccountSanction is a sanction on the account or one of its
// characters.
type AccountSanction struct {
	ID int64
	// Kind is mute, jail, kick or ban.
	Kind string
	// Character is the character's name, or "" for a ban of the account.
	Character string
	Reason    string
	Start     time.Time
	// End is when it ends, or ended. Permanent is set for a ban without
	// an end.
	End       time.Time
	Permanent bool
	// Lifted is when staff ended it early, or zero.
	Lifted time.Time
}

// Active reports whether the sanction is in effect at now.
func (s AccountSanction) Active(now time.Time) bool {
	return s.Lifted.IsZero() && (s.Permanent || s.End.After(now))
}

// The views of the safety page.
type (
	blockView struct {
		ID     int64
		Name   string
		Online bool
		Names  []string
	}
	reportView struct {
		Date, Character, Target, Kind, Reason, Status string
	}
	sanctionView struct {
		Kind, Who, Reason, Start, End, State string
	}
)

// safetyNotices are the messages the safety page shows for ?notice=.
var safetyNotices = map[string]string{
	"unblocked":   "The name is off the block list.",
	"not-blocked": "That character does not block that name.",
}

var kindNames = map[string]string{
	ReportPlayer: "Player", ReportLine: "Chat line", ReportGuildName: "Guild name", ReportGuildEmblem: "Guild emblem",
	"mute": "Mute", "jail": "Jail", "kick": "Kick", "ban": "Ban",
}

func (s *Service) getSafety(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	p := s.newPage(w, r, "Blocks, reports and sanctions")
	p.Account = sess.AccountName
	if msg, ok := safetyNotices[r.FormValue("notice")]; ok {
		p.Notice = msg
	}
	if err := s.fillSafety(ctx, p, sess.AccountID); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.render(w, http.StatusOK, "safety.html", p)
}

func (s *Service) fillSafety(ctx context.Context, p *page, account int64) error {
	g := s.cfg.Game
	if g == nil {
		p.NoGame = true
		return nil
	}
	blocks, err := g.Blocks(ctx, account)
	if err != nil {
		return err
	}
	for _, b := range blocks {
		p.Blocks = append(p.Blocks, blockView{ID: b.CharacterID, Name: b.Character, Online: b.Online, Names: b.Names})
	}
	reports, err := g.Reports(ctx, account)
	if err != nil {
		return err
	}
	for _, rp := range reports {
		status := "Open"
		switch {
		case rp.Closed && rp.ActionTaken:
			status = "Action taken"
		case rp.Closed:
			status = "No action"
		}
		p.Reports = append(p.Reports, reportView{Date: minute(rp.At), Character: rp.Character, Target: rp.Target,
			Kind: kindNames[rp.Kind], Reason: rp.Reason, Status: status})
	}
	sanctions, err := g.Sanctions(ctx, account)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, sn := range sanctions {
		v := sanctionView{Kind: kindNames[sn.Kind], Who: sn.Character, Reason: sn.Reason, Start: minute(sn.Start), End: minute(sn.End)}
		if v.Who == "" {
			v.Who = "The account"
		}
		switch {
		case sn.Permanent:
			v.End = "Never"
		case sn.Kind == "kick":
			v.End = "At once"
		}
		switch {
		case !sn.Lifted.IsZero():
			v.State = "Lifted " + minute(sn.Lifted)
		case sn.Active(now):
			v.State = "In effect"
		default:
			v.State = "Ended"
		}
		p.Sanctions = append(p.Sanctions, v)
	}
	return nil
}

// minute writes a time to the minute, in UTC.
func minute(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 MST") }

// accountAction starts a post of the account page: the form and its
// CSRF token, the session, and the account page's rate limit. It writes
// the answer and returns false when one of them fails.
func (s *Service) accountAction(ctx context.Context, w http.ResponseWriter, r *http.Request) (db.UseSessionRow, bool) {
	if !s.form(w, r) {
		return db.UseSessionRow{}, false
	}
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return sess, false
	}
	if ok, wait := s.limit.allow(actAccount, s.clientAddr(r), time.Now()); !ok {
		p := s.newPage(w, r, "Slow down")
		p.Error = "Too many changes from your network. Try again in " + seconds(wait) + "."
		s.render(w, http.StatusTooManyRequests, "message.html", p)
		return sess, false
	}
	return sess, true
}

// unblock takes a name off a character's block list.
func (s *Service) unblock(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.accountAction(ctx, w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PostForm.Get("character"), 10, 64)
	name := strings.TrimSpace(r.PostForm.Get("name"))
	if err != nil || id <= 0 || name == "" || len(name) > 4*MaxCharName {
		s.render(w, http.StatusBadRequest, "message.html", &page{Title: "Bad request", Error: "Choose a name to unblock."})
		return
	}
	if s.cfg.Game == nil {
		s.render(w, http.StatusServiceUnavailable, "message.html", &page{Title: "Not available", Error: "Block lists are not available on this server."})
		return
	}
	done, err := s.cfg.Game.Unblock(ctx, sess.AccountID, id, name)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if !done {
		s.redirect(w, r, withNotice(Prefix+"safety", "not-blocked"))
		return
	}
	s.redirect(w, r, withNotice(Prefix+"safety", "unblocked"))
}
