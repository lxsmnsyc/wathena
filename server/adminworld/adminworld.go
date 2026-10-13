// Package adminworld is the world side of the admin API (proposal 0002
// review decisions 33 and 34, decisions 0028 and 0029). It implements
// admin.World in the all-in-one build:
//   - Role changes go to the world's staff registry.
//   - An online character is read from the zone that holds it, on that
//     zone's goroutine. The chat hub says which map a character is on.
//     An offline character is read from the game's database.
//   - Item history and the character logs read the item and zeny logs.
//   - Restores run in a zone, through the transfers and the mail, in
//     one commit (restore.go).
//
// A distributed build can serve the same calls over the network from
// the world service.
package adminworld

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/currency"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// Config configures a World.
type Config struct {
	// Staff is the world's staff registry.
	Staff *staff.Registry
	World *world.World
	// Store is the game's store. Without it, only role changes work.
	Store *store.Store
	// Chat is the world's chat hub, which knows where players are.
	Chat *chat.Hub
	// Items, Jobs, StorageRecords and Currencies name things for people.
	Items          *item.Table
	Jobs           *status.Tables
	StorageRecords []*storage.Record
	Currencies     *currency.Table
	// Storages, Mail and Social are the world's registries, which
	// restores take items from.
	Storages *storage.Registry
	Mail     *mail.Registry
	Social   *social.Registry
	// Kicker closes game connections, for kicks and bans (decision
	// 0030). Without it, kicks find no one online.
	Kicker Kicker
	// Clock is the server clock, which mutes and jails run on.
	// time.Now when nil.
	Clock func() time.Time
	Log   *slog.Logger
	// Tools are what the world, events and mods tools need (decision
	// 0031).
	Tools Tools
}

// World is admin.World for the all-in-one build.
type World struct {
	cfg   Config
	log   *slog.Logger
	tools tools
}

var _ admin.World = (*World)(nil)

// New makes a World.
func New(cfg Config) *World {
	w := &World{cfg: cfg, log: cfg.Log}
	if w.log == nil {
		w.log = slog.Default()
	}
	w.initTools()
	return w
}

// errNoStore is a call that needs the database.
var errNoStore = &admin.WorldError{Code: "unavailable", Message: "The game has no database."}

// StaffChanged passes a role change to the staff registry.
func (w *World) StaffChanged(ctx context.Context, account int64, role string) error {
	if w.cfg.Staff == nil {
		return nil
	}
	return w.cfg.Staff.StaffChanged(ctx, account, role)
}

// locate finds the zones of online characters. The chat hub knows most.
// A character that has a session but that the hub does not list, such
// as an autotrader without its connection, is looked for in every zone.
func (w *World) locate(ctx context.Context, ids []int64) (map[int64]*zone.Zone, error) {
	out := map[int64]*zone.Zone{}
	var maps map[int64]string
	if w.cfg.Chat != nil {
		maps = w.cfg.Chat.Locate(ids)
	}
	for id, m := range maps {
		if z := w.cfg.World.Zone(m); z != nil {
			out[id] = z
		}
	}
	for _, id := range ids {
		if out[id] != nil || w.cfg.Store == nil || !w.cfg.Store.Live(id) {
			continue
		}
		for _, z := range w.cfg.World.Zones() {
			here, err := z.HasCharacter(ctx, id)
			if err != nil {
				return nil, err
			}
			if here {
				out[id] = z
				break
			}
		}
	}
	return out, nil
}

