package store

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// The staff's moderation on the game role (decision 0030):
//   - Staff sanctions are rows of sanctions, with the audit row of each
//     in the same transaction. A mute or a jail of an online character
//     is also in its zone, which saves the same row again by its start.
//   - A ban acts on an account and has no character. A permanent ban
//     ends at 'infinity'. A kick ends when it starts.
//   - A report closes with an outcome, a note and who closed it, and its
//     audit row, in one transaction.

// Kinds of sanctions.
const (
	SanctionMute = "mute"
	SanctionJail = "jail"
	SanctionKick = "kick"
	SanctionBan  = "ban"
)

// ErrNoRow is a sanction or a report that does not exist.
var ErrNoRow = errors.New("store: no such row")

// SanctionRow is one row of sanctions.
type SanctionRow struct {
	ID int64
	// CharacterID and Character are the character of a mute, a jail or
	// a kick, or 0 and "" for a ban.
	CharacterID int64
	Character   string
	AccountID   int64
	Account     string
	Kind        string
	Reason      string
	Start       time.Time
	// End is when it ends. It is zero for a permanent ban.
	End       time.Time
	Permanent bool
	By        string
	// LiftedAt is when staff lifted it, or zero.
	LiftedAt   time.Time
	LiftedBy   string
	LiftReason string
}

// Active reports whether the sanction is in effect at now.
func (r SanctionRow) Active(now time.Time) bool {
	return r.LiftedAt.IsZero() && (r.Permanent || r.End.After(now))
}

// endsAt is a sanction's end as a column value: 'infinity' for a
// permanent ban, which a zero end means.
func endsAt(end time.Time) pgtype.Timestamptz {
	if end.IsZero() {
		return pgtype.Timestamptz{InfinityModifier: pgtype.Infinity, Valid: true}
	}
	return timestamp(end)
}

// readEnd reads an end column: zero and true for 'infinity'.
func readEnd(t pgtype.Timestamptz) (time.Time, bool) {
	if t.InfinityModifier == pgtype.Infinity {
		return time.Time{}, true
	}
	return t.Time, false
}

// CharacterSanction is a mute, a jail or a kick to record.
type CharacterSanction struct {
	Character int64
	Kind      string
	Reason    string
	// Start and End are the sanction's, as the zone gave them. A kick
	// ends when it starts.
	Start, End time.Time
	By         string
	// ReturnMap, ReturnX and ReturnY are where a jailed character goes
	// back to.
	ReturnMap        string
	ReturnX, ReturnY int
}

// AddCharacterSanction records a staff's mute, jail or kick of a
// character with its audit row, and returns the sanction's ID.
func (s *Store) AddCharacterSanction(ctx context.Context, c CharacterSanction, audit AuditRow) (int64, error) {
	var id int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var err error
		id, err = q.AddCharacterSanction(ctx, db.AddCharacterSanctionParams{CharacterID: c.Character, Kind: c.Kind, Reason: c.Reason,
			StartedAt: timestamp(c.Start), EndsAt: timestamp(c.End), IssuedBy: c.By, ReturnMap: c.ReturnMap,
			ReturnX: c.ReturnX, ReturnY: c.ReturnY})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		audit.Arguments = withArg(audit.Arguments, "sanction_id", id)
		return writeAudit(ctx, q, audit)
	})
	return id, err
}

// AddBan bans an account with its audit row, and returns the ban's ID.
// A zero end is a permanent ban.
func (s *Store) AddBan(ctx context.Context, account int64, reason string, start, end time.Time, by string, audit AuditRow) (int64, error) {
	var id int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var err error
		id, err = q.AddBan(ctx, db.AddBanParams{AccountID: account, Reason: reason, StartedAt: timestamp(start),
			EndsAt: endsAt(end), IssuedBy: by})
		if err != nil {
			return err
		}
		audit.Arguments = withArg(audit.Arguments, "sanction_id", id)
		return writeAudit(ctx, q, audit)
	})
	return id, err
}

