package zone

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// GM commands typed in chat (proposal 0002 review decision 35, decision
// 0030), as rAthena's atcommand.cpp:
//   - A line that starts with @ is a command when the player's staff
//     role has the permission of at least one command. It never goes out
//     as chat, never reaches the chat log, the chat hooks or the chat
//     event. For anyone else, such a line is normal chat, as in rAthena
//     (review decision 40).
//   - Each command needs a permission of the player's staff role, which
//     counts only for a login with a second factor. Without it, and for
//     a command that does not exist, the answer is "Unknown command.".
//   - Each command that acts writes a row of the GM audit log.
//   - A command that acts on another player finds it by name in the
//     chat hub, on any map, and runs its part on the zone that holds
//     that player, through the player's session. The answer comes back
//     the same way, so no zone waits for another.
//   - @mute, @jail and @kick go through the Moderator, which also serves
//     the admin API, so a sanction is the same from both.

// UnknownCommand is the answer to a command that does not exist or that
// the player's role does not allow.
const UnknownCommand = "Unknown command."

// Moderator gives the sanctions of GM commands. In the all-in-one build
// it is server/adminworld.
type Moderator interface {
	// Command mutes, jails or kicks an online player by name. It returns
	// the answer for the staff member, or a *CommandError.
	Command(ctx context.Context, o StaffOrder) (string, error)
}

// StaffOrder is a sanction a GM command asks for.
type StaffOrder struct {
	// Actor is the staff member: AccountID, AccountName and Role.
	Actor staff.AuditRow
	// Kind is mute, jail or kick.
	Kind string
	// Target is the character's name, in any case.
	Target  string
	Minutes int
	Reason  string
}

// CommandError is an answer to a command that did nothing.
type CommandError struct{ Text string }

func (e *CommandError) Error() string { return e.Text }

// Limits of command arguments.
const (
	// MaxSanctionMinutes is the longest mute or jail: a year.
	MaxSanctionMinutes = 525_600
	// maxCommandItems is the most copies of an item that cannot stack
	// one @item makes.
	maxCommandItems = 100
	// commandWait bounds the work of one command on other zones.
	commandWait = 30 * time.Second
)

// gmCommand is one command.
type gmCommand struct {
	permission string
	usage      string
	run        func(z *Zone, p *Player, c gmCall)
}

// gmCall is one use of a command.
type gmCall struct {
	name, args, role, usage string
}

var gmCommands = map[string]gmCommand{
	"warp":   {"commands.warp", "@warp <map> [x y]", (*Zone).commandWarp},
	"jumpto": {"commands.warp", "@jumpto <name>", (*Zone).commandJumpTo},
	"recall": {"commands.recall", "@recall <name>", (*Zone).commandRecall},
	"hide":   {"commands.hide", "@hide", (*Zone).commandHide},
	"kick":   {"moderation.kick", "@kick <name> <reason>", (*Zone).commandKick},
	"mute":   {"moderation.mute", "@mute <name> <minutes> <reason>", (*Zone).commandSanction},
	"jail":   {"moderation.jail", "@jail <name> <minutes> <reason>", (*Zone).commandSanction},
	"item":   {"items.create", "@item <item> <amount> <reason>", (*Zone).commandItem},
}

// commandUser reports whether the player's role, for this login, has the
// permission of at least one command. Only then is a line that starts
// with @ a command.
func (z *Zone) commandUser(p *Player) bool {
	role := z.staffRole(p)
	if role == "" {
		return false
	}
	for _, c := range gmCommands {
		if z.Staff.Roles().Has(role, c.permission) {
			return true
		}
	}
	return false
}

// gmCommand runs a line that starts with @.
func (z *Zone) gmCommand(p *Player, text string) {
	name, args, _ := strings.Cut(strings.TrimPrefix(text, "@"), " ")
	name = strings.ToLower(name)
	c, ok := gmCommands[name]
	role := z.staffRole(p)
	if !ok || role == "" || !z.Staff.Roles().Has(role, c.permission) {
		z.answer(p, UnknownCommand)
		return
	}
	c.run(z, p, gmCall{name: name, args: strings.TrimSpace(args), role: role, usage: c.usage})
}

// answer shows a command's answer to the player who typed it.
func (z *Zone) answer(p *Player, text string) {
	p.pending = append(p.pending, systemMessage(text))
}

