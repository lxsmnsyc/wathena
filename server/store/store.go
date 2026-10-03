// Package store keeps accounts and characters in PostgreSQL (strategy
// sections 3.5 and 4.1, item 5).
//
// How saving works:
//   - Load starts a session for a character. It raises the character's
//     save version, so saves from any older session fail.
//   - The zone hands copies of the character to Session.Save, which never
//     blocks. A pool of workers writes them.
//   - Each session writes its saves in order, one at a time. A newer copy
//     replaces one that is still waiting, since every copy is complete.
//   - A save writes the character row and the inventory slots that
//     changed in one transaction. The row is only written when the save
//     version is the one the session holds.
//   - A failed save is tried again after RetryDelay, unless a newer copy
//     came in. A save that lost its version is dropped and logged, the
//     session saves nothing more, and OnStale tells its owner.
//   - Load waits for the character's saves still in the queue, so a new
//     session reads what the last one saved.
//   - Commit writes the characters that transfers changed, with the
//     transfers' log lines, in one transaction (commit.go). Commits are
//     written one at a time, in the order they came. A session's saves
//     that came after its commit wait until the commit is written, and
//     its saves that came before are dropped, since the commit's copy is
//     newer.
//
// Game code uses only the queries sqlc generates in server/store/db,
// which always bind their parameters.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Defaults for Options.
const (
	DefaultWorkers    = 4
	DefaultRetryDelay = 5 * time.Second
	// saveTimeout bounds one save transaction.
	saveTimeout = 10 * time.Second
)

// ErrStale is a save from a session whose character was loaded again
// since, such as by a newer login.
var ErrStale = errors.New("store: the character was loaded by a newer session")

// ErrNotFound is a character that does not exist.
var ErrNotFound = errors.New("store: no such character")

// Options configure a Store.
type Options struct {
	// Workers is how many saves run at once. It is DefaultWorkers when 0.
	Workers int
	// RetryDelay is the wait before a failed save runs again. It is
	// DefaultRetryDelay when 0.
	RetryDelay time.Duration
	// Log gets save failures. It is slog.Default() when nil.
	Log *slog.Logger
}

// Store loads and saves characters.
type Store struct {
	pool  *pgxpool.Pool
	q     *db.Queries
	log   *slog.Logger
	retry time.Duration

	mu    sync.Mutex
	queue []*Session
	// busy counts, by character ID, the sessions with a save waiting or
	// running. changed closes, and is replaced, when a count drops.
	busy    map[int64]int
	changed chan struct{}
	wake    chan struct{}
	stop    chan struct{}
	workers sync.WaitGroup
	// lane holds the commits not written yet, in order (commit.go).
	// laneWake wakes the commit worker.
	lane     []*commit
	laneWake chan struct{}
}

