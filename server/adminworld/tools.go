package adminworld

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/metrics"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/restart"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/worldevent"
)

// The world, events and mods tools (proposal 0002 review decision 36,
// decision 0031):
//   - The world view reads each zone on its goroutine, and its last
//     minute of ticks from its measurements.
//   - Announcements go to the world or one map.
//   - Restarts go to the planner (server/restart).
//   - Event changes are saved over the config in event_overrides, with
//     their audit rows, then go to the scheduler, which applies them at
//     its next check, within a second.
//   - Mod switches are saved in mod_switches, with their audit rows,
//     then go to the zones.

// Tools are what the tools need, beyond Config.
type Tools struct {
	// Metrics are the process's measurements.
	Metrics *metrics.Registry
	// Started is when the process started, and Version its version.
	Started time.Time
	Version string
	// Restart plans restarts. Without it, restarts are refused.
	Restart *restart.Planner
	// Scheduler and Runs run the world events. Without them, there are
	// no events.
	Scheduler *worldevent.Scheduler
	Runs      *worldevent.Runs
	// Events are the events as the config sets them, which a reset goes
	// back to. Overrides are the staff changes that waserver read at
	// start.
	Events    []worldevent.Setting
	Overrides []store.EventOverride
	// Mods are the loaded mods, and ScriptMods the IDs of those with Lua
	// scripts. Switches are the mods staff turned off, read at start.
	Mods       *mod.Set
	ScriptMods []string
	Switches   []store.ModSwitch
}

// tools is the state of the tools.
type tools struct {
	// change serializes each event change and mod switch, from its
	// check to its last write. mu guards the fields.
	change    sync.Mutex
	mu        sync.Mutex
	overrides map[string]store.EventOverride
	switches  []store.ModSwitch
}

func (w *World) initTools() {
	w.tools.overrides = map[string]store.EventOverride{}
	for _, o := range w.cfg.Tools.Overrides {
		w.tools.overrides[o.Event] = o
	}
	w.tools.switches = slices.Clone(w.cfg.Tools.Switches)
}

// MaxListed is the most players the world view lists.
const MaxListed = 1000

// WorldView reads the zones, the process, the players online and the
// planned restart.
func (w *World) WorldView(ctx context.Context) (*adminv1.WorldView, error) {
	t := w.cfg.Tools
	v := &adminv1.WorldView{}
	var zones []*adminv1.ZoneView
	var players []*adminv1.OnlinePlayer
	wall := time.Now()
	for _, z := range w.cfg.World.Zones() {
		s, err := z.Summary(ctx)
		if err != nil {
			return nil, err
		}
		r := z.Metrics.Recent(wall)
		zones = append(zones, adminv1.ZoneView_builder{Map: mapID(z.Map), Players: int32(len(s.Players)), Monsters: int32(s.Monsters), // #nosec G115 -- counts of one zone.
			Ticks: int32(r.Ticks), P50Us: r.P50.Microseconds(), P99Us: r.P99.Microseconds(), MaxUs: r.Max.Microseconds(), // #nosec G115 -- at most WindowTicks.
			OverBudget: int32(r.OverBudget), Asleep: len(s.Players) == 0}.Build()) // #nosec G115 -- at most WindowTicks.
		for _, p := range s.Players {
			players = append(players, adminv1.OnlinePlayer_builder{CharacterId: p.CharacterID, Name: p.Name, Map: mapID(z.Map),
				X: int32(p.X), Y: int32(p.Y), BaseLevel: int32(p.BaseLevel), JobName: w.jobName(p.Job)}.Build()) // #nosec G115 -- cells and levels are small.
		}
	}
	slices.SortFunc(players, func(a, b *adminv1.OnlinePlayer) int { return strings.Compare(a.GetName(), b.GetName()) })
	v.SetPlayersOnline(int32(len(players))) // #nosec G115 -- players of one process.
	if len(players) > MaxListed {
		players = players[:MaxListed]
	}
	v.SetZones(zones)
	v.SetPlayers(players)
	p := adminv1.ProcessView_builder{Version: t.Version}
	if t.Metrics != nil {
		s := t.Metrics.Process.Snapshot()
		p.CpuCores, p.HeapBytes, p.TotalBytes, p.Goroutines = s.CPU, s.HeapBytes, s.TotalBytes, s.Goroutines
	}
	if !t.Started.IsZero() {
		p.StartedAt = timestamppb.New(t.Started)
		p.UptimeSeconds = int64(wall.Sub(t.Started).Seconds())
	}
	v.SetProcess(p.Build())
	if t.Restart != nil {
		if pl, ok := t.Restart.Current(); ok {
			v.SetRestart(w.restartPlan(pl))
		}
	}
	v.SetNow(timestamppb.New(w.now()))
	return v, nil
}

