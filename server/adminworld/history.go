package adminworld

import (
	"cmp"
	"context"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/db"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Limits of an item history: the serials it follows, the lines it
// reads, and the rounds of queries.
const (
	maxHistorySerials = 100
	maxHistoryLines   = 2000
	maxHistoryRounds  = 20
)

// settleWait bounds the wait for the commits queued before a read.
const settleWait = 2 * time.Second

// settle waits a little for the commits the zones handed to the store,
// so the database shows what the zones did.
func (w *World) settle(ctx context.Context) {
	sctx, cancel := context.WithTimeout(ctx, settleWait)
	defer cancel()
	_ = w.cfg.Store.Settle(sctx)
}

// ownerRef names an owner of the logs.
type ownerRef struct {
	kind    transfer.Kind
	id      int64
	storage uint32
}

// holderRef turns a saved place of a serial into an owner.
func holderRef(h store.Holder) ownerRef {
	switch h.Holder {
	case "inventory":
		return ownerRef{transfer.KindInventory, h.ID, 0}
	case "cart":
		return ownerRef{transfer.KindCart, h.ID, 0}
	case "account", "character":
		return ownerRef{transfer.KindStorage, h.ID, h.Storage}
	case "mail":
		return ownerRef{transfer.KindMail, h.ID, 0}
	case "guild_storage":
		return ownerRef{transfer.KindGuildStorage, h.ID, 0}
	}
	return ownerRef{}
}

// characterKind reports whether an owner's ID is a character's.
func (w *World) characterKind(r ownerRef) bool {
	switch r.kind {
	case transfer.KindInventory, transfer.KindCart, transfer.KindVending:
		return true
	case transfer.KindStorage:
		if rec := w.record(r.storage); rec != nil && !rec.Account {
			return true
		}
	}
	return false
}

// owners names owners for people: characters, accounts, guilds and
// mails.
func (w *World) owners(ctx context.Context, refs []ownerRef) (map[ownerRef]*adminv1.Owner, error) {
	q := w.cfg.Store.Queries()
	var chars, accounts, guilds, mails []int64
	for _, r := range refs {
		switch {
		case r.id <= 0:
		case w.characterKind(r):
			chars = append(chars, r.id)
		case r.kind == transfer.KindStorage || r.kind == transfer.KindStaff:
			accounts = append(accounts, r.id)
		case r.kind == transfer.KindGuildStorage:
			guilds = append(guilds, r.id)
		case r.kind == transfer.KindMail:
			mails = append(mails, r.id)
		}
	}
	type mailTo struct {
		to   int64
		name string
	}
	mailOf := map[int64]mailTo{}
	if len(mails) > 0 {
		rows, err := q.MailOwners(ctx, unique(mails))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			mailOf[r.ID] = mailTo{r.RecipientID, r.RecipientName}
			chars = append(chars, r.RecipientID)
		}
	}
	type charName struct {
		name    string
		account int64
	}
	charOf := map[int64]charName{}
	if len(chars) > 0 {
		rows, err := q.CharacterNames(ctx, unique(chars))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			charOf[r.ID] = charName{r.Name, r.AccountID}
		}
	}
	accountOf := map[int64]string{}
	if len(accounts) > 0 {
		rows, err := q.AccountNames(ctx, unique(accounts))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			accountOf[r.ID] = r.Name
		}
	}
	guildOf := map[int64]string{}
	if len(guilds) > 0 {
		rows, err := q.GuildNames(ctx, unique(guilds))
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			guildOf[r.ID] = r.Name
		}
	}
	out := map[ownerRef]*adminv1.Owner{}
	for _, r := range refs {
		if out[r] != nil {
			continue
		}
		o := adminv1.Owner_builder{Kind: r.kind.String(), Id: r.id, Storage: r.storage}
		switch {
		case r.id <= 0:
		case w.characterKind(r):
			if c, ok := charOf[r.id]; ok {
				o.Name, o.CharacterId, o.AccountId = c.name, r.id, c.account
			}
		case r.kind == transfer.KindStorage || r.kind == transfer.KindStaff:
			o.Name, o.AccountId = accountOf[r.id], r.id
		case r.kind == transfer.KindGuildStorage:
			o.Name = guildOf[r.id]
		case r.kind == transfer.KindMail:
			if m, ok := mailOf[r.id]; ok {
				o.Name, o.CharacterId, o.AccountId = "mail "+strconv.FormatInt(r.id, 10)+" to "+m.name, m.to, charOf[m.to].account
			}
		}
		if r.kind == transfer.KindStorage && r.storage > 0 {
			if rec := w.record(r.storage); rec != nil && o.Name != "" {
				o.Name += " (" + rec.DisplayName + ")"
			}
		}
		out[r] = o.Build()
	}
	return out, nil
}

