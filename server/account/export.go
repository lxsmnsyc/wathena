package account

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// The data export (decision 0032). A player downloads one JSON file
// with the account's data, built at the request. It needs a login from
// the last 10 minutes and works once a day. It never holds a secret: no
// password hash, TOTP secret, recovery code, passkey key, session or
// ticket.
//
// The format is stable. A change that removes or renames a field, or
// changes its meaning, raises ExportVersion. New fields keep it.
// docs/decisions/0032-account-page.md documents every field.

// ExportFormat names the file's format, and ExportVersion its version.
const (
	ExportFormat  = "wathena-account-export"
	ExportVersion = 1
)

// MaxExportRows is the most lines an export holds of each log: chat,
// item log and zeny log. A log that holds more lists its oldest lines,
// and its name goes in Export.Truncated.
const MaxExportRows = 100_000

// Export is the data export.
type Export struct {
	Format     string        `json:"format"`
	Version    int           `json:"version"`
	ExportedAt time.Time     `json:"exported_at"`
	Account    ExportAccount `json:"account"`
	// Logins are the login records of the last 90 days.
	Logins []ExportLogin `json:"logins"`
	// Storages are the account's storages, which its characters share.
	Storages []ExportStorage `json:"storages"`
	// Currencies are the account's currencies.
	Currencies []ExportCurrency `json:"currencies"`
	// Variables are the mods' saved variables of the account (decision
	// 0036).
	Variables  []ExportVariable  `json:"variables"`
	Characters []ExportCharacter `json:"characters"`
	// Chat holds the lines the characters said that the chat log still
	// keeps.
	Chat []ExportChatLine `json:"chat"`
	// ItemLog and ZenyLog hold the log lines of the characters'
	// inventories, carts, vending shops and storages, and of the
	// account's storages.
	ItemLog []ExportItemLine `json:"item_log"`
	ZenyLog []ExportZenyLine `json:"zeny_log"`
	// Reports are the reports the characters filed.
	Reports []ExportReport `json:"reports"`
	// Sanctions are the sanctions on the account and its characters.
	Sanctions []ExportSanction `json:"sanctions"`
	// Truncated names the lists that reached MaxExportRows: chat,
	// item_log or zeny_log.
	Truncated []string `json:"truncated"`
}

// ExportAccount is the account itself.
type ExportAccount struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Email is always null: the account service stores no email.
	Email     *string   `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	// TwoFactor is set when the account has TOTP or a passkey on.
	TwoFactor bool `json:"two_factor"`
}

// ExportLogin is one login on the account pages.
type ExportLogin struct {
	At        time.Time `json:"at"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
}

// GameExport is the game's part of the export, which Game.Export reads.
type GameExport struct {
	Storages   []ExportStorage
	Currencies []ExportCurrency
	Variables  []ExportVariable
	Characters []ExportCharacter
	Chat       []ExportChatLine
	ItemLog    []ExportItemLine
	ZenyLog    []ExportZenyLine
	Reports    []ExportReport
	Sanctions  []ExportSanction
	Truncated  []string
}

// ExportCharacter is one character.
type ExportCharacter struct {
	ID      int64  `json:"id"`
	Slot    int    `json:"slot"`
	Name    string `json:"name"`
	Job     int    `json:"job"`
	JobName string `json:"job_name"`
	// Sex is female or male.
	Sex          string      `json:"sex"`
	BaseLevel    int         `json:"base_level"`
	JobLevel     int         `json:"job_level"`
	BaseExp      int64       `json:"base_exp"`
	JobExp       int64       `json:"job_exp"`
	Stats        ExportStats `json:"stats"`
	StatusPoints int         `json:"status_points"`
	SkillPoints  int         `json:"skill_points"`
	HP           int64       `json:"hp"`
	SP           int64       `json:"sp"`
	Zeny         int64       `json:"zeny"`
	// Position is where the character logged out, and SavePoint where
	// it returns to, or null for the world's start point.
	Position  ExportPosition  `json:"position"`
	SavePoint *ExportPosition `json:"save_point"`
	CreatedAt time.Time       `json:"created_at"`
	// Party and Guild are their names, or "".
	Party  string        `json:"party"`
	Guild  string        `json:"guild"`
	Skills []ExportSkill `json:"skills"`
	// Inventory holds the equipment too: an item whose equip is not 0
	// is worn there.
	Inventory []ExportItem `json:"inventory"`
	Cart      []ExportItem `json:"cart"`
	// Storages are the character's own extra storages.
	Storages []ExportStorage `json:"storages"`
	// Mail is the character's mailbox.
	Mail       []ExportMail     `json:"mail"`
	Currencies []ExportCurrency `json:"currencies"`
	BlockList  []string         `json:"block_list"`
	// Variables are the mods' saved variables of the character (decision
	// 0036).
	Variables []ExportVariable `json:"variables"`
}

