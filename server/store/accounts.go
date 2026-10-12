package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Errors of CreateCharacter.
var (
	ErrNameTaken = errors.New("store: the character name is taken")
	ErrSlotTaken = errors.New("store: the character slot is taken")
)

// CreateCharacter makes a character on an account, with its start
// items, in one transaction, and returns its ID. The account service
// runs it as the wathena_account role, which may insert characters and
// inventory rows but not change them.
func CreateCharacter(ctx context.Context, pool *pgxpool.Pool, account int64, c *character.Character) (int64, error) {
	var id int64
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		id, err = q.CreateCharacter(ctx, createParams(account, c))
		if errors.Is(err, pgx.ErrNoRows) {
			// ON CONFLICT DO NOTHING: the name or the slot is taken.
			if _, err := q.CharacterByName(ctx, c.Name); err == nil {
				return ErrNameTaken
			}
			return ErrSlotTaken
		}
		if err != nil {
			return err
		}
		if len(c.Items) > 0 {
			return q.InsertInventory(ctx, insertParams(id, c.Items))
		}
		return nil
	})
	return id, err
}

// Ticket is a used game ticket: the character it opens, and the login
// session that made it. The gateway closes the connection when that
// session ends.
type Ticket struct {
	SessionID              int64
	AccountID, CharacterID int64
	Name                   string
	Job, Sex               int
	Hair, HairColor        int
	// AccountName is the account's name, for the config's staff owners.
	AccountName string
	// TwoFactor is set when the ticket's session logged in with a
	// second factor, so a staff role counts (decision 0028).
	TwoFactor bool
}

// ErrTicketInvalid is a ticket that does not exist, was used, or
// expired.
var ErrTicketInvalid = errors.New("store: the game ticket is not valid")

// RedeemTicket uses up the game ticket with a token hash and returns
// its character. A ticket works once: the row is deleted in the same
// statement that reads it. The game role runs it.
func (s *Store) RedeemTicket(ctx context.Context, hash []byte) (Ticket, error) {
	row, err := s.q.RedeemTicket(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrTicketInvalid
	}
	if err != nil {
		return Ticket{}, err
	}
	if !row.Live {
		return Ticket{}, ErrTicketInvalid
	}
	return Ticket{
		SessionID: row.SessionID, AccountID: row.AccountID, CharacterID: row.CharacterID, Name: row.Name,
		Job: row.Job, Sex: row.Sex, Hair: row.Hair, HairColor: row.HairColor,
		AccountName: row.AccountName, TwoFactor: row.TwoFactor,
	}, nil
}