func unique(ids []int64) []int64 {
	slices.Sort(ids)
	return slices.Compact(ids)
}

// FindSerial finds the saved place that holds a serial, or nil.
func (w *World) FindSerial(ctx context.Context, serial item.Serial) (*adminv1.Owner, error) {
	if w.cfg.Store == nil {
		return nil, errNoStore
	}
	w.settle(ctx)
	holders, err := w.cfg.Store.ItemHolders(ctx, serial)
	if err != nil || len(holders) == 0 {
		return nil, err
	}
	if len(holders) > 1 {
		w.log.Error("a serial is held in more than one place", "serial", serial.String(), "places", len(holders))
	}
	ref := holderRef(holders[0])
	names, err := w.owners(ctx, []ownerRef{ref})
	if err != nil {
		return nil, err
	}
	return names[ref], nil
}

// logLine is one item log line of the history queries.
type logLine struct {
	id             int64
	at             time.Time
	action         string
	from, to       ownerRef
	serial, source item.Serial
	itemID         uint32
	amount, refine int
	cards          []uint32
	hasSource      bool
}

func kindOf(s string) transfer.Kind {
	k, _ := transfer.ParseKind(s)
	return k
}

func lineOf(id int64, at pgtype.Timestamptz, action, fromKind string, fromID int64, fromStorage int, toKind string, toID int64,
	toStorage int, serial, source pgtype.UUID, itemID, amount, refine int, cards []int) logLine {
	l := logLine{id: id, at: at.Time, action: action,
		from:   ownerRef{kindOf(fromKind), fromID, uint32(max(fromStorage, 0))}, // #nosec G115 -- record IDs are small.
		to:     ownerRef{kindOf(toKind), toID, uint32(max(toStorage, 0))},       // #nosec G115 -- record IDs are small.
		serial: serial.Bytes, source: source.Bytes, hasSource: source.Valid,
		itemID: uint32(max(itemID, 0)), amount: amount, refine: refine} // #nosec G115 -- item IDs are above 0.
	for _, c := range cards {
		l.cards = append(l.cards, uint32(max(c, 0))) // #nosec G115 -- card IDs are above 0.
	}
	return l
}

