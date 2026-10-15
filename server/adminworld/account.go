package adminworld

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/db"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// The game side of the account page (proposal 0002 review decision 37,
// decision 0032). The account service runs as the account role, which
// cannot read the game's tables, so it asks here:
//   - Block lists come from the zone of an online character, and from
//     the database for an offline one. Unblocking works either way, as
//     a staff sanction does.
//   - Reports and sanctions come from the database.
//   - The export reads offline characters only. It is refused while a
//     character of the account has a session, and holds the characters
//     while it reads, so no login changes them meanwhile.

// AccountPage is account.Game for the all-in-one build.
type AccountPage struct{ *World }

var _ account.Game = AccountPage{}

// AccountPage returns the world's side of the account page.
func (w *World) AccountPage() AccountPage { return AccountPage{w} }

// maxFiledReports is how many reports the page and the export list.
const maxFiledReports = 1000

// accountCharacters lists an account's characters.
func (w *World) accountCharacters(ctx context.Context, acc int64) ([]store.AccountCharacter, []int64, error) {
	if w.cfg.Store == nil {
		return nil, nil, nil
	}
	chars, err := w.cfg.Store.CharactersOf(ctx, acc)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int64, 0, len(chars))
	for _, c := range chars {
		ids = append(ids, c.ID)
	}
	return chars, ids, nil
}

// Blocks lists the block list of each of the account's characters.
func (w AccountPage) Blocks(ctx context.Context, acc int64) ([]account.CharacterBlocks, error) {
	chars, ids, err := w.accountCharacters(ctx, acc)
	if err != nil || len(chars) == 0 {
		return nil, err
	}
	zones, err := w.locate(ctx, ids)
	if err != nil {
		return nil, err
	}
	online := map[int64][]string{}
	byZone := map[*zone.Zone][]int64{}
	for id, z := range zones {
		byZone[z] = append(byZone[z], id)
	}
	for z, list := range byZone {
		got, err := z.AccountBlocks(ctx, list)
		if err != nil {
			return nil, err
		}
		for id, names := range got {
			online[id] = names
		}
	}
	saved, err := w.cfg.Store.BlocksOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]account.CharacterBlocks, 0, len(chars))
	for _, c := range chars {
		names, on := online[c.ID]
		if !on {
			names = saved[c.ID]
		}
		out = append(out, account.CharacterBlocks{CharacterID: c.ID, Character: c.Name, Online: on, Names: names})
	}
	return out, nil
}

// Unblock takes a name off a character's block list: in its zone when it
// is online, else in the database under a hold.
func (w AccountPage) Unblock(ctx context.Context, acc, char int64, name string) (bool, error) {
	_, ids, err := w.accountCharacters(ctx, acc)
	if err != nil || !slices.Contains(ids, char) {
		return false, err
	}
	for range sanctionTries {
		zones, err := w.locate(ctx, []int64{char})
		if err != nil {
			return false, err
		}
		if z := zones[char]; z != nil {
			found, removed, err := z.AccountUnblock(ctx, char, name)
			if err != nil || found {
				return removed, err
			}
		} else {
			removed, live, err := w.cfg.Store.UnblockOffline(ctx, char, name)
			if err != nil || !live {
				return removed, err
			}
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(sanctionWait):
		}
	}
	return false, errors.New("adminworld: the character keeps moving")
}

// filedReports reads the reports the account's characters filed.
func (w *World) filedReports(ctx context.Context, ids []int64) ([]account.FiledReport, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := w.cfg.Store.Queries().ReportsFiled(ctx, db.ReportsFiledParams{Ids: ids, MaxRows: maxFiledReports})
	if err != nil {
		return nil, err
	}
	out := make([]account.FiledReport, 0, len(rows))
	for _, r := range rows {
		kind := account.ReportPlayer
		switch {
		case r.GuildID != 0 && r.EmblemVersion != 0:
			kind = account.ReportGuildEmblem
		case r.GuildID != 0:
			kind = account.ReportGuildName
		case r.LineID.Valid:
			kind = account.ReportLine
		}
		out = append(out, account.FiledReport{ID: r.ID, At: r.CreatedAt.Time, Character: r.ReporterName, Target: r.TargetName,
			Kind: kind, Reason: r.Reason, Closed: r.Status == "closed",
			ActionTaken: r.Outcome.Valid && (r.Outcome.String == "warned" || r.Outcome.String == "sanctioned")})
	}
	return out, nil
}