// New makes a store on a pool and starts its workers. Close stops them.
// The pool stays open.
func New(pool *pgxpool.Pool, opt Options) *Store {
	s := &Store{
		pool:    pool,
		q:       db.New(pool),
		log:     opt.Log,
		retry:   opt.RetryDelay,
		busy:    map[int64]int{},
		changed: make(chan struct{}),
		stop:    make(chan struct{}),
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	if s.retry <= 0 {
		s.retry = DefaultRetryDelay
	}
	n := opt.Workers
	if n <= 0 {
		n = DefaultWorkers
	}
	s.wake = make(chan struct{}, n)
	s.laneWake = make(chan struct{}, 1)
	for range n {
		s.workers.Go(s.work)
	}
	s.workers.Go(s.commitWork)
	s.workers.Go(s.partitionWork)
	return s
}

// Close waits for waiting and running saves until ctx ends, then stops
// the workers. It returns an error naming how many characters were left
// unsaved.
func (s *Store) Close(ctx context.Context) error {
	err := s.Drain(ctx)
	close(s.stop)
	s.workers.Wait()
	return err
}

// Drain waits until no save or commit is waiting or running, or ctx
// ends.
func (s *Store) Drain(ctx context.Context) error {
	for {
		s.mu.Lock()
		n, commits, ch := len(s.busy), len(s.lane), s.changed
		s.mu.Unlock()
		if n == 0 && commits == 0 {
			return nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return fmt.Errorf("store: %d characters and %d transfer commits not saved: %w", n, commits, ctx.Err())
		}
	}
}

// Session is one login of a character. It saves the character in the
// order Save gets it. It implements zone.Saver.
type Session struct {
	s  *Store
	id int64

	// The fields below are guarded by s.mu.
	// version is the save version the database holds for this session.
	version int64
	// saved is what the database holds, which the next save compares
	// its inventory to.
	saved *character.Character
	// pending is the newest copy not written yet. final is set once the
	// session's last save came in.
	pending *character.Character
	final   bool
	// active is set while a save waits, runs, or waits to run again.
	active bool
	// running is set while a save of the session is being written.
	// lane counts the commits not written yet that save the session.
	// parked is set when a save waits for those commits.
	running bool
	lane    int
	parked  bool
	ended   bool
	err     error
	// stale is set once a save failed with ErrStale. onStale is called
	// then, on its own goroutine.
	stale   bool
	onStale func()
}

// Load starts a session for a character and returns the character. It
// first waits for saves that earlier sessions of the character still
// have in the queue.
func (s *Store) Load(ctx context.Context, id int64) (*character.Character, *Session, error) {
	if err := s.idle(ctx, id); err != nil {
		return nil, nil, err
	}
	var c *character.Character
	var version int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		row, err := q.ClaimCharacter(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		items, err := q.Inventory(ctx, id)
		if err != nil {
			return err
		}
		c, err = fromRow(&row, items)
		if err != nil {
			return err
		}
		version = row.SaveVersion
		if err := loadStatuses(ctx, q, c); err != nil {
			return err
		}
		if err := loadModeration(ctx, q, c); err != nil {
			return err
		}
		return loadVariables(ctx, q, c)
	})
	if err != nil {
		return nil, nil, err
	}
	return c, &Session{s: s, id: id, version: version, saved: c}, nil
}

