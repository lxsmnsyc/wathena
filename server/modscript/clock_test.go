package modscript

import (
	"testing"
	"time"
)

// TestStrftime checks clock.format's conversions against C's strftime
// in the C locale, as gettimestr calls it.
func TestStrftime(t *testing.T) {
	at := time.Date(2026, 1, 4, 13, 5, 9, 0, time.UTC)
	for format, want := range map[string]string{
		"%Y-%m-%d %H:%M:%S": "2026-01-04 13:05:09",
		"%a %A %b %B %e":    "Sun Sunday Jan January  4",
		"%I:%M %p %j %%":    "01:05 PM 004 %",
		"%D %T %F %R":       "01/04/26 13:05:09 2026-01-04 13:05",
		"%U %W %w %u %V":    "01 00 0 7 01",
		"%c":                "Sun Jan  4 13:05:09 2026",
		"%y %C %Z":          "26 20 UTC",
	} {
		got, err := Strftime(format, at)
		if err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", format, got, err, want)
		}
	}
	for _, bad := range []string{"%Q", "%"} {
		if _, err := Strftime(bad, at); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

// TestClockFields checks clock.get's fields: gettime's DT_ values.
func TestClockFields(t *testing.T) {
	at := time.Date(2026, 12, 31, 23, 59, 58, 0, time.UTC)
	for field, want := range map[string]int{"second": 58, "minute": 59, "hour": 23, "day_of_week": 4, "day_of_month": 31,
		"month": 12, "year": 2026, "day_of_year": 364, "date": 20261231} {
		if got, ok := clockField(at, field); !ok || got != want {
			t.Errorf("%s: %d %v, want %d", field, got, ok, want)
		}
	}
	if _, ok := clockField(at, "week"); ok {
		t.Error("week is not a field")
	}
}