// usage answers with a command's usage.
func (z *Zone) usage(p *Player, c gmCall) {
	z.answer(p, "Usage: "+c.usage)
}

// commandAudit adds the audit row of a command to the zone's next
// commit.
func (z *Zone) commandAudit(p *Player, role, action, target string, targetAccount int64, args map[string]any, reason string, review bool) {
	z.txAudit = append(z.txAudit, staff.AuditRow{AccountID: p.record.AccountID, AccountName: z.staffName(p), Role: role,
		Action: action, Source: "game", Target: target, TargetAccount: targetAccount, Arguments: args, Reason: reason,
		NeedsReview: review})
}

// staffName is the account name of a player's login, for audit rows.
func (z *Zone) staffName(p *Player) string {
	if p.sess != nil && p.sess.staff.AccountName != "" {
		return p.sess.staff.AccountName
	}
	return p.Name
}

// actor is the staff member behind a command.
func (z *Zone) actor(p *Player, role string) staff.AuditRow {
	return staff.AuditRow{AccountID: p.record.AccountID, AccountName: z.staffName(p), Role: role, Source: "game"}
}

// mayHide reports whether a player's login may use @hide, as
// pc_can_use_command(sd, "hide") in pc_authok.
func (z *Zone) mayHide(p *Player) bool {
	role := z.staffRole(p)
	return role != "" && z.Staff.Roles().Has(role, gmCommands["hide"].permission)
}

// gmHidden reports whether an entity is a player hidden with @hide.
func gmHidden(e *entity) bool { return e.player != nil && e.player.gmHidden }

// post runs f on the zone that holds a session's player, as a command
// of the player. A player between zones gets it once it arrived. It
// gives up after commandWait, or when the session ended.
func post(ctx context.Context, s *Session, f func(z *Zone, p *Player)) bool {
	for {
		z, _ := s.where()
		if z != nil && s.send(command{staff: f}) {
			return true
		}
		if s.ended() {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// postAsync is post on its own goroutine, for a zone's goroutine, which
// must never wait.
func postAsync(s *Session, f func(z *Zone, p *Player)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), commandWait)
		defer cancel()
		post(ctx, s, f)
	}()
}

// later answers the player of a session from any goroutine.
func later(s *Session, text string) {
	postAsync(s, func(z *Zone, p *Player) { z.answer(p, text) })
}

// findOnline finds an online player by name, with its session.
func (z *Zone) findOnline(name string) (chat.Online, *Session, bool) {
	f, ok := z.chatHub().Find(name)
	if !ok {
		return f, nil, false
	}
	s, ok := f.Inbox.(*Session)
	return f, s, ok
}

// lookupMap finds the zone of a map by its name, with or without map/.
func (z *Zone) lookupMap(name string) *Zone {
	if name == "" {
		return nil
	}
	if to := z.zoneOf(name); to != nil {
		return to
	}
	if strings.Contains(name, "/") {
		return nil
	}
	return z.zoneOf("map/" + strings.ToLower(name))
}

// commandMove moves a player to a cell of a zone: a teleport on its own map,
// else a map change. 0,0 is a random cell.
func (z *Zone) commandMove(p *Player, to *Zone, x, y int) bool {
	if to == z {
		z.teleport(p, x, y)
		return true
	}
	if p.sess == nil {
		return false
	}
	return z.depart(p, to, x, y)
}

// cannotMove answers why a player cannot be moved by a command, or "".
func (z *Zone) cannotMove(p *Player, who string) string {
	switch {
	case p.entity.dead:
		return who + " cannot warp while dead."
	case z.jailed(p):
		return who + " cannot warp while in jail."
	}
	return ""
}

