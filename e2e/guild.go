package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// Guild and guild storage steps (phase 5 step 5).

// Items and guild skills of the guild scenarios.
const (
	// Emperium makes a guild with the default rules
	// (guild_emperium_check).
	Emperium uint32 = 714
	// The guild skills, as rAthena's GD_ skills.
	SkillApproval      uint32 = 10000
	SkillGuardResearch uint32 = 10002
	SkillExtension     uint32 = 10004
	SkillLeadership    uint32 = 10006
	SkillGloryWounds   uint32 = 10007
	SkillSoulCold      uint32 = 10008
	SkillHawkEyes      uint32 = 10009
	SkillBattleOrder   uint32 = 10010
	SkillRegeneration  uint32 = 10011
	SkillEmergencyCall uint32 = 10013
	SkillGuildStorage  uint32 = 10016
	// NewbiePosition is the position of a new member: the last of the
	// default 20.
	NewbiePosition uint32 = 19
	// GuildRuleset is the ruleset mod in testdata/mods that allows
	// Emergency Call outside GvG maps.
	GuildRuleset = "e2e.guildrules"
)

// EmergencyCallSkills are the skills to raise, in order, for Emergency
// Call: 10 points, so guild level 11.
var EmergencyCallSkills = []uint32{SkillApproval, SkillGuardResearch,
	SkillExtension, SkillExtension, SkillExtension, SkillExtension, SkillExtension,
	SkillBattleOrder, SkillRegeneration, SkillEmergencyCall}

// GuildStorageSkills are the skills to raise, in order, for renewal's
// Guild Storage: 20 points, so guild level 21.
var GuildStorageSkills = append(slices.Clone(EmergencyCallSkills),
	SkillExtension, SkillExtension, SkillExtension, SkillExtension, SkillExtension,
	SkillGloryWounds, SkillSoulCold, SkillLeadership, SkillHawkEyes, SkillGuildStorage)

// GuildPosition is one position for SetPositions.
func GuildPosition(index uint32, name string, invite, expel, storage bool, tax uint32) *wathenav1.GuildPosition {
	return wathenav1.GuildPosition_builder{Index: index, Name: name, CanInvite: invite, CanExpel: expel, CanUseStorage: storage, ExpTax: tax}.Build()
}

// CreateGuild makes a guild with the player as its master, and returns
// the GuildInfo that follows GUILD_OUTCOME_CREATED.
func (p *Player) CreateGuild(name string) *wathenav1.GuildInfo {
	p.t.Helper()
	p.send("create a guild", func(ctx context.Context) error { return p.GuildCreate(ctx, name) })
	if r := p.GuildResult(); r.GetOutcome() != wathenav1.GuildOutcome_GUILD_OUTCOME_CREATED || r.GetName() != name {
		p.t.Fatalf("%s: creating guild %s: %v", p.Name, name, r)
	}
	return p.GuildInfo(func(info *wathenav1.GuildInfo) bool { return info.GetName() == name })
}

// GuildResult waits for the next GuildResult.
func (p *Player) GuildResult() *wathenav1.GuildResult {
	p.t.Helper()
	return p.Expect("a guild result", (*wathenav1.Event).HasGuildResult).GetGuildResult()
}

// GuildOutcome waits for a GuildResult with an outcome, and fails the
// test on a GuildResult about the same name with another outcome.
func (p *Player) GuildOutcome(want wathenav1.GuildOutcome, name string) *wathenav1.GuildResult {
	p.t.Helper()
	return p.Expect(fmt.Sprintf("guild result %s for %q", want, name), func(e *wathenav1.Event) bool {
		r := e.GetGuildResult()
		if r == nil {
			return false
		}
		if r.GetName() == name && r.GetOutcome() != want && failed(r.GetOutcome()) {
			p.t.Fatalf("%s: wanted %s for %q, got %v", p.Name, want, name, r)
		}
		return r.GetOutcome() == want && r.GetName() == name
	}).GetGuildResult()
}