// Reports lists the reports the account's characters filed.
func (w AccountPage) Reports(ctx context.Context, acc int64) ([]account.FiledReport, error) {
	_, ids, err := w.accountCharacters(ctx, acc)
	if err != nil {
		return nil, err
	}
	return w.filedReports(ctx, ids)
}

// Sanctions lists the sanctions on the account and its characters.
func (w AccountPage) Sanctions(ctx context.Context, acc int64) ([]account.AccountSanction, error) {
	if w.cfg.Store == nil {
		return nil, nil
	}
	rows, err := w.cfg.Store.SearchSanctions(ctx, store.SanctionQuery{Account: acc, Limit: maxFiledReports})
	if err != nil {
		return nil, err
	}
	out := make([]account.AccountSanction, 0, len(rows))
	for _, r := range rows {
		out = append(out, account.AccountSanction{ID: r.ID, Kind: r.Kind, Character: r.Character, Reason: r.Reason,
			Start: r.Start, End: r.End, Permanent: r.Permanent, Lifted: r.LiftedAt})
	}
	return out, nil
}

// LeadsGuild returns a character of the account that leads a guild
// with other members, and the guild's name.
func (w AccountPage) LeadsGuild(ctx context.Context, acc int64) (string, string, error) {
	chars, _, err := w.accountCharacters(ctx, acc)
	if err != nil || w.cfg.Social == nil {
		return "", "", err
	}
	for _, c := range chars {
		if g, ok := w.cfg.Social.GuildLedWithOthers(c.ID); ok {
			return c.Name, g, nil
		}
	}
	return "", "", nil
}

// Deleting closes the game connections and autotrade shops of an
// account set to be deleted.
func (w AccountPage) Deleting(ctx context.Context, acc int64, text string) {
	_, ids, err := w.accountCharacters(ctx, acc)
	if err != nil {
		w.log.Error("cannot list the characters of an account set to be deleted", "account", acc, "err", err)
		return
	}
	if len(ids) > 0 && w.cfg.Kicker != nil {
		w.cfg.Kicker.Kick(ctx, ids, wathenav1.DisconnectReason_DISCONNECT_REASON_ACCOUNT_DELETION, text)
	}
}