// Now is the server clock.
func (w *World) Now() time.Time { return w.now() }

func mapID(asset string) string { return strings.TrimPrefix(asset, "map/") }

// assetOf is a map's asset name from the name the API shows.
func assetOf(m string) string {
	if strings.Contains(m, "/") {
		return m
	}
	return "map/" + m
}

// Announce shows a line to every player, or to one map's players.
func (w *World) Announce(_ context.Context, text, mapName string) error {
	if mapName == "" {
		w.cfg.World.Announce(text, 0)
		return nil
	}
	if !w.cfg.World.AnnounceMap(assetOf(mapName), text, 0) {
		return admin.ErrNotFound
	}
	return nil
}

func (w *World) restartPlan(pl restart.Plan) *adminv1.RestartPlan {
	_, closed := w.cfg.Tools.Restart.Closed()
	return adminv1.RestartPlan_builder{At: timestamppb.New(pl.At), Reason: pl.Reason, Message: pl.Message, PlannedBy: pl.By,
		PlannedAt: timestamppb.New(pl.Planned), LoginsClosed: closed}.Build()
}

var errNoPlanner = &admin.WorldError{Code: "unavailable", Message: "This server cannot plan restarts."}

// PlanRestart plans a restart.
func (w *World) PlanRestart(_ context.Context, o admin.RestartOrder) (*adminv1.RestartPlan, error) {
	p := w.cfg.Tools.Restart
	if p == nil {
		return nil, errNoPlanner
	}
	pl, err := p.Plan(restart.Plan{At: o.At, Reason: o.Reason, Message: o.Message, By: o.AccountName, ByID: o.AccountID})
	switch {
	case errors.Is(err, restart.ErrPlanned):
		return nil, &admin.WorldError{Code: "conflict", Message: "A restart is planned already. Cancel it first."}
	case errors.Is(err, restart.ErrNotTime):
		return nil, &admin.WorldError{Code: "bad_request", Message: "A restart is planned from 1 minute to 7 days ahead of the server clock."}
	case err != nil:
		return nil, &admin.WorldError{Code: "bad_request", Message: err.Error()}
	}
	return w.restartPlan(pl), nil
}

// CancelRestart cancels the planned restart.
func (w *World) CancelRestart(context.Context) (*adminv1.RestartPlan, error) {
	p := w.cfg.Tools.Restart
	if p == nil {
		return nil, errNoPlanner
	}
	pl, err := p.Cancel()
	switch {
	case errors.Is(err, restart.ErrNoPlan):
		return nil, &admin.WorldError{Code: "not_found", Message: "No restart is planned."}
	case errors.Is(err, restart.ErrTooLate):
		return nil, &admin.WorldError{Code: "conflict", Message: "The restart has begun."}
	case err != nil:
		return nil, err
	}
	return adminv1.RestartPlan_builder{At: timestamppb.New(pl.At), Reason: pl.Reason, Message: pl.Message, PlannedBy: pl.By,
		PlannedAt: timestamppb.New(pl.Planned)}.Build(), nil
}

// ApplyOverrides applies the staff changes of world events that the
// database keeps over the config's settings, at start. A change of an
// event that no longer exists, or whose saved trigger no longer reads,
// is left out with a warning.
func ApplyOverrides(settings []worldevent.Setting, overrides []store.EventOverride) ([]worldevent.Setting, []string) {
	byName := map[string]store.EventOverride{}
	for _, o := range overrides {
		byName[o.Event] = o
	}
	out := slices.Clone(settings)
	var warnings []string
	seen := map[string]bool{}
	for i, s := range out {
		o, ok := byName[s.Def.Name]
		if !ok {
			continue
		}
		seen[s.Def.Name] = true
		n, err := applyOverride(s, o)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("the staff change of %s is left out: %v", s.Def.Name, err))
			continue
		}
		out[i] = n
	}
	for _, o := range overrides {
		if !seen[o.Event] {
			warnings = append(warnings, fmt.Sprintf("a staff change names the event %s, which does not exist", o.Event))
		}
	}
	return out, warnings
}