// failed reports whether a guild outcome is a refusal.
func failed(o wathenav1.GuildOutcome) bool {
	switch o {
	case wathenav1.GuildOutcome_GUILD_OUTCOME_NO_GUILD, wathenav1.GuildOutcome_GUILD_OUTCOME_NO_PERMISSION,
		wathenav1.GuildOutcome_GUILD_OUTCOME_NOT_MASTER, wathenav1.GuildOutcome_GUILD_OUTCOME_NOT_MEMBER,
		wathenav1.GuildOutcome_GUILD_OUTCOME_SKILL_REFUSED, wathenav1.GuildOutcome_GUILD_OUTCOME_NOT_ONLINE:
		return true
	}
	return false
}

// GuildInfo waits for a GuildInfo that ok accepts.
func (p *Player) GuildInfo(ok func(*wathenav1.GuildInfo) bool) *wathenav1.GuildInfo {
	p.t.Helper()
	return p.Expect("the guild", func(e *wathenav1.Event) bool { return e.HasGuildInfo() && ok(e.GetGuildInfo()) }).GetGuildInfo()
}

// InviteToGuild invites a player by name and returns the outcome.
func (p *Player) InviteToGuild(name string) *wathenav1.GuildResult {
	p.t.Helper()
	p.send("invite "+name, func(ctx context.Context) error { return p.GuildInvite(ctx, name) })
	return p.GuildResult()
}

// GuildInvited waits for a guild invite from a player.
func (p *Player) GuildInvited(from string) *wathenav1.GuildInvite {
	p.t.Helper()
	return p.Expect("a guild invite from "+from, func(e *wathenav1.Event) bool {
		return e.HasGuildInvite() && e.GetGuildInvite().GetFrom() == from
	}).GetGuildInvite()
}

// AcceptGuild accepts a guild invite and returns the guild the player
// joined.
func (p *Player) AcceptGuild(inv *wathenav1.GuildInvite) *wathenav1.GuildInfo {
	p.t.Helper()
	p.send("accept the invite", func(ctx context.Context) error { return p.GuildReply(ctx, inv.GetGuildId(), true) })
	return p.GuildInfo(func(info *wathenav1.GuildInfo) bool { return info.GetId() == inv.GetGuildId() })
}

// MakeGuild makes a guild with master as its master, who needs an
// Emperium, and invites the members, who accept. It returns when every
// player's guild lists them all.
func MakeGuild(name string, master *Player, members ...*Player) uint64 {
	master.t.Helper()
	id := master.CreateGuild(name).GetId()
	for _, m := range members {
		if r := master.InviteToGuild(m.Name); r.GetOutcome() != wathenav1.GuildOutcome_GUILD_OUTCOME_INVITE_SENT {
			master.t.Fatalf("inviting %s: %v", m.Name, r)
		}
		m.AcceptGuild(m.GuildInvited(master.Name))
		master.GuildOutcome(wathenav1.GuildOutcome_GUILD_OUTCOME_ACCEPTED, m.Name)
	}
	all := append([]*Player{master}, members...)
	for _, p := range all {
		p.Until(fmt.Sprintf("%d members in %s", len(all), name), func(_ View, g Game) bool {
			return g.Guild != nil && g.Guild.ID == id && len(g.Guild.Members) == len(all)
		})
	}
	return id
}

// SetPositions changes positions as the master, and waits for the
// GuildInfo that shows them.
func (p *Player) SetPositions(positions ...*wathenav1.GuildPosition) *wathenav1.GuildInfo {
	p.t.Helper()
	p.send("change positions", func(ctx context.Context) error { return p.GuildPositions(ctx, positions...) })
	return p.Expect("the new positions", func(e *wathenav1.Event) bool {
		if r := e.GetGuildResult(); r != nil && failed(r.GetOutcome()) {
			p.t.Fatalf("%s: changing positions: %v", p.Name, r)
		}
		info := e.GetGuildInfo()
		if info == nil {
			return false
		}
		have := info.GetPositions()
		for _, want := range positions {
			i := int(want.GetIndex())
			if i >= len(have) || have[i].GetName() != want.GetName() || have[i].GetCanInvite() != want.GetCanInvite() ||
				have[i].GetCanExpel() != want.GetCanExpel() || have[i].GetCanUseStorage() != want.GetCanUseStorage() ||
				have[i].GetExpTax() != want.GetExpTax() {
				return false
			}
		}
		return true
	}).GetGuildInfo()
}