// commandWarp is @warp <map> [x y], as atcommand_mapmove: a cell that
// is outside the map or not walkable becomes a random one.
func (z *Zone) commandWarp(p *Player, c gmCall) {
	f := strings.Fields(c.args)
	if len(f) != 1 && len(f) != 3 {
		z.usage(p, c)
		return
	}
	to := z.lookupMap(f[0])
	if to == nil {
		z.answer(p, "Map not found.")
		return
	}
	if why := z.cannotMove(p, "You"); why != "" {
		z.answer(p, why)
		return
	}
	x, y := 0, 0
	if len(f) == 3 {
		var errX, errY error
		x, errX = strconv.Atoi(f[1])
		y, errY = strconv.Atoi(f[2])
		if errX != nil || errY != nil {
			z.usage(p, c)
			return
		}
		if x < 0 || y < 0 || x >= to.grid.Width || y >= to.grid.Height || to.grid.NoPass(x, y) {
			z.answer(p, "Invalid coordinates, using a random cell.")
			x, y = 0, 0
		}
	}
	z.commandAudit(p, c.role, "commands.warp", "", 0, map[string]any{"map": strings.TrimPrefix(to.Map, "map/"), "x": x, "y": y}, "", false)
	// The answer goes before the move, so it leaves with the player.
	z.answer(p, "Warped.")
	z.commandMove(p, to, x, y)
}

// commandJumpTo is @jumpto <name>: the staff member goes to a player on
// any map, as atcommand_jumpto.
func (z *Zone) commandJumpTo(p *Player, c gmCall) {
	if c.args == "" {
		z.usage(p, c)
		return
	}
	f, ts, ok := z.findOnline(c.args)
	if !ok {
		z.answer(p, "Character not found.")
		return
	}
	if f.CharacterID == p.record.ID {
		z.answer(p, "You are already there.")
		return
	}
	if why := z.cannotMove(p, "You"); why != "" {
		z.answer(p, why)
		return
	}
	me, role := p.sess, c.role
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), commandWait)
		defer cancel()
		ok := post(ctx, ts, func(tz *Zone, tp *Player) {
			m, x, y, name := tz.Map, tp.entity.x, tp.entity.y, tp.Name
			postAsync(me, func(z *Zone, p *Player) {
				to := z.zoneOf(m)
				if to == nil {
					z.answer(p, "Map not found.")
					return
				}
				if why := z.cannotMove(p, "You"); why != "" {
					z.answer(p, why)
					return
				}
				z.commandAudit(p, role, "commands.jumpto", name, 0, map[string]any{"map": strings.TrimPrefix(m, "map/"), "x": x, "y": y}, "", false)
				z.answer(p, "Jumped to "+name+".")
				z.commandMove(p, to, x, y)
			})
		})
		if !ok {
			later(me, "Character not found.")
		}
	}()
}

// commandRecall is @recall <name>: a player on any map comes to the
// staff member, as atcommand_recall.
func (z *Zone) commandRecall(p *Player, c gmCall) {
	if c.args == "" {
		z.usage(p, c)
		return
	}
	f, ts, ok := z.findOnline(c.args)
	if !ok {
		z.answer(p, "Character not found.")
		return
	}
	if f.CharacterID == p.record.ID {
		z.answer(p, "You cannot recall yourself.")
		return
	}
	me, role, m, x, y := p.sess, c.role, z.Map, p.entity.x, p.entity.y
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), commandWait)
		defer cancel()
		ok := post(ctx, ts, func(tz *Zone, tp *Player) {
			name, account := tp.Name, tp.record.AccountID
			if why := tz.cannotMove(tp, name); why != "" {
				later(me, why)
				return
			}
			to := tz.zoneOf(m)
			if to == nil || !tz.commandMove(tp, to, x, y) {
				later(me, name+" could not be recalled.")
				return
			}
			postAsync(me, func(z *Zone, p *Player) {
				z.commandAudit(p, role, "commands.recall", name, account, map[string]any{"map": strings.TrimPrefix(m, "map/"), "x": x, "y": y}, "", false)
				z.answer(p, name+" recalled!")
			})
		})
		if !ok {
			later(me, "Character not found.")
		}
	}()
}

// commandHide is @hide, as atcommand_hide: other players do not see the
// staff member, and monsters ignore it, until the next @hide. It lasts
// across map changes, and across logins while the login may use @hide.
func (z *Zone) commandHide(p *Player, c gmCall) {
	if c.args != "" {
		z.usage(p, c)
		return
	}
	p.gmHidden = !p.gmHidden
	z.commandAudit(p, c.role, "commands.hide", "", 0, map[string]any{"hidden": p.gmHidden}, "", false)
	if p.gmHidden {
		// Monsters that chase or attack the staff member stop.
		for e := range z.entities.all() {
			if e.target == p.entity && e.monster != nil {
				e.target = nil
			}
		}
		z.answer(p, "Invisible: On")
		return
	}
	z.answer(p, "Invisible: Off")
}