// applyOverride is an event's setting with a staff change over it.
func applyOverride(s worldevent.Setting, o store.EventOverride) (worldevent.Setting, error) {
	if o.On != nil {
		s.On = *o.On
	}
	if o.Trigger != nil {
		t, err := worldevent.ParseJSONTrigger(o.Trigger)
		if err != nil {
			return s, fmt.Errorf("trigger: %w", err)
		}
		s.Def.Trigger = t
	}
	if o.DurationMinutes > 0 {
		if err := worldevent.CheckMinutes(o.DurationMinutes); err != nil {
			return s, fmt.Errorf("duration: %w", err)
		}
		s.Def.Duration = time.Duration(o.DurationMinutes) * time.Minute
	}
	return s, nil
}

func (w *World) configOf(name string) (worldevent.Setting, bool) {
	for _, s := range w.cfg.Tools.Events {
		if s.Def.Name == name {
			return s, true
		}
	}
	return worldevent.Setting{}, false
}

// Events lists the world events.
func (w *World) Events(context.Context) (*adminv1.EventList, error) {
	out := &adminv1.EventList{}
	if w.cfg.Tools.Scheduler == nil {
		return out, nil
	}
	var results map[string][]worldevent.Result
	if w.cfg.Tools.Runs != nil {
		results = w.cfg.Tools.Runs.Results()
	}
	var list []*adminv1.EventInfo
	for _, st := range w.cfg.Tools.Scheduler.Status() {
		list = append(list, w.eventInfo(st, results[st.Def.Name]))
	}
	out.SetEvents(list)
	return out, nil
}

func (w *World) eventInfo(st worldevent.Status, results []worldevent.Result) *adminv1.EventInfo {
	d := st.Def
	e := adminv1.EventInfo_builder{Name: d.Name, DisplayName: d.DisplayName(), Mod: d.Mod, Type: cmp.Or(d.Type, "script"), On: st.On,
		TriggerText: d.Trigger.Describe(), DurationMinutes: int32(d.Duration.Minutes()), Failed: st.Failed} // #nosec G115 -- at most a week of minutes.
	if v, err := structpb.NewStruct(d.Trigger.Value()); err == nil {
		e.Trigger = v
	}
	if !st.Next.IsZero() {
		e.NextRun = timestamppb.New(st.Next)
	}
	if !st.FailedAt.IsZero() {
		e.FailedAt = timestamppb.New(st.FailedAt)
	}
	if r := st.Active; r != nil {
		e.Runs = []*adminv1.EventRun{adminv1.EventRun_builder{Scheduled: timestamppb.New(r.Scheduled), Started: timestamppb.New(r.Started),
			Ends: timestamppb.New(r.Ends)}.Build()}
	}
	for _, r := range results {
		e.Results = append(e.Results, adminv1.EventResult_builder{Scheduled: timestamppb.New(r.Scheduled), Started: timestamppb.New(r.Started),
			Ended: timestamppb.New(r.Ended), Result: r.Result}.Build())
	}
	if c, ok := w.configOf(d.Name); ok {
		e.ConfigOn, e.ConfigTriggerText, e.ConfigDurationMinutes = c.On, c.Def.Trigger.Describe(), int32(c.Def.Duration.Minutes()) // #nosec G115 -- as above.
	}
	w.tools.mu.Lock()
	o, ok := w.tools.overrides[d.Name]
	w.tools.mu.Unlock()
	if ok {
		m := adminv1.EventOverride_builder{ChangedBy: o.By, ChangedAt: timestamppb.New(o.At), DurationMinutes: int32(o.DurationMinutes)} // #nosec G115 -- as above.
		if o.On != nil {
			m.On = o.On
		}
		if o.Trigger != nil {
			if v, err := structpb.NewStruct(o.Trigger); err == nil {
				m.Trigger = v
			}
		}
		e.Override = m.Build()
	}
	return e.Build()
}