// SetMemberPosition gives a member a position as the master, and waits
// for GUILD_OUTCOME_POSITION_CHANGED.
func (p *Player) SetMemberPosition(name string, position uint32) {
	p.t.Helper()
	p.send("give "+name+" a position", func(ctx context.Context) error { return p.GuildMemberPosition(ctx, name, position) })
	p.GuildOutcome(wathenav1.GuildOutcome_GUILD_OUTCOME_POSITION_CHANGED, name)
}

// SetGuildNotice changes the notice as the master, and waits for the
// GuildInfo that shows it.
func (p *Player) SetGuildNotice(title, text string) {
	p.t.Helper()
	p.send("change the notice", func(ctx context.Context) error { return p.GuildNotice(ctx, title, text) })
	p.GuildInfo(func(info *wathenav1.GuildInfo) bool {
		return info.GetNotice().GetTitle() == title && info.GetNotice().GetText() == text
	})
}

// LevelGuild says the fixture's "!guild level <n>", which gives the
// player's guild the exp to reach level n, and waits for the GuildInfo
// at that level.
func (p *Player) LevelGuild(level int) *wathenav1.GuildInfo {
	p.t.Helper()
	p.Say("!guild level " + strconv.Itoa(level))
	return p.GuildInfo(func(info *wathenav1.GuildInfo) bool { return int(info.GetLevel()) == level })
}

// RaiseGuildSkill raises a guild skill as the master, and returns the
// GuildInfo with its new level.
func (p *Player) RaiseGuildSkill(skill uint32) *wathenav1.GuildInfo {
	p.t.Helper()
	before := p.Game().Guild.Skill(skill)
	p.send("raise a guild skill", func(ctx context.Context) error { return p.GuildSkillUp(ctx, skill) })
	p.GuildOutcome(wathenav1.GuildOutcome_GUILD_OUTCOME_SKILL_RAISED, strconv.Itoa(int(skill)))
	return p.GuildInfo(func(info *wathenav1.GuildInfo) bool { return guildSkill(info, skill) == before+1 })
}

// guildSkill is a guild skill's level in a GuildInfo.
func guildSkill(info *wathenav1.GuildInfo, skill uint32) int {
	for _, s := range info.GetSkills() {
		if s.GetSkillId() == skill {
			return int(s.GetLevel())
		}
	}
	return 0
}

// SayToGuild says a line to the guild and waits until the player hears
// it back.
func (p *Player) SayToGuild(text string) *wathenav1.ChatMessage {
	p.t.Helper()
	p.send("guild chat", func(ctx context.Context) error { return p.GuildChat(ctx, text) })
	return p.HearsGuild(p.Name, text)
}

// HearsGuild waits for a guild line from a member.
func (p *Player) HearsGuild(from, text string) *wathenav1.ChatMessage {
	p.t.Helper()
	return p.Expect(fmt.Sprintf("%s to say %q to the guild", from, text), func(e *wathenav1.Event) bool {
		if r := e.GetChatRefused(); r != nil {
			p.t.Fatalf("%s: a line was refused: %s", p.Name, r.GetReason())
		}
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_GUILD && c.GetFromName() == from && c.GetText() == text
	}).GetChat()
}

// OpenGuildStorage opens the player's guild storage through the server's
// debug endpoint, as an NPC would, and returns the window.
func (p *Player) OpenGuildStorage(s *Server) *wathenav1.StorageOpen {
	p.t.Helper()
	ctx, cancel := p.ctx()
	defer cancel()
	o, err := s.OpenGuildStorage(ctx, p.Name)
	if err != nil {
		p.t.Fatalf("%s: opening the guild storage: %v", p.Name, err)
	}
	if o != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
		p.t.Fatalf("%s: opening the guild storage: %s", p.Name, o)
	}
	return p.Expect("the guild storage", (*wathenav1.Event).HasStorageOpen).GetStorageOpen()
}