// LiftSanction ends a mute, a jail or a ban early, with its audit row.
// It returns false when the sanction was not in effect.
func (s *Store) LiftSanction(ctx context.Context, id int64, at time.Time, by, reason string, audit AuditRow) (bool, error) {
	lifted := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.LiftSanctionByID(ctx, db.LiftSanctionByIDParams{At: timestamp(at), LiftedBy: by, LiftReason: reason, ID: id})
		if err != nil || n == 0 {
			return err
		}
		lifted = true
		return writeAudit(ctx, q, audit)
	})
	return lifted, err
}

// Sanction reads a sanction. It returns ErrNoRow for one that does not
// exist.
func (s *Store) Sanction(ctx context.Context, id int64) (SanctionRow, error) {
	r, err := s.q.SanctionByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return SanctionRow{}, ErrNoRow
	}
	if err != nil {
		return SanctionRow{}, err
	}
	return sanctionRow(db.SearchSanctionsRow(r)), nil
}

// SanctionQuery filters SearchSanctions. Zero fields do not filter.
type SanctionQuery struct {
	// Character finds a character's sanctions and its account's bans.
	Character int64
	// Account finds every sanction of an account.
	Account int64
	Kind    string
	// Active keeps the sanctions in effect at Now.
	Active   bool
	Now      time.Time
	BeforeID int64
	Limit    int
}

// SearchSanctions lists sanctions newest first.
func (s *Store) SearchSanctions(ctx context.Context, sq SanctionQuery) ([]SanctionRow, error) {
	p := db.SearchSanctionsParams{Active: sq.Active, Now: timestamp(sq.Now), MaxRows: int32(min(max(sq.Limit, 1), 1000))} // #nosec G115 -- 1 to 1000.
	if sq.Now.IsZero() {
		p.Now = timestamp(time.Now())
	}
	if sq.Character > 0 {
		p.Character = pgtype.Int8{Int64: sq.Character, Valid: true}
	}
	if sq.Account > 0 {
		p.Account = pgtype.Int8{Int64: sq.Account, Valid: true}
	}
	if sq.Kind != "" {
		p.Kind = pgtype.Text{String: sq.Kind, Valid: true}
	}
	if sq.BeforeID > 0 {
		p.BeforeID = pgtype.Int8{Int64: sq.BeforeID, Valid: true}
	}
	rows, err := s.q.SearchSanctions(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]SanctionRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, sanctionRow(r))
	}
	return out, nil
}

func sanctionRow(r db.SearchSanctionsRow) SanctionRow {
	out := SanctionRow{ID: r.ID, CharacterID: r.CharacterID.Int64, Character: r.CharacterName, AccountID: r.AccountID,
		Account: r.AccountName, Kind: r.Kind, Reason: r.Reason, Start: r.StartedAt.Time, By: r.IssuedBy,
		LiftedBy: r.LiftedBy, LiftReason: r.LiftReason}
	out.End, out.Permanent = readEnd(r.EndsAt)
	if r.LiftedAt.Valid {
		out.LiftedAt = r.LiftedAt.Time
	}
	return out
}

// Ban is a ban in effect.
type Ban struct {
	ID     int64
	Reason string
	// End is zero for a permanent ban.
	End       time.Time
	Permanent bool
}

// ActiveBan returns the ban in effect of a character's account, which
// refuses the character's game logins.
func (s *Store) ActiveBan(ctx context.Context, character int64) (Ban, bool, error) {
	r, err := s.q.ActiveBanOfCharacter(ctx, character)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ban{}, false, nil
	}
	if err != nil {
		return Ban{}, false, err
	}
	b := Ban{ID: r.ID, Reason: r.Reason}
	b.End, b.Permanent = readEnd(r.EndsAt)
	return b, true, nil
}

// ActiveBans returns the bans in effect of some accounts, by account.
func (s *Store) ActiveBans(ctx context.Context, accounts []int64) (map[int64]SanctionRow, error) {
	out := map[int64]SanctionRow{}
	if len(accounts) == 0 {
		return out, nil
	}
	rows, err := s.q.ActiveBansOfAccounts(ctx, accounts)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		b := SanctionRow{ID: r.ID, AccountID: r.AccountID, Kind: SanctionBan, Reason: r.Reason, Start: r.StartedAt.Time, By: r.IssuedBy}
		b.End, b.Permanent = readEnd(r.EndsAt)
		out[r.AccountID] = b
	}
	return out, nil
}

