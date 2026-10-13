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

	// Moderation (decision 0030). Each call returns a *WorldError for
	// what the caller did wrong.

	// Reports lists reports newest first.
	Reports(ctx context.Context, q ReportQuery) (*adminv1.ReportPage, error)
	// Report reads a report with its chat, the target's earlier reports
	// and its sanctions. It returns ErrNotFound for a report that does
	// not exist.
	Report(ctx context.Context, id int64) (*adminv1.ReportDetail, error)
	// CloseReport closes an open report with an outcome, and writes its
	// audit row.
	CloseReport(ctx context.Context, c CloseReport) (*adminv1.ReportSummary, error)
	// Sanction mutes, jails or kicks a character, or bans an account,
	// and writes its audit row. A kick and a ban close the game
	// connections.
	Sanction(ctx context.Context, o SanctionOrder) (*adminv1.Sanction, error)
	// SanctionByID reads a sanction. It returns ErrNotFound for one that
	// does not exist.
	SanctionByID(ctx context.Context, id int64) (*adminv1.Sanction, error)
	// LiftSanction ends a mute, a jail or a ban early, and writes its
	// audit row.
	LiftSanction(ctx context.Context, l Lift) (*adminv1.Sanction, error)
	// Sanctions lists sanctions newest first.
	Sanctions(ctx context.Context, q SanctionQuery) (*adminv1.SanctionList, error)
	// Bans returns the bans in effect of some accounts, by account.
	Bans(ctx context.Context, accounts []int64) (map[int64]*adminv1.Sanction, error)

	// The world, events and mods tools (decision 0031). Each call
	// returns a *WorldError for what the caller did wrong.

	// WorldView reads the zones with their last minute of ticks, the
	// process, the players online and the planned restart.
	WorldView(ctx context.Context) (*adminv1.WorldView, error)
	// Announce shows a line to every player, or to the players of one
	// map. It returns ErrNotFound for a map that does not run.
	Announce(ctx context.Context, text, mapName string) error
	// PlanRestart plans a restart.
	PlanRestart(ctx context.Context, o RestartOrder) (*adminv1.RestartPlan, error)
	// CancelRestart cancels the planned restart, and returns it.
	CancelRestart(ctx context.Context) (*adminv1.RestartPlan, error)
	// Events lists the world events.
	Events(ctx context.Context) (*adminv1.EventList, error)
	// EventAction starts, stops, changes or resets an event, writes its
	// audit row, and returns the event after it.
	EventAction(ctx context.Context, o EventOrder) (*adminv1.EventInfo, error)
	// Mods lists the loaded mods.
	Mods(ctx context.Context) (*adminv1.ModList, error)
	// SwitchMod turns a mod off or on, writes its audit row, and returns
	// the mod after it.
	SwitchMod(ctx context.Context, o ModOrder) (*adminv1.ModInfo, error)
}

// RestartOrder is POST /restart.
type RestartOrder struct {
	Actor
	At              time.Time
	Reason, Message string
}

// Actions of EventOrder.
const (
	EventStart  = "start"
	EventStop   = "stop"
	EventChange = "change"
	EventReset  = "reset"
)

// EventOrder is a staff action on a world event: POST
// /events/{name}/start, /stop and /reset, and PATCH /events/{name}.
type EventOrder struct {
	Actor
	Name string
	// Action is EventStart, EventStop, EventChange or EventReset.
	Action string
	// On, Trigger and DurationMinutes are a change. Nil and 0 keep the
	// event's value. Trigger has the form of data/events.yml, as JSON
	// gives it.
	On              *bool
	Trigger         map[string]any
	DurationMinutes int
	Reason          string
}

// ModOrder is POST /mods/{id}/disable or /enable.
type ModOrder struct {
	Actor
	Mod string
	// Map is a map's name, or "" for every zone.
	Map    string
	Off    bool
	Reason string
}

// Actor is the staff member behind a call, for its audit row.
type Actor struct {
	AccountID   int64
	AccountName string
	Role        string
}

// ReportQuery filters GET /reports. Empty fields do not filter.
type ReportQuery struct {
	// Status is open or closed.
	Status string
	Target string
	// Kind is player, line, guild_name or guild_emblem.
	Kind     string
	BeforeID int64
	Limit    int
}

// CloseReport is POST /reports/{id}/close.
type CloseReport struct {
	Actor
	ID int64
	// Outcome is no_action, warned or sanctioned.
	Outcome, Note string
	// SanctionID links the sanction of a sanctioned report, or 0.
	SanctionID int64
}

// SanctionOrder is POST /sanctions.
type SanctionOrder struct {
	Actor
	// Kind is mute, jail, kick or ban.
	Kind string
	// Character is the character of a mute, a jail or a kick, and
	// Account the account of a ban.
	Character, Account int64
	// Minutes is how long it lasts. A ban of 0 minutes is permanent.
	Minutes int
	Reason  string
}

// Lift is DELETE /sanctions/{id}.
type Lift struct {
	Actor
	ID     int64
	Reason string
}

// SanctionQuery filters GET /sanctions. Zero fields do not filter.
type SanctionQuery struct {
	// Character finds a character's sanctions and its account's bans.
	Character int64
	// Account finds every sanction of an account.
	Account int64
	Kind    string
	// Active keeps the sanctions in effect.
	Active   bool
	BeforeID int64
	Limit    int
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
