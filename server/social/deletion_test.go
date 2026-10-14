package social

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/rules"
)

// TestForgetCharacters: the characters of a deleted account leave their
// party, which breaks up when one led it, and their guilds: a master
// hands the guild to the next member, and a master alone breaks it up.
// They go off the friend lists in memory.
func TestForgetCharacters(t *testing.T) {
	w := &world{r: New(rules.DefaultLimits(), rules.DefaultParty()), boxes: map[string]*box{}, ps: map[string]*Player{}}
	for i, n := range []string{"Ann", "Bob", "Cid", "Dan"} {
		b := &box{}
		w.boxes[n] = b
		w.ps[n] = w.r.Online(Login{Name: n, CharID: int64(101 + i), AccountID: int64(i + 1), Inbox: b, Map: "map/one", Level: 50})
	}
	w.party(t, "Ann", "Bob")
	shared := w.guild(t, "Ann", "Cid")
	alone := w.guild(t, "Dan")
	if o := w.r.FriendAdd(w.ps["Ann"], "Bob"); o != FriendRequestSent {
		t.Fatalf("friend request: %v", o)
	}
	if o := w.r.FriendReply(w.ps["Bob"], "Ann", true); o != FriendAdded {
		t.Fatalf("friend reply: %v", o)
	}
	if g, ok := w.r.GuildLedWithOthers(101); !ok || g != "Guild Ann" {
		t.Errorf("Ann leads %q %v", g, ok)
	}
	if _, ok := w.r.GuildLedWithOthers(104); ok {
		t.Error("Dan leads a guild alone, so the request may go ahead")
	}
	if _, ok := w.r.GuildLedWithOthers(102); ok {
		t.Error("Bob leads nothing")
	}
	// The account's logins are refused, so its characters are offline.
	w.r.Offline(w.ps["Ann"], w.boxes["Ann"])
	w.r.Offline(w.ps["Dan"], w.boxes["Dan"])
	w.boxes["Bob"].got, w.boxes["Cid"].got = nil, nil

	w.r.ForgetCharacters([]int64{101, 104})

	if res := w.boxes["Bob"].results(); len(res) == 0 || res[len(res)-1] != PartyDisbanded {
		t.Errorf("Bob's party results %v", res)
	}
	if w.ps["Bob"].party != nil {
		t.Error("Bob is still in the party")
	}
	if n, ok := w.boxes["Bob"].last(NoticeFriends); !ok || len(n.Friends) != 0 {
		t.Errorf("Bob's friends %+v", n.Friends)
	}
	g, ok := w.r.Guild(shared)
	if !ok || g.Master != "Cid" {
		t.Errorf("the shared guild %+v %v", g, ok)
	}
	if _, _, ok := w.r.GuildMasterOf(103); !ok {
		t.Error("Cid is not the master")
	}
	if g, _ := w.r.Guild(alone); !g.Ended {
		t.Error("Dan's guild still runs")
	}
	if w.r.players["ann"] != nil || w.r.players["dan"] != nil {
		t.Error("the registry still knows the deleted characters")
	}
}