// idle waits until no session of a character has a save waiting or
// running.
func (s *Store) idle(ctx context.Context, id int64) error {
	for {
		s.mu.Lock()
		n, ch := s.busy[id], s.changed
		s.mu.Unlock()
		if n == 0 {
			return nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// OnStale sets a function to call once when a save of this session is
// rejected because a newer session loaded the character. The session
// saves nothing after that. f runs on its own goroutine, so it may block
// without holding up the store. When the session already went stale, f
// runs at once.
func (x *Session) OnStale(f func()) {
	s := x.s
	s.mu.Lock()
	defer s.mu.Unlock()
	x.onStale = f
	if x.stale && f != nil {
		go f()
	}
}

// ID is the character's database ID.
func (x *Session) ID() int64 { return x.id }

// Save queues a copy of the character to be written. It never blocks.
// The store owns c afterwards. final ends the session once c is written:
// later saves are ignored.
func (x *Session) Save(c *character.Character, final bool) {
	s := x.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if x.ended || x.final {
		return
	}
	x.pending, x.final = c, final
	if !x.active {
		x.active = true
		s.busy[x.id]++
		s.push(x)
	}
}

// Flush waits until the session has no save waiting or running, and
// returns the error of its last save, if any.
func (x *Session) Flush(ctx context.Context) error {
	s := x.s
	for {
		s.mu.Lock()
		active, err, ch := x.active, x.err, s.changed
		s.mu.Unlock()
		if !active {
			return err
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// push queues a session for a worker. s.mu must be held.
func (s *Store) push(x *Session) {
	s.queue = append(s.queue, x)
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Store) work() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.mu.Unlock()
			select {
			case <-s.wake:
				continue
			case <-s.stop:
				return
			}
		}
		x := s.queue[0]
		s.queue[0] = nil
		s.queue = s.queue[1:]
		if x.lane > 0 {
			// A commit of the session comes first. It queues the
			// session again once it is written.
			x.parked = true
			s.mu.Unlock()
			continue
		}
		if x.pending == nil {
			// A commit replaced the copy that waited.
			s.idleSession(x)
			s.mu.Unlock()
			continue
		}
		c, final, base, version := x.pending, x.final, x.saved, x.version
		x.pending = nil
		x.running = true
		s.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		err := s.write(ctx, x.id, version, base, c)
		cancel()
		s.finish(x, c, final, err)
	}
}

// finish records a save's result and queues the session again when it
// has more to write.
func (s *Store) finish(x *Session, c *character.Character, final bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x.err = err
	x.running = false
	switch {
	case err == nil:
		x.saved = c
		x.version++
	case errors.Is(err, ErrStale):
		// Another session owns the character now. Nothing this session
		// holds may be written.
		s.log.Warn("save dropped: the character was loaded by a newer session", "character", x.id)
		s.goStale(x)
	default:
		s.log.Error("save failed; trying again", "character", x.id, "err", err)
		if x.pending == nil {
			x.pending = c
			x.final = x.final || final
		}
		time.AfterFunc(s.retry, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.push(x)
		})
		s.broadcast()
		return
	}
	if x.pending != nil {
		s.push(x)
		s.broadcast()
		return
	}
	if final {
		x.ended = true
	}
	s.idleSession(x)
}

// goStale ends a session whose character a newer session loaded.
// s.mu must be held.
func (s *Store) goStale(x *Session) {
	x.pending, x.ended = nil, true
	if !x.stale {
		x.stale = true
		if x.onStale != nil {
			go x.onStale()
		}
	}
}

// idleSession marks a session as having no save waiting or running.
// s.mu must be held.
func (s *Store) idleSession(x *Session) {
	x.active = false
	s.done(x.id)
}

// done counts one save or commit of a character as finished, and wakes
// those who wait. s.mu must be held.
func (s *Store) done(id int64) {
	if s.busy[id]--; s.busy[id] <= 0 {
		delete(s.busy, id)
	}
	s.broadcast()
}

// broadcast wakes everything that waits on s.changed. s.mu must be held.
func (s *Store) broadcast() {
	close(s.changed)
	s.changed = make(chan struct{})
}

// write saves a character in one transaction. base is what the database
// holds for the session, so only changed inventory slots are written.
func (s *Store) write(ctx context.Context, id, version int64, base, c *character.Character) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return writeCharacter(ctx, s.q.WithTx(tx), id, version, base, c)
	})
}

// writeCharacter saves a character in a transaction. It returns ErrStale
// before it writes anything when the save version is not the session's.
func writeCharacter(ctx context.Context, q *db.Queries, id, version int64, base, c *character.Character) error {
	if c.ID != id {
		return fmt.Errorf("store: save of character %d in the session of %d", c.ID, id)
	}
	n, err := q.SaveCharacter(ctx, saveParams(c, version))
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrStale
	}
	del, ins := diffItems(base.Items, c.Items)
	if len(del) > 0 {
		if err := q.DeleteInventorySlots(ctx, db.DeleteInventorySlotsParams{CharacterID: id, Slots: del}); err != nil {
			return err
		}
	}
	if len(ins) > 0 {
		if err := q.InsertInventory(ctx, insertParams(id, ins)); err != nil {
			return err
		}
	}
	if err := writeStatuses(ctx, q, id, base, c); err != nil {
		return err
	}
	if err := writeModeration(ctx, q, id, base, c); err != nil {
		return err
	}
	return writeVariables(ctx, q, c, base.Variables, c.Variables)
}

