package account

import (
	"net/http"
	"strconv"
	"strings"
)

// The privacy notice (strategy section 7.10, decision 0032), at
// /account/privacy. Anyone may read it, logged in or not. The server
// owner writes the contact details and a text in the config. The page
// lists the retention periods in effect.

// Privacy is the privacy notice's config.
type Privacy struct {
	// Contact is how players reach the server's owner about their data,
	// such as an email address. Text is the owner's own notice. The page
	// shows both as plain text, with paragraphs at blank lines.
	Contact, Text string
	// The retention periods in effect, in days (the retention block,
	// decision 0049). Each is the default of strategy section 7.10 when
	// 0: 90, 30, 180 and 730 days.
	// LoginIPDays is also how far back the data export looks for login
	// records, so it holds every record the server keeps.
	LoginIPDays, ChatLogDays, ItemLogDays, AuditDays int
}

// loginDays is the login records' period in effect.
func (p Privacy) loginDays() int { return orDefault(p.LoginIPDays, 90) }

// retentionRow is one line of the retention table.
type retentionRow struct {
	Data, Kept, Notes string
}

// paragraphs splits a text at its blank lines.
func paragraphs(text string) []string {
	var out []string
	for _, p := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func days(n int) string {
	switch {
	case n == 1:
		return "1 day"
	case n == 365:
		return "1 year"
	case n%365 == 0:
		return strconv.Itoa(n/365) + " years"
	}
	return strconv.Itoa(n) + " days"
}

// orDefault is n, or def when n is 0.
func orDefault(n, def int) int {
	if n <= 0 {
		return def
	}
	return n
}

// retention lists what the server keeps and for how long.
func (s *Service) retention() []retentionRow {
	p := s.cfg.Privacy
	chat := orDefault(p.ChatLogDays, 30)
	return []retentionRow{
		{"Account name, password hash and two-factor settings", "Life of the account", "Deleted with the account."},
		{"Login records with IP addresses", days(p.loginDays()), "Used to find ban evasion."},
		{"Login sessions", "Up to " + days(int(s.cfg.SessionLifetime.Hours()/24)), "Ended at logout, or after " + days(int(s.cfg.SessionIdle.Hours()/24)) + " unused."},
		{"Characters, items, mail and block lists", "Life of the account", "Deleted with the account."},
		{"Chat lines", days(chat), "Lines that an open report holds are kept until the report closes. A deleted account's lines go with it."},
		{"Reports", "Kept", "A report keeps the reporter's and the target's character IDs and names, also after an account is deleted."},
		{"Sanctions", "Life of the account", "The staff audit log keeps a record of each sanction."},
		{"Item and zeny logs", days(orDefault(p.ItemLogDays, 180)), "Needed to trace duplication and theft. They keep character IDs after an account is deleted. Old lines are deleted a month at a time, so a line may be kept up to 31 days longer."},
		{"Staff audit log", days(orDefault(p.AuditDays, 730)), "Records what staff did, with the IDs of the accounts and characters they acted on. A row that waits for a staff review is kept until it is reviewed."},
		{"Backups", "Up to 4 weeks", "Backups are encrypted copies of the data above. With the default backup schedule, data deleted from the server stays in backups for up to 4 weeks."},
	}
}

func (s *Service) getPrivacy(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "Privacy notice")
	p.Contact = s.cfg.Privacy.Contact
	p.OwnerText = paragraphs(s.cfg.Privacy.Text)
	p.Retention = s.retention()
	p.DeletionDays = int(DeletionWait.Hours() / 24)
	s.render(w, http.StatusOK, "privacy.html", p)
}
