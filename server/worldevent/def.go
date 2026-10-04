// Package worldevent starts world events without staff action
// (strategy section 8.8): the events of data/events.yml, with schedule,
// random and condition triggers, on the server clock and in the
// server's time zone. mods/README.md, "World events", has the rules for
// mod authors.
package worldevent

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/schedule"
)

// TriggerKind is what starts an event.
type TriggerKind int

// Kinds of trigger.
const (
	// Clock starts the event at a time of a schedule.Spec.
	Clock TriggerKind = iota
	// Random starts the event with a chance at the start of each hour.
	Random
	// Condition starts the event once a Lua function has returned true
	// for a time.
	Condition
)

// Trigger is what starts an event.
type Trigger struct {
	Kind TriggerKind
	// Spec is the time of a Clock trigger.
	Spec schedule.Spec
	// Chance is the chance per hour of a Random trigger, per 10,000.
	Chance int
	// Module and Function name the condition of a Condition trigger: a
	// function of the module that require finds by that name. Lasting
	// is how long it must hold.
	Module, Function string
	Lasting          time.Duration
}

// Def is one world event.
type Def struct {
	// Name is the event's full name, such as example.bosses:Night.
	Name string
	// Mod is the mod that added the event.
	Mod      string
	Trigger  Trigger
	Duration time.Duration
	// Script is the event's entry script, as <mod>:<path>.
	Script string
	// Map is the map whose zone runs the event, as an asset name such as
	// map/prt_fild08, or "" for the world's start map.
	Map string
	// Where is the file and line of the record, for messages.
	Where string
	// Type is the event's type: TypeScript (""), TypeBoss, TypeInvasion
	// or TypeTreasure. The type's own fields are in Boss, Invasion or
	// Treasure, and Rewards are what a typed event gives.
	Type     string
	Boss     *Boss
	Invasion *Invasion
	Treasure *Treasure
	Rewards  *Rewards
}

// typeOfField is the type that has a field, or "".
func typeOfField(k string) string {
	for _, t := range []string{TypeBoss, TypeInvasion, TypeTreasure} {
		if slices.Contains(typedFields[t], k) {
			return t
		}
	}
	return ""
}

// Typed reports whether the event is a typed event.
func (d Def) Typed() bool { return d.Type != TypeScript }

// MaxDuration is the longest run, in minutes: a week.
const MaxDuration = 7 * 24 * 60

// Parse reads an event record of data/events.yml.
func Parse(r *mod.Record) (Def, error) {
	d := Def{Name: r.Name, Mod: r.Mod, Where: r.Where}
	var errs []string
	if v, ok := r.Fields["type"]; ok {
		t, _ := v.(string)
		if _, known := typedFields[t]; !known {
			errs = append(errs, "type must be boss, invasion or treasure_hunt; an event without type is a script event")
		} else {
			d.Type = t
		}
	}
	fields := knownFields(d.Type)
	for k := range r.Fields {
		if slices.Contains(fields, k) {
			continue
		}
		if d.Type == TypeScript {
			if typ := typeOfField(k); typ != "" {
				errs = append(errs, fmt.Sprintf("%q is a field of typed events; set type, such as type: %s", k, typ))
				continue
			}
			errs = append(errs, fmt.Sprintf("%q is not a field; events have %s", k, strings.Join(fields, ", ")))
			continue
		}
		errs = append(errs, fmt.Sprintf("%q is not a field; events of type %s have %s", k, d.Type, strings.Join(fields, ", ")))
	}
	t, err := ParseTrigger(r.Fields["trigger"])
	if err != nil {
		errs = append(errs, "trigger: "+err.Error())
	}
	d.Trigger = t
	n, err := minutes(r.Fields["duration_minutes"])
	if err != nil {
		errs = append(errs, "duration_minutes: "+err.Error())
	}
	d.Duration = time.Duration(n) * time.Minute
	if _, ok := r.Fields["script"]; !ok {
		if !d.Typed() {
			errs = append(errs, "script is required: the entry script that runs the event, or a type such as type: boss")
		}
	} else if d.Script, err = mod.ScriptOf(r); err != nil {
		errs = append(errs, "script: "+err.Error())
	}
	if d.Typed() {
		errs = append(errs, parseTyped(&d, r.Fields)...)
	} else if _, ok := r.Fields["rewards"]; ok {
		errs = append(errs, "rewards are given by typed events; set type, such as type: boss")
	}
	if v, ok := r.Fields["map"]; ok {
		m, ok := v.(string)
		if !ok || m == "" || strings.ContainsAny(m, " \\") {
			errs = append(errs, "map must be a map name, such as prt_fild08")
		} else {
			d.Map = AssetOf(m)
		}
	}
	if len(errs) > 0 {
		return d, errors.New(strings.Join(errs, "; "))
	}
	return d, nil
}

func minutes(v any) (int, error) {
	n, ok := v.(int)
	if !ok {
		return 0, fmt.Errorf("want whole minutes, from 1 to %d", MaxDuration)
	}
	if n < 1 || n > MaxDuration {
		return 0, fmt.Errorf("%d is not from 1 to %d", n, MaxDuration)
	}
	return n, nil
}

// ParseTrigger reads a trigger: a mapping with one of daily, weekly,
// monthly, hourly, random or condition.
func ParseTrigger(v any) (Trigger, error) {
	m, ok := v.(map[string]any)
	if !ok || len(m) != 1 {
		return Trigger{}, errors.New("want one of daily, weekly, monthly, hourly, random or condition, such as { daily: \"20:00\" }")
	}
	for kind, arg := range m {
		return parseKind(kind, arg)
	}
	return Trigger{}, nil
}

