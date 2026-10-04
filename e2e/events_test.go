package e2e

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// Typed world events (phase 5 step 8, decision 0025). The fixture's
// events start from "!event start <name>", which calls
// world_events.start.

// Announced waits for an announcement with a text.
func (p *Player) Announced(text string) {
	p.t.Helper()
	p.Expect(fmt.Sprintf("the announcement %q", text), func(e *wathenav1.Event) bool {
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_ANNOUNCEMENT && c.GetText() == text
	})
}

// EventNotice waits for a world event's start or end notice.
func (p *Player) EventNotice(name string, started bool) *wathenav1.WorldEventNotice {
	p.t.Helper()
	return p.Expect(fmt.Sprintf("the notice that %s started %v", name, started), func(e *wathenav1.Event) bool {
		n := e.GetWorldEventNotice()
		return n != nil && n.GetEvent().GetName() == name && n.GetStarted() == started
	}).GetWorldEventNotice()
}

// StartEvent starts a fixture event and waits for its notice.
func (p *Player) StartEvent(name string) *wathenav1.WorldEventNotice {
	p.t.Helper()
	p.MustCommand("!event start " + name)
	return p.EventNotice("e2e.fixture:"+name, true)
}

// untilZeny waits until the player holds an amount of zeny.
func (p *Player) untilZeny(want int64) {
	p.t.Helper()
	p.Until(fmt.Sprintf("%d zeny", want), func(_ botclient.View, g botclient.Game) bool {
		return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == want
	})
}