// EventAction starts, stops, changes or resets an event.
func (w *World) EventAction(ctx context.Context, o admin.EventOrder) (*adminv1.EventInfo, error) {
	t := w.cfg.Tools
	if t.Scheduler == nil {
		return nil, admin.ErrNotFound
	}
	cfg, ok := w.configOf(o.Name)
	if !ok {
		return nil, &admin.WorldError{Code: "not_found", Message: "There is no such event."}
	}
	audit := store.AuditRow{AccountID: o.AccountID, AccountName: o.AccountName, Role: o.Role, Action: "events." + o.Action, Source: "admin",
		Target: o.Name, Reason: o.Reason}
	switch o.Action {
	case admin.EventStart, admin.EventStop:
		err := t.Scheduler.Do(ctx, worldevent.Change{Event: o.Name, Start: o.Action == admin.EventStart, Stop: o.Action == admin.EventStop})
		switch {
		case errors.Is(err, worldevent.ErrRunning):
			return nil, &admin.WorldError{Code: "conflict", Message: "The event runs already."}
		case errors.Is(err, worldevent.ErrNotRunning):
			return nil, &admin.WorldError{Code: "conflict", Message: "The event does not run now."}
		case errors.Is(err, worldevent.ErrNoEvent):
			return nil, &admin.WorldError{Code: "not_found", Message: "There is no such event."}
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			return nil, err
		case err != nil:
			return nil, &admin.WorldError{Code: "conflict", Message: "The event did not " + o.Action + ": " + err.Error()}
		}
		if w.cfg.Store != nil {
			if err := w.cfg.Store.WriteAudit(ctx, audit); err != nil {
				return nil, err
			}
		}
	case admin.EventChange, admin.EventReset:
		st := w.cfg.Store
		if st == nil {
			return nil, errNoStore
		}
		w.tools.change.Lock()
		defer w.tools.change.Unlock()
		w.tools.mu.Lock()
		old, had := w.tools.overrides[o.Name]
		w.tools.mu.Unlock()
		next := store.EventOverride{Event: o.Name, By: o.AccountName, ByID: o.AccountID, At: w.now()}
		setting := cfg
		if o.Action == admin.EventChange {
			if had {
				next.On, next.Trigger, next.DurationMinutes = old.On, old.Trigger, old.DurationMinutes
			}
			if o.On != nil {
				next.On = o.On
			}
			if o.Trigger != nil {
				next.Trigger = o.Trigger
			}
			if o.DurationMinutes > 0 {
				next.DurationMinutes = o.DurationMinutes
			}
			var err error
			if setting, err = applyOverride(cfg, next); err != nil {
				return nil, &admin.WorldError{Code: "bad_request", Message: "The change does not read: " + err.Error()}
			}
			if setting.On {
				if err := w.cfg.World.CheckEvent(setting.Def); err != nil {
					return nil, &admin.WorldError{Code: "bad_request", Message: "The event cannot run in this world: " + err.Error()}
				}
			}
			args := map[string]any{"on": setting.On, "trigger": setting.Def.Trigger.Describe(), "minutes": int(setting.Def.Duration.Minutes())}
			audit.Arguments = args
			if err := st.SetEventOverride(ctx, next, audit); err != nil {
				return nil, err
			}
		} else {
			found, err := st.ResetEventOverride(ctx, o.Name, audit)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, &admin.WorldError{Code: "conflict", Message: "The event follows the config already."}
			}
		}
		w.tools.mu.Lock()
		if o.Action == admin.EventChange {
			w.tools.overrides[o.Name] = next
		} else {
			delete(w.tools.overrides, o.Name)
		}
		w.tools.mu.Unlock()
		if err := t.Scheduler.Do(ctx, worldevent.Change{Event: o.Name, Set: &setting}); err != nil {
			return nil, err
		}
	default:
		return nil, &admin.WorldError{Code: "bad_request", Message: "There is no such action."}
	}
	for _, st := range t.Scheduler.Status() {
		if st.Def.Name == o.Name {
			var results []worldevent.Result
			if t.Runs != nil {
				results = t.Runs.Results()[o.Name]
			}
			return w.eventInfo(st, results), nil
		}
	}
	return nil, admin.ErrNotFound
}