func parseKind(kind string, arg any) (Trigger, error) {
	t := Trigger{Kind: Clock}
	sub, _ := arg.(map[string]any)
	known := func(names ...string) error {
		if sub == nil {
			return fmt.Errorf("%s: want a mapping of %s", kind, strings.Join(names, ", "))
		}
		for k := range sub {
			if !slices.Contains(names, k) {
				return fmt.Errorf("%s: %q is not a field; it has %s", kind, k, strings.Join(names, ", "))
			}
		}
		return nil
	}
	clock := func(v any) error {
		s, _ := v.(string)
		h, mi, err := schedule.ParseClock(s)
		t.Spec.Hour, t.Spec.Minute = h, mi
		return err
	}
	switch kind {
	case "daily":
		t.Spec.Kind = schedule.Daily
		if err := clock(arg); err != nil {
			return t, fmt.Errorf("daily: %w", err)
		}
	case "weekly":
		t.Spec.Kind = schedule.Weekly
		if err := known("day", "time"); err != nil {
			return t, err
		}
		s, _ := sub["day"].(string)
		d, err := schedule.ParseWeekday(s)
		if err != nil {
			return t, fmt.Errorf("weekly: day: %w", err)
		}
		t.Spec.Weekday = d
		if err := clock(sub["time"]); err != nil {
			return t, fmt.Errorf("weekly: time: %w", err)
		}
	case "monthly":
		t.Spec.Kind = schedule.Monthly
		if err := known("day", "time"); err != nil {
			return t, err
		}
		d, ok := sub["day"].(int)
		if !ok || d < 1 || d > 31 {
			return t, errors.New("monthly: day must be 1 to 31")
		}
		t.Spec.Day = d
		if err := clock(sub["time"]); err != nil {
			return t, fmt.Errorf("monthly: time: %w", err)
		}
	case "hourly":
		t.Spec.Kind = schedule.Hourly
		if err := known("minute"); err != nil {
			return t, err
		}
		mi, ok := sub["minute"].(int)
		if !ok || mi < 0 || mi > 59 {
			return t, errors.New("hourly: minute must be 0 to 59")
		}
		t.Spec.Minute = mi
	case "random":
		t.Kind = Random
		if err := known("chance_per_hour"); err != nil {
			return t, err
		}
		c, ok := sub["chance_per_hour"].(int)
		if !ok || c < 1 || c > 10000 {
			return t, errors.New("random: chance_per_hour must be 1 to 10000, per 10,000")
		}
		t.Chance = c
	case "condition":
		t.Kind = Condition
		if err := known("module", "function", "lasting_minutes"); err != nil {
			return t, err
		}
		t.Module, _ = sub["module"].(string)
		t.Function, _ = sub["function"].(string)
		if t.Module == "" || t.Function == "" {
			return t, errors.New("condition: module and function are required, such as { module: example.bosses.conditions, function: town_is_quiet }")
		}
		if v, ok := sub["lasting_minutes"]; ok {
			n, ok := v.(int)
			if !ok || n < 0 || n > MaxDuration {
				return t, fmt.Errorf("condition: lasting_minutes must be 0 to %d", MaxDuration)
			}
			t.Lasting = time.Duration(n) * time.Minute
		}
	default:
		return t, fmt.Errorf("%q is not a trigger; want daily, weekly, monthly, hourly, random or condition", kind)
	}
	return t, nil
}

// Override is what the server owner changes about an event in the
// config (mods.events): a new trigger, a new duration, or neither when
// the event is disabled. Enabled turns on one of core's events, which
// are off by default.
type Override struct {
	Trigger         any  `yaml:"trigger"`
	DurationMinutes int  `yaml:"duration_minutes"`
	Disabled        bool `yaml:"disabled"`
	Enabled         bool `yaml:"enabled"`
}

// Apply applies the owner's overrides to the events, by full name. A
// disabled event is left out, and so is a core event that is not
// enabled. An override of an event that does not exist is an error.
func Apply(defs []Def, overrides map[string]Override) ([]Def, error) {
	var errs []error
	byName := map[string]bool{}
	var out []Def
	for _, d := range defs {
		byName[d.Name] = true
		o, ok := overrides[d.Name]
		if o.Disabled && o.Enabled {
			errs = append(errs, fmt.Errorf("mods.events.%s: enabled and disabled cannot both be set", d.Name))
			continue
		}
		if d.Mod == mod.Core && !o.Enabled || o.Disabled {
			// Core's events are samples, off until the owner turns them
			// on.
			continue
		}
		if !ok {
			out = append(out, d)
			continue
		}
		if o.Trigger != nil {
			t, err := ParseTrigger(o.Trigger)
			if err != nil {
				errs = append(errs, fmt.Errorf("mods.events.%s: trigger: %w", d.Name, err))
				continue
			}
			d.Trigger = t
		}
		if o.DurationMinutes != 0 {
			n, err := minutes(o.DurationMinutes)
			if err != nil {
				errs = append(errs, fmt.Errorf("mods.events.%s: duration_minutes: %w", d.Name, err))
				continue
			}
			d.Duration = time.Duration(n) * time.Minute
		}
		out = append(out, d)
	}
	for _, name := range sortedNames(overrides) {
		if !byName[name] {
			errs = append(errs, fmt.Errorf("mods.events: there is no event %s", name))
		}
	}
	return out, errors.Join(errs...)
}

func sortedNames(m map[string]Override) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Describe says what starts an event, such as "weekly on saturday at
// 20:00".
func (t Trigger) Describe() string {
	switch t.Kind {
	case Random:
		return fmt.Sprintf("a chance of %d in 10,000 each hour", t.Chance)
	case Condition:
		return fmt.Sprintf("when %s.%s has held for %d minutes", t.Module, t.Function, int(t.Lasting.Minutes()))
	}
	return t.Spec.String()
}