// loadVariables reads a character's variables and its account's.
func loadVariables(ctx context.Context, q *db.Queries, c *character.Character) error {
	chars, err := q.CharacterVariables(ctx, c.ID)
	if err != nil {
		return err
	}
	accounts, err := q.AccountVariables(ctx, c.AccountID)
	if err != nil {
		return err
	}
	if len(chars)+len(accounts) == 0 {
		return nil
	}
	c.Variables = make(map[character.Variable]string, len(chars)+len(accounts))
	for _, v := range chars {
		c.Variables[character.Variable{Mod: v.ModID, Name: v.Name}] = string(v.Value)
	}
	for _, v := range accounts {
		c.Variables[character.Variable{Mod: v.ModID, Name: v.Name, Account: true}] = string(v.Value)
	}
	return nil
}

// writeVariables writes the variables that changed from old to cur: new
// and changed values, and deletions of those that went back to their
// default. Account variables only change in the rows this character
// changed, so two characters of an account write different names.
func writeVariables(ctx context.Context, q *db.Queries, c *character.Character, old, cur map[character.Variable]string) error {
	var set, del [2]struct {
		mods, names []string
		vals        [][]byte
	}
	idx := func(v character.Variable) int {
		if v.Account {
			return 1
		}
		return 0
	}
	for k, v := range cur {
		if prev, ok := old[k]; ok && prev == v {
			continue
		}
		s := &set[idx(k)]
		s.mods, s.names, s.vals = append(s.mods, k.Mod), append(s.names, k.Name), append(s.vals, []byte(v))
	}
	for k := range old {
		if _, ok := cur[k]; !ok {
			d := &del[idx(k)]
			d.mods, d.names = append(d.mods, k.Mod), append(d.names, k.Name)
		}
	}
	if len(del[0].mods) > 0 {
		if err := q.DeleteCharacterVariables(ctx, db.DeleteCharacterVariablesParams{CharacterID: c.ID, ModIds: del[0].mods, Names: del[0].names}); err != nil {
			return err
		}
	}
	if len(set[0].mods) > 0 {
		if err := q.UpsertCharacterVariables(ctx, db.UpsertCharacterVariablesParams{CharacterID: c.ID, ModIds: set[0].mods, Names: set[0].names, Vals: set[0].vals}); err != nil {
			return err
		}
	}
	if len(del[1].mods) > 0 {
		if err := q.DeleteAccountVariables(ctx, db.DeleteAccountVariablesParams{AccountID: c.AccountID, ModIds: del[1].mods, Names: del[1].names}); err != nil {
			return err
		}
	}
	if len(set[1].mods) > 0 {
		return q.UpsertAccountVariables(ctx, db.UpsertAccountVariablesParams{AccountID: c.AccountID, ModIds: set[1].mods, Names: set[1].names, Vals: set[1].vals})
	}
	return nil
}

// diffItems returns the slots whose rows must go and the items whose rows
// must be written, to turn the inventory from old into new.
func diffItems(old, cur []character.Item) (del []int, ins []character.Item) {
	was := make(map[int]character.Item, len(old))
	for _, it := range old {
		was[it.Slot] = it
	}
	for _, it := range cur {
		prev, ok := was[it.Slot]
		delete(was, it.Slot)
		if ok && prev.Equal(&it) {
			continue
		}
		if ok {
			del = append(del, it.Slot)
		}
		ins = append(ins, it)
	}
	for slot := range was {
		del = append(del, slot)
	}
	return del, ins
}

func insertParams(id int64, items []character.Item) db.InsertInventoryParams {
	p := db.InsertInventoryParams{CharacterID: id}
	for _, it := range items {
		p.Slots = append(p.Slots, it.Slot)
		p.ItemIds = append(p.ItemIds, int(it.ID))
		p.Amounts = append(p.Amounts, it.Amount)
		serial := it.Serial
		if serial.IsZero() {
			// The zone gives every item a serial. One without is a bug,
			// but losing the item would be worse.
			serial = item.NewSerial()
		}
		p.Serials = append(p.Serials, pgtype.UUID{Bytes: serial, Valid: true})
		p.Equips = append(p.Equips, int(it.Equip))
		p.Refines = append(p.Refines, it.Refine)
		p.Cards = append(p.Cards, cardsLiteral(it.Cards))
		p.Rolls = append(p.Rolls, rollsJSON(it.Rolls))
		cards := make([]item.Rolls, len(it.CardRolls))
		copy(cards, it.CardRolls)
		p.CardRolls = append(p.CardRolls, mustJSON(cards))
		opts := it.Options
		if opts == nil {
			opts = []item.Option{}
		}
		p.RandomOptions = append(p.RandomOptions, mustJSON(opts))
		p.Identified = append(p.Identified, !it.Unidentified)
	}
	return p
}