// CharacterPlace is a character's name, account and saved position.
type CharacterPlace struct {
	ID          int64
	Name        string
	AccountID   int64
	AccountName string
	Map         string
	X, Y        int
}

// Place reads a character's name, account and saved position. It
// returns ErrNotFound for a character that does not exist.
func (s *Store) Place(ctx context.Context, id int64) (CharacterPlace, error) {
	r, err := s.q.CharacterPlace(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return CharacterPlace{}, ErrNotFound
	}
	if err != nil {
		return CharacterPlace{}, err
	}
	return CharacterPlace{ID: r.ID, Name: r.Name, AccountID: r.AccountID, AccountName: r.AccountName, Map: r.Map, X: r.X, Y: r.Y}, nil
}

// AccountName reads an account's name. It returns ErrNoRow for an
// account that does not exist.
func (s *Store) AccountName(ctx context.Context, id int64) (string, error) {
	n, err := s.q.AccountName(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoRow
	}
	return n, err
}

// AccountCharacters are the IDs of an account's characters.
func (s *Store) AccountCharacters(ctx context.Context, account int64) ([]int64, error) {
	return s.q.CharacterIDsOfAccount(ctx, account)
}

// ReportRow is one report of the queue.
type ReportRow struct {
	ID                int64
	At                time.Time
	ReporterID        int64
	Reporter          string
	TargetID          int64
	Target            string
	TargetAccount     int64
	Line              chat.LineID
	Reason, Map       string
	Status            string
	ClosedAt          time.Time
	Outcome, Note, By string
	SanctionID        int64
	Guild             int64
	EmblemVersion     int
	EmblemPNG         []byte
}

// Kind is player, line, guild_name or guild_emblem.
func (r ReportRow) Kind() string {
	switch {
	case r.Guild != 0 && r.EmblemVersion != 0:
		return "guild_emblem"
	case r.Guild != 0:
		return "guild_name"
	case !r.Line.IsZero():
		return "line"
	}
	return "player"
}

// ReportQuery filters Reports. Empty fields do not filter.
type ReportQuery struct {
	// Status is open or closed.
	Status string
	// Target is a character or guild name, in any case.
	Target string
	// Kind is player, line, guild_name or guild_emblem.
	Kind     string
	BeforeID int64
	Limit    int
}

// Reports lists reports newest first.
func (s *Store) Reports(ctx context.Context, rq ReportQuery) ([]ReportRow, error) {
	p := db.SearchReportsParams{MaxRows: int32(min(max(rq.Limit, 1), 1000))} // #nosec G115 -- 1 to 1000.
	if rq.Status != "" {
		p.Status = pgtype.Text{String: rq.Status, Valid: true}
	}
	if rq.Target != "" {
		p.Target = pgtype.Text{String: rq.Target, Valid: true}
	}
	if rq.Kind != "" {
		p.Kind = pgtype.Text{String: rq.Kind, Valid: true}
	}
	if rq.BeforeID > 0 {
		p.BeforeID = pgtype.Int8{Int64: rq.BeforeID, Valid: true}
	}
	rows, err := s.q.SearchReports(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]ReportRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, reportRow(db.ReportByIDRow{ID: r.ID, CreatedAt: r.CreatedAt, ReporterID: r.ReporterID,
			ReporterName: r.ReporterName, TargetID: r.TargetID, TargetName: r.TargetName, LineID: r.LineID, Reason: r.Reason,
			Map: r.Map, Status: r.Status, ClosedAt: r.ClosedAt, Outcome: r.Outcome, Note: r.Note, ClosedBy: r.ClosedBy,
			SanctionID: r.SanctionID, GuildID: r.GuildID, EmblemVersion: r.EmblemVersion, TargetAccountID: r.TargetAccountID}))
	}
	return out, nil
}

