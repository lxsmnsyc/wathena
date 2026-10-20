package modscript

import (
	"errors"
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// Parties, friends and limits (proposal 0002, review decision 27):
// party handles, wa.parties, the party events, friends, and wa.rules'
// limits.

// PartyOptions are a party's options as the API names them: exp_share
// "each" or "even", item_share "each" or "even", and item_pickup "each"
// or "shared".
type PartyOptions struct {
	ExpEven, ItemEven, PickupShared bool
}

// PartyHost is a Host with parties and friends. A Host without it raises
// an error on these calls.
type PartyHost interface {
	// PartyField reads a field of a party handle: id, name, leader,
	// leader_name, exp_share, item_share or item_pickup. leader is a
	// player ID of the zone, or nil.
	PartyField(party int64, field string) (script.Value, error)
	// PartyMembers are the player IDs of the online members in the zone,
	// in slot order.
	PartyMembers(party int64) ([]int64, error)
	// PartyMemberNames are every member's name, in slot order.
	PartyMemberNames(party int64) ([]string, error)
	// PartyMemberList is every member, online or not, in slot order
	// (review decision 58).
	PartyMemberList(party int64) ([]PartyMember, error)
	// CreateParty makes a party for a leader and returns its ID, or 0
	// when the name is taken or the leader has a party.
	CreateParty(leader int64, name string) (int64, error)
	// PartyAdd, PartyRemove and PartySetLeader change a party's members.
	// They return false when nothing changed.
	PartyAdd(party, player int64) (bool, error)
	PartyRemove(party, player int64) (bool, error)
	PartySetLeader(party, player int64) (bool, error)
	// PartySetOptions sets the options that set names. Even exp share
	// still needs the members' levels within party_share_level.
	PartySetOptions(party int64, set map[string]bool, opts PartyOptions) error
	// PartyDisband breaks a party up.
	PartyDisband(party int64) error
	// FindParty finds a party by ID or by name.
	FindParty(id int64) bool
	FindPartyNamed(name string) (int64, bool)
	// IsFriend reports whether a player has another on its friend list,
	// and FriendNames the names on a player's list.
	IsFriend(player, other int64) (bool, error)
	FriendNames(player int64) ([]string, error)
}

// PartyMember is one member of party:get_member_list, as rAthena's
// getpartymember types 0 to 2 read it.
type PartyMember struct {
	Name                   string
	CharacterID, AccountID int64
	Online                 bool
}

// PartyEvent is a party_join, party_leave or party_change event.
type PartyEvent struct {
	// Kind is "party_join", "party_leave" or "party_change".
	Kind  string
	Party int64
	// Player and Reason are set for party_join and party_leave.
	Player int64
	Reason string
	// Leader is the leader's player ID in the zone, or 0, and Options the
	// options, for party_change.
	Leader  int64
	Options PartyOptions
}

func shareWord(even bool, yes string) string {
	if even {
		return yes
	}
	return "each"
}

// optionsTable is a party's options as the party_change event shows them.
func optionsTable(o PartyOptions) script.Value {
	return script.NewTable(
		script.KV("exp_share", script.String(shareWord(o.ExpEven, "even"))),
		script.KV("item_share", script.String(shareWord(o.ItemEven, "even"))),
		script.KV("item_pickup", script.String(shareWord(o.PickupShared, "shared"))),
	)
}

// FireParty runs the handlers of a party event.
func (r *Runtime) FireParty(e PartyEvent) {
	fields := []script.Field{script.KV("party", script.Int(e.Party))}
	if e.Kind == "party_change" {
		if e.Leader != 0 {
			fields = append(fields, script.KV("leader", script.Int(e.Leader)))
		}
		fields = append(fields, script.KV("options", optionsTable(e.Options)))
	} else {
		fields = append(fields, script.KV("player", script.Int(e.Player)), script.KV("reason", script.String(e.Reason)))
	}
	r.Fire(e.Kind, Arg{"party_event", script.NewTable(fields...)})
}

func (r *Runtime) partyHost(what string) (PartyHost, error) {
	h, ok := r.host.(PartyHost)
	if !ok {
		return nil, errors.New(what + ": parties do not exist here")
	}
	return h, nil
}

// needManage checks party.manage. A test case's own code needs no
// permission to set up parties (review decision 27).
func (r *Runtime) needManage(what string) error {
	if r.opts.Testing && r.current == nil {
		return nil
	}
	return r.need(what, "party.manage")
}

func (r *Runtime) partyField(c *script.Call) error {
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return errors.New("not a party: read fields of a party handle, such as party.name")
	}
	if err := r.need("reading a party", "game.read"); err != nil {
		return err
	}
	h, err := r.partyHost("reading a party")
	if err != nil {
		return err
	}
	v, err := h.PartyField(id, c.String(1))
	if err != nil {
		return err
	}
	c.Return(v)
	return nil
}

