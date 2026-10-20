package zone

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/social"
)

// The script calls of parties and friends (modscript.PartyHost, review
// decision 27). Party handles hold the party's ID. Players are handles
// of this zone, so a leader or member on another map is nil or left out.

func (h scriptHost) socialPlayer(id int64) (*Player, error) {
	p, err := h.player(id)
	if err != nil {
		return nil, err
	}
	if p.soc == nil {
		return nil, errors.New("the player is not in the world's party registry")
	}
	return p, nil
}

func (h scriptHost) partyInfo(id int64) (social.Info, error) {
	in, ok := h.z.socialHub().Party(id)
	if !ok {
		return social.Info{}, errors.New("there is no such party")
	}
	return in, nil
}

// PartyField reads a field of a party handle.
func (h scriptHost) PartyField(id int64, field string) (script.Value, error) {
	in, err := h.partyInfo(id)
	if err != nil {
		return script.Nil, err
	}
	share := func(even bool, yes string) script.Value {
		if even {
			return script.String(yes)
		}
		return script.String("each")
	}
	switch field {
	case "id":
		return script.Int(in.ID), nil
	case "name":
		return script.String(in.Name), nil
	case "leader":
		if !in.LeaderOnline {
			return script.Nil, nil
		}
		if id := h.z.playerNamed(in.Leader); id != 0 {
			return script.Int(id), nil
		}
		return script.Nil, nil
	case "leader_name":
		return script.String(in.Leader), nil
	case "exp_share":
		return share(in.Options.ExpEven, "even"), nil
	case "item_share":
		return share(in.Options.ItemEven, "even"), nil
	case "item_pickup":
		return share(in.Options.PickupShared, "shared"), nil
	}
	return script.Nil, fmt.Errorf("party has no field %q", field)
}

// PartyMembers are the online members in this zone, in slot order.
func (h scriptHost) PartyMembers(id int64) ([]int64, error) {
	in, err := h.partyInfo(id)
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, name := range in.Online {
		if pid := h.z.playerNamed(name); pid != 0 {
			out = append(out, pid)
		}
	}
	return out, nil
}

// PartyMemberNames are every member's name.
func (h scriptHost) PartyMemberNames(id int64) ([]string, error) {
	in, err := h.partyInfo(id)
	if err != nil {
		return nil, err
	}
	return in.Members, nil
}

// PartyMemberList is every member with its IDs, as getpartymember.
func (h scriptHost) PartyMemberList(id int64) ([]modscript.PartyMember, error) {
	in, err := h.partyInfo(id)
	if err != nil {
		return nil, err
	}
	out := make([]modscript.PartyMember, len(in.Roster))
	for i, m := range in.Roster {
		out[i] = modscript.PartyMember{Name: m.Name, CharacterID: m.CharID, AccountID: m.AccountID, Online: m.Online}
	}
	return out, nil
}

// CreateParty makes a party for a leader, as party_create.
func (h scriptHost) CreateParty(leader int64, name string) (int64, error) {
	p, err := h.socialPlayer(leader)
	if err != nil {
		return 0, err
	}
	return h.z.socialHub().ScriptCreate(p.soc, name), nil
}

// PartyAdd puts a player in a party, as party_addmember.
func (h scriptHost) PartyAdd(id, player int64) (bool, error) {
	p, err := h.socialPlayer(player)
	if err != nil {
		return false, err
	}
	return h.z.socialHub().AddMember(id, p.soc)
}

// PartyRemove takes a player out of a party, as party_delmember.
func (h scriptHost) PartyRemove(id, player int64) (bool, error) {
	p, err := h.socialPlayer(player)
	if err != nil {
		return false, err
	}
	return h.z.socialHub().RemoveMember(id, p.soc)
}

// PartySetLeader makes a member the leader, as party_changeleader.
func (h scriptHost) PartySetLeader(id, player int64) (bool, error) {
	p, err := h.socialPlayer(player)
	if err != nil {
		return false, err
	}
	ok, ch, err := h.z.socialHub().ScriptSetLeader(id, p.soc)
	if ch != nil {
		h.z.partyChanged(ch)
	}
	return ok, err
}

// PartySetOptions sets a party's options, as party_changeoption.
func (h scriptHost) PartySetOptions(id int64, set map[string]bool, o modscript.PartyOptions) error {
	in, err := h.partyInfo(id)
	if err != nil {
		return err
	}
	opts := in.Options
	if set["exp_share"] {
		opts.ExpEven = o.ExpEven
	}
	if set["item_share"] {
		opts.ItemEven = o.ItemEven
	}
	if set["item_pickup"] {
		opts.PickupShared = o.PickupShared
	}
	_, ch, err := h.z.socialHub().ScriptSetOptions(id, opts)
	if ch != nil {
		h.z.partyChanged(ch)
	}
	return err
}

// PartyDisband breaks a party up, as party_destroy.
func (h scriptHost) PartyDisband(id int64) error {
	return h.z.socialHub().Disband(id)
}

// FindParty reports whether a party runs.
func (h scriptHost) FindParty(id int64) bool {
	in, ok := h.z.socialHub().Party(id)
	return ok && !in.Ended
}

// FindPartyNamed finds a party by name.
func (h scriptHost) FindPartyNamed(name string) (int64, bool) {
	return h.z.socialHub().Find(name)
}

// IsFriend reports whether a player has another on its friend list.
func (h scriptHost) IsFriend(player, other int64) (bool, error) {
	p, err := h.socialPlayer(player)
	if err != nil {
		return false, err
	}
	o, err := h.player(other)
	if err != nil {
		return false, err
	}
	return h.z.socialHub().IsFriend(p.soc, o.Name), nil
}

// FriendNames are the names on a player's friend list.
func (h scriptHost) FriendNames(player int64) ([]string, error) {
	p, err := h.socialPlayer(player)
	if err != nil {
		return nil, err
	}
	return h.z.socialHub().FriendNames(p.soc), nil
}
