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
	// ChatLogDays is how many days chat lines are kept (chat.log_days).
	// 30 when 0.
	ChatLogDays int
}

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
	if n == 1 {
		return "1 day"
	}
	return strconv.Itoa(n) + " days"
}

// retention lists what the server keeps and for how long.
func (s *Service) retention() []retentionRow {
	chat := s.cfg.Privacy.ChatLogDays
	if chat <= 0 {
		chat = 30
	}
	return []retentionRow{
		{"Account name, password hash and two-factor settings", "Life of the account", "Deleted with the account."},
		{"Login records with IP addresses", "90 days", "Used to find ban evasion."},
		{"Login sessions", "Up to " + days(int(s.cfg.SessionLifetime.Hours()/24)), "Ended at logout, or after " + days(int(s.cfg.SessionIdle.Hours()/24)) + " unused."},
		{"Characters, items, mail and block lists", "Life of the account", "Deleted with the account."},
		{"Chat lines", days(chat), "Lines that an open report holds are kept until the report closes. A deleted account's lines go with it."},
		{"Reports", "Kept", "A report keeps the reporter's and the target's character IDs and names, also after an account is deleted."},
		{"Sanctions", "Life of the account", "The staff audit log keeps a record of each sanction."},
		{"Item and zeny logs", "180 days", "Needed to trace duplication and theft. They keep character IDs after an account is deleted. The job that deletes old lines comes in a later version, so until then lines are kept."},
		{"Staff audit log", "2 years", "Records what staff did, with the IDs of the accounts and characters they acted on. The job that deletes old rows comes in a later version."},
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