// StorePut puts an amount of an item from its first inventory slot into
// the open storage, and returns the StorageAdd.
func (p *Player) StorePut(id uint32, amount int) *wathenav1.StorageAdd {
	p.t.Helper()
	slot := p.Game().SlotOf(id)
	if slot < 0 {
		p.t.Fatalf("%s has no item %d to store", p.Name, id)
	}
	p.send("store", func(ctx context.Context) error { return p.StoragePut(ctx, slot, amount) })
	return p.Expect(fmt.Sprintf("item %d in the storage", id), func(e *wathenav1.Event) bool {
		if r := e.GetStorageResult(); r != nil {
			p.t.Fatalf("%s: storing item %d failed: %s", p.Name, id, r.GetOutcome())
		}
		a := e.GetStorageAdd()
		return a != nil && a.GetItem().GetItemId() == id
	}).GetStorageAdd()
}

// StoreTake takes an amount of an item from its first storage slot, and
// waits for the InventoryAdd.
func (p *Player) StoreTake(id uint32, amount int) *wathenav1.InventoryAdd {
	p.t.Helper()
	slot := -1
	if st := p.Game().Storage; st != nil {
		for s, it := range st.Items {
			if it.ID == id && (slot < 0 || s < slot) {
				slot = s
			}
		}
	}
	if slot < 0 {
		p.t.Fatalf("%s sees no item %d in the storage", p.Name, id)
	}
	p.send("take", func(ctx context.Context) error { return p.StorageTake(ctx, slot, amount) })
	return p.Expect(fmt.Sprintf("item %d from the storage", id), func(e *wathenav1.Event) bool {
		if r := e.GetStorageResult(); r != nil {
			p.t.Fatalf("%s: taking item %d failed: %s", p.Name, id, r.GetOutcome())
		}
		a := e.GetInventoryAdd()
		return a != nil && a.GetItemId() == id
	}).GetInventoryAdd()
}

// CloseStorage closes the storage window and waits for StorageClose.
func (p *Player) CloseStorage() {
	p.t.Helper()
	p.send("close the storage", func(ctx context.Context) error { return p.StorageClose(ctx) })
	p.Expect("the storage to close", (*wathenav1.Event).HasStorageClose)
}