// rollsJSON writes rolls as a JSON object, {} for none.
func rollsJSON(r item.Rolls) string {
	if len(r) == 0 {
		return "{}"
	}
	return mustJSON(r)
}

// mustJSON writes a value that always encodes: rolls, lists of rolls and
// options.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// readExtra reads an inventory row's rolls, card rolls and random
// options. The server checks their values against the item data when the
// character enters a zone.
func readExtra(it *db.InventoryRow) (item.Extra, error) {
	x := item.Extra{Unidentified: !it.Identified}
	if err := json.Unmarshal(it.Rolls, &x.Rolls); err != nil {
		return x, fmt.Errorf("store: rolls of slot %d: %w", it.Slot, err)
	}
	if err := json.Unmarshal(it.CardRolls, &x.CardRolls); err != nil {
		return x, fmt.Errorf("store: card rolls of slot %d: %w", it.Slot, err)
	}
	if err := json.Unmarshal(it.RandomOptions, &x.Options); err != nil {
		return x, fmt.Errorf("store: random options of slot %d: %w", it.Slot, err)
	}
	if len(x.CardRolls) == 0 {
		x.CardRolls = nil
	}
	if len(x.Options) == 0 {
		x.Options = nil
	}
	return x, nil
}

// cardsLiteral writes cards as a PostgreSQL array literal, such as
// {4001,4002}, which InsertInventory casts to integer[].
func cardsLiteral(cards []uint32) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, c := range cards {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatUint(uint64(c), 10))
	}
	b.WriteByte('}')
	return b.String()
}

func saveParams(c *character.Character, version int64) db.SaveCharacterParams {
	return db.SaveCharacterParams{
		ID: c.ID, SaveVersion: version,
		Job: c.Job, Sex: int(c.Sex()), Hair: c.Hair, HairColor: c.HairColor, ClothColor: c.ClothColor,
		BaseLevel: c.BaseLevel, JobLevel: c.JobLevel, BaseExp: c.BaseExp, JobExp: c.JobExp,
		StatusPoints: c.StatusPoints, SkillPoints: c.SkillPoints,
		Str: c.Stats.Str, Agi: c.Stats.Agi, Vit: c.Stats.Vit, Int: c.Stats.Int, Dex: c.Stats.Dex, Luk: c.Stats.Luk,
		Hp: int(c.HP), Sp: int(c.SP), Map: c.Map, X: c.X, Y: c.Y, Zeny: c.Zeny,
		InventorySize: c.InventorySize,
		SaveMap:       c.SaveMap, SaveX: c.SaveX, SaveY: c.SaveY,
		JobChangeLevel: c.JobChangeLevel, RebirthJob: c.RebirthJob, Options: c.Options,
		Companion: c.Companion, Mount: c.Mount,
	}
}

