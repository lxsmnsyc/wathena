package worldevent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"sync"
	"time"
)

// Runs runs world events in a Field (review decisions 16 and 31). It is
// the scheduler's Runner, and the zones' Sink.
//   - A script event's run is its script, in an event context in the
//     zone of its map.
//   - A typed run places its monsters and NPCs through the Field, and
//     the zones send it notes of what happens. Tick, which the
//     scheduler calls every second, reads the notes, starts invasion
//     waves, and ends runs that are won.
//   - Every start and end is a notice to every zone and an
//     announcement.
//   - A run keeps its progress in a ProgressStore: rewards given (which
//     the zones write with the rewards), invasion waves started, and its
//     end. A run that a restart cut short goes on from its progress.
//
// The scheduler's goroutine calls Start, End, Stop, Condition and Tick.
// Zones call Note, Hint, Request and Running from their own goroutines,
// so those only take a lock, and Runs never holds it while it calls the
// Field.
type Runs struct {
	field Field
	opts  RunsOptions
	// active are the runs that last, by run ID. Only the scheduler's
	// goroutine uses it.
	active map[int64]*activeRun

	mu       sync.Mutex
	notes    []Note
	requests []Request
	// chests are each treasure hunt's chests, for hints.
	chests map[int64][]chest
	// running are the notices of the runs that last, for logins.
	running map[int64]Notice
	// defs are the scheduler's events, by name.
	defs map[string]Def
}

// RunsOptions configure Runs.
type RunsOptions struct {
	// Levels are the level of each field that runs, by asset name: the
	// level of its monsters (FieldLevels).
	Levels map[string]int
	// Rand picks maps, monsters and chests' maps. It must be a secret
	// stream.
	Rand Rand
	// Progress keeps runs' progress. Without it, progress lives in
	// memory.
	Progress ProgressStore
	Log      *slog.Logger
}

// ProgressStore keeps the units of a run's progress, so a run that a
// restart cut short goes on, and never gives a reward twice. A run is
// its event and its scheduled time.
type ProgressStore interface {
	EventProgress(ctx context.Context, event string, scheduled time.Time) ([]string, error)
	RecordEventProgress(ctx context.Context, event string, scheduled time.Time, unit string) error
}

// Request is a start or stop of an event from world_events.
type Request struct {
	Event string
	Start bool
}

// Results of runs.
const (
	ResultWon     = "won"
	ResultLost    = "lost"
	ResultStopped = "stopped"
)

// Units of progress that Runs records itself.
const (
	// UnitEnd is a run that ended before its time.
	UnitEnd = "end"
	// UnitBoss is a boss that died, and UnitResult an invasion's win.
	UnitBoss   = "boss"
	UnitResult = "result"
)

type chest struct {
	Map  string
	X, Y int
	Open bool
}

type activeRun struct {
	def  Def
	info RunInfo
	ends time.Time
	// maps are the maps with the run's monsters or NPCs, and scriptMap
	// the map whose zone runs its script, or "".
	maps      []string
	scriptMap string
	// where is the map the notices name.
	where string
	// boss is the boss's name.
	boss string
	// next is the invasion's next wave, and waveAt when the last wave
	// started.
	next   int
	waveAt time.Time
	// opened counts the chests that were opened.
	opened, chests int
	done           map[string]bool
	// says are the announcements of the run's start, made after its
	// notice, and started is set once it went out.
	says    []string
	started bool
}

// say announces a line of a run: at once, or after the start notice
// while the run starts.
func (r *Runs) say(a *activeRun, text string) {
	if a.started {
		r.field.Announce(text)
		return
	}
	a.says = append(a.says, text)
}

// NewRuns makes Runs for a Field. Call SetEvents with the scheduler's
// events.
func NewRuns(field Field, opts RunsOptions) *Runs {
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	if opts.Progress == nil {
		opts.Progress = &MemoryStore{}
	}
	return &Runs{field: field, opts: opts, active: map[int64]*activeRun{}, chests: map[int64][]chest{},
		running: map[int64]Notice{}, defs: map[string]Def{}}
}

// SetEvents sets the events that requests may name.
func (r *Runs) SetEvents(defs []Def) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defs = map[string]Def{}
	for _, d := range defs {
		r.defs[d.Name] = d
	}
}

// Note reports what happened to a run in a zone.
func (r *Runs) Note(n Note) {
	r.mu.Lock()
	r.notes = append(r.notes, n)
	r.mu.Unlock()
}

