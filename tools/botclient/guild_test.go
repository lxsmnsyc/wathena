package botclient

import (
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// offline is a bot without a connection, for tests that feed it frames.
func offline(netID uint32) *Bot {
	return &Bot{Name: "Alice", netID: netID, seen: map[uint32]*Entity{}, hit: map[uint32]bool{}, sent: map[uint32]time.Time{},
		done: make(chan struct{}), ready: make(chan struct{}), record: true, changed: make(chan struct{}), game: newGameState()}
}

func frame(events ...*wathenav1.Event) *wathenav1.ServerMessage {
	return wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{Events: events}.Build()}.Build()
}

// TestGuildState: the bot keeps its guild from GuildInfo with member
// updates merged in, an open invite, its storage window, and the guild
// badges of the entities it sees.
func TestGuildState(t *testing.T) {
	b := offline(1)
	badge := wathenav1.GuildBadge_builder{GuildId: 7, Name: "Crew", Position: "Newbie"}.Build()
	b.apply(frame(
		wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{Id: 1, Kind: wathenav1.EntityKind_ENTITY_KIND_PLAYER, Name: "Alice"}.Build()}.Build(),
		wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{Id: 2, Kind: wathenav1.EntityKind_ENTITY_KIND_PLAYER, Name: "Bob", Guild: badge}.Build()}.Build(),
		wathenav1.Event_builder{GuildInvite: wathenav1.GuildInvite_builder{GuildId: 7, GuildName: "Crew", From: "Bob"}.Build()}.Build(),
	))
	if inv := b.Game().GuildInvite; inv.GetGuildId() != 7 || inv.GetFrom() != "Bob" {
		t.Fatalf("invite %v", inv)
	}
	if e := b.View().Entities; len(e) != 1 || e[0].Guild.GetName() != "Crew" || e[0].Guild.GetPosition() != "Newbie" {
		t.Fatalf("entities %+v", e)
	}

	member := func(name string, pos uint32, exp int64) *wathenav1.GuildMember {
		return wathenav1.GuildMember_builder{Name: name, Position: pos, Online: true, Map: "map/test", Exp: exp, Master: pos == 0}.Build()
	}
	b.apply(frame(
		wathenav1.Event_builder{GuildInfo: wathenav1.GuildInfo_builder{Id: 7, Name: "Crew", Level: 2, Exp: 5, NextExp: 400000, SkillPoints: 1,
			Master: "Bob", EmblemVersion: 1, Position: 19, Members: []*wathenav1.GuildMember{member("Bob", 0, 0), member("Alice", 19, 0)},
			Skills: []*wathenav1.GuildSkill{wathenav1.GuildSkill_builder{SkillId: 10000, Level: 1, MaxLevel: 1}.Build()}}.Build()}.Build(),
		wathenav1.Event_builder{GuildMember: wathenav1.GuildMemberUpdate_builder{Member: member("Alice", 19, 5)}.Build()}.Build(),
		wathenav1.Event_builder{EntityGuild: wathenav1.EntityGuild_builder{Id: 1, Guild: badge}.Build()}.Build(),
		wathenav1.Event_builder{EntityGuild: wathenav1.EntityGuild_builder{Id: 2}.Build()}.Build(),
	))
	g := b.Game()
	if g.GuildInvite != nil {
		t.Error("the invite is still open after joining")
	}
	if g.Guild == nil || g.Guild.ID != 7 || g.Guild.Level != 2 || g.Guild.Skill(10000) != 1 || g.Guild.Emblem != 1 || g.Guild.Position != 19 {
		t.Fatalf("guild %+v", g.Guild)
	}
	if m, ok := g.Guild.Member("alice"); !ok || m.Exp != 5 || m.Position != 19 {
		t.Errorf("Alice in the guild: %+v", m)
	}
	if m, ok := g.Guild.Member("Bob"); !ok || !m.Master {
		t.Errorf("Bob in the guild: %+v", m)
	}
	if v := b.View(); v.Guild.GetName() != "Crew" || v.Entities[0].Guild != nil {
		t.Errorf("own badge %v, Bob's %v", v.Guild, v.Entities[0].Guild)
	}

	item := func(slot, id, amount uint32) *wathenav1.InventoryItem {
		return wathenav1.InventoryItem_builder{Slot: slot, ItemId: id, Amount: amount}.Build()
	}
	b.apply(frame(
		wathenav1.Event_builder{StorageOpen: wathenav1.StorageOpen_builder{Kind: wathenav1.StorageKind_STORAGE_KIND_GUILD, Size: 200,
			Items: []*wathenav1.InventoryItem{item(0, 909, 3)}}.Build()}.Build(),
		wathenav1.Event_builder{StorageAdd: wathenav1.StorageAdd_builder{Kind: wathenav1.StorageKind_STORAGE_KIND_GUILD, Item: item(0, 909, 5), Amount: 2}.Build()}.Build(),
		wathenav1.Event_builder{StorageAdd: wathenav1.StorageAdd_builder{Kind: wathenav1.StorageKind_STORAGE_KIND_GUILD, Item: item(1, 501, 1), Amount: 1}.Build()}.Build(),
		wathenav1.Event_builder{StorageRemove: wathenav1.StorageRemove_builder{Kind: wathenav1.StorageKind_STORAGE_KIND_GUILD, Slot: 1, Amount: 1}.Build()}.Build(),
	))
	if st := b.Game().Storage; st == nil || st.Size != 200 || st.Count(909) != 5 || st.Count(501) != 0 || len(st.Items) != 1 {
		t.Fatalf("storage %+v", st)
	}
	b.apply(frame(
		wathenav1.Event_builder{StorageClose: wathenav1.StorageClose_builder{Kind: wathenav1.StorageKind_STORAGE_KIND_GUILD}.Build()}.Build(),
		wathenav1.Event_builder{GuildInfo: wathenav1.GuildInfo_builder{}.Build()}.Build(),
	))
	if g := b.Game(); g.Storage != nil || g.Guild != nil {
		t.Errorf("after closing and leaving: storage %+v, guild %+v", g.Storage, g.Guild)
	}
}
