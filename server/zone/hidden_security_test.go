package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/social"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestHiddenPlayersAreNotSent checks that a viewer who cannot see a hidden
// or cloaked player is never sent that player's entity (docs/strategy.md:
// "Hidden players ... are never sent").
func TestHiddenPlayersAreNotSent(t *testing.T) {
	for name, opt := range map[string]int{"hiding": sc.OptionHide, "cloaking": sc.OptionCloak} {
		t.Run(name, func(t *testing.T) {
			z := New("map/test", open(40), rng.FromKey([32]byte{2}).Stream(), nil)
			a := add(z, "alice")
			a.entity.sc = sc.NewList()
			a.entity.sc.Option |= opt
			b := add(z, "bob")
			z.stepAt(50)
			for _, e := range events(b) {
				if ap := e.GetAppear(); ap != nil && ap.GetName() == "alice" {
					t.Fatalf("bob was sent the %s player alice", name)
				}
			}
		})
	}
}

// TestHiddenPlayersStayVisibleToAllies checks who still sees a hiding
// player: the player itself and its party. Others who saw the player
// before get a vanish when it hides, and an appear when it shows again.
func TestHiddenPlayersStayVisibleToAllies(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{2}).Stream(), nil)
	a, b, c := add(z, "alice"), add(z, "bob"), add(z, "carol")
	party := &social.PartyView{ID: 7}
	a.partyState.view, c.partyState.view = party, party
	z.stepAt(50)
	events(a)
	events(b)
	events(c)
	a.entity.sc = sc.NewList()
	a.entity.sc.Option |= sc.OptionHide
	a.entity.optionChanged = true
	z.stepAt(100)
	vanished := func(p *Player) bool {
		for _, e := range events(p) {
			if v := e.GetVanish(); v != nil && v.GetId() == p.netIDs.id(a.entity) {
				return true
			}
		}
		return false
	}
	if _, known := b.netIDs.get(a.entity); known {
		t.Error("bob still knows the hiding player")
	}
	if vanished(c) {
		t.Error("carol, in alice's party, lost sight of her")
	}
	if _, known := c.netIDs.get(a.entity); !known {
		t.Error("carol does not know alice")
	}
	if _, known := a.netIDs.get(a.entity); !known {
		t.Error("alice does not see herself")
	}
	a.entity.sc.Option &^= sc.OptionHide
	z.stepAt(150)
	shown := false
	for _, e := range events(b) {
		if ap := e.GetAppear(); ap != nil && ap.GetName() == "alice" {
			shown = true
		}
	}
	if !shown {
		t.Error("bob was not sent alice once she stopped hiding")
	}
}

// TestHiddenMonstersAreNotSent checks that a monster under Hiding or
// Cloaking is sent only to viewers that may see it: detectors. Others
// who saw it get a vanish, and an appear when it shows again.
func TestHiddenMonstersAreNotSent(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{2}).Stream(), nil)
	z.Mods = &Mods{Monsters: map[uint32]Spawn{}}
	b := add(z, "bob")
	m := aiMob(t, z, 1002, b.entity.x+3, b.entity.y, 0, nil)
	z.stepAt(50)
	if _, known := b.netIDs.get(m); !known {
		t.Fatal("bob does not see the monster before it hides")
	}
	events(b)
	if m.sc == nil {
		m.sc = sc.NewList()
	}
	m.sc.Option |= sc.OptionHide
	m.optionChanged = true
	z.stepAt(100)
	if _, known := b.netIDs.get(m); known {
		t.Error("bob still knows the hiding monster")
	}
	for _, e := range events(b) {
		if o := e.GetOptionChange(); o != nil {
			t.Error("bob was sent the hiding monster's option bits")
		}
	}
	c := add(z, "carol")
	c.entity.status.Mode |= modeDetector
	z.stepAt(150)
	if _, known := c.netIDs.get(m); !known {
		t.Error("carol, a detector, does not see the hiding monster")
	}
	m.sc.Option &^= sc.OptionHide
	m.optionChanged = true
	z.stepAt(200)
	if _, known := b.netIDs.get(m); !known {
		t.Error("bob was not sent the monster once it stopped hiding")
	}
}
