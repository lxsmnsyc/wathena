// Package schedule finds when clock times come, in the server's time
// zone (strategy section 8.8). wa.schedules and the world event
// scheduler use it.
//
// Rules:
//   - Times are wall clock times of a time zone, such as 20:00 in
//     Asia/Manila. The results are instants, which the server keeps in
//     UTC.
//   - A time that does not exist, because clocks jump forward that day,
//     runs at the next valid minute: 02:30 runs at 03:00.
//   - A time that happens twice, because clocks go back that day, runs
//     once, at its first instant.
//   - A monthly time on a day that a month does not have, such as the
//     31st, does not run that month.
package schedule

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Kind is how often a Spec repeats.
type Kind int

// Kinds of Spec.
const (
	// Daily runs every day at Hour:Minute.
	Daily Kind = iota
	// Weekly runs every Weekday at Hour:Minute.
	Weekly
	// Monthly runs on Day of every month at Hour:Minute.
	Monthly
	// Hourly runs every hour at Minute.
	Hourly
)

// Spec is a repeating clock time.
type Spec struct {
	Kind    Kind
	Weekday time.Weekday
	// Day is the day of the month, 1 to 31, for Monthly.
	Day          int
	Hour, Minute int
}

// String describes the spec, such as "weekly on saturday at 20:00".
func (s Spec) String() string {
	at := fmt.Sprintf("%02d:%02d", s.Hour, s.Minute)
	switch s.Kind {
	case Weekly:
		return "weekly on " + strings.ToLower(s.Weekday.String()) + " at " + at
	case Monthly:
		return fmt.Sprintf("monthly on day %d at %s", s.Day, at)
	case Hourly:
		return fmt.Sprintf("hourly at minute %d", s.Minute)
	}
	return "daily at " + at
}

// ParseClock reads a time of day, "HH:MM", from 00:00 to 23:59.
func ParseClock(s string) (hour, minute int, err error) {
	h, m, ok := strings.Cut(s, ":")
	if ok && len(h) == 2 && len(m) == 2 {
		hour, err1 := strconv.Atoi(h)
		minute, err2 := strconv.Atoi(m)
		if err1 == nil && err2 == nil && hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59 {
			return hour, minute, nil
		}
	}
	return 0, 0, fmt.Errorf("%q is not a time of day: want HH:MM, from 00:00 to 23:59", s)
}

// Weekdays are the day names, from sunday, as time.Weekday counts.
var Weekdays = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

// ParseWeekday reads a day name, such as saturday.
func ParseWeekday(s string) (time.Weekday, error) {
	for i, d := range Weekdays {
		if strings.EqualFold(s, d) {
			return time.Weekday(i), nil
		}
	}
	return 0, fmt.Errorf("%q is not a day: want one of %s", s, strings.Join(Weekdays, ", "))
}

// Check checks a spec's values.
func (s Spec) Check() error {
	switch {
	case s.Kind < Daily || s.Kind > Hourly:
		return errors.New("unknown schedule kind")
	case s.Hour < 0 || s.Hour > 23 || s.Minute < 0 || s.Minute > 59:
		return errors.New("the time must be from 00:00 to 23:59")
	case s.Kind == Weekly && (s.Weekday < time.Sunday || s.Weekday > time.Saturday):
		return errors.New("the day must be sunday to saturday")
	case s.Kind == Monthly && (s.Day < 1 || s.Day > 31):
		return errors.New("the day of the month must be 1 to 31")
	}
	return nil
}

// Next returns the first time the spec comes strictly after after, in
// the time zone loc. The result is in UTC.
func Next(s Spec, after time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	local := after.In(loc)
	// Start a day early: a time on the day before can resolve to an
	// instant after a jump.
	y, mo, d := local.AddDate(0, 0, -1).Date()
	// A monthly spec on the 31st can skip months. 15 months covers
	// every case.
	days := 3
	switch s.Kind {
	case Weekly:
		days = 9
	case Monthly:
		days = 460
	}
	for i := range days {
		date := time.Date(y, mo, d+i, 12, 0, 0, 0, time.UTC)
		dy, dm, dd := date.Date()
		switch s.Kind {
		case Weekly:
			if date.Weekday() != s.Weekday {
				continue
			}
		case Monthly:
			if dd != s.Day {
				continue
			}
		}
		hours := []int{s.Hour}
		if s.Kind == Hourly {
			hours = make([]int, 24)
			for h := range hours {
				hours[h] = h
			}
		}
		for _, h := range hours {
			if t := Resolve(dy, dm, dd, h, s.Minute, loc); t.After(after) {
				return t.UTC()
			}
		}
	}
	// Not reached for a spec that Check accepts.
	return time.Time{}
}

// Resolve returns the instant of a wall clock time in a time zone. A
// time that happens twice gives its first instant. A time that does not
// exist gives the next valid minute.
func Resolve(y int, mo time.Month, d, hour, minute int, loc *time.Location) time.Time {
	if t, ok := exact(y, mo, d, hour, minute, loc); ok {
		return t
	}
	for k := 1; k <= 24*60; k++ {
		w := time.Date(y, mo, d, hour, minute+k, 0, 0, time.UTC)
		if t, ok := exact(w.Year(), w.Month(), w.Day(), w.Hour(), w.Minute(), loc); ok {
			return t
		}
	}
	return time.Date(y, mo, d, hour, minute, 0, 0, loc)
}

// exact finds the first instant whose wall clock in loc reads the
// given minute.
func exact(y int, mo time.Month, d, hour, minute int, loc *time.Location) (time.Time, bool) {
	base := time.Date(y, mo, d, hour, minute, 0, 0, loc)
	var best time.Time
	// Offsets change by at most a few hours, in steps of 15 minutes.
	for off := -4 * 60; off <= 4*60; off += 15 {
		u := base.Add(time.Duration(off) * time.Minute)
		w := u.In(loc)
		wy, wm, wd := w.Date()
		if wy == y && wm == mo && wd == d && w.Hour() == hour && w.Minute() == minute && w.Second() == 0 {
			if best.IsZero() || u.Before(best) {
				best = u
			}
		}
	}
	return best, !best.IsZero()
}

// MinuteStart returns the start of the minute that t is in.
func MinuteStart(t time.Time) time.Time { return t.Truncate(time.Minute) }