// ExportVariable is a mod's saved variable (proposal 0002, "Variables"):
// the mod's ID, the variable's name, and its value as the mod saved it,
// a JSON number, boolean, string or object. A variable at its default
// value is not saved, so it is not in the export.
type ExportVariable struct {
	Mod   string          `json:"mod"`
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

// ExportStats are the raised stats, without bonuses.
type ExportStats struct {
	Str int `json:"str"`
	Agi int `json:"agi"`
	Vit int `json:"vit"`
	Int int `json:"int"`
	Dex int `json:"dex"`
	Luk int `json:"luk"`
}

// ExportPosition is a map and a cell.
type ExportPosition struct {
	Map string `json:"map"`
	X   int    `json:"x"`
	Y   int    `json:"y"`
}

// ExportSkill is a learned skill.
type ExportSkill struct {
	ID    int `json:"id"`
	Level int `json:"level"`
}

// ExportItem is a stack or an item instance.
type ExportItem struct {
	Serial string   `json:"serial"`
	ItemID uint32   `json:"item_id"`
	Name   string   `json:"name"`
	Amount int      `json:"amount"`
	Slot   int      `json:"slot"`
	Refine int      `json:"refine"`
	Cards  []uint32 `json:"cards"`
	// Equip is the equip location bits where the item is worn, or 0.
	Equip      uint32 `json:"equip"`
	Identified bool   `json:"identified"`
	// Bound is rAthena's bound type: 0 none, 1 account, 2 guild, 3
	// party, 4 character.
	Bound int            `json:"bound"`
	Rolls map[string]int `json:"rolls"`
}

// ExportStorage is a storage and its items.
type ExportStorage struct {
	// Kind is storage for the account's storage and extra for an extra
	// storage.
	Kind    string       `json:"kind"`
	Storage uint32       `json:"storage"`
	Name    string       `json:"name"`
	Items   []ExportItem `json:"items"`
}

// ExportCurrency is an amount of a currency.
type ExportCurrency struct {
	ID     uint32 `json:"id"`
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}

// ExportMail is a mail in a character's mailbox.
type ExportMail struct {
	ID       int64        `json:"id"`
	From     string       `json:"from"`
	Title    string       `json:"title"`
	Body     string       `json:"body"`
	SentAt   time.Time    `json:"sent_at"`
	Zeny     int64        `json:"zeny"`
	Read     bool         `json:"read"`
	Returned bool         `json:"returned"`
	Items    []ExportItem `json:"items"`
}

// ExportChatLine is a chat line a character said.
type ExportChatLine struct {
	At        time.Time `json:"at"`
	Character string    `json:"character"`
	// Kind is local, whisper, channel, party or guild.
	Kind string `json:"kind"`
	// Target is a whisper's target or a channel's name.
	Target string `json:"target"`
	Map    string `json:"map"`
	Text   string `json:"text"`
}

// ExportOwner is where a log line's items or zeny came from or went.
// Kind is an owner kind of server/transfer, such as inventory, storage,
// mail, shop or monster, and ID its ID, as decision 0017 lists.
type ExportOwner struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id"`
	Storage uint32 `json:"storage"`
}

// ExportItemLine is one line of the item log.
type ExportItemLine struct {
	At     time.Time   `json:"at"`
	Action string      `json:"action"`
	From   ExportOwner `json:"from"`
	To     ExportOwner `json:"to"`
	Serial string      `json:"serial"`
	// Source is the serial the amount came from, or "".
	Source string   `json:"source"`
	ItemID uint32   `json:"item_id"`
	Name   string   `json:"name"`
	Amount int      `json:"amount"`
	Refine int      `json:"refine"`
	Cards  []uint32 `json:"cards"`
	Bound  int      `json:"bound"`
}

// ExportZenyLine is one line of the zeny and currency log.
type ExportZenyLine struct {
	At     time.Time   `json:"at"`
	Action string      `json:"action"`
	From   ExportOwner `json:"from"`
	To     ExportOwner `json:"to"`
	Amount int64       `json:"amount"`
	// Currency is the currency's ID, or 0 for zeny.
	Currency uint32 `json:"currency"`
}

// ExportReport is a report a character filed.
type ExportReport struct {
	ID        int64     `json:"id"`
	At        time.Time `json:"at"`
	Character string    `json:"character"`
	Target    string    `json:"target"`
	Kind      string    `json:"kind"`
	Reason    string    `json:"reason"`
	// Status is open, action_taken or no_action.
	Status string `json:"status"`
}

