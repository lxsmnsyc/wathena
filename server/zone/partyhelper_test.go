package zone

import (
	"fmt"
	"testing"

	"github.com/lxsmnsyc/wathena/server/social"
)

// formParty puts players in one party of the zone's social registry, the
// first as its leader, and gives each the party view, as the registry's
// notices would. Players without a session join the registry without an
// inbox. It returns the party's ID.
func formParty(t *testing.T, z *Zone, opts social.Options, players ...*Player) int64 {
	t.Helper()
	reg := z.socialHub()
	for _, p := range players {
		if p.soc == nil {
			e := p.entity
			p.soc = reg.Online(social.Login{Name: p.Name, Map: z.Map, X: e.x, Y: e.y, HP: e.hp, Level: p.progress.BaseLevel, Job: p.char.Job})
		}
	}
	id, o := reg.Create(players[0].soc, fmt.Sprintf("Party of %s", players[0].Name), opts)
	if id == 0 {
		t.Fatalf("formParty: %v", o)
	}
	for _, p := range players[1:] {
		if ok, err := reg.AddMember(id, p.soc); !ok || err != nil {
			t.Fatalf("formParty: %s did not join: %v", p.Name, err)
		}
	}
	if opts.ExpEven {
		if _, _, err := reg.ScriptSetOptions(id, opts); err != nil {
			t.Fatal(err)
		}
	}
	refreshParty(z, id)
	return id
}

// refreshParty gives every player of the zone in a party its current
// view.
func refreshParty(z *Zone, id int64) {
	v := z.socialHub().View(id)
	for _, p := range z.players {
		if p.soc != nil && z.socialHub().PartyOf(p.soc) == id {
			p.partyState.view = v
		}
	}
}
