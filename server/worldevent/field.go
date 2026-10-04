package worldevent

import (
	"context"
	"time"
)

// Field is what the runs of world events need of the maps. The world
// implements it with its zones (server/world), and wactl mod test with
// its one map (tools/modkit). Every call may wait on a zone, so the
// event manager never holds its lock during one.
type Field interface {
	// Has reports whether a map runs, by asset name.
	Has(mapName string) bool
	// StartMap is the asset name of the world's start map, where a
	// script event without a map runs.
	StartMap() string
	// StartScript runs an event's script in the zone of a map, in a new
	// event context.
	StartScript(ctx context.Context, mapName string, s ScriptRun) error
	// EndScript ends a run's script context.
	EndScript(ctx context.Context, mapName string, run int64) error
	// Condition runs a condition trigger's function in the zone of a
	// map.
	Condition(ctx context.Context, mapName, mod, module, fn string) (bool, error)
	// Spawn places a typed run's monsters on a map, and returns where
	// each landed.
	Spawn(ctx context.Context, s Spawn) ([]Placed, error)
	// Alive counts a run's monsters that live on a map.
	Alive(ctx context.Context, mapName string, run int64) (int, error)
	// Hints places a treasure hunt's hint NPCs on a map.
	Hints(ctx context.Context, mapName string, run int64, hints []Hint) error
	// Reward gives a run's rewards on a map, once for a unit of
	// progress, such as an invasion's win.
	Reward(ctx context.Context, r RewardRun) error
	// Clear removes a run's monsters and NPCs from a map, and forgets
	// the run there.
	Clear(ctx context.Context, mapName string, run int64) error
	// Announce shows a line to every player of the world. It never
	// waits.
	Announce(text string)
	// Notice tells every zone that a run started or ended: the zones fire
	// world_event_start and world_event_end and tell their players. It
	// never waits.
	Notice(n Notice)
}

// Roles of a typed run's monsters.
const (
	RoleBoss    = "boss"
	RoleInvader = "invader"
	RoleChest   = "chest"
)

// RunInfo names a run for the zones.
type RunInfo struct {
	ID int64
	// Event is the event's full name, Display the name players see, and
	// Type its type.
	Event, Display, Type string
	// Scheduled is the run's scheduled time, which names it in the
	// progress rows.
	Scheduled time.Time
}

// ScriptRun is the start of an event's script.
type ScriptRun struct {
	RunInfo
	// Entry is the entry script, as <mod>:<path>.
	Entry string
	// Rewards are the typed event's rewards, or nil.
	Rewards *Rewards
}

// Spawn is a typed run's monsters on one map.
type Spawn struct {
	RunInfo
	Map     string
	Monster uint32
	Amount  int
	Role    string
	// Unit names the progress a death makes: "boss", or "chest:<i>" for
	// chest i. An invader's death makes none.
	Unit string
	// Edge places the monsters on cells at the map's edges, where an
	// invasion enters. Otherwise each lands on a random walkable cell.
	Edge bool
	// Rewards are given at each death of these monsters, or nil.
	Rewards *Rewards
	// Announce announces a boss's death to the world.
	Announce bool
}

// Placed is where a spawned monster landed, and its name.
type Placed struct {
	X, Y int
	Name string
}

// RewardRun is a reward a run gives on a map for a unit of progress,
// such as an invasion's "result".
type RewardRun struct {
	RunInfo
	Map     string
	Unit    string
	Rewards *Rewards
}

// Notice is a run that started or ended, for the zones and the client.
type Notice struct {
	Name, DisplayName, Type string
	Started                 bool
	// Result is "won", "lost" or "stopped" when a run ended, or "" for a
	// script event that ran its time.
	Result string
	// Map is where the event happens, as an asset name, or "".
	Map  string
	Ends time.Time
}

// Kinds of Note.
const (
	// NoteDied is the death of a boss or a chest.
	NoteDied = iota + 1
	// NoteFinish is run:finish from the run's script.
	NoteFinish
)

// Note is what happened to a run in a zone. Zones send notes to the
// event manager without waiting.
type Note struct {
	Kind int
	Run  int64
	Map  string
	// Unit is the progress of a death, such as "boss" or "chest:2".
	Unit string
	// Monster is the dead monster's name, Killer the name of the player
	// who killed it, or "", and MVP the name of the player with the most
	// damage dealt and taken, or "".
	Monster, Killer, MVP string
	X, Y                 int
	// Result is the result of run:finish: "won" or "lost".
	Result string
}

// Sink is the event manager as the zones see it. Its methods never wait
// on a zone.
type Sink interface {
	// Note reports what happened to a run in a zone.
	Note(n Note)
	// Hint is what a hint NPC of a run says to a player who clicks it.
	Hint(run int64, mapName string, x, y int) []string
	// Request asks the scheduler to start or stop an event by name, for
	// world_events.start and world_events.stop. mod names the calling
	// mod, whose own event a plain name finds first. It reports false
	// when there is no such event, or it already runs (start), or does
	// not run (stop).
	Request(name, mod string, start bool) (bool, error)
	// Running lists the runs that last now, for a player who logs in.
	Running() []Notice
}
