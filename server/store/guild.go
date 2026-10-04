package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Guilds (decision 0021). The social registry holds them in memory and
// hands each change to SocialStore. Guild writes go through the
// transfer commit lane when the store gives SocialStore one, so a
// guild's row exists before a commit writes its storage, and a commit
// after the guild broke up finds it gone. Every guild loads at start.

// positionJSON is a guild position as guilds.positions keeps it.
type positionJSON struct {
	Name    string `json:"name"`
	Invite  bool   `json:"invite"`
	Expel   bool   `json:"expel"`
	Storage bool   `json:"storage"`
	ExpTax  int    `json:"exp_tax"`
}

// expulsionJSON is an expulsion as guilds.expulsions keeps it.
type expulsionJSON struct {
	CharacterID int64  `json:"character_id"`
	Name        string `json:"name"`
	Reason      string `json:"reason"`
}

func positionsJSON(ps []social.Position) []byte {
	out := make([]positionJSON, len(ps))
	for i, p := range ps {
		out[i] = positionJSON{Name: p.Name, Invite: p.Invite, Expel: p.Expel, Storage: p.Storage, ExpTax: p.ExpTax}
	}
	b, _ := json.Marshal(out)
	return b
}

func skillsJSON(s map[int]int) []byte {
	m := map[string]int{}
	for k, v := range s {
		if v > 0 {
			m[strconv.Itoa(k)] = v
		}
	}
	b, _ := json.Marshal(m)
	return b
}

func expulsionsJSON(es []social.Expulsion) []byte {
	out := make([]expulsionJSON, len(es))
	for i, e := range es {
		out[i] = expulsionJSON{CharacterID: e.CharID, Name: e.Name, Reason: e.Reason}
	}
	b, _ := json.Marshal(out)
	return b
}

func optionalTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// GuildIDFloor is the highest guild ID that the item log or the reports
// name, for social.Registry.SetGuildIDFloor.
func GuildIDFloor(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	return db.New(pool).GuildIDFloor(ctx)
}