// ItemHistory lists the moves of a serial, of the stacks it came from,
// and of the stacks it went into, oldest first. It follows source links
// back from the serial and its ancestors, and forward from the serial
// and its descendants.
func (w *World) ItemHistory(ctx context.Context, serial item.Serial) (*adminv1.ItemHistory, error) {
	if w.cfg.Store == nil {
		return nil, errNoStore
	}
	w.settle(ctx)
	q := w.cfg.Store.Queries()
	relation := map[item.Serial]string{serial: "self"}
	order := []item.Serial{serial}
	lines := map[int64]logLine{}
	bySerial := []item.Serial{serial}
	bySource := []item.Serial{serial}
	truncated := false
	add := func(s item.Serial, rel string) {
		if _, ok := relation[s]; ok || s.IsZero() {
			return
		}
		if len(relation) >= maxHistorySerials {
			truncated = true
			return
		}
		relation[s] = rel
		order = append(order, s)
		bySerial = append(bySerial, s)
		if rel == "descendant" {
			bySource = append(bySource, s)
		}
	}
	for round := 0; round < maxHistoryRounds && (len(bySerial) > 0 || len(bySource) > 0); round++ {
		if len(lines) >= maxHistoryLines {
			truncated = true
			break
		}
		left := int32(maxHistoryLines - len(lines) + 1) // #nosec G115 -- at most maxHistoryLines + 1.
		serials, sources := uuids(bySerial), uuids(bySource)
		bySerial, bySource = nil, nil
		var found []logLine
		if len(serials) > 0 {
			rows, err := q.ItemLogBySerials(ctx, db.ItemLogBySerialsParams{Serials: serials, MaxRows: left})
			if err != nil {
				return nil, err
			}
			if len(rows) >= int(left) {
				truncated = true
			}
			for _, r := range rows {
				found = append(found, lineOf(r.ID, r.At, r.Action, r.FromKind, r.FromID, r.FromStorage, r.ToKind, r.ToID, r.ToStorage,
					r.Serial, r.Source, r.ItemID, r.Amount, r.Refine, r.Cards))
			}
		}
		if len(sources) > 0 {
			rows, err := q.ItemLogBySources(ctx, db.ItemLogBySourcesParams{Serials: sources, MaxRows: left})
			if err != nil {
				return nil, err
			}
			if len(rows) >= int(left) {
				truncated = true
			}
			for _, r := range rows {
				found = append(found, lineOf(r.ID, r.At, r.Action, r.FromKind, r.FromID, r.FromStorage, r.ToKind, r.ToID, r.ToStorage,
					r.Serial, r.Source, r.ItemID, r.Amount, r.Refine, r.Cards))
			}
		}
		for _, l := range found {
			if _, ok := lines[l.id]; ok {
				continue
			}
			lines[l.id] = l
			if !l.hasSource || l.source == l.serial {
				continue
			}
			switch relation[l.serial] {
			case "self", "ancestor":
				// The serial came from source: an ancestor.
				add(l.source, "ancestor")
			}
			switch relation[l.source] {
			case "self", "descendant":
				// Source went into the serial: a descendant.
				add(l.serial, "descendant")
			}
		}
	}
	list := make([]logLine, 0, len(lines))
	for _, l := range lines {
		list = append(list, l)
	}
	slices.SortFunc(list, func(a, b logLine) int { return cmp.Or(a.at.Compare(b.at), cmp.Compare(a.id, b.id)) })
	var refs []ownerRef
	for _, l := range list {
		refs = append(refs, l.from, l.to)
	}
	names, err := w.owners(ctx, refs)
	if err != nil {
		return nil, err
	}
	out := adminv1.ItemHistory_builder{Serial: serial.String(), Truncated: truncated}
	for _, l := range list {
		rel := relation[l.serial]
		if rel == "" {
			rel = relation[l.source]
		}
		m := adminv1.ItemMove_builder{Id: l.id, At: timestamppb.New(l.at), Action: l.action, From: names[l.from], To: names[l.to],
			Serial: l.serial.String(), ItemId: l.itemID, ItemName: w.itemName(l.itemID), Amount: int32(l.amount), // #nosec G115 -- a stack.
			Refine: int32(l.refine), Cards: l.cards, Relation: rel} // #nosec G115 -- refines are small.
		if l.hasSource {
			m.Source = l.source.String()
		}
		out.Moves = append(out.Moves, m.Build())
	}
	for _, s := range order {
		out.Serials = append(out.Serials, s.String())
	}
	holder, err := w.findHolder(ctx, serial)
	if err != nil {
		return nil, err
	}
	out.Holder = holder
	if len(out.Moves) == 0 && holder == nil {
		return nil, admin.ErrNotFound
	}
	return out.Build(), nil
}

// findHolder is FindSerial without the settle.
func (w *World) findHolder(ctx context.Context, serial item.Serial) (*adminv1.Owner, error) {
	holders, err := w.cfg.Store.ItemHolders(ctx, serial)
	if err != nil || len(holders) == 0 {
		return nil, err
	}
	ref := holderRef(holders[0])
	names, err := w.owners(ctx, []ownerRef{ref})
	if err != nil {
		return nil, err
	}
	return names[ref], nil
}

func uuids(list []item.Serial) []pgtype.UUID {
	out := make([]pgtype.UUID, len(list))
	for i, s := range list {
		out[i] = pgtype.UUID{Bytes: s, Valid: true}
	}
	return out
}

// characterKinds are the owners of the logs whose ID is the character's.
var characterKinds = []string{transfer.KindInventory.String(), transfer.KindCart.String(), transfer.KindVending.String()}

// logCursor is where a page of a character's logs starts: before a time
// and row ID in each log.
type logCursor struct {
	itemAt, zenyAt time.Time
	itemID, zenyID int64
}

func (c logCursor) String() string {
	return strings.Join([]string{strconv.FormatInt(c.itemAt.UnixNano(), 10), strconv.FormatInt(c.itemID, 10),
		strconv.FormatInt(c.zenyAt.UnixNano(), 10), strconv.FormatInt(c.zenyID, 10)}, ".")
}

func parseLogCursor(s string) (logCursor, bool) {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return logCursor{}, false
	}
	var n [4]int64
	for i, p := range parts {
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return logCursor{}, false
		}
		n[i] = v
	}
	return logCursor{itemAt: time.Unix(0, n[0]), itemID: n[1], zenyAt: time.Unix(0, n[2]), zenyID: n[3]}, true
}

