package social

import (
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/rules"
)

// requests counts the friend request dialogs a box got.
func (b *box) requests() int {
	n := 0
	for _, x := range b.got {
		if x.Kind == NoticeFriendRequest {
			n++
		}
	}
	return n
}

// TestRepeatedFriendRequestsDoNotBloatTarget checks that repeated friend
// requests from one player to the same target show the target one
// dialog, and that the request still works.
func TestRepeatedFriendRequestsDoNotBloatTarget(t *testing.T) {
	w := newWorld(t, rules.DefaultLimits(), rules.DefaultParty(), "Ann", "Bob")
	now := time.Unix(1000, 0)
	w.r.Clock = func() time.Time { return now }
	for range 100 {
		if o := w.r.FriendAdd(w.ps["Ann"], "Bob"); o != FriendRequestSent {
			t.Fatalf("add: %v", o)
		}
		now = now.Add(2 * friendAskEvery)
	}
	if n := w.boxes["Bob"].requests(); n != 1 {
		t.Errorf("Bob got %d request dialogs, want 1", n)
	}
	if o := w.r.FriendReply(w.ps["Bob"], "Ann", true); o != FriendAdded {
		t.Errorf("reply: %v", o)
	}
}

// TestFriendRequestsAreThrottled checks that an asker reaches at most
// one player a second, so it cannot flood a target by asking it in turn
// with another player.
func TestFriendRequestsAreThrottled(t *testing.T) {
	w := newWorld(t, rules.DefaultLimits(), rules.DefaultParty(), "Ann", "Bob", "Cid")
	now := time.Unix(1000, 0)
	w.r.Clock = func() time.Time { return now }
	if o := w.r.FriendAdd(w.ps["Ann"], "Bob"); o != FriendRequestSent {
		t.Fatalf("first: %v", o)
	}
	if o := w.r.FriendAdd(w.ps["Ann"], "Cid"); o != 0 {
		t.Errorf("a request too soon: %v", o)
	}
	if n := w.boxes["Cid"].requests(); n != 0 {
		t.Errorf("Cid got %d dialogs from a request too soon", n)
	}
	now = now.Add(friendAskEvery)
	if o := w.r.FriendAdd(w.ps["Ann"], "Cid"); o != FriendRequestSent {
		t.Errorf("a request a second later: %v", o)
	}
	if n := w.boxes["Cid"].requests(); n != 1 {
		t.Errorf("Cid got %d dialogs, want 1", n)
	}
}
