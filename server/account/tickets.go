package account

import (
	"context"
	"errors"

	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// Redeemer uses up game tickets. *store.Store is one, on the game
// role's pool.
type Redeemer interface {
	RedeemTicket(ctx context.Context, hash []byte) (store.Ticket, error)
}

// GameTickets checks the session token of Hello as a game ticket. It
// implements gateway.Sessions. Each ticket works once.
type GameTickets struct {
	Store Redeemer
}

// Check redeems a ticket and returns its character.
func (t GameTickets) Check(ctx context.Context, token []byte) (gateway.Login, error) {
	hash, ok := tokenHash(string(token))
	if !ok {
		return gateway.Login{}, gateway.ErrSessionInvalid
	}
	tk, err := t.Store.RedeemTicket(ctx, hash)
	if errors.Is(err, store.ErrTicketInvalid) {
		return gateway.Login{}, gateway.ErrSessionInvalid
	}
	if err != nil {
		return gateway.Login{}, err
	}
	look := zone.Look{Job: uint32(tk.Job), Sex: uint32(tk.Sex), Hair: uint32(tk.Hair), HairColor: uint32(tk.HairColor)} // #nosec G115 -- CHECK constraints keep these 0 or more.
	return gateway.Login{CharacterID: tk.CharacterID, Name: tk.Name, Look: look}, nil
}
