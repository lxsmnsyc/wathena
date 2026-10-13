package admin

import (
	"context"
	"errors"
	"time"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/item"
)

// World is what the API needs from the world service. In the all-in-one
// build it is server/adminworld, in the same process. A distributed
// build can put a network client here.
//
// Game data comes from the world: an online character is read from the
// zone that holds it, on that zone's goroutine, and an offline one from
// the game's database. Restores run in the world, through the transfers
// and the mail.
type World interface {
	// StaffChanged tells the world an account's new role, or "" for
	// none, so the game's Lua calls see it at once.
	StaffChanged(ctx context.Context, account int64, role string) error
	// JobName is a job's name for people, or "".
	JobName(job int) string
	// Characters reads whether characters are online, where they are,
	// and their sanctions in effect, by ID. Unknown IDs are left out.
	Characters(ctx context.Context, ids []int64) (map[int64]CharacterState, error)
	// Character reads a character's details. It returns ErrNotFound for
	// a character that does not exist.
	Character(ctx context.Context, id int64) (*adminv1.CharacterDetail, error)
	// FindSerial finds where a serial is now. It returns nil for a
	// serial that is in no holder.
	FindSerial(ctx context.Context, serial item.Serial) (*adminv1.Owner, error)
	// ItemHistory lists every move of a serial, following splits and
	// merges both ways.
	ItemHistory(ctx context.Context, serial item.Serial) (*adminv1.ItemHistory, error)
	// CharacterLogs lists a page of a character's item and zeny log,
	// newest first.
	CharacterLogs(ctx context.Context, id int64, q LogQuery) (*adminv1.CharacterLogs, error)
	// Restore gives items and zeny back to a character by mail. It
	// returns a *WorldError for what the caller did wrong or must try
	// again.
	Restore(ctx context.Context, r Restore) (*adminv1.RestoreResult, error)
}

// CharacterState is what the search shows of a character from the game.
type CharacterState struct {
	Online bool
	// Position is where an online character is, or nil.
	Position  *adminv1.Position
	Sanctions []*adminv1.Sanction
}

// LogQuery is a page of a character's logs.
type LogQuery struct {
	// Cursor is the next_cursor of the last page, or empty.
	Cursor string
	Limit  int
	// Since and Until bound the lines' times when not zero.
	Since, Until time.Time
}

// Restore is a restore the API asks the world for (decision 0029).
type Restore struct {
	// AccountID, AccountName and Role are the staff member's, for the
	// audit row.
	AccountID   int64
	AccountName string
	Role        string
	// Character gets the items and zeny by mail.
	Character int64
	Items     []RestoreItem
	Zeny      int64
	Reason    string
	// MayCreate is set when the role has items.create, so the restore
	// may make copies of gone serials.
	MayCreate bool
}

// RestoreItem is one serial of a restore. Amount 0 is the whole stack,
// or for a copy the amount of the serial's last log line.
type RestoreItem struct {
	Serial item.Serial
	Amount int
}

// ErrNotFound is a character, a serial or a row that does not exist.
var ErrNotFound = errors.New("admin: not found")

// WorldError is an error of the world with the API's error code, such as
// bad_request, forbidden, not_found or conflict.
type WorldError struct {
	Code, Message string
}

func (e *WorldError) Error() string { return "admin: " + e.Code + ": " + e.Message }