// partyCall is a method of a party handle.
func (r *Runtime) partyCall(c *script.Call) error {
	op, id := c.String(0), c.Int(1)
	what := "party:" + op
	h, err := r.partyHost(what)
	if err != nil {
		return err
	}
	switch op {
	case "get_members":
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
		ids, err := h.PartyMembers(id)
		if err != nil {
			return err
		}
		vals := make([]script.Value, len(ids))
		for i, n := range ids {
			vals[i] = script.Int(n)
		}
		c.Return(script.List(vals...))
		return nil
	case "get_member_list":
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
		list, err := h.PartyMemberList(id)
		if err != nil {
			return err
		}
		vals := make([]script.Value, len(list))
		for i, m := range list {
			vals[i] = script.NewTable(script.KV("name", script.String(m.Name)), script.KV("character_id", script.Int(m.CharacterID)),
				script.KV("account_id", script.Int(m.AccountID)), script.KV("online", script.Bool(m.Online)))
		}
		c.Return(script.List(vals...))
		return nil
	case "get_member_names":
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
		names, err := h.PartyMemberNames(id)
		if err != nil {
			return err
		}
		vals := make([]script.Value, len(names))
		for i, n := range names {
			vals[i] = script.String(n)
		}
		c.Return(script.List(vals...))
		return nil
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.needManage(what); err != nil {
		return err
	}
	switch op {
	case "add", "remove", "set_leader":
		player, ok := c.Arg(2).AsInt()
		if !ok {
			return errors.New(what + ": want a player handle")
		}
		f := map[string]func(int64, int64) (bool, error){"add": h.PartyAdd, "remove": h.PartyRemove, "set_leader": h.PartySetLeader}[op]
		changed, err := f(id, player)
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		c.Return(script.Bool(changed))
		return nil
	case "set_options":
		t, ok := c.Arg(2).AsTable()
		if !ok {
			return errors.New(what + `: want a table such as { exp_share = "even" }`)
		}
		set := map[string]bool{}
		var o PartyOptions
		for _, f := range t.Fields {
			k, _ := f.Key.AsString()
			v, _ := f.Value.AsString()
			switch {
			case k == "exp_share" && (v == "each" || v == "even"):
				o.ExpEven = v == "even"
			case k == "item_share" && (v == "each" || v == "even"):
				o.ItemEven = v == "even"
			case k == "item_pickup" && (v == "each" || v == "shared"):
				o.PickupShared = v == "shared"
			case k == "exp_share" || k == "item_share":
				return fmt.Errorf(`%s: %s is "each" or "even"`, what, k)
			case k == "item_pickup":
				return fmt.Errorf(`%s: item_pickup is "each" or "shared"`, what)
			default:
				return fmt.Errorf("%s: %q is not an option; the options are exp_share, item_share and item_pickup", what, k)
			}
			set[k] = true
		}
		if err := h.PartySetOptions(id, set, o); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		return nil
	case "disband":
		if err := h.PartyDisband(id); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		return nil
	}
	return fmt.Errorf("party: no method %q", op)
}

func (r *Runtime) partiesCreate(c *script.Call) error {
	const what = "parties.create"
	leader, ok := c.Arg(0).AsInt()
	if !ok {
		return errors.New(what + ": want a player handle as the leader")
	}
	name, ok := c.Arg(1).AsString()
	if !ok {
		return errors.New(what + ": the name must be a string")
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.needManage(what); err != nil {
		return err
	}
	h, err := r.partyHost(what)
	if err != nil {
		return err
	}
	id, err := h.CreateParty(leader, name)
	if err != nil {
		return err
	}
	if id == 0 {
		c.Return(script.Nil)
		return nil
	}
	c.Return(script.Int(id))
	return nil
}

// partiesFind is parties.get(id) and parties.find(name).
func (r *Runtime) partiesFind(c *script.Call) error {
	by := c.String(0)
	what := "parties." + by
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	h, err := r.partyHost(what)
	if err != nil {
		return err
	}
	if by == "get" {
		id, ok := c.Arg(1).AsInt()
		if !ok {
			return errors.New(what + ": the ID must be an integer")
		}
		if id <= 0 || !h.FindParty(id) {
			c.Return(script.Nil)
			return nil
		}
		c.Return(script.Int(id))
		return nil
	}
	name, ok := c.Arg(1).AsString()
	if !ok {
		return errors.New(what + ": the name must be a string")
	}
	id, found := h.FindPartyNamed(name)
	if !found {
		c.Return(script.Nil)
		return nil
	}
	c.Return(script.Int(id))
	return nil
}

// playerFriend is player:is_friend and player:get_friend_names.
func (r *Runtime) playerFriend(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	op := c.String(1)
	what := "player:" + op
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	h, err := r.partyHost(what)
	if err != nil {
		return err
	}
	switch op {
	case "is_friend":
		other, ok := c.Arg(2).AsInt()
		if !ok {
			return errors.New(what + ": want a player handle")
		}
		b, err := h.IsFriend(id, other)
		if err != nil {
			return err
		}
		c.Return(script.Bool(b))
		return nil
	case "get_friend_names":
		names, err := h.FriendNames(id)
		if err != nil {
			return err
		}
		vals := make([]script.Value, len(names))
		for i, n := range names {
			vals[i] = script.String(n)
		}
		c.Return(script.List(vals...))
		return nil
	}
	return fmt.Errorf("player: no method %q", op)
}

// rulesLimits is the table of rules.limits: the world's limits by name.
func (r *Runtime) rulesLimits(c *script.Call) error {
	l := rules.LimitsOrDefault(r.opts.Rules)
	var fields []script.Field
	for _, v := range l.Named() {
		fields = append(fields, script.KV(v.Name, script.Int(int64(v.Value))))
	}
	c.Return(script.NewTable(fields...))
	return nil
}

// PartyEventNames are the party events.
var PartyEventNames = []string{"party_join", "party_leave", "party_change"}

// IsPartyEvent reports whether an event is a party event.
func IsPartyEvent(name string) bool { return slices.Contains(PartyEventNames, name) }
