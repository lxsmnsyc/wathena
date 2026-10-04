package account

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/lxsmnsyc/wathena/server/emblem"
)

// Guild emblems (strategy section 7.6, decision 0021): a guild master
// uploads the guild's emblem on the account pages. The upload is size
// limited, decoded, checked and re-encoded by server/emblem; only the
// server's PNG is kept and served. Players report emblems from the
// client.

// Guilds is what the account pages ask the game world about guilds.
type Guilds interface {
	// MasterOf returns the guild a character masters, with its name.
	MasterOf(charID int64) (guild int64, name string, ok bool)
	// SetEmblem gives a guild a new emblem, a PNG that emblem.Process
	// made, and returns its version.
	SetEmblem(guild int64, png []byte) (int, error)
}

// maxEmblemForm is the largest emblem form read: the upload's 50,000
// bytes and the form's other fields.
const maxEmblemForm = emblem.MaxUpload + 16<<10

// guildView is a guild a character of the account masters.
type guildView struct {
	Slot int
	Name string
	// Character is the master's name.
	Character string
}

// masteredGuilds lists the guilds the account's characters master.
func (s *Service) masteredGuilds(ctx context.Context, account int64) ([]guildView, error) {
	if s.cfg.Guilds == nil {
		return nil, nil
	}
	rows, err := s.q.AccountCharacters(ctx, account)
	if err != nil {
		return nil, err
	}
	var out []guildView
	for _, c := range rows {
		if _, name, ok := s.cfg.Guilds.MasterOf(c.ID); ok {
			out = append(out, guildView{Slot: c.Slot, Name: name, Character: c.Name})
		}
	}
	return out, nil
}

func (s *Service) getEmblem(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	p := s.newPage(w, r, "Guild emblem")
	p.Account = sess.AccountName
	var err error
	if p.Guilds, err = s.masteredGuilds(ctx, sess.AccountID); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.render(w, http.StatusOK, "emblem.html", p)
}

func (s *Service) postEmblem(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxEmblemForm)
	if err := r.ParseMultipartForm(maxEmblemForm); err != nil { // #nosec G120 -- MaxBytesReader bounds the body above.
		s.render(w, http.StatusBadRequest, "message.html", &page{Title: "Bad request", Error: "The form could not be read. An emblem is at most 50,000 bytes."})
		return
	}
	if !s.checkCSRF(r) {
		p := s.newPage(w, r, "Try again")
		p.Error = "The form expired or came from another site. Go back, reload the page, and try again. Cookies must be on."
		s.render(w, http.StatusForbidden, "message.html", p)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	p := s.newPage(w, r, "Guild emblem")
	p.Account = sess.AccountName
	var err error
	if p.Guilds, err = s.masteredGuilds(ctx, sess.AccountID); err != nil {
		s.unavailable(w, r, err)
		return
	}
	slot, err := strconv.Atoi(r.PostFormValue("slot"))
	guild := int64(0)
	if err == nil {
		rows, err := s.q.AccountCharacters(ctx, sess.AccountID)
		if err != nil {
			s.unavailable(w, r, err)
			return
		}
		for _, c := range rows {
			if c.Slot == slot && s.cfg.Guilds != nil {
				guild, _, _ = s.cfg.Guilds.MasterOf(c.ID)
			}
		}
	}
	if guild == 0 {
		p.Error = "Only a guild's master can change its emblem."
		s.render(w, http.StatusForbidden, "emblem.html", p)
		return
	}
	f, _, err := r.FormFile("emblem")
	if err != nil {
		p.Error = "Choose an image file."
		s.render(w, http.StatusBadRequest, "emblem.html", p)
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, emblem.MaxUpload+1))
	if err != nil {
		p.Error = "The image could not be read."
		s.render(w, http.StatusBadRequest, "emblem.html", p)
		return
	}
	png, err := emblem.Process(data)
	if err != nil {
		p.Error = "The emblem was refused: " + err.Error() + "."
		s.render(w, http.StatusBadRequest, "emblem.html", p)
		return
	}
	if _, err := s.cfg.Guilds.SetEmblem(guild, png); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			s.unavailable(w, r, err)
			return
		}
		p.Error = "The guild could not take the emblem: " + err.Error() + "."
		s.render(w, http.StatusConflict, "emblem.html", p)
		return
	}
	s.log.Info("guild emblem changed", "account", sess.AccountID, "guild", guild)
	p.Notice = "The guild has its new emblem."
	s.render(w, http.StatusOK, "emblem.html", p)
}