// Signatures of ModInfo.
const (
	SignatureBuiltin    = "builtin"
	SignatureFolder     = "folder"
	SignatureUnsigned   = "unsigned"
	SignatureUnverified = "unverified"
)

// signatureOf says what the server knows of a mod's signature: a .zip
// file with a .minisig file next to it is unverified, since the server
// has no keys to check it with.
func signatureOf(m *mod.Mod) string {
	switch {
	case m.Builtin:
		return SignatureBuiltin
	case !strings.HasSuffix(strings.ToLower(m.Path), ".zip"):
		return SignatureFolder
	}
	if _, err := os.Stat(m.Path + ".minisig"); err == nil {
		return SignatureUnverified
	}
	return SignatureUnsigned
}

// Mods lists the loaded mods.
func (w *World) Mods(context.Context) (*adminv1.ModList, error) {
	out := &adminv1.ModList{}
	var maps []string
	zones := w.cfg.World.Zones()
	for _, z := range zones {
		maps = append(maps, mapID(z.Map))
	}
	out.SetMaps(maps)
	if w.cfg.Tools.Mods == nil {
		return out, nil
	}
	errs := map[string]map[string]modErrors{}
	for _, z := range zones {
		for id, e := range z.ModErrors() {
			if errs[id] == nil {
				errs[id] = map[string]modErrors{}
			}
			errs[id][z.Map] = modErrors{e.Count, e.Last, e.At}
		}
	}
	var list []*adminv1.ModInfo
	for _, m := range w.cfg.Tools.Mods.Mods {
		list = append(list, w.modInfo(m, errs[m.ID()]))
	}
	out.SetMods(list)
	return out, nil
}

type modErrors struct {
	count int64
	last  string
	at    time.Time
}

func (w *World) modInfo(m *mod.Mod, errs map[string]modErrors) *adminv1.ModInfo {
	man := m.Manifest
	info := adminv1.ModInfo_builder{Id: m.ID(), Name: man.Name, Version: man.Version, Builtin: m.Builtin, Signature: signatureOf(m),
		Granted: m.Grants.List(), Requested: slices.Clone(man.Permissions), Scripts: slices.Contains(w.cfg.Tools.ScriptMods, m.ID())}
	for _, p := range man.Permissions {
		if !m.Grants.Has(p) {
			info.Pending = append(info.Pending, p)
		}
	}
	w.tools.mu.Lock()
	var switches []store.ModSwitch
	for _, s := range w.tools.switches {
		if s.Mod == m.ID() {
			switches = append(switches, s)
		}
	}
	w.tools.mu.Unlock()
	everywhere := false
	off := map[string]bool{}
	for _, s := range switches {
		info.Switches = append(info.Switches, adminv1.ModSwitch_builder{Map: mapID(s.Map), Reason: s.Reason, ChangedBy: s.By,
			ChangedAt: timestamppb.New(s.At)}.Build())
		if s.Map == "" {
			everywhere = true
		}
		off[s.Map] = true
	}
	for _, z := range w.cfg.World.Zones() {
		e, failed := errs[z.Map]
		if !failed && !off[z.Map] {
			continue
		}
		mz := adminv1.ModZone_builder{Map: mapID(z.Map), Off: everywhere || off[z.Map], Errors: e.count, LastError: e.last}
		if !e.at.IsZero() {
			mz.LastErrorAt = timestamppb.New(e.at)
		}
		info.Zones = append(info.Zones, mz.Build())
		info.Errors += e.count
	}
	return info.Build()
}

