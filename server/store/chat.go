package store

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Block lists and sanctions are saved with the character. The chat log
// and reports are written by ChatLog, on its own goroutine (decision
// 0018).

func timestamp(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func uuidOf(id chat.LineID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

// loadModeration reads a character's block list and the sanctions that
// have not ended.
func loadModeration(ctx context.Context, q *db.Queries, c *character.Character) error {
	blocks, err := q.CharacterBlocks(ctx, c.ID)
	if err != nil {
		return err
	}
	if len(blocks) > 0 {
		c.Blocks = blocks
	}
	rows, err := q.ActiveSanctions(ctx, c.ID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		c.Sanctions = append(c.Sanctions, character.Sanction{
			Kind: r.Kind, Reason: r.Reason, Start: r.StartedAt.Time, End: r.EndsAt.Time, By: r.IssuedBy,
			ReturnMap: r.ReturnMap, ReturnX: r.ReturnX, ReturnY: r.ReturnY,
		})
	}
	return nil
}

// writeModeration writes the block list when it changed, the sanctions
// that are new or changed, and records the end of the sanctions that
// went before their time.
func writeModeration(ctx context.Context, q *db.Queries, id int64, base, c *character.Character) error {
	if !slices.Equal(base.Blocks, c.Blocks) {
		if err := q.DeleteCharacterBlocks(ctx, id); err != nil {
			return err
		}
		if len(c.Blocks) > 0 {
			p := db.InsertCharacterBlocksParams{CharacterID: id, Names: c.Blocks}
			for i := range c.Blocks {
				p.Positions = append(p.Positions, i)
			}
			if err := q.InsertCharacterBlocks(ctx, p); err != nil {
				return err
			}
		}
	}
	key := func(s *character.Sanction) string { return s.Kind + " " + s.Start.UTC().Format(time.RFC3339Nano) }
	was := map[string]*character.Sanction{}
	for i := range base.Sanctions {
		was[key(&base.Sanctions[i])] = &base.Sanctions[i]
	}
	for i := range c.Sanctions {
		s := &c.Sanctions[i]
		k := key(s)
		prev := was[k]
		delete(was, k)
		if prev != nil && prev.Equal(s) {
			continue
		}
		if err := q.UpsertSanction(ctx, db.UpsertSanctionParams{
			CharacterID: id, Kind: s.Kind, Reason: s.Reason, StartedAt: timestamp(s.Start), EndsAt: timestamp(s.End),
			IssuedBy: s.By, ReturnMap: s.ReturnMap, ReturnX: s.ReturnX, ReturnY: s.ReturnY,
		}); err != nil {
			return err
		}
	}
	// A sanction that left the list before its end was lifted. One that
	// ran out needs nothing: LiftSanction leaves it.
	now := time.Now()
	for _, s := range was {
		if err := q.LiftSanction(ctx, db.LiftSanctionParams{At: timestamp(now), CharacterID: id, Kind: s.Kind, StartedAt: timestamp(s.Start)}); err != nil {
			return err
		}
	}
	return nil
}

// Defaults of ChatLogOptions.
const (
	// DefaultChatQueue is how many lines and reports may wait for the
	// database. More are dropped, and the drops are logged.
	DefaultChatQueue = 10_000
	// DefaultChatRetention is how long chat lines are kept (strategy
	// section 7.10).
	DefaultChatRetention = 30 * 24 * time.Hour
	// chatBatch is the most lines one transaction writes.
	chatBatch = 500
	// chatFlush is how long a line waits for others to join its batch.
	chatFlush = time.Second
	// retentionInterval is how often old lines are deleted.
	retentionInterval = 12 * time.Hour
)

// ChatLogOptions configure a ChatLog.
type ChatLogOptions struct {
	// Retention is how long lines are kept, except those of open
	// reports. DefaultChatRetention when 0.
	Retention time.Duration
	// Queue is DefaultChatQueue when 0.
	Queue int
	Log   *slog.Logger
}

// ChatLog writes the chat log and the reports to the database. It
// implements chat.Log: Line and Report never block. Lines and reports
// are written in the order they came, so a report's lines are in the
// log before the report.
type ChatLog struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	log       *slog.Logger
	retention time.Duration
	in        chan chatItem
	stop      chan struct{}
	done      sync.WaitGroup

	mu      sync.Mutex
	dropped int
}

type chatItem struct {
	line   *chat.Line
	report *chat.Report
}

// NewChatLog starts a chat log on a pool. Close stops it.
func NewChatLog(pool *pgxpool.Pool, opt ChatLogOptions) *ChatLog {
	l := &ChatLog{pool: pool, q: db.New(pool), log: opt.Log, retention: opt.Retention, stop: make(chan struct{})}
	if l.log == nil {
		l.log = slog.Default()
	}
	if l.retention <= 0 {
		l.retention = DefaultChatRetention
	}
	n := opt.Queue
	if n <= 0 {
		n = DefaultChatQueue
	}
	l.in = make(chan chatItem, n)
	l.done.Go(l.write)
	l.done.Go(l.expire)
	return l
}

// Line queues a line.
func (l *ChatLog) Line(line chat.Line) { l.push(chatItem{line: &line}) }

// Report queues a report.
func (l *ChatLog) Report(r chat.Report) { l.push(chatItem{report: &r}) }

func (l *ChatLog) push(it chatItem) {
	select {
	case l.in <- it:
	default:
		l.mu.Lock()
		l.dropped++
		n := l.dropped
		l.mu.Unlock()
		if n == 1 || n%1000 == 0 {
			l.log.Warn("the chat log is behind; lines are dropped", "dropped", n)
		}
	}
}

// Close writes what waits, up to ctx, and stops the writer.
func (l *ChatLog) Close(ctx context.Context) error {
	close(l.stop)
	done := make(chan struct{})
	go func() {
		l.done.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// write batches the queue into transactions.
func (l *ChatLog) write() {
	var batch []chatItem
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		if err := l.writeBatch(ctx, batch); err != nil {
			l.log.Error("cannot write the chat log", "lines", len(batch), "err", err)
		}
		cancel()
		batch = batch[:0]
	}
	t := time.NewTimer(chatFlush)
	defer t.Stop()
	for {
		select {
		case it := <-l.in:
			batch = append(batch, it)
			if len(batch) >= chatBatch {
				flush()
			}
		case <-t.C:
			flush()
			t.Reset(chatFlush)
		case <-l.stop:
			for {
				select {
				case it := <-l.in:
					batch = append(batch, it)
					if len(batch) >= chatBatch {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// writeBatch writes lines and reports in one transaction, in order.
func (l *ChatLog) writeBatch(ctx context.Context, items []chatItem) error {
	return pgx.BeginFunc(ctx, l.pool, func(tx pgx.Tx) error {
		q := l.q.WithTx(tx)
		var p db.InsertChatLinesParams
		writeLines := func() error {
			if len(p.Ids) == 0 {
				return nil
			}
			err := q.InsertChatLines(ctx, p)
			p = db.InsertChatLinesParams{}
			return err
		}
		for _, it := range items {
			if ln := it.line; ln != nil {
				p.Ids = append(p.Ids, uuidOf(ln.ID))
				p.Ats = append(p.Ats, timestamp(ln.At))
				p.Kinds = append(p.Kinds, ln.Kind)
				p.CharacterIds = append(p.CharacterIds, ln.CharacterID)
				p.Names = append(p.Names, ln.Name)
				p.Targets = append(p.Targets, ln.To)
				p.Maps = append(p.Maps, ln.Map)
				p.Texts = append(p.Texts, ln.Text)
				continue
			}
			if err := writeLines(); err != nil {
				return err
			}
			if err := writeReport(ctx, q, it.report); err != nil {
				return err
			}
		}
		return writeLines()
	})
}

func writeReport(ctx context.Context, q *db.Queries, r *chat.Report) error {
	p := db.InsertReportParams{CreatedAt: timestamp(r.At), ReporterID: r.ReporterID, ReporterName: r.Reporter,
		TargetID: r.TargetID, TargetName: r.Target, Reason: r.Reason, Map: r.Map}
	if !r.Line.IsZero() {
		p.LineID = uuidOf(r.Line)
	}
	id, err := q.InsertReport(ctx, p)
	if err != nil {
		return err
	}
	if len(r.Context) == 0 {
		return nil
	}
	lp := db.InsertReportLinesParams{ReportID: id}
	for _, ln := range r.Context {
		lp.LineIds = append(lp.LineIds, uuidOf(ln.ID))
	}
	return q.InsertReportLines(ctx, lp)
}

// expire deletes old lines at start and every retentionInterval.
func (l *ChatLog) expire() {
	t := time.NewTicker(retentionInterval)
	defer t.Stop()
	for {
		if _, err := l.DeleteOld(context.Background(), time.Now().Add(-l.retention)); err != nil {
			l.log.Warn("cannot delete old chat lines", "err", err)
		}
		select {
		case <-t.C:
		case <-l.stop:
			return
		}
	}
}

// DeleteOld deletes the lines from before a time, except those of open
// reports, and returns how many went.
func (l *ChatLog) DeleteOld(ctx context.Context, before time.Time) (int64, error) {
	var total int64
	for {
		ctx, cancel := context.WithTimeout(ctx, saveTimeout)
		n, err := l.q.DeleteOldChatLines(ctx, timestamp(before))
		cancel()
		total += n
		if err != nil || n < 10000 {
			return total, err
		}
	}
}

// StoredReport is a saved report with its chat lines.
type StoredReport struct {
	ID     int64
	Status string
	chat.Report
}

// Reports reads the newest reports with their lines, newest first, for
// the staff tools and the tests.
func (l *ChatLog) Reports(ctx context.Context, limit int) ([]StoredReport, error) {
	return readReports(ctx, l.q, limit)
}

func readReports(ctx context.Context, q *db.Queries, limit int) ([]StoredReport, error) {
	rows, err := q.Reports(ctx, int32(min(max(limit, 1), 1000))) // #nosec G115 -- 1 to 1000.
	if err != nil {
		return nil, err
	}
	out := make([]StoredReport, 0, len(rows))
	for _, r := range rows {
		s := StoredReport{ID: r.ID, Status: r.Status, Report: chat.Report{
			At: r.CreatedAt.Time, ReporterID: r.ReporterID, Reporter: r.ReporterName, TargetID: r.TargetID,
			Target: r.TargetName, Reason: r.Reason, Map: r.Map,
		}}
		if r.LineID.Valid {
			s.Line = r.LineID.Bytes
		}
		lines, err := q.ReportLines(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		for _, ln := range lines {
			s.Context = append(s.Context, chat.Line{ID: ln.ID.Bytes, At: ln.At.Time, Kind: ln.Kind, CharacterID: ln.CharacterID,
				Name: ln.Name, To: ln.Target, Map: ln.Map, Text: ln.Text})
		}
		out = append(out, s)
	}
	return out, nil
}

// CloseReport closes an open report. Its lines then follow the
// retention period. It returns false for a report that is not open.
func (l *ChatLog) CloseReport(ctx context.Context, id int64) (bool, error) {
	n, err := l.q.CloseReport(ctx, db.CloseReportParams{At: timestamp(time.Now()), ID: id})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