// Request queues a start or stop of an event. A plain name is the
// calling mod's event first.
func (r *Runs) Request(name, mod string, start bool) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.defs[name]
	if !ok && !strings.Contains(name, ":") {
		d, ok = r.defs[mod+":"+name]
	}
	if !ok {
		return false, nil
	}
	runs := false
	for _, n := range r.running {
		if n.Name == d.Name {
			runs = true
		}
	}
	for _, q := range r.requests {
		if q.Event == d.Name {
			// A request that waits decides what the event will do.
			runs = q.Start
		}
	}
	if runs == start {
		return false, nil
	}
	r.requests = append(r.requests, Request{Event: d.Name, Start: start})
	return true, nil
}

// Requests returns the requests since the last call.
func (r *Runs) Requests() []Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.requests
	r.requests = nil
	return out
}

// Running lists the runs that last now.
func (r *Runs) Running() []Notice {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Notice, 0, len(r.running))
	for _, n := range r.running {
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b Notice) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Hint is what a hint NPC of a treasure hunt says: where the chests
// that are still there lie, nearest first, and how many there are.
func (r *Runs) Hint(run int64, mapName string, x, y int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var left []chest
	for _, c := range r.chests[run] {
		if !c.Open {
			left = append(left, c)
		}
	}
	if len(left) == 0 {
		return []string{"Every chest has been found. Thank you for playing!"}
	}
	slices.SortStableFunc(left, func(a, b chest) int {
		da, db := hintDistance(a, mapName, x, y), hintDistance(b, mapName, x, y)
		if da != db {
			if da < db {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Map, b.Map)
	})
	lines := []string{fmt.Sprintf("%d %s still hidden.", len(left), plural(len(left), "chest is", "chests are"))}
	for i, c := range left {
		if i == 3 {
			break
		}
		if c.Map == mapName {
			lines = append(lines, fmt.Sprintf("One lies to the %s of here, %d cells away.", Direction(c.X-x, c.Y-y), max(abs(c.X-x), abs(c.Y-y))))
			continue
		}
		lines = append(lines, fmt.Sprintf("One lies on %s, near %d,%d.", MapID(c.Map), roundTo(c.X, 10), roundTo(c.Y, 10)))
	}
	return lines
}

func hintDistance(c chest, mapName string, x, y int) int {
	if c.Map == mapName {
		return max(abs(c.X-x), abs(c.Y-y))
	}
	return math.MaxInt32
}

func roundTo(v, n int) int { return (v + n/2) / n * n }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Direction names the direction of an offset, where y grows to the
// north as in rAthena's maps: "north", "northeast" and so on.
func Direction(dx, dy int) string {
	if dx == 0 && dy == 0 {
		return "spot"
	}
	a := math.Atan2(float64(dy), float64(dx)) * 180 / math.Pi
	names := []string{"east", "northeast", "north", "northwest", "west", "southwest", "south", "southeast"}
	i := int(math.Round(a/45)+8) % 8
	return names[i]
}

// Condition runs a condition trigger's function in the zone of the
// event's map.
func (r *Runs) Condition(ctx context.Context, d Def) (bool, error) {
	return r.field.Condition(ctx, r.mapOf(d), d.Mod, d.Trigger.Module, d.Trigger.Function)
}

// mapOf is the map whose zone runs an event's script or condition.
func (r *Runs) mapOf(d Def) string {
	if d.Map != "" {
		return d.Map
	}
	return r.field.StartMap()
}

func (r *Runs) pick(n int) int {
	if n <= 1 || r.opts.Rand == nil {
		return 0
	}
	return r.opts.Rand.IntN(n)
}

// maps are the maps a boss or treasure hunt may use that run.
func (r *Runs) maps(band *Band, list []string) []string {
	var out []string
	for _, m := range MapsOf(band, list, r.opts.Levels) {
		if r.field.Has(m) {
			out = append(out, m)
		}
	}
	return out
}

// Start starts a run. A run with Resume set goes on from its progress.
func (r *Runs) Start(ctx context.Context, d Def, run Run) error {
	info := RunInfo{ID: run.ID, Event: d.Name, Display: d.DisplayName(), Type: d.Type, Scheduled: run.Scheduled}
	a := &activeRun{def: d, info: info, ends: run.Ends, done: map[string]bool{}, waveAt: run.Started}
	if run.Resume {
		units, err := r.opts.Progress.EventProgress(ctx, d.Name, run.Scheduled)
		if err != nil {
			return fmt.Errorf("the run's progress: %w", err)
		}
		for _, u := range units {
			a.done[u] = true
		}
		if a.done[UnitEnd] {
			return errors.New("the run ended before the restart")
		}
	}
	var err error
	switch d.Type {
	case TypeScript:
		a.scriptMap = r.mapOf(d)
		a.where = d.Map
	case TypeBoss:
		err = r.startBoss(ctx, a)
	case TypeInvasion:
		err = r.startInvasion(ctx, a)
	case TypeTreasure:
		err = r.startTreasure(ctx, a)
	}
	if err != nil {
		r.clear(ctx, a)
		return err
	}
	if d.Script != "" {
		if a.scriptMap == "" {
			a.scriptMap = d.Map
		}
		if a.scriptMap == "" {
			a.scriptMap = cmpOr(a.where, r.field.StartMap())
		}
		if err := r.field.StartScript(ctx, a.scriptMap, ScriptRun{RunInfo: info, Entry: d.Script, Rewards: d.Rewards}); err != nil {
			a.scriptMap = ""
			r.clear(ctx, a)
			return err
		}
	}
	r.active[run.ID] = a
	n := Notice{Name: d.Name, DisplayName: info.Display, Type: d.Type, Started: true, Map: a.where, Ends: run.Ends}
	r.mu.Lock()
	r.running[run.ID] = n
	r.mu.Unlock()
	r.field.Notice(n)
	a.started = true
	for _, text := range a.says {
		r.field.Announce(text)
	}
	a.says = nil
	if d.Type == TypeInvasion {
		if a.next > 0 {
			// The wave a restart cut short comes again.
			r.startWave(ctx, a, run.Started)
		} else {
			// The first wave comes at once, or after its time.
			r.waves(ctx, a, run.Started)
		}
	}
	return nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (r *Runs) startBoss(ctx context.Context, a *activeRun) error {
	b := a.def.Boss
	if a.done[UnitBoss] {
		return errors.New("the boss died before the restart")
	}
	if len(b.MonsterIDs) == 0 {
		return errors.New("the server has no monster data")
	}
	maps := r.maps(b.Band, b.Maps)
	if len(maps) == 0 {
		return errors.New("no map of its level band or maps runs")
	}
	m := maps[r.pick(len(maps))]
	monster := b.MonsterIDs[r.pick(len(b.MonsterIDs))]
	placed, err := r.field.Spawn(ctx, Spawn{RunInfo: a.info, Map: m, Monster: monster, Amount: 1, Role: RoleBoss, Unit: UnitBoss,
		Rewards: a.def.Rewards, Announce: b.Announce})
	a.maps, a.where = []string{m}, m
	if err != nil {
		return err
	}
	if len(placed) > 0 {
		a.boss = placed[0].Name
	}
	if b.Announce {
		r.say(a, fmt.Sprintf("%s has appeared on %s!", a.boss, MapID(m)))
	}
	return nil
}

func (r *Runs) startInvasion(ctx context.Context, a *activeRun) error {
	in := a.def.Invasion
	if !r.field.Has(in.Town) {
		return fmt.Errorf("its town %s does not run", MapID(in.Town))
	}
	if a.done[UnitResult] {
		return errors.New("the invasion was won before the restart")
	}
	a.maps, a.where = []string{in.Town}, in.Town
	// A run that a restart cut short starts again at the wave it was in.
	for i := range in.Waves {
		if a.done[fmt.Sprintf("wave:%d", i+1)] {
			a.next = i
		}
	}
	r.say(a, fmt.Sprintf("%s is under attack! Defend the town!", MapID(in.Town)))
	return nil
}

func (r *Runs) startTreasure(ctx context.Context, a *activeRun) error {
	t := a.def.Treasure
	maps := r.maps(t.Band, t.Maps)
	if len(maps) == 0 {
		return errors.New("no map of its level band or maps runs")
	}
	var chests []chest
	for i := range t.Chests {
		unit := fmt.Sprintf("chest:%d", i+1)
		if a.done[unit] {
			chests = append(chests, chest{Open: true})
			a.opened++
			continue
		}
		m := maps[r.pick(len(maps))]
		placed, err := r.field.Spawn(ctx, Spawn{RunInfo: a.info, Map: m, Monster: t.ChestID, Amount: 1, Role: RoleChest, Unit: unit,
			Rewards: a.def.Rewards})
		if !slices.Contains(a.maps, m) {
			a.maps = append(a.maps, m)
		}
		if err != nil {
			return err
		}
		c := chest{Map: m, Open: true}
		if len(placed) > 0 {
			c = chest{Map: m, X: placed[0].X, Y: placed[0].Y}
		}
		chests = append(chests, c)
	}
	a.chests = len(chests)
	r.mu.Lock()
	r.chests[a.info.ID] = chests
	r.mu.Unlock()
	byMap := map[string][]Hint{}
	var where []string
	for _, h := range t.Hints {
		if !r.field.Has(h.Map) {
			r.opts.Log.Warn("world event hint NPC skipped: its map does not run", "event", a.def.Name, "map", MapID(h.Map))
			continue
		}
		byMap[h.Map] = append(byMap[h.Map], h)
		if !slices.Contains(where, h.Map) {
			where = append(where, h.Map)
		}
	}
	for _, m := range where {
		if !slices.Contains(a.maps, m) {
			a.maps = append(a.maps, m)
		}
		if err := r.field.Hints(ctx, m, a.info.ID, byMap[m]); err != nil {
			return err
		}
	}
	if len(where) > 0 {
		a.where = where[0]
	}
	text := fmt.Sprintf("%d treasure %s hidden on the fields!", a.chests-a.opened, plural(a.chests-a.opened, "chest is", "chests are"))
	if len(where) > 0 {
		var names []string
		for _, m := range where {
			names = append(names, MapID(m))
		}
		text += " Ask the " + t.Hints[0].DisplayName + " in " + strings.Join(names, ", ") + " for hints."
	}
	r.say(a, text)
	if a.opened == a.chests {
		return errors.New("every chest was opened before the restart")
	}
	return nil
}

// waves starts the invasion waves that are due.
func (r *Runs) waves(ctx context.Context, a *activeRun, now time.Time) {
	in := a.def.Invasion
	if a.next >= len(in.Waves) {
		return
	}
	w := in.Waves[a.next]
	if w.Cleared && a.next > 0 {
		alive, err := r.field.Alive(ctx, in.Town, a.info.ID)
		if err != nil || alive > 0 {
			return
		}
	} else if !w.Cleared && now.Before(a.waveAt.Add(w.After)) {
		return
	}
	r.startWave(ctx, a, now)
}

func (r *Runs) startWave(ctx context.Context, a *activeRun, now time.Time) {
	in := a.def.Invasion
	i := a.next
	a.next++
	a.waveAt = now
	for _, m := range in.Waves[i].Monsters {
		if _, err := r.field.Spawn(ctx, Spawn{RunInfo: a.info, Map: in.Town, Monster: m.ID, Amount: m.Amount, Role: RoleInvader, Edge: true}); err != nil {
			r.opts.Log.Warn("invasion wave not placed", "event", a.def.Name, "wave", i+1, "err", err)
		}
	}
	unit := fmt.Sprintf("wave:%d", i+1)
	if !a.done[unit] {
		a.done[unit] = true
		if err := r.opts.Progress.RecordEventProgress(ctx, a.def.Name, a.info.Scheduled, unit); err != nil {
			r.opts.Log.Warn("world event progress not saved", "event", a.def.Name, "unit", unit, "err", err)
		}
	}
	if len(in.Waves) > 1 {
		r.say(a, fmt.Sprintf("Wave %d of %d attacks %s!", i+1, len(in.Waves), MapID(in.Town)))
	}
}

// Tick reads the zones' notes and moves the typed runs on at now. It
// returns the events whose runs finished.
func (r *Runs) Tick(ctx context.Context, now time.Time) []string {
	r.mu.Lock()
	notes := r.notes
	r.notes = nil
	r.mu.Unlock()
	var finished []string
	for _, n := range notes {
		a := r.active[n.Run]
		if a == nil {
			continue
		}
		switch n.Kind {
		case NoteDied:
			if r.died(a, n) {
				finished = append(finished, a.def.Name)
				r.finish(ctx, a, ResultWon)
			}
		case NoteFinish:
			if n.Result == ResultWon && a.def.Type == TypeInvasion {
				r.winInvasion(ctx, a)
			}
			finished = append(finished, a.def.Name)
			r.finish(ctx, a, n.Result)
		}
	}
	for _, id := range r.ids() {
		a := r.active[id]
		if a.def.Type != TypeInvasion {
			continue
		}
		r.waves(ctx, a, now)
		if a.next < len(a.def.Invasion.Waves) {
			continue
		}
		alive, err := r.field.Alive(ctx, a.def.Invasion.Town, id)
		if err != nil || alive > 0 {
			continue
		}
		r.winInvasion(ctx, a)
		finished = append(finished, a.def.Name)
		r.finish(ctx, a, ResultWon)
	}
	return finished
}

// ids are the active runs' IDs, in order.
func (r *Runs) ids() []int64 {
	out := make([]int64, 0, len(r.active))
	for id := range r.active {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// died is a boss's or chest's death. It reports whether the run is won.
func (r *Runs) died(a *activeRun, n Note) bool {
	who := cmpOr(n.Killer, "Someone")
	switch a.def.Type {
	case TypeBoss:
		if a.def.Boss.Announce {
			text := fmt.Sprintf("%s defeated %s on %s!", who, n.Monster, MapID(n.Map))
			if n.MVP != "" && n.MVP != n.Killer {
				text += " MVP: " + n.MVP + "."
			}
			r.field.Announce(text)
		}
		a.done[UnitBoss] = true
		return true
	case TypeTreasure:
		i := 0
		if _, err := fmt.Sscanf(n.Unit, "chest:%d", &i); err != nil || i < 1 {
			return false
		}
		r.mu.Lock()
		list := r.chests[a.info.ID]
		if i <= len(list) && !list[i-1].Open {
			list[i-1].Open = true
			a.opened++
		}
		r.mu.Unlock()
		left := a.chests - a.opened
		if left > 0 {
			r.field.Announce(fmt.Sprintf("%s found a treasure chest on %s! %d %s left.", who, MapID(n.Map), left, plural(left, "chest is", "chests are")))
			return false
		}
		r.field.Announce(fmt.Sprintf("%s found the last treasure chest on %s!", who, MapID(n.Map)))
		return true
	}
	return false
}

func (r *Runs) winInvasion(ctx context.Context, a *activeRun) {
	town := a.def.Invasion.Town
	if err := r.field.Reward(ctx, RewardRun{RunInfo: a.info, Map: town, Unit: UnitResult, Rewards: a.def.Rewards}); err != nil {
		r.opts.Log.Error("invasion rewards not given", "event", a.def.Name, "err", err)
	}
	r.field.Announce(fmt.Sprintf("%s drove back the invasion!", MapID(town)))
}

// End ends a run whose time is over.
func (r *Runs) End(ctx context.Context, d Def, run Run) error {
	a := r.active[run.ID]
	if a == nil {
		return nil
	}
	result := ""
	switch d.Type {
	case TypeBoss:
		result = ResultLost
		if a.def.Boss.Announce && a.boss != "" {
			r.field.Announce(fmt.Sprintf("%s has left %s.", a.boss, MapID(a.where)))
		}
	case TypeInvasion:
		result = ResultLost
		r.field.Announce(fmt.Sprintf("The invasion of %s was not stopped in time.", MapID(a.def.Invasion.Town)))
	case TypeTreasure:
		result = ResultLost
		left := a.chests - a.opened
		r.field.Announce(fmt.Sprintf("The treasure hunt is over. %d %s not found.", left, plural(left, "chest was", "chests were")))
	}
	r.finishAt(ctx, a, result, false)
	return nil
}

// Stop ends a run before its time, for world_events.stop.
func (r *Runs) Stop(ctx context.Context, _ Def, run Run) error {
	a := r.active[run.ID]
	if a == nil {
		return nil
	}
	r.field.Announce(fmt.Sprintf("%s was stopped.", a.info.Display))
	r.finish(ctx, a, ResultStopped)
	return nil
}

// finish ends a run before its time.
func (r *Runs) finish(ctx context.Context, a *activeRun, result string) {
	r.finishAt(ctx, a, result, true)
}

func (r *Runs) finishAt(ctx context.Context, a *activeRun, result string, early bool) {
	delete(r.active, a.info.ID)
	r.clear(ctx, a)
	if a.scriptMap != "" {
		if err := r.field.EndScript(ctx, a.scriptMap, a.info.ID); err != nil {
			r.opts.Log.Error("world event script did not end", "event", a.def.Name, "err", err)
		}
	}
	if early {
		if err := r.opts.Progress.RecordEventProgress(ctx, a.def.Name, a.info.Scheduled, UnitEnd); err != nil {
			r.opts.Log.Warn("world event progress not saved", "event", a.def.Name, "unit", UnitEnd, "err", err)
		}
	}
	r.mu.Lock()
	delete(r.running, a.info.ID)
	delete(r.chests, a.info.ID)
	r.mu.Unlock()
	r.field.Notice(Notice{Name: a.def.Name, DisplayName: a.info.Display, Type: a.def.Type, Result: result, Map: a.where})
	r.opts.Log.Info("world event finished", "event", a.def.Name, "result", result)
}

// clear removes a run's monsters and NPCs from its maps.
func (r *Runs) clear(ctx context.Context, a *activeRun) {
	for _, m := range a.maps {
		if err := r.field.Clear(ctx, m, a.info.ID); err != nil {
			r.opts.Log.Error("world event not cleared from a map", "event", a.def.Name, "map", MapID(m), "err", err)
		}
	}
}

// Active reports whether a run lasts.
func (r *Runs) Active(id int64) bool { return r.active[id] != nil }
