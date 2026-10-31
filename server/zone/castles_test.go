package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/castle"
)

// flag shows the badge of its castle's owner, as rAthena's flags run
// flagemblem on OnRecvCastle; castle_change keeps it current.
const flag = `
local events = require "wa.events"
local castles = require "wa.castles"
return function()
  local npc = require("wa.context"):get_instance()
  local castle = castles.get("prtg_cas01")
  npc:set_guild(castle.owner)
  events.on("castle_change", function(change)
    if change.castle == castle then
      npc:set_guild(castle.owner)
    end
  end)
end`

func entityBadges(evs []*wathenav1.Event, id uint32) (appear, change []*wathenav1.GuildBadge, changed bool) {
	for _, ev := range evs {
		if a := ev.GetAppear(); a != nil && a.GetId() == id {
			appear = append(appear, a.GetGuild())
		}
		if g := ev.GetEntityGuild(); g != nil && g.GetId() == id {
			change, changed = append(change, g.GetGuild()), true
		}
	}
	return appear, change, changed
}

// TestCastleFlags checks npc:set_guild, the badge in EntityAppear and
// EntityGuild, castle_change in a zone, a new emblem and a guild break.
func TestCastleFlags(t *testing.T) {
	reg := castle.New([]*castle.Record{{ID: 15, Name: "Kriemhild", Map: "prtg_cas01", Edition: "fe", NPC: "Agit#prtg_cas01"}}, nil)
	var guild int64
	z, p, n, logs := npcZoneWith(t, flag, func(z *Zone) {
		z.Castles = reg
		z.castleFeed = reg.Subscribe(nil)
		z.Social = z.socialHub()
		z.Social.Castles = reg
	})
	if n.npc.badge != nil {
		t.Fatalf("an unowned castle's flag shows a badge: %v", n.npc.badge)
	}
	guild = formGuild(t, z, p)
	id, _ := p.netIDs.get(n)

	// An owner change reaches the zone's castle_change handlers.
	reg.SetOwner("Kriemhild", guild, castle.SourceScript)
	z.stepAt(z.now + 50)
	_, change, changed := entityBadges(events(p), id)
	if !changed || change[0].GetGuildId() != uint64(guild) || change[0].GetName() != "Guild of a" {
		t.Fatalf("EntityGuild %v, logs %s", change, logs)
	}
	if b := z.appear(id, n).GetAppear().GetGuild(); b.GetGuildId() != uint64(guild) {
		t.Errorf("EntityAppear's badge %v", b)
	}

	// A new emblem sends EntityGuild with its version.
	if _, _, err := z.Social.SetEmblem(guild, []byte("png")); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	if _, change, _ := entityBadges(events(p), id); len(change) != 1 || change[0].GetEmblemVersion() != 1 {
		t.Errorf("after a new emblem: %v", change)
	}

	// A guild that breaks up clears the castle's owner and the flag's
	// badge.
	if out, _ := z.Social.GuildDisband(p.soc, "Guild of a"); out == 0 {
		t.Fatal("disband")
	}
	z.stepAt(z.now + 50)
	if _, change, changed := entityBadges(events(p), id); !changed || change[len(change)-1] != nil {
		t.Errorf("after a break up: %v", change)
	}
	if s, _ := reg.State("Kriemhild"); s.Owner != 0 {
		t.Errorf("the castle kept its owner: %+v", s)
	}
}