func reportRow(r db.ReportByIDRow) ReportRow {
	out := ReportRow{ID: r.ID, At: r.CreatedAt.Time, ReporterID: r.ReporterID, Reporter: r.ReporterName, TargetID: r.TargetID,
		Target: r.TargetName, TargetAccount: r.TargetAccountID, Reason: r.Reason, Map: r.Map, Status: r.Status,
		Outcome: r.Outcome, Note: r.Note, By: r.ClosedBy, SanctionID: r.SanctionID.Int64, Guild: r.GuildID,
		EmblemVersion: r.EmblemVersion, EmblemPNG: r.EmblemPng}
	if r.LineID.Valid {
		out.Line = r.LineID.Bytes
	}
	if r.ClosedAt.Valid {
		out.ClosedAt = r.ClosedAt.Time
	}
	return out
}

// ReportLine is a chat line of a report's detail.
type ReportLine struct {
	chat.Line
	// Seen is set for the lines the reporter saw, which the report
	// keeps. The others are the target's own lines around it.
	Seen bool
}

// AroundReport is how far before and after a report the detail looks
// for the target's own lines, as the 5 minutes a report keeps.
const AroundReport = chat.ContextWindow

// maxReportLines is the most lines a report's detail shows.
const maxReportLines = 400

// Report reads a report with its lines: the lines the reporter saw, and
// the target's own lines from AroundReport before to AroundReport after
// the report, oldest first. It returns ErrNoRow for a report that does
// not exist.
func (s *Store) Report(ctx context.Context, id int64) (ReportRow, []ReportLine, error) {
	r, err := s.q.ReportByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReportRow{}, nil, ErrNoRow
	}
	if err != nil {
		return ReportRow{}, nil, err
	}
	rep := reportRow(r)
	seen, err := s.q.ReportLines(ctx, id)
	if err != nil {
		return rep, nil, err
	}
	byID := map[chat.LineID]bool{}
	var lines []ReportLine
	for _, ln := range seen {
		l := ReportLine{Line: chat.Line{ID: ln.ID.Bytes, At: ln.At.Time, Kind: ln.Kind, CharacterID: ln.CharacterID,
			Name: ln.Name, To: ln.Target, Map: ln.Map, Text: ln.Text}, Seen: true}
		byID[l.ID] = true
		lines = append(lines, l)
	}
	if rep.TargetID > 0 {
		around, err := s.q.ChatAround(ctx, db.ChatAroundParams{CharacterID: rep.TargetID, Since: timestamp(rep.At.Add(-AroundReport)),
			Until: timestamp(rep.At.Add(AroundReport)), MaxRows: maxReportLines})
		if err != nil {
			return rep, nil, err
		}
		for _, ln := range around {
			if byID[ln.ID.Bytes] {
				continue
			}
			lines = append(lines, ReportLine{Line: chat.Line{ID: ln.ID.Bytes, At: ln.At.Time, Kind: ln.Kind,
				CharacterID: ln.CharacterID, Name: ln.Name, To: ln.Target, Map: ln.Map, Text: ln.Text}})
		}
	}
	slices.SortStableFunc(lines, func(a, b ReportLine) int { return a.At.Compare(b.At) })
	if len(lines) > maxReportLines {
		lines = lines[len(lines)-maxReportLines:]
	}
	return rep, lines, nil
}

// CloseReport closes an open report with an outcome, a note, who closed
// it and the sanction it links, with its audit row. It returns false
// for a report that is not open.
func (s *Store) CloseReport(ctx context.Context, id int64, outcome, note, by string, sanction int64, audit AuditRow) (bool, error) {
	closed := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.CloseReportWithOutcome(ctx, db.CloseReportWithOutcomeParams{At: timestamp(time.Now()), Outcome: outcome,
			Note: note, ClosedBy: by, SanctionID: pgtype.Int8{Int64: sanction, Valid: sanction > 0}, ID: id})
		if err != nil || n == 0 {
			return err
		}
		closed = true
		return writeAudit(ctx, q, audit)
	})
	return closed, err
}

// withArg returns args with one more argument.
func withArg(args map[string]any, key string, v any) map[string]any {
	out := make(map[string]any, len(args)+1)
	for k, x := range args {
		out[k] = x
	}
	out[key] = v
	return out
}