func TestWorldEventBoss(t *testing.T) {
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	n := alice.StartEvent("Boss")
	if n.GetEvent().GetType() != "boss" || n.GetEvent().GetMap() != StartMap || n.GetEvent().GetDisplayName() != "Boss" || n.GetEvent().GetEndsAtMs() == 0 {
		t.Fatalf("notice %v", n)
	}
	alice.Announced("Boss has appeared on placeholder/map!")
	// A second start is refused while it runs.
	if got := alice.Command("!event start Boss"); got != "false" {
		t.Fatalf("a second start answered %s", got)
	}
	// A player who logs in during the run gets the list of runs.
	bob := srv.Join(t, "Bob")
	list := bob.Expect("the list of world events", func(e *wathenav1.Event) bool { return e.HasWorldEventList() }).GetWorldEventList()
	if len(list.GetEvents()) != 1 || list.GetEvents()[0].GetName() != "e2e.fixture:Boss" {
		t.Fatalf("list %v", list)
	}
	zeny, bobZeny := alice.zeny(), bob.zeny()
	alice.Kill(alice.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Boss"))
	alice.Announced("Alice defeated Boss on placeholder/map!")
	end := alice.EventNotice("e2e.fixture:Boss", false)
	if end.GetResult() != "won" {
		t.Fatalf("end %v", end)
	}
	// Alice did all the damage: she gets the reward, and Bob nothing.
	alice.untilZeny(zeny + 100)
	bob.EventNotice("e2e.fixture:Boss", false)
	if got := bob.zeny(); got != bobZeny {
		t.Fatalf("Bob got %d zeny", got-bobZeny)
	}
	// The event may run again.
	alice.StartEvent("Boss")
	alice.MustCommand("!event stop Boss")
	alice.Announced("Boss was stopped.")
	if end := alice.EventNotice("e2e.fixture:Boss", false); end.GetResult() != "stopped" {
		t.Fatalf("stop %v", end)
	}
	srv.CheckLogs()
}

func TestWorldEventInvasion(t *testing.T) {
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	alice.StartEvent("Raid")
	alice.Announced("placeholder/map is under attack! Defend the town!")
	alice.Announced("Wave 1 of 2 attacks placeholder/map!")
	apples := bob.Count(Apple)
	// The raiders enter at the map's edges and come to the players.
	for range 3 {
		alice.Kill(alice.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Raider"))
	}
	alice.Announced("Wave 2 of 2 attacks placeholder/map!")
	for range 2 {
		alice.Kill(alice.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Raider"))
	}
	alice.Announced("placeholder/map drove back the invasion!")
	if end := alice.EventNotice("e2e.fixture:Raid", false); end.GetResult() != "won" {
		t.Fatalf("end %v", end)
	}
	// Everyone on the map gets the reward, Bob who did not fight too.
	bob.Until("2 more Apples", func(_ botclient.View, g botclient.Game) bool { return g.Count(Apple) == apples+2 })
	srv.CheckLogs()
}

func TestWorldEventTreasureHunt(t *testing.T) {
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	alice.StartEvent("Hunt")
	alice.Announced("2 treasure chests are hidden on the fields! Ask the Treasure Hunter in placeholder/map for hints.")
	talk := alice.Talk("Treasure Hunter")
	talk.Says("2 chests are still hidden.")
	talk.Says("One lies on e2e_field, near ")
	talk.Close()
	// The Guide warps to the field, where the chests are.
	alice.Talk("Guide").Pick("Go to the field")
	alice.MapChange(FieldMap)
	zeny := alice.zeny()
	for i, line := range []string{"Alice found a treasure chest on e2e_field! 1 chest is left.", "Alice found the last treasure chest on e2e_field!"} {
		chest := alice.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Chest")
		alice.Kill(chest)
		alice.Announced(line)
		alice.untilZeny(zeny + 50*int64(i+1))
	}
	// The chests' drops are the loot.
	alice.Until("2 Jellopy on the ground", func(v botclient.View, _ botclient.Game) bool {
		n := 0
		for _, e := range v.Entities {
			if e.Kind == wathenav1.EntityKind_ENTITY_KIND_ITEM && e.ItemID == Jellopy {
				n++
			}
		}
		return n == 2
	})
	if end := alice.EventNotice("e2e.fixture:Hunt", false); end.GetResult() != "won" {
		t.Fatalf("end %v", end)
	}
	srv.CheckLogs()
}

// TestWorldEventRestartInTheDatabase checks the restart rules of typed
// runs: a boss whose run a restart cut short comes back while its
// window is open, and a boss that died and gave its reward does not, so
// the reward is never given twice.
func TestWorldEventRestartInTheDatabase(t *testing.T) {
	srv := Start(t, Options{Database: true})
	alice := srv.Join(t, "Alice")
	alice.StartEvent("Boss")
	alice.Announced("Boss has appeared on placeholder/map!")
	srv.Restart()
	// The run goes on at the start, before Alice is back: she gets it in
	// the list of runs at login, and the boss is there again.
	alice = srv.Join(t, "Alice")
	alice.Expect("the list of world events", func(e *wathenav1.Event) bool { return len(e.GetWorldEventList().GetEvents()) == 1 })
	zeny := alice.zeny()
	alice.Kill(alice.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Boss"))
	alice.untilZeny(zeny + 100)
	alice.EventNotice("e2e.fixture:Boss", false)
	// The reward was saved with its progress row.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv.Restart()
	alice = srv.Join(t, "Alice")
	if got := alice.zeny(); got != zeny+100 {
		t.Fatalf("zeny after the restart: %d, want %d", got, zeny+100)
	}
	// The run ended: the boss does not come back.
	time.Sleep(2 * time.Second)
	if alice.Seen(func(e *wathenav1.Event) bool {
		c := e.GetChat()
		return c != nil && strings.Contains(c.GetText(), "has appeared")
	}) != nil {
		t.Fatal("the boss came back after it died")
	}
	rows, err := srv.DB.Query(ctx, "SELECT unit FROM world_event_progress WHERE event = 'e2e.fixture:Boss' ORDER BY unit")
	if err != nil {
		t.Fatal(err)
	}
	units, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(units, []string{"boss", "end"}) {
		t.Fatalf("progress rows %q", units)
	}
	var logged int
	if err := srv.DB.QueryRow(ctx, "SELECT count(*) FROM zeny_log WHERE action = 'event' AND from_kind = 'event' AND amount = 100").Scan(&logged); err != nil {
		t.Fatal(err)
	}
	if logged != 1 {
		t.Fatalf("%d event zeny log lines", logged)
	}
	srv.CheckLogs()
}