// LoadGuilds reads every saved guild with its members, relations,
// emblem and storage, and each guild's storage log.
func LoadGuilds(ctx context.Context, pool *pgxpool.Pool) ([]social.GuildRecord, map[int64][]social.StorageLogEntry, error) {
	q := db.New(pool)
	rows, err := q.Guilds(ctx)
	if err != nil {
		return nil, nil, err
	}
	byID := map[int64]*social.GuildRecord{}
	out := make([]social.GuildRecord, len(rows))
	for i, r := range rows {
		g := social.GuildRecord{ID: r.ID, Name: r.Name, Level: r.Level, Exp: r.Exp, SkillPoints: r.SkillPoints,
			Notice: social.GuildNotice{Title: r.NoticeTitle, Text: r.NoticeText}, Skills: map[int]int{}}
		if r.LastMasterChange.Valid {
			g.LastMasterChange = r.LastMasterChange.Time
		}
		var skills map[string]int
		if err := json.Unmarshal(r.Skills, &skills); err != nil {
			return nil, nil, fmt.Errorf("guild %d: skills: %w", r.ID, err)
		}
		for k, v := range skills {
			id, err := strconv.Atoi(k)
			if err != nil {
				return nil, nil, fmt.Errorf("guild %d: skill %q", r.ID, k)
			}
			g.Skills[id] = v
		}
		var ps []positionJSON
		if err := json.Unmarshal(r.Positions, &ps); err != nil {
			return nil, nil, fmt.Errorf("guild %d: positions: %w", r.ID, err)
		}
		for _, p := range ps {
			g.Positions = append(g.Positions, social.Position{Name: p.Name, Invite: p.Invite, Expel: p.Expel, Storage: p.Storage, ExpTax: p.ExpTax})
		}
		var es []expulsionJSON
		if err := json.Unmarshal(r.Expulsions, &es); err != nil {
			return nil, nil, fmt.Errorf("guild %d: expulsions: %w", r.ID, err)
		}
		for _, e := range es {
			g.Expulsions = append(g.Expulsions, social.Expulsion{CharID: e.CharacterID, Name: e.Name, Reason: e.Reason})
		}
		out[i] = g
		byID[r.ID] = &out[i]
	}
	members, err := q.GuildMembers(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, m := range members {
		if g := byID[m.GuildID]; g != nil {
			g.Members = append(g.Members, social.GuildMemberRecord{Slot: m.Slot, CharID: m.ID, AccountID: m.AccountID, Name: m.Name,
				Position: m.Position, Exp: m.Exp, Level: m.BaseLevel, Job: m.Job, Map: m.Map})
		}
	}
	rels, err := q.GuildRelations(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, r := range rels {
		if g := byID[r.GuildID]; g != nil {
			g.Relations = append(g.Relations, social.Relation{Guild: r.OtherID, Name: r.Name, Enemy: r.Enemy})
		}
	}
	emblems, err := q.GuildEmblems(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range emblems {
		if g := byID[e.GuildID]; g != nil {
			g.Emblem = social.Emblem{Version: e.Version, PNG: e.Png}
		}
	}
	slots, err := q.GuildStorage(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, s := range slots {
		g := byID[s.GuildID]
		if g == nil {
			continue
		}
		it, err := storageItem(s)
		if err != nil {
			return nil, nil, fmt.Errorf("guild %d: %w", s.GuildID, err)
		}
		g.Storage = append(g.Storage, it)
	}
	logRows, err := q.GuildStorageLog(ctx, social.MaxStorageLog)
	if err != nil {
		return nil, nil, err
	}
	logs := map[int64][]social.StorageLogEntry{}
	for _, l := range logRows {
		id, err := itemID(l.ItemID)
		if err != nil {
			return nil, nil, err
		}
		e := social.StorageLogEntry{At: l.At.Time, Name: l.Name, ItemID: id, Amount: l.Amount, Refine: l.Refine}
		for _, c := range l.Cards {
			if cid, err := itemID(c); err == nil {
				e.Cards = append(e.Cards, cid)
			}
		}
		logs[l.GuildID] = append(logs[l.GuildID], e)
	}
	for id := range logs {
		// The query gives newest first; the registry keeps oldest first.
		slices.Reverse(logs[id])
	}
	return out, logs, nil
}

// storageItem reads a guild storage row as inventory rows are read.
func storageItem(s db.GuildStorageRow) (character.Item, error) {
	id, err := itemID(s.ItemID)
	if err != nil {
		return character.Item{}, err
	}
	it := character.Item{Slot: s.Slot, ID: id, Amount: s.Amount, Serial: s.Serial.Bytes, Refine: s.Refine}
	row := db.InventoryRow{Slot: s.Slot, Rolls: s.Rolls, CardRolls: s.CardRolls, RandomOptions: s.RandomOptions, Identified: s.Identified,
		Bound: s.Bound}
	if it.Extra, err = readExtra(&row); err != nil {
		return character.Item{}, err
	}
	for _, card := range s.Cards {
		cid, err := itemID(card)
		if err != nil {
			return character.Item{}, err
		}
		it.Cards = append(it.Cards, cid)
	}
	return it, nil
}

// storageParams are InsertGuildStorage's parameters, as insertParams.
func storageParams(guild int64, items []character.Item) db.InsertGuildStorageParams {
	inv := insertParams(0, items)
	return db.InsertGuildStorageParams{GuildID: guild, Slots: inv.Slots, ItemIds: inv.ItemIds, Amounts: inv.Amounts, Serials: inv.Serials,
		Refines: inv.Refines, Cards: inv.Cards, Rolls: inv.Rolls, CardRolls: inv.CardRolls, RandomOptions: inv.RandomOptions,
		Identified: inv.Identified, Bounds: inv.Bounds}
}

// StorageWrite is a storage that a commit saves: what the last commit
// of it wrote, and what it holds now. Guild names a guild storage. Key
// names an account's storage, an extra storage or a cart (storage.go).
type StorageWrite struct {
	Guild     int64
	Key       storage.Key
	Base, Cur []character.Item
}

// writeStorages writes guild storages in a commit's transaction. A guild
// that is gone, because it broke up before the commit, is skipped: its
// storage went with it.
func writeStorages(ctx context.Context, q *db.Queries, ws []StorageWrite) error {
	if len(ws) == 0 {
		return nil
	}
	if err := writePersonalStorages(ctx, q, ws); err != nil {
		return err
	}
	var ids []int64
	for _, w := range ws {
		if w.Guild == 0 {
			continue
		}
		if !slices.Contains(ids, w.Guild) {
			ids = append(ids, w.Guild)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	slices.Sort(ids)
	found, err := q.LockGuilds(ctx, ids)
	if err != nil {
		return err
	}
	for _, w := range ws {
		if w.Guild == 0 || !slices.Contains(found, w.Guild) {
			continue
		}
		del, ins := diffItems(w.Base, w.Cur)
		if len(del) > 0 {
			if err := q.DeleteGuildStorageSlots(ctx, db.DeleteGuildStorageSlotsParams{GuildID: w.Guild, Slots: del}); err != nil {
				return err
			}
		}
		if len(ins) > 0 {
			if err := q.InsertGuildStorage(ctx, storageParams(w.Guild, ins)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Guild writes of SocialStore.

// guildWrite runs a guild write in order: on the commit lane when the
// store gave one, else on the SocialStore's own queue.
func (s *SocialStore) guildWrite(f func(ctx context.Context, q *db.Queries) error) {
	if s.Lane != nil {
		s.Lane(f)
		return
	}
	s.push(f)
}

func insertGuildMember(ctx context.Context, q *db.Queries, guild int64, m social.GuildMemberRecord) error {
	if m.CharID == 0 {
		return nil
	}
	if err := q.ClearGuildSlot(ctx, db.ClearGuildSlotParams{GuildID: guild, Slot: m.Slot, CharacterID: m.CharID}); err != nil {
		return err
	}
	return q.UpsertGuildMember(ctx, db.UpsertGuildMemberParams{GuildID: guild, CharacterID: m.CharID, Slot: m.Slot,
		Position: m.Position, Exp: m.Exp})
}

func writeRelations(ctx context.Context, q *db.Queries, g social.GuildRecord) error {
	if err := q.DeleteGuildRelations(ctx, g.ID); err != nil {
		return err
	}
	if len(g.Relations) == 0 {
		return nil
	}
	p := db.InsertGuildRelationsParams{GuildID: g.ID}
	for _, r := range g.Relations {
		p.OtherIds = append(p.OtherIds, r.Guild)
		p.Enemies = append(p.Enemies, r.Enemy)
	}
	return q.InsertGuildRelations(ctx, p)
}

// CreateGuild saves a new guild with its master. A guild whose master
// only lives in memory is not saved.
func (s *SocialStore) CreateGuild(g social.GuildRecord) {
	if len(g.Members) == 0 || g.Members[0].CharID == 0 {
		return
	}
	s.guildWrite(func(ctx context.Context, q *db.Queries) error {
		if err := q.InsertGuild(ctx, db.InsertGuildParams{ID: g.ID, Name: g.Name, Level: g.Level, Exp: g.Exp, SkillPoints: g.SkillPoints,
			Skills: skillsJSON(g.Skills), Positions: positionsJSON(g.Positions), NoticeTitle: g.Notice.Title, NoticeText: g.Notice.Text,
			Expulsions: expulsionsJSON(g.Expulsions), LastMasterChange: optionalTime(g.LastMasterChange)}); err != nil {
			return err
		}
		for _, m := range g.Members {
			if err := q.LeaveGuilds(ctx, m.CharID); err != nil {
				return err
			}
			if err := insertGuildMember(ctx, q, g.ID, m); err != nil {
				return err
			}
		}
		return nil
	})
}

// SaveGuild saves a guild's name, positions, notice, skills, relations,
// expulsions and master change time. Level, exp and skill points go
// through SaveGuildExp.
func (s *SocialStore) SaveGuild(g social.GuildRecord) {
	s.guildWrite(func(ctx context.Context, q *db.Queries) error {
		if err := q.UpdateGuild(ctx, db.UpdateGuildParams{ID: g.ID, Name: g.Name, Skills: skillsJSON(g.Skills),
			Positions: positionsJSON(g.Positions), NoticeTitle: g.Notice.Title, NoticeText: g.Notice.Text,
			Expulsions: expulsionsJSON(g.Expulsions), LastMasterChange: optionalTime(g.LastMasterChange)}); err != nil {
			return err
		}
		return writeRelations(ctx, q, g)
	})
}

// SaveGuildExp saves a guild's level, exp and skill points. Each call
// queues a write that runs only when no later call came, so many kills
// make one write. A write that runs again after a failed transaction
// writes the same values.
func (s *SocialStore) SaveGuildExp(guild int64, level int, exp int64, points int) {
	s.mu.Lock()
	if s.expSeq == nil {
		s.expSeq = map[int64]uint64{}
	}
	s.seq++
	seq := s.seq
	s.expSeq[guild] = seq
	s.mu.Unlock()
	s.guildWrite(func(ctx context.Context, q *db.Queries) error {
		s.mu.Lock()
		latest := s.expSeq[guild] == seq
		s.mu.Unlock()
		if !latest {
			return nil
		}
		return q.UpdateGuildExp(ctx, db.UpdateGuildExpParams{ID: guild, Level: level, Exp: exp, SkillPoints: points})
	})
}

// SaveGuildMemberExp saves what a member paid through the exp tax, with
// the same merging as SaveGuildExp.
func (s *SocialStore) SaveGuildMemberExp(guild, charID, exp int64) {
	if charID == 0 {
		return
	}
	k := [2]int64{guild, charID}
	s.mu.Lock()
	if s.memberSeq == nil {
		s.memberSeq = map[[2]int64]uint64{}
	}
	s.seq++
	seq := s.seq
	s.memberSeq[k] = seq
	s.mu.Unlock()
	s.guildWrite(func(ctx context.Context, q *db.Queries) error {
		s.mu.Lock()
		latest := s.memberSeq[k] == seq
		s.mu.Unlock()
		if !latest {
			return nil
		}
		return q.UpdateGuildMemberExp(ctx, db.UpdateGuildMemberExpParams{GuildID: guild, CharacterID: charID, Exp: exp})
	})
}

// SaveGuildEmblem saves a guild's emblem.
func (s *SocialStore) SaveGuildEmblem(guild int64, e social.Emblem) {
	s.guildWrite(func(ctx context.Context, q *db.Queries) error {
		return q.UpsertGuildEmblem(ctx, db.UpsertGuildEmblemParams{GuildID: guild, Version: e.Version, Png: e.PNG})
	})
}

// DeleteGuild deletes a guild with its members, relations, emblem and
// storage.
func (s *SocialStore) DeleteGuild(id int64) {
	s.guildWrite(func(ctx context.Context, q *db.Queries) error { return q.DeleteGuild(ctx, id) })
}

// AddGuildMember saves a member who joined.
func (s *SocialStore) AddGuildMember(guild int64, m social.GuildMemberRecord) {
	if m.CharID == 0 {
		return
	}
	s.guildWrite(func(ctx context.Context, q *db.Queries) error {
		if err := q.LeaveGuilds(ctx, m.CharID); err != nil {
			return err
		}
		return insertGuildMember(ctx, q, guild, m)
	})
}

// SaveGuildMember saves a member's slot and position.
func (s *SocialStore) SaveGuildMember(guild int64, m social.GuildMemberRecord) {
	if m.CharID == 0 {
		return
	}
	s.guildWrite(func(ctx context.Context, q *db.Queries) error { return insertGuildMember(ctx, q, guild, m) })
}

// RemoveGuildMember saves a member who left.
func (s *SocialStore) RemoveGuildMember(guild, charID int64, _ string) {
	if charID == 0 {
		return
	}
	s.guildWrite(func(ctx context.Context, q *db.Queries) error {
		return q.DeleteGuildMember(ctx, db.DeleteGuildMemberParams{GuildID: guild, CharacterID: charID})
	})
}