// Characters reads whether characters are online, where, and their
// sanctions in effect.
func (w *World) Characters(ctx context.Context, ids []int64) (map[int64]admin.CharacterState, error) {
	out := map[int64]admin.CharacterState{}
	zones, err := w.locate(ctx, ids)
	if err != nil {
		return nil, err
	}
	byZone := map[*zone.Zone][]int64{}
	for id, z := range zones {
		byZone[z] = append(byZone[z], id)
	}
	for z, list := range byZone {
		pos, err := z.StaffPositions(ctx, list)
		if err != nil {
			return nil, err
		}
		for id, p := range pos {
			out[id] = admin.CharacterState{Online: true, Position: position(p.Map, p.X, p.Y)}
		}
	}
	if w.cfg.Store != nil && len(ids) > 0 {
		rows, err := w.cfg.Store.Queries().SanctionsOf(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			st := out[r.CharacterID]
			m := sanction(character.Sanction{Kind: r.Kind, Reason: r.Reason, Start: r.StartedAt.Time, End: r.EndsAt.Time, By: r.IssuedBy})
			m.SetId(r.ID)
			m.SetCharacterId(r.CharacterID)
			m.SetActive(true)
			st.Sanctions = append(st.Sanctions, m)
			out[r.CharacterID] = st
		}
	}
	return out, nil
}