// CharacterLogs lists a page of a character's item and zeny log, newest
// first.
func (w *World) CharacterLogs(ctx context.Context, id int64, lq admin.LogQuery) (*adminv1.CharacterLogs, error) {
	if w.cfg.Store == nil {
		return nil, errNoStore
	}
	q := w.cfg.Store.Queries()
	if _, err := q.ReadCharacter(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, admin.ErrNotFound
		}
		return nil, err
	}
	end := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	until := end
	if !lq.Until.IsZero() {
		until = lq.Until
	}
	since := time.Unix(0, 0)
	if !lq.Since.IsZero() {
		since = lq.Since
	}
	cur := logCursor{itemAt: end, zenyAt: end, itemID: math.MaxInt64, zenyID: math.MaxInt64}
	if lq.Cursor != "" {
		var ok bool
		if cur, ok = parseLogCursor(lq.Cursor); !ok {
			return nil, &admin.WorldError{Code: "bad_request", Message: "cursor is the next_cursor of the last page."}
		}
	}
	ts := func(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
	rows := int32(lq.Limit + 1) // #nosec G115 -- at most admin.MaxLogPage + 1.
	items, err := q.CharacterItemLog(ctx, db.CharacterItemLogParams{Kinds: characterKinds, CharacterID: id, BeforeAt: ts(cur.itemAt),
		BeforeID: cur.itemID, Since: ts(since), Until: ts(until), MaxRows: rows})
	if err != nil {
		return nil, err
	}
	zeny, err := q.CharacterZenyLog(ctx, db.CharacterZenyLogParams{Kinds: characterKinds, CharacterID: id, BeforeAt: ts(cur.zenyAt),
		BeforeID: cur.zenyID, Since: ts(since), Until: ts(until), MaxRows: rows})
	if err != nil {
		return nil, err
	}
	type entry struct {
		at   time.Time
		item *logLine
		zeny *db.ZenyLog
	}
	var all []entry
	for _, r := range items {
		l := lineOf(r.ID, r.At, r.Action, r.FromKind, r.FromID, r.FromStorage, r.ToKind, r.ToID, r.ToStorage, r.Serial, r.Source,
			r.ItemID, r.Amount, r.Refine, r.Cards)
		all = append(all, entry{at: l.at, item: &l})
	}
	for i := range zeny {
		all = append(all, entry{at: zeny[i].At.Time, zeny: &zeny[i]})
	}
	slices.SortStableFunc(all, func(a, b entry) int { return b.at.Compare(a.at) })
	out := &adminv1.CharacterLogs{}
	if len(all) > lq.Limit {
		all = all[:lq.Limit]
		next := cur
		for _, e := range all {
			if e.item != nil {
				next.itemAt, next.itemID = e.item.at, e.item.id
			} else {
				next.zenyAt, next.zenyID = e.zeny.At.Time, e.zeny.ID
			}
		}
		out.SetNextCursor(next.String())
	}
	var refs []ownerRef
	for _, e := range all {
		if e.item != nil {
			refs = append(refs, e.item.from, e.item.to)
		} else {
			refs = append(refs, ownerRef{kindOf(e.zeny.FromKind), e.zeny.FromID, 0}, ownerRef{kindOf(e.zeny.ToKind), e.zeny.ToID, 0})
		}
	}
	names, err := w.owners(ctx, refs)
	if err != nil {
		return nil, err
	}
	var entries []*adminv1.LogEntry
	for _, e := range all {
		if l := e.item; l != nil {
			b := adminv1.LogEntry_builder{Id: l.id, At: timestamppb.New(l.at), Kind: "item", Action: l.action, From: names[l.from],
				To: names[l.to], Serial: l.serial.String(), ItemId: l.itemID, ItemName: w.itemName(l.itemID), Amount: int64(l.amount),
				Refine: int32(l.refine), Cards: l.cards} // #nosec G115 -- refines are small.
			if l.hasSource {
				b.Source = l.source.String()
			}
			entries = append(entries, b.Build())
			continue
		}
		z := e.zeny
		cur := uint32(max(z.Currency, 0)) // #nosec G115 -- currency IDs are small.
		entries = append(entries, adminv1.LogEntry_builder{Id: z.ID, At: timestamppb.New(z.At.Time), Kind: "zeny", Action: z.Action,
			From: names[ownerRef{kindOf(z.FromKind), z.FromID, 0}], To: names[ownerRef{kindOf(z.ToKind), z.ToID, 0}], Amount: z.Amount,
			Currency: cur, CurrencyName: w.currencyName(cur)}.Build())
	}
	out.SetEntries(entries)
	return out, nil
}

// storageKeyOf is the storage key of an owner of kind storage or cart.
func (w *World) storageKeyOf(r ownerRef) storage.Key {
	if r.kind == transfer.KindCart {
		return storage.Key{Holder: storage.HolderCart, ID: r.id}
	}
	if w.characterKind(r) {
		return storage.Key{Holder: storage.HolderCharacter, ID: r.id, Storage: r.storage}
	}
	return storage.Key{Holder: storage.HolderAccount, ID: r.id, Storage: r.storage}
}
