package zone

import (
	"fmt"
	"maps"
	"runtime/debug"
	"slices"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Panic recovery (review 2026-10). Game logic runs on the zone's
// goroutine, so a panic there would stop the process and every zone. The
// zone recovers it, logs it with its stack, and limits what it stops:
//   - A panic in a command handler ends the sender's session. The
//     handler may have changed items halfway, so nothing it queued for
//     the next commit is written, and the players its transfers changed
//     are ended too, with the sender's trade partner. These players are
//     tainted: their sessions end with their last saved copy, never
//     with what is in memory, so a half-made move cannot be saved as a
//     duplicate.
//   - When the handler's commit also held storages, mails, shop stock,
//     store rows or event units, or the panic came from inside a
//     transfer, the zone cannot tell which objects changed halfway. It
//     then stops, as below.
//   - A panic while the zone builds one player's frame ends that
//     player's session with a normal save, since building frames
//     changes no game state. The zone goes on.
//   - Any other panic, such as in a tick's game logic, a script or a
//     commit, leaves state the zone cannot check. The zone stops: every
//     player is tainted and disconnected with
//     DISCONNECT_REASON_SERVER_ERROR, nothing pending is committed, and
//     later joins get ErrStopped. Other zones go on.

// serverErrorText is what a player sees when a server error ends its
// session.
const serverErrorText = "A server error ended your session. Your character is as it was at its last save."

// guard runs f. It returns the value of a panic in f with its stack, or
// nil.
func guard(f func()) (r any, stack []byte) {
	defer func() {
		if r = recover(); r != nil {
			stack = debug.Stack()
		}
	}()
	f()
	return nil, nil
}

// safely runs f, and stops the zone after a panic in it. what names the
// work for the log.
func (z *Zone) safely(what string, f func()) {
	if r, stack := guard(f); r != nil {
		z.logPanic(what, nil, r, stack)
		z.failed = true
	}
}

func (z *Zone) logPanic(what string, p *Player, r any, stack []byte) {
	name := ""
	if p != nil {
		name = p.Name
	}
	z.logError("zone: recovered a panic", "map", z.Map, "in", what, "player", name, "panic", fmt.Sprint(r), "stack", string(stack))
}

// handleSafely handles a command, and recovers a panic in its handler.
func (z *Zone) handleSafely(c command) {
	r, stack := guard(func() { z.handle(c) })
	if r == nil {
		return
	}
	p := c.player
	z.logPanic("command", p, r, stack)
	players, shared := z.discardTransfers()
	if shared || transferPanic(r) {
		z.failed = true
		return
	}
	if p != nil {
		players = append(players, p)
		if p.trade != nil {
			players = append(players, p.trade.partner)
		}
	}
	for _, q := range players {
		q.tainted = true
	}
	for _, q := range players {
		z.endTainted(q)
	}
}

// transferPanic reports whether a panic came from inside a transfer,
// whose boxes the zone does not know.
func transferPanic(r any) bool {
	s, ok := r.(string)
	return ok && strings.HasPrefix(s, "transfer:")
}

// discardTransfers drops what the transfers since the last commit
// queued, so none of it is written. It returns the players they changed,
// and whether they changed other objects too.
func (z *Zone) discardTransfers() (players []*Player, shared bool) {
	players = z.txPlayers
	for _, p := range players {
		p.transferred = false
	}
	shared = len(z.txStorageRefs) > 0 || len(z.txStorages) > 0 || len(z.txPersonal) > 0 ||
		len(z.txMails) > 0 || len(z.txShops) > 0 || len(z.txStores) > 0 || len(z.txEventUnits) > 0 ||
		len(z.txOffline) > 0 || len(z.txMailRows) > 0 || len(z.txAudit) > 0
	z.txPlayers, z.txStorageRefs, z.txStorages, z.txPersonal = nil, nil, nil, nil
	z.txMails, z.txShops, z.txStores, z.txEventUnits = nil, nil, nil, nil
	z.txOffline, z.txMailRows, z.txAudit = nil, nil, nil
	z.txLogs = transfer.Logs{}
	return players, shared
}

// endTainted tells a tainted player why its session ends, and takes it
// out of the zone. Its save writes its last saved copy (save).
func (z *Zone) endTainted(p *Player) {
	if p.gone() || !z.entities.has(p.entity) {
		return
	}
	z.queue(p, wathenav1.ServerMessage_builder{Disconnect: wathenav1.Disconnect_builder{
		Reason: wathenav1.DisconnectReason_DISCONNECT_REASON_SERVER_ERROR, Message: serverErrorText,
	}.Build()}.Build())
	// An autotrader leaves too.
	p.autotrade = false
	if r, stack := guard(func() { z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT) }); r != nil {
		z.logPanic("leave", p, r, stack)
		z.failed = true
		z.drop(p)
		z.save(p, true)
	}
}

// updateSafely sends a player what changed this tick, and ends the
// player's session after a panic.
func (z *Zone) updateSafely(p *Player) {
	r, stack := guard(func() { z.update(p) })
	if r == nil {
		return
	}
	z.logPanic("frame", p, r, stack)
	p.pending = nil
	z.queue(p, wathenav1.ServerMessage_builder{Disconnect: wathenav1.Disconnect_builder{
		Reason: wathenav1.DisconnectReason_DISCONNECT_REASON_SERVER_ERROR, Message: serverErrorText,
	}.Build()}.Build())
	// The connection closes, and the gateway then makes the player leave
	// with a normal save.
	z.drop(p)
}

// failStop ends every player's session after a panic the zone cannot
// contain. Nothing pending is committed, and each player's session ends
// with its last saved copy.
func (z *Zone) failStop() {
	z.logError("zone: stopping after a panic", "map", z.Map, "players", len(z.players))
	z.discardTransfers()
	players := slices.Collect(maps.Values(z.players))
	for _, p := range players {
		p.tainted = true
	}
	for _, p := range players {
		z.endTainted(p)
	}
	// What the leaves queued is not written either, but their storages
	// go back to the registry, which loads them again from the database.
	z.discardTransfers()
	z.releaseStorages()
	z.stopSender()
}