// splitName reads a leading character name: in double quotes, or the
// words before the first word that fits, which next checks.
func splitName(args string, next func(rest []string) bool) (string, []string, bool) {
	if strings.HasPrefix(args, `"`) {
		name, rest, ok := strings.Cut(args[1:], `"`)
		if !ok || strings.TrimSpace(name) == "" {
			return "", nil, false
		}
		return strings.TrimSpace(name), strings.Fields(rest), true
	}
	f := strings.Fields(args)
	for i := 1; i < len(f); i++ {
		if next(f[i:]) {
			return strings.Join(f[:i], " "), f[i:], true
		}
	}
	return "", nil, false
}

// commandKick is @kick <name> <reason>. A name with spaces goes in
// double quotes, or is the longest run of words that names an online
// player.
func (z *Zone) commandKick(p *Player, c gmCall) {
	var name, reason string
	if strings.HasPrefix(c.args, `"`) {
		n, rest, ok := splitName(c.args, nil)
		if !ok {
			z.usage(p, c)
			return
		}
		name, reason = n, strings.Join(rest, " ")
	} else {
		f := strings.Fields(c.args)
		for i := len(f) - 1; i >= 1; i-- {
			if _, ok := z.chatHub().Find(strings.Join(f[:i], " ")); ok {
				name, reason = strings.Join(f[:i], " "), strings.Join(f[i:], " ")
				break
			}
		}
		if name == "" && len(f) >= 2 {
			name, reason = f[0], strings.Join(f[1:], " ")
		}
	}
	if name == "" || reason == "" {
		z.usage(p, c)
		return
	}
	z.moderate(p, StaffOrder{Actor: z.actor(p, c.role), Kind: "kick", Target: name, Reason: reason})
}

// commandSanction is @mute and @jail <name> <minutes> <reason>.
func (z *Zone) commandSanction(p *Player, c gmCall) {
	isMinutes := func(rest []string) bool {
		n, err := strconv.Atoi(rest[0])
		return err == nil && n > 0 && len(rest) > 1
	}
	name, rest, ok := splitName(c.args, isMinutes)
	if !ok || len(rest) < 2 {
		z.usage(p, c)
		return
	}
	minutes, err := strconv.Atoi(rest[0])
	if err != nil || minutes < 1 {
		z.usage(p, c)
		return
	}
	if minutes > MaxSanctionMinutes {
		z.answer(p, fmt.Sprintf("A %s lasts at most %d minutes.", c.name, MaxSanctionMinutes))
		return
	}
	z.moderate(p, StaffOrder{Actor: z.actor(p, c.role), Kind: c.name, Target: name, Minutes: minutes,
		Reason: strings.Join(rest[1:], " ")})
}

// moderate hands a sanction to the Moderator, and shows its answer.
func (z *Zone) moderate(p *Player, o StaffOrder) {
	if len([]rune(o.Reason)) > 255 {
		z.answer(p, "The reason is at most 255 characters.")
		return
	}
	if z.Moderator == nil {
		z.answer(p, "Moderation is not available on this server.")
		return
	}
	me, m := p.sess, z.Moderator
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), commandWait)
		defer cancel()
		text, err := m.Command(ctx, o)
		var ce *CommandError
		switch {
		case errors.As(err, &ce):
			text = ce.Text
		case err != nil:
			text = "The server cannot do this right now. Try again later."
		}
		later(me, text)
	}()
}