// OpenGuildStorage opens a player's guild storage with --dev's
// POST /debug/wathena/guild-storage, as an NPC would. The outcome is
// the protocol's: STORAGE_OUTCOME_UNSPECIFIED for a storage that
// opened.
func (s *Server) OpenGuildStorage(ctx context.Context, name string) (wathenav1.StorageOutcome, error) {
	body, err := s.post(ctx, s.Debug+"/debug/wathena/guild-storage?name="+url.QueryEscape(name), nil)
	if err != nil {
		return 0, err
	}
	var out struct {
		Outcome int32 `json:"outcome"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, err
	}
	return wathenav1.StorageOutcome(out.Outcome), nil
}

// UploadEmblem sets a guild's emblem with --dev's
// POST /debug/wathena/emblem, and returns its version.
func (s *Server) UploadEmblem(ctx context.Context, guild string, image []byte) (int, error) {
	body, err := s.post(ctx, s.Debug+"/debug/wathena/emblem?guild="+url.QueryEscape(guild), image)
	if err != nil {
		return 0, err
	}
	var out struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, err
	}
	return out.Version, nil
}

// HTTPError is an answer from the server with a status other than 200.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("status %d: %s", e.Status, e.Body) }

func (s *Server) post(ctx context.Context, u string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G704 -- the test server's loopback debug address.
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{Status: resp.StatusCode, Body: strings.TrimSpace(string(data))}
	}
	return data, nil
}

// HTTP is the game server's base URL, such as http://127.0.0.1:8300.
func (s *Server) HTTP() string {
	return "http://" + strings.TrimSuffix(strings.TrimPrefix(s.WS, "ws://"), "/ws")
}

// Guild storage in the duplication test.

// StorageCounts count what the bots of a GuildStorageMove did.
type StorageCounts struct {
	// Opened counts the times a bot opened the storage, and InUse the
	// times the server refused with STORAGE_OUTCOME_IN_USE.
	Opened, InUse atomic.Int64
	// Put and Taken count the moves into and out of the storage.
	Put, Taken atomic.Int64
	// Dropped counts the connections closed with the storage open.
	Dropped atomic.Int64
}

// GuildStorageMove opens the bot's guild storage through the debug
// endpoint, as an NPC would. When another member has it open, the
// server refuses with STORAGE_OUTCOME_IN_USE. Otherwise the bot puts in
// and takes out parts of stacks of an item one to four times, and
// closes the storage. One time in 8 the bot closes its connection
// instead, with the storage open, and logs in again.
func GuildStorageMove(s *Server, id uint32, counts *StorageCounts) Move {
	return Move{Name: "storage", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		if b.Game().Storage != nil {
			// A move that ended with the test's time left it open.
			return false, closeStorage(ctx, b)
		}
		mark := b.Mark()
		o, err := s.OpenGuildStorage(ctx, b.Name)
		if err != nil {
			var he *HTTPError
			if errors.As(err, &he) && he.Status == http.StatusNotFound {
				// The bot's character is not in the world: its connection
				// closed.
				return false, nil
			}
			return false, ignoreDone(ctx, err)
		}
		switch o {
		case wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED:
		case wathenav1.StorageOutcome_STORAGE_OUTCOME_IN_USE:
			counts.InUse.Add(1)
			// The bot is told too.
			_, _, err := b.Wait(ctx, mark, func(e *wathenav1.Event) bool {
				return e.GetStorageResult().GetOutcome() == wathenav1.StorageOutcome_STORAGE_OUTCOME_IN_USE
			})
			return false, ignoreDone(ctx, err)
		default:
			return false, fmt.Errorf("opening the guild storage: %s", o)
		}
		if _, _, err := b.Wait(ctx, mark, (*wathenav1.Event).HasStorageOpen); err != nil {
			return false, ignoreDone(ctx, err)
		}
		counts.Opened.Add(1)
		for range 1 + rnd.IntN(4) {
			if err := storageStep(ctx, b, id, rnd, counts); err != nil {
				return true, ignoreDone(ctx, err)
			}
		}
		if rnd.IntN(8) == 0 {
			// The connection drops with the storage open. The server
			// closes it, so another bot can open it.
			counts.Dropped.Add(1)
			_ = b.CloseNow()
			return true, errLeft
		}
		return true, ignoreDone(ctx, closeStorage(ctx, b))
	}}
}

// storageStep puts part of a random stack of an item into the open
// storage, or takes part of a random storage stack out.
func storageStep(ctx context.Context, b *botclient.Bot, id uint32, rnd *rand.Rand, counts *StorageCounts) error {
	g := b.Game()
	var held, stored []botclient.Item
	for _, it := range g.Items {
		if it.ID == id && it.Equip == 0 {
			held = append(held, it)
		}
	}
	if g.Storage != nil {
		for _, it := range g.Storage.Items {
			stored = append(stored, it)
		}
	}
	bySlot := func(a, b botclient.Item) int { return a.Slot - b.Slot }
	slices.SortFunc(held, bySlot)
	slices.SortFunc(stored, bySlot)
	put := len(held) > 0 && (len(stored) == 0 || rnd.IntN(2) == 0)
	if !put && len(stored) == 0 {
		return nil
	}
	mark := b.Mark()
	if put {
		it := held[rnd.IntN(len(held))]
		if err := b.StoragePut(ctx, it.Slot, 1+rnd.IntN(it.Amount)); err != nil {
			return err
		}
		e, _, err := b.Wait(ctx, mark, func(e *wathenav1.Event) bool {
			r := e.GetInventoryRemove()
			return e.HasStorageResult() || e.HasStorageClose() ||
				r != nil && int(r.GetSlot()) == it.Slot && r.GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_STORED
		})
		if err != nil {
			return err
		}
		if r := e.GetStorageResult(); r != nil {
			return fmt.Errorf("putting item %d from slot %d into the storage: %s", id, it.Slot, r.GetOutcome())
		}
		if e.HasStorageClose() {
			return errors.New("the storage closed")
		}
		counts.Put.Add(1)
		return nil
	}
	it := stored[rnd.IntN(len(stored))]
	if err := b.StorageTake(ctx, it.Slot, 1+rnd.IntN(it.Amount)); err != nil {
		return err
	}
	e, _, err := b.Wait(ctx, mark, func(e *wathenav1.Event) bool {
		r := e.GetStorageRemove()
		return e.HasStorageResult() || e.HasStorageClose() || r != nil && int(r.GetSlot()) == it.Slot
	})
	if err != nil {
		return err
	}
	if r := e.GetStorageResult(); r != nil {
		return fmt.Errorf("taking item %d from storage slot %d: %s", id, it.Slot, r.GetOutcome())
	}
	if e.HasStorageClose() {
		return errors.New("the storage closed")
	}
	counts.Taken.Add(1)
	return nil
}

// closeStorage closes the bot's storage window and waits until it shows
// closed.
func closeStorage(ctx context.Context, b *botclient.Bot) error {
	if err := b.StorageClose(ctx); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return b.WaitState(wait, func(_ botclient.View, g botclient.Game) bool { return g.Storage == nil })
}

// SettleStorage closes a storage window a bot left open, then settles
// as next does.
func SettleStorage(next func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error)) func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
	return func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		if b.Game().Storage != nil {
			return true, closeStorage(ctx, b)
		}
		return next(ctx, b, rnd)
	}
}

// GuildStorageSetup puts every bot in one guild, led by the first bot,
// which needs an Emperium. The fixture takes the guild to level 21, the
// master raises the skills up to Guild Storage, and new members may use
// the storage.
func GuildStorageSetup(name string) func(ctx context.Context, bots []*botclient.Bot) error {
	return func(ctx context.Context, bots []*botclient.Bot) error {
		master := bots[0]
		if err := master.GuildCreate(ctx, name); err != nil {
			return err
		}
		if err := master.WaitState(ctx, func(_ botclient.View, g botclient.Game) bool { return g.Guild != nil }); err != nil {
			return fmt.Errorf("%s: no guild: %w", master.Name, err)
		}
		for i, m := range bots[1:] {
			if err := master.GuildInvite(ctx, m.Name); err != nil {
				return err
			}
			var inv *wathenav1.GuildInvite
			if err := m.WaitState(ctx, func(_ botclient.View, g botclient.Game) bool { inv = g.GuildInvite; return inv != nil }); err != nil {
				return fmt.Errorf("%s: no invite: %w", m.Name, err)
			}
			if err := m.GuildReply(ctx, inv.GetGuildId(), true); err != nil {
				return err
			}
			if err := master.WaitState(ctx, func(_ botclient.View, g botclient.Game) bool {
				return g.Guild != nil && len(g.Guild.Members) == i+2
			}); err != nil {
				return fmt.Errorf("%s did not join: %w", m.Name, err)
			}
		}
		if err := master.Chat(ctx, "!guild level 21"); err != nil {
			return err
		}
		if err := master.WaitState(ctx, func(_ botclient.View, g botclient.Game) bool {
			return g.Guild.Level == 21 && g.Guild.SkillPoints == len(GuildStorageSkills)
		}); err != nil {
			return fmt.Errorf("the guild did not reach level 21: %w", err)
		}
		for i, skill := range GuildStorageSkills {
			want := countOf(GuildStorageSkills[:i+1], skill)
			if err := master.GuildSkillUp(ctx, skill); err != nil {
				return err
			}
			if err := master.WaitState(ctx, func(_ botclient.View, g botclient.Game) bool { return g.Guild.Skill(skill) >= want }); err != nil {
				return fmt.Errorf("raising guild skill %d: %w", skill, err)
			}
		}
		newbie := GuildPosition(NewbiePosition, "Newbie", false, false, true, 0)
		if err := master.GuildPositions(ctx, newbie); err != nil {
			return err
		}
		for _, b := range bots {
			if err := b.WaitState(ctx, func(_ botclient.View, g botclient.Game) bool {
				gd := g.Guild
				return gd != nil && len(gd.Members) == len(bots) && gd.Skill(SkillGuildStorage) == 1 &&
					len(gd.Positions) > int(NewbiePosition) && gd.Positions[NewbiePosition].GetCanUseStorage()
			}); err != nil {
				return fmt.Errorf("%s: the guild is not ready: %w", b.Name, err)
			}
		}
		return nil
	}
}

// countOf counts a skill in a list.
func countOf(list []uint32, skill uint32) int {
	n := 0
	for _, s := range list {
		if s == skill {
			n++
		}
	}
	return n
}