// Character reads a character's details: from its zone when online,
// else from the database.
func (w *World) Character(ctx context.Context, id int64) (*adminv1.CharacterDetail, error) {
	zones, err := w.locate(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	if z := zones[id]; z != nil {
		v, found, err := z.StaffView(ctx, id)
		if err != nil {
			return nil, err
		}
		if found {
			storages := make([]store.StorageView, 0, len(v.Storages))
			for _, s := range v.Storages {
				storages = append(storages, store.StorageView{Key: s.Key, Items: s.Items})
			}
			d := w.detail(&store.OfflineView{Character: v.Character, Party: v.Party, Guild: v.Guild, Storages: storages, Mails: v.Mails,
				AccountCurrencies: v.AccountCurrencies})
			d.SetOnline(true)
			d.SetPosition(position(v.Map, v.X, v.Y))
			return d, w.addBan(ctx, d)
		}
	}
	if w.cfg.Store == nil {
		return nil, admin.ErrNotFound
	}
	v, err := w.cfg.Store.ReadCharacter(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, admin.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d := w.detail(v)
	return d, w.addBan(ctx, d)
}

// addBan adds the account's ban in effect to a character's details.
func (w *World) addBan(ctx context.Context, d *adminv1.CharacterDetail) error {
	b, err := w.banOf(ctx, d.GetAccountId())
	if b != nil {
		d.SetBan(b)
	}
	return err
}

// detail builds a character's details from what was read.
func (w *World) detail(v *store.OfflineView) *adminv1.CharacterDetail {
	c := v.Character
	d := adminv1.CharacterDetail_builder{
		Id: c.ID, Name: c.Name, AccountId: c.AccountID, Job: int32(c.Job), JobName: w.jobName(c.Job), Male: c.Male, // #nosec G115 -- job IDs are small.
		BaseLevel: int32(c.BaseLevel), JobLevel: int32(c.JobLevel), BaseExp: c.BaseExp, JobExp: c.JobExp, // #nosec G115 -- levels up to 275.
		Stats: adminv1.Stats_builder{Str: int32(c.Stats.Str), Agi: int32(c.Stats.Agi), Vit: int32(c.Stats.Vit), // #nosec G115 -- stats up to 65535.
			Int: int32(c.Stats.Int), Dex: int32(c.Stats.Dex), Luk: int32(c.Stats.Luk)}.Build(), // #nosec G115 -- stats up to 65535.
		StatusPoints: int32(c.StatusPoints), SkillPoints: int32(c.SkillPoints), // #nosec G115 -- points fit.
		Hp: c.HP, Sp: c.SP, Zeny: c.Zeny, Position: position(c.Map, c.X, c.Y), Party: v.Party, Guild: v.Guild,
	}.Build()
	if c.SaveMap != "" {
		d.SetSavePoint(position(c.SaveMap, c.SaveX, c.SaveY))
	}
	if !c.CreatedAt.IsZero() {
		d.SetCreatedAt(timestamppb.New(c.CreatedAt))
	}
	var cur []*adminv1.Currency
	for _, id := range sortedKeys(c.Currencies) {
		cur = append(cur, adminv1.Currency_builder{Id: id, Name: w.currencyName(id), Amount: c.Currencies[id]}.Build())
	}
	for _, id := range sortedKeys(v.AccountCurrencies) {
		cur = append(cur, adminv1.Currency_builder{Id: id, Name: w.currencyName(id), Amount: v.AccountCurrencies[id], Account: true}.Build())
	}
	d.SetCurrencies(cur)
	d.SetInventory(w.items(c.Items))
	var sts []*adminv1.Storage
	for _, s := range v.Storages {
		sts = append(sts, w.storage(s.Key, s.Items))
	}
	d.SetStorages(sts)
	var mails []*adminv1.Mail
	for _, m := range v.Mails {
		mails = append(mails, adminv1.Mail_builder{Id: m.ID, From: m.FromName, Title: m.Title, SentAt: timestamppb.New(m.Sent), Zeny: m.Zeny,
			Read: m.Read, Returned: m.Returned, Items: w.items(m.Items)}.Build())
	}
	d.SetMails(mails)
	var sanctions []*adminv1.Sanction
	now := time.Now()
	for _, s := range c.Sanctions {
		if s.End.After(now) {
			sanctions = append(sanctions, sanction(s))
		}
	}
	d.SetSanctions(sanctions)
	return d
}

// storage is a storage's message.
func (w *World) storage(k storage.Key, items []character.Item) *adminv1.Storage {
	s := adminv1.Storage_builder{Storage: k.Storage, Items: w.items(items)}
	switch {
	case k.Holder == storage.HolderCart:
		s.Kind = "cart"
	case k.Storage == 0:
		s.Kind = "storage"
	default:
		s.Kind = "extra"
		if r := w.record(k.Storage); r != nil {
			s.Name = r.DisplayName
		}
	}
	return s.Build()
}

// items are item messages.
func (w *World) items(list []character.Item) []*adminv1.Item {
	out := make([]*adminv1.Item, 0, len(list))
	for _, it := range list {
		m := adminv1.Item_builder{Serial: it.Serial.String(), ItemId: it.ID, Name: w.itemName(it.ID), Amount: int32(it.Amount), // #nosec G115 -- a stack.
			Slot: int32(it.Slot), Refine: int32(it.Refine), Cards: slices.Clone(it.Cards), Equip: it.Equip, // #nosec G115 -- slots and refines are small.
			Identified: !it.Unidentified, Bound: int32(it.Bound)}
		if len(it.Rolls) > 0 {
			m.Rolls = map[string]int32{}
			for _, r := range it.Rolls {
				m.Rolls[r.Name] = int32(r.Value) // #nosec G115 -- roll values are small.
			}
		}
		out = append(out, m.Build())
	}
	return out
}

// JobName is a job's name for people, or "".
func (w *World) JobName(job int) string { return w.jobName(job) }

func (w *World) jobName(job int) string {
	if w.cfg.Jobs == nil {
		return ""
	}
	return w.cfg.Jobs.DisplayName(job)
}

func (w *World) itemName(id uint32) string {
	if w.cfg.Items == nil {
		return ""
	}
	if d, ok := w.cfg.Items.Lookup(id); ok {
		return d.Name
	}
	return ""
}

func (w *World) currencyName(id uint32) string {
	if w.cfg.Currencies == nil {
		return ""
	}
	if r := w.cfg.Currencies.ByID(id); r != nil {
		if r.DisplayName != "" {
			return r.DisplayName
		}
		return r.Name
	}
	return ""
}

func (w *World) record(id uint32) *storage.Record {
	for _, r := range w.cfg.StorageRecords {
		if r.ID == id {
			return r
		}
	}
	return nil
}

func position(m string, x, y int) *adminv1.Position {
	return adminv1.Position_builder{Map: strings.TrimPrefix(m, "map/"), X: int32(x), Y: int32(y)}.Build() // #nosec G115 -- cells below 1024.
}

func sanction(s character.Sanction) *adminv1.Sanction {
	return adminv1.Sanction_builder{Kind: s.Kind, Reason: s.Reason, StartedAt: timestamppb.New(s.Start), EndsAt: timestamppb.New(s.End),
		IssuedBy: s.By}.Build()
}

func sortedKeys(m map[uint32]int64) []uint32 {
	out := make([]uint32, 0, len(m))
	for k, v := range m {
		if v != 0 {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}