// SwitchMod turns a mod off or on, in one zone or every zone.
func (w *World) SwitchMod(ctx context.Context, o admin.ModOrder) (*adminv1.ModInfo, error) {
	set := w.cfg.Tools.Mods
	if set == nil {
		return nil, &admin.WorldError{Code: "not_found", Message: "No loaded mod has that ID."}
	}
	m := set.Mod(o.Mod)
	switch {
	case m == nil:
		return nil, &admin.WorldError{Code: "not_found", Message: "No loaded mod has that ID."}
	case m.Builtin:
		return nil, &admin.WorldError{Code: "bad_request", Message: "Core and its ruleset mods cannot be turned off."}
	case w.cfg.Store == nil:
		return nil, errNoStore
	}
	w.tools.change.Lock()
	defer w.tools.change.Unlock()
	asset := ""
	if o.Map != "" {
		asset = assetOf(o.Map)
		if w.cfg.World.Zone(asset) == nil {
			return nil, &admin.WorldError{Code: "not_found", Message: "That map does not run in this world."}
		}
	}
	w.tools.mu.Lock()
	everywhere, here, some := false, false, false
	for _, s := range w.tools.switches {
		if s.Mod == o.Mod {
			everywhere = everywhere || s.Map == ""
			here = here || s.Map == asset
			some = true
		}
	}
	w.tools.mu.Unlock()
	switch {
	case o.Off && everywhere:
		return nil, &admin.WorldError{Code: "conflict", Message: "The mod is off in every zone already."}
	case o.Off && here:
		return nil, &admin.WorldError{Code: "conflict", Message: "The mod is off in that zone already."}
	case !o.Off && asset != "" && everywhere:
		return nil, &admin.WorldError{Code: "conflict", Message: "The mod is off in every zone. Turn it on in every zone first."}
	case !o.Off && !here && (asset != "" || !some):
		return nil, &admin.WorldError{Code: "conflict", Message: "The mod is not off there."}
	}
	action := "mods.enable"
	if o.Off {
		action = "mods.disable"
	}
	target := cmp.Or(mapID(asset), "every zone")
	audit := store.AuditRow{AccountID: o.AccountID, AccountName: o.AccountName, Role: o.Role, Action: action, Source: "admin",
		Target: o.Mod, Arguments: map[string]any{"map": target}, Reason: o.Reason}
	sw := store.ModSwitch{Mod: o.Mod, Map: asset, Reason: o.Reason, By: o.AccountName, ByID: o.AccountID, At: w.now()}
	if err := w.cfg.Store.SwitchMod(ctx, sw, o.Off, audit); err != nil {
		return nil, err
	}
	w.tools.mu.Lock()
	switch {
	case o.Off && asset == "":
		w.tools.switches = slices.DeleteFunc(w.tools.switches, func(s store.ModSwitch) bool { return s.Mod == o.Mod })
		w.tools.switches = append(w.tools.switches, sw)
	case o.Off:
		w.tools.switches = append(w.tools.switches, sw)
	case asset == "":
		w.tools.switches = slices.DeleteFunc(w.tools.switches, func(s store.ModSwitch) bool { return s.Mod == o.Mod })
	default:
		w.tools.switches = slices.DeleteFunc(w.tools.switches, func(s store.ModSwitch) bool { return s.Mod == o.Mod && s.Map == asset })
	}
	w.tools.mu.Unlock()
	// The zones follow: every zone, or the one named.
	var maps []string
	if asset != "" {
		maps = []string{asset}
	}
	// A zone's error is a zone that stopped, or for a mod turned on, its
	// scripts that failed to start, which the runtime logged and counted.
	if err := w.cfg.World.SetModOff(ctx, o.Mod, maps, o.Off); err != nil {
		w.log.Warn("some zones reported errors for a mod switch", "mod", o.Mod, "off", o.Off, "err", err)
	}
	w.log.Info("mod switched by staff", "mod", o.Mod, "map", target, "off", o.Off, "by", o.AccountName)
	return w.modInfo(m, w.errorsOf(o.Mod)), nil
}

func (w *World) errorsOf(mod string) map[string]modErrors {
	out := map[string]modErrors{}
	for _, z := range w.cfg.World.Zones() {
		if e, ok := z.ModErrors()[mod]; ok {
			out[z.Map] = modErrors{e.Count, e.Last, e.At}
		}
	}
	return out
}

// OffAt returns the mods that the switches turn off, by map asset name,
// with "" for every zone, for the zones at start.
func OffAt(switches []store.ModSwitch) map[string][]string {
	out := map[string][]string{}
	for _, s := range switches {
		out[s.Map] = append(out[s.Map], s.Mod)
	}
	return out
}