// Export reads the game's part of an account's data export.
func (w AccountPage) Export(ctx context.Context, acc int64) (*account.GameExport, error) {
	chars, ids, err := w.accountCharacters(ctx, acc)
	if err != nil {
		return nil, err
	}
	out := &account.GameExport{}
	st := w.cfg.Store
	if st == nil {
		return out, nil
	}
	hold, err := st.Hold(ids...)
	if errors.Is(err, store.ErrLive) {
		return nil, account.ErrCharacterOnline
	}
	if err != nil {
		return nil, err
	}
	defer hold.Release()
	q := st.Queries()
	blocks, err := st.BlocksOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	accountShown := false
	for _, c := range chars {
		v, err := st.ReadCharacter(ctx, c.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		skills, err := q.Skills(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		ec := w.exportCharacter(v, skills, blocks[c.ID])
		vars, err := q.CharacterVariables(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		for _, r := range vars {
			ec.Variables = append(ec.Variables, account.ExportVariable{Mod: r.ModID, Name: r.Name, Value: r.Value})
		}
		sortVariables(ec.Variables)
		for _, s := range v.Storages {
			switch {
			case s.Key.Holder == storage.HolderCart:
				ec.Cart = w.exportItems(s.Items)
			case s.Key.Holder == storage.HolderCharacter:
				ec.Storages = append(ec.Storages, w.exportStorage(s.Key, s.Items))
			case !accountShown:
				out.Storages = append(out.Storages, w.exportStorage(s.Key, s.Items))
			}
		}
		if !accountShown {
			out.Currencies = w.exportCurrencies(v.AccountCurrencies)
		}
		accountShown = true
		out.Characters = append(out.Characters, ec)
	}
	// The account's variables, which every character shares. An account
	// without characters may still have some.
	vars, err := q.AccountVariables(ctx, acc)
	if err != nil {
		return nil, err
	}
	for _, r := range vars {
		out.Variables = append(out.Variables, account.ExportVariable{Mod: r.ModID, Name: r.Name, Value: r.Value})
	}
	sortVariables(out.Variables)
	if err := w.exportLogs(ctx, acc, ids, out); err != nil {
		return nil, err
	}
	reports, err := w.filedReports(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range reports {
		out.Reports = append(out.Reports, account.ExportReport{ID: r.ID, At: r.At.UTC(), Character: r.Character, Target: r.Target,
			Kind: r.Kind, Reason: r.Reason, Status: account.ReportStatus(r)})
	}
	sanctions, err := w.Sanctions(ctx, acc)
	if err != nil {
		return nil, err
	}
	for _, s := range sanctions {
		e := account.ExportSanction{ID: s.ID, Kind: s.Kind, Character: s.Character, Reason: s.Reason, StartedAt: s.Start.UTC()}
		if !s.Permanent {
			end := s.End.UTC()
			e.EndsAt = &end
		}
		if !s.Lifted.IsZero() {
			lifted := s.Lifted.UTC()
			e.LiftedAt = &lifted
		}
		out.Sanctions = append(out.Sanctions, e)
	}
	return out, nil
}

// sortVariables sorts exported variables by mod and name.
func sortVariables(vs []account.ExportVariable) {
	slices.SortFunc(vs, func(a, b account.ExportVariable) int {
		return cmp.Or(strings.Compare(a.Mod, b.Mod), strings.Compare(a.Name, b.Name))
	})
}

// exportLogs reads the chat lines and the item and zeny logs.
func (w *World) exportLogs(ctx context.Context, acc int64, ids []int64, out *account.GameExport) error {
	if len(ids) == 0 {
		return nil
	}
	q := w.cfg.Store.Queries()
	limit := int32(account.MaxExportRows + 1)
	lines, err := q.ChatLinesOf(ctx, db.ChatLinesOfParams{Ids: ids, MaxRows: limit})
	if err != nil {
		return err
	}
	if len(lines) > account.MaxExportRows {
		lines, out.Truncated = lines[:account.MaxExportRows], append(out.Truncated, "chat")
	}
	for _, l := range lines {
		out.Chat = append(out.Chat, account.ExportChatLine{At: l.At.Time.UTC(), Character: l.Name, Kind: l.Kind, Target: l.Target,
			Map: l.Map, Text: l.Text})
	}
	accountStorages, characterStorages := []int{}, []int{}
	for _, r := range w.cfg.StorageRecords {
		if r.Account {
			accountStorages = append(accountStorages, int(r.ID))
		} else {
			characterStorages = append(characterStorages, int(r.ID))
		}
	}
	kinds := []string{"inventory", "cart", "vending"}
	items, err := q.AccountItemLog(ctx, db.AccountItemLogParams{Kinds: kinds, Ids: ids, AccountID: acc, AccountStorages: accountStorages,
		CharacterStorages: characterStorages, MaxRows: limit})
	if err != nil {
		return err
	}
	if len(items) > account.MaxExportRows {
		items, out.Truncated = items[:account.MaxExportRows], append(out.Truncated, "item_log")
	}
	for _, r := range items {
		l := account.ExportItemLine{At: r.At.Time.UTC(), Action: r.Action,
			From:   account.ExportOwner{Kind: r.FromKind, ID: r.FromID, Storage: uint32(r.FromStorage)},                // #nosec G115 -- record IDs are small.
			To:     account.ExportOwner{Kind: r.ToKind, ID: r.ToID, Storage: uint32(r.ToStorage)},                      // #nosec G115 -- record IDs are small.
			Serial: item.Serial(r.Serial.Bytes).String(), ItemID: uint32(r.ItemID), Name: w.itemName(uint32(r.ItemID)), // #nosec G115 -- item IDs are positive.
			Amount: r.Amount, Refine: r.Refine, Cards: cards(r.Cards), Bound: r.Bound}
		if r.Source.Valid {
			l.Source = item.Serial(r.Source.Bytes).String()
		}
		out.ItemLog = append(out.ItemLog, l)
	}
	zeny, err := q.AccountZenyLog(ctx, db.AccountZenyLogParams{Kinds: kinds, Ids: ids, AccountID: acc, MaxRows: limit})
	if err != nil {
		return err
	}
	if len(zeny) > account.MaxExportRows {
		zeny, out.Truncated = zeny[:account.MaxExportRows], append(out.Truncated, "zeny_log")
	}
	for _, r := range zeny {
		out.ZenyLog = append(out.ZenyLog, account.ExportZenyLine{At: r.At.Time.UTC(), Action: r.Action,
			From: account.ExportOwner{Kind: r.FromKind, ID: r.FromID}, To: account.ExportOwner{Kind: r.ToKind, ID: r.ToID},
			Amount: r.Amount, Currency: uint32(r.Currency)}) // #nosec G115 -- currency IDs are positive.
	}
	return nil
}

func cards(list []int) []uint32 {
	out := make([]uint32, 0, len(list))
	for _, c := range list {
		out = append(out, uint32(c)) // #nosec G115 -- card IDs are positive.
	}
	return out
}

// exportCharacter builds a character's part of the export.
func (w *World) exportCharacter(v *store.OfflineView, skills []db.SkillsRow, blocks []string) account.ExportCharacter {
	c := v.Character
	sex := "female"
	if c.Male {
		sex = "male"
	}
	e := account.ExportCharacter{ID: c.ID, Slot: c.Slot, Name: c.Name, Job: c.Job, JobName: w.jobName(c.Job), Sex: sex,
		BaseLevel: c.BaseLevel, JobLevel: c.JobLevel, BaseExp: c.BaseExp, JobExp: c.JobExp,
		Stats:        account.ExportStats{Str: c.Stats.Str, Agi: c.Stats.Agi, Vit: c.Stats.Vit, Int: c.Stats.Int, Dex: c.Stats.Dex, Luk: c.Stats.Luk},
		StatusPoints: c.StatusPoints, SkillPoints: c.SkillPoints, HP: c.HP, SP: c.SP, Zeny: c.Zeny,
		Position: account.ExportPosition{Map: c.Map, X: c.X, Y: c.Y}, CreatedAt: c.CreatedAt.UTC(), Party: v.Party, Guild: v.Guild,
		Skills: []account.ExportSkill{}, Inventory: w.exportItems(c.Items), Cart: []account.ExportItem{},
		Storages: []account.ExportStorage{}, Mail: []account.ExportMail{}, Currencies: w.exportCurrencies(c.Currencies),
		BlockList: slices.Clone(blocks)}
	if e.BlockList == nil {
		e.BlockList = []string{}
	}
	if c.SaveMap != "" {
		e.SavePoint = &account.ExportPosition{Map: c.SaveMap, X: c.SaveX, Y: c.SaveY}
	}
	for _, s := range skills {
		e.Skills = append(e.Skills, account.ExportSkill{ID: s.SkillID, Level: s.Level})
	}
	for _, m := range v.Mails {
		e.Mail = append(e.Mail, account.ExportMail{ID: m.ID, From: m.FromName, Title: m.Title, Body: m.Body, SentAt: m.Sent.UTC(),
			Zeny: m.Zeny, Read: m.Read, Returned: m.Returned, Items: w.exportItems(m.Items)})
	}
	return e
}

func (w *World) exportItems(list []character.Item) []account.ExportItem {
	out := make([]account.ExportItem, 0, len(list))
	for _, it := range list {
		e := account.ExportItem{Serial: it.Serial.String(), ItemID: it.ID, Name: w.itemName(it.ID), Amount: it.Amount, Slot: it.Slot,
			Refine: it.Refine, Cards: slices.Clone(it.Cards), Equip: it.Equip, Identified: !it.Unidentified, Bound: int(it.Bound),
			Rolls: map[string]int{}}
		if e.Cards == nil {
			e.Cards = []uint32{}
		}
		for _, r := range it.Rolls {
			e.Rolls[r.Name] = r.Value
		}
		out = append(out, e)
	}
	return out
}

func (w *World) exportStorage(k storage.Key, items []character.Item) account.ExportStorage {
	s := account.ExportStorage{Kind: "storage", Storage: k.Storage, Items: w.exportItems(items)}
	if k.Storage != 0 {
		s.Kind = "extra"
		if r := w.record(k.Storage); r != nil {
			s.Name = r.DisplayName
		}
	}
	return s
}

func (w *World) exportCurrencies(m map[uint32]int64) []account.ExportCurrency {
	out := make([]account.ExportCurrency, 0, len(m))
	for _, id := range sortedKeys(m) {
		out = append(out, account.ExportCurrency{ID: id, Name: w.currencyName(id), Amount: m[id]})
	}
	return out
}