// commandItem is @item <item> <amount> <reason>, as atcommand_item: the
// items come from the staff end into the staff member's inventory. The
// audit row needs a review, as every action that makes items does.
func (z *Zone) commandItem(p *Player, c gmCall) {
	f := strings.Fields(c.args)
	if len(f) < 3 {
		z.usage(p, c)
		return
	}
	var d *item.Def
	if id, err := strconv.ParseUint(f[0], 10, 32); err == nil {
		d, _ = z.Items.Lookup(uint32(id))
	} else {
		d, _ = z.Items.ByName(f[0])
	}
	if d == nil {
		z.answer(p, "Invalid item ID or name.")
		return
	}
	amount, err := strconv.Atoi(f[1])
	if err != nil || amount < 1 || amount > item.MaxAmount {
		z.answer(p, fmt.Sprintf("The amount is 1 to %d.", item.MaxAmount))
		return
	}
	stack := d.Stackable
	if !stack && amount > maxCommandItems {
		z.answer(p, fmt.Sprintf("At most %d of an item that does not stack.", maxCommandItems))
		return
	}
	reason := strings.Join(f[2:], " ")
	if len([]rune(reason)) > 255 {
		z.answer(p, "The reason is at most 255 characters.")
		return
	}
	o := origin{action: transfer.ActionCommand, from: transfer.Owner{Kind: transfer.KindStaff, ID: p.record.AccountID}}
	step := amount
	if !stack {
		step = 1
	}
	given := 0
	for given < amount {
		if r := z.addItem(p, d.ID, step, o); r != item.AddSuccess {
			p.pending = append(p.pending, pickUpFailed(0, pickUpFailure(r)))
			break
		}
		given += step
	}
	if given == 0 {
		z.answer(p, "The items do not fit.")
		return
	}
	z.commandAudit(p, c.role, "commands.item", p.Name, p.record.AccountID,
		map[string]any{"item_id": d.ID, "item": d.AegisName, "amount": given}, reason, true)
	z.answer(p, fmt.Sprintf("Item created: %d x %s.", given, displayName(d)))
}

func displayName(d *item.Def) string {
	if d.Name != "" {
		return d.Name
	}
	return d.AegisName
}

// StaffSanction mutes or jails an online character of this zone for the
// staff tools, as addSanction does for mods. found is false when the
// character is not here.
func (z *Zone) StaffSanction(ctx context.Context, id int64, kind string, minutes int, reason, by string) (s character.Sanction, name string, found bool, err error) {
	err = z.do(ctx, func() error {
		p := z.playerByID(id)
		if p == nil {
			return nil
		}
		found, name = true, p.Name
		z.addSanction(p, kind, int64(minutes), reason, by)
		s = *p.sanction(kind, z.wallClock())
		return nil
	})
	return s, name, found, err
}

// StaffLift ends a mute or a jail of an online character of this zone
// early, by its start. A freed player leaves the jail. found is false
// when the character is not here.
func (z *Zone) StaffLift(ctx context.Context, id int64, kind string, start time.Time) (found bool, err error) {
	err = z.do(ctx, func() error {
		p := z.playerByID(id)
		if p == nil {
			return nil
		}
		found = true
		z.liftSanction(p, kind, start)
		return nil
	})
	return found, err
}

// liftSanction takes a sanction of a kind that started at start off a
// player. The player sees that it ended. A jail that ends sends the
// player back.
func (z *Zone) liftSanction(p *Player, kind string, start time.Time) {
	i := slices.IndexFunc(p.sanctions, func(s character.Sanction) bool {
		return s.Kind == kind && s.Start.Truncate(time.Microsecond).Equal(start.Truncate(time.Microsecond))
	})
	if i < 0 {
		return
	}
	ended := p.sanctions[i]
	p.sanctions = slices.Delete(slices.Clone(p.sanctions), i, i+1)
	p.pending = append(p.pending, sanctionNotice(&ended, true))
	if kind != character.SanctionJail || z.jailed(p) {
		return
	}
	p.jailPending = false
	if z.Map == z.jail().Map {
		z.unjail(p, &ended)
	}
}

// ErrNotOnline is a player that is not online.
var ErrNotOnline = errors.New("zone: the player is not online")

// DevStaffRole gives an online player's account a role of the staff
// registry, as if it logged in with a second factor, for --dev and the
// end-to-end tests. "" takes the role away.
func DevStaffRole(ctx context.Context, hub *chat.Hub, reg *staff.Registry, name, role string) error {
	f, ok := hub.Find(name)
	if !ok {
		return ErrNotOnline
	}
	s, ok := f.Inbox.(*Session)
	if !ok {
		return ErrNotOnline
	}
	done := make(chan error, 1)
	if !post(ctx, s, func(z *Zone, p *Player) {
		if p.sess != nil {
			p.sess.staff = staff.Login{AccountName: p.Name, TwoFactor: true}
		}
		done <- reg.StaffChanged(ctx, p.record.AccountID, role)
	}) {
		return ErrNotOnline
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