// fromRow turns database rows into a character.
func fromRow(r *db.Character, items []db.InventoryRow) (*character.Character, error) {
	c := &character.Character{Info: character.Info{
		ID: r.ID, AccountID: r.AccountID, Slot: r.Slot, Name: r.Name,
		Job: r.Job, Male: r.Sex == 1, Hair: r.Hair, HairColor: r.HairColor, ClothColor: r.ClothColor,
		BaseLevel: r.BaseLevel, JobLevel: r.JobLevel, BaseExp: r.BaseExp, JobExp: r.JobExp,
		StatusPoints: r.StatusPoints, SkillPoints: r.SkillPoints,
		HP: int64(r.Hp), SP: int64(r.Sp), Map: r.Map, X: r.X, Y: r.Y, Zeny: r.Zeny,
		InventorySize: r.InventorySize,
		Stats:         status.Stats{Str: r.Str, Agi: r.Agi, Vit: r.Vit, Int: r.Int, Dex: r.Dex, Luk: r.Luk},
		SaveMap:       r.SaveMap, SaveX: r.SaveX, SaveY: r.SaveY,
		JobChangeLevel: r.JobChangeLevel, RebirthJob: r.RebirthJob, Options: r.Options,
		Companion: r.Companion, Mount: r.Mount, CreatedAt: r.CreatedAt.Time,
	}}
	for _, it := range items {
		id, err := itemID(it.ItemID)
		if err != nil {
			return nil, err
		}
		ci := character.Item{Slot: it.Slot, ID: id, Amount: it.Amount, Serial: it.Serial.Bytes, Refine: it.Refine}
		if ci.Extra, err = readExtra(&it); err != nil {
			return nil, err
		}
		if it.Equip < 0 || it.Equip > math.MaxUint32 {
			return nil, fmt.Errorf("store: equip %d out of range", it.Equip)
		}
		ci.Equip = uint32(it.Equip) // #nosec G115 -- checked above, and the CHECK constraint keeps it below 2^22.
		for _, card := range it.Cards {
			cid, err := itemID(card)
			if err != nil {
				return nil, err
			}
			ci.Cards = append(ci.Cards, cid)
		}
		c.Items = append(c.Items, ci)
	}
	return c, nil
}

// itemID checks an item ID from the database, which its CHECK constraint
// keeps above 0.
func itemID(v int) (uint32, error) {
	if v <= 0 || int64(v) > math.MaxUint32 {
		return 0, fmt.Errorf("store: item ID %d out of range", v)
	}
	return uint32(v), nil
}

// DevCharacter returns the ID of the character with the name, first
// making an account and a character of that name from c when there are
// none. It is only for waserver --dev, where the session token is the
// character name. The account is marked as a dev account.
func (s *Store) DevCharacter(ctx context.Context, c *character.Character) (int64, error) {
	row, err := s.q.CharacterByName(ctx, c.Name)
	if err == nil {
		return row.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	var id int64
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		account, err := q.CreateAccount(ctx, db.CreateAccountParams{Name: c.Name, Dev: true})
		if errors.Is(err, pgx.ErrNoRows) {
			account, err = q.AccountByName(ctx, c.Name)
		}
		if err != nil {
			return err
		}
		id, err = q.CreateCharacter(ctx, createParams(account, c))
		if err != nil {
			return err
		}
		if len(c.Items) > 0 {
			return q.InsertInventory(ctx, insertParams(id, c.Items))
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Another login made the character first, or the account's
		// slot is taken by a character of another name.
		row, err = s.q.CharacterByName(ctx, c.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("store: cannot make character %q", c.Name)
		}
		return row.ID, err
	}
	return id, err
}

func createParams(account int64, c *character.Character) db.CreateCharacterParams {
	p := saveParams(c, 0)
	return db.CreateCharacterParams{
		AccountID: account, Slot: c.Slot, Name: c.Name,
		Job: p.Job, Sex: p.Sex, Hair: p.Hair, HairColor: p.HairColor, ClothColor: p.ClothColor,
		BaseLevel: p.BaseLevel, JobLevel: p.JobLevel, BaseExp: p.BaseExp, JobExp: p.JobExp,
		StatusPoints: p.StatusPoints, SkillPoints: p.SkillPoints,
		Str: p.Str, Agi: p.Agi, Vit: p.Vit, Int: p.Int, Dex: p.Dex, Luk: p.Luk,
		Hp: p.Hp, Sp: p.Sp, Map: p.Map, X: p.X, Y: p.Y, Zeny: p.Zeny, InventorySize: p.InventorySize,
	}
}