// ExportSanction is a sanction on the account or a character.
type ExportSanction struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	// Character is "" for a ban of the account.
	Character string    `json:"character"`
	Reason    string    `json:"reason"`
	StartedAt time.Time `json:"started_at"`
	// EndsAt is null for a permanent ban.
	EndsAt   *time.Time `json:"ends_at"`
	LiftedAt *time.Time `json:"lifted_at"`
}

// ReportStatus is a report's status in the export and on the page:
// open, action_taken or no_action.
func ReportStatus(r FiledReport) string {
	switch {
	case !r.Closed:
		return "open"
	case r.ActionTaken:
		return "action_taken"
	}
	return "no_action"
}

// buildExport reads the whole export of an account.
func (s *Service) buildExport(ctx context.Context, account int64) (*Export, error) {
	acc, err := s.q.AccountOverview(ctx, account)
	if err != nil {
		return nil, err
	}
	twoFactor, err := s.q.TwoFactorOn(ctx, account)
	if err != nil {
		return nil, err
	}
	logins, err := s.q.RecentLogins(ctx, account)
	if err != nil {
		return nil, err
	}
	e := &Export{Format: ExportFormat, Version: ExportVersion, ExportedAt: time.Now().UTC(),
		Account: ExportAccount{ID: acc.ID, Name: acc.Name, CreatedAt: acc.CreatedAt.Time.UTC(), TwoFactor: twoFactor},
		Logins:  make([]ExportLogin, 0, len(logins))}
	for _, l := range logins {
		v := ExportLogin{At: l.At.Time.UTC(), UserAgent: l.UserAgent}
		if l.Ip != nil {
			v.IP = l.Ip.String()
		}
		e.Logins = append(e.Logins, v)
	}
	if s.cfg.Game != nil {
		g, err := s.cfg.Game.Export(ctx, account)
		if err != nil {
			return nil, err
		}
		e.Storages, e.Currencies, e.Variables, e.Characters = g.Storages, g.Currencies, g.Variables, g.Characters
		e.Chat, e.ItemLog, e.ZenyLog = g.Chat, g.ItemLog, g.ZenyLog
		e.Reports, e.Sanctions, e.Truncated = g.Reports, g.Sanctions, g.Truncated
	}
	nonNil(&e.Storages)
	nonNil(&e.Currencies)
	nonNil(&e.Variables)
	nonNil(&e.Characters)
	for i := range e.Characters {
		nonNil(&e.Characters[i].Variables)
	}
	nonNil(&e.Chat)
	nonNil(&e.ItemLog)
	nonNil(&e.ZenyLog)
	nonNil(&e.Reports)
	nonNil(&e.Sanctions)
	nonNil(&e.Truncated)
	return e, nil
}

// nonNil makes a nil list empty, so the JSON has [] and not null.
func nonNil[T any](l *[]T) {
	if *l == nil {
		*l = []T{}
	}
}

// dataNotices are the messages the data page shows for ?notice=.
var dataNotices = map[string]string{
	"deletion-cancelled": "The deletion is cancelled. Your characters can play again.",
	"deletion-requested": "Your account is set to be deleted. You can cancel until then.",
}

// export builds the export and sends it as a download.
func (s *Service) export(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*requestTimeout)
	defer cancel()
	sess, ok := s.accountAction(ctx, w, r)
	if !ok || !s.needRecent(w, r, sess, Prefix+"data") {
		return
	}
	refuse := func(status int, msg string) {
		p := s.newPage(w, r, "Your data")
		p.Error = msg
		if err := s.fillData(ctx, p, sess); err != nil {
			s.unavailable(w, r, err)
			return
		}
		s.render(w, status, "data.html", p)
	}
	last, err := s.q.LastExport(ctx, sess.AccountID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		s.unavailable(w, r, err)
		return
	case time.Since(last.Time) < 24*time.Hour:
		refuse(http.StatusTooManyRequests, "You can download your data once a day. Try again after "+minute(last.Time.Add(24*time.Hour))+".")
		return
	}
	e, err := s.buildExport(ctx, sess.AccountID)
	if errors.Is(err, ErrCharacterOnline) {
		refuse(http.StatusConflict, "A character of your account is in the game. Log out every character, and close shops on autotrade, then try again.")
		return
	}
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	body, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	n, err := s.q.RecordExport(ctx, sess.AccountID)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if n == 0 {
		// Another request exported meanwhile.
		refuse(http.StatusTooManyRequests, "You can download your data once a day.")
		return
	}
	s.log.Info("account data exported", "account", sess.AccountID)
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="wathena-`+sess.AccountName+`-`+e.ExportedAt.Format("2006-01-02")+`.json"`)
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
