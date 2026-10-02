package schedule

import (
	"testing"
	"time"
)

func load(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestNextDaily(t *testing.T) {
	manila := load(t, "Asia/Manila")
	s := Spec{Kind: Daily, Hour: 20, Minute: 0}
	after := time.Date(2026, 10, 31, 19, 59, 0, 0, manila)
	got := Next(s, after, manila)
	if want := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// Strictly after: at 20:00 the next one is tomorrow.
	got = Next(s, got, manila)
	if want := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNextWeeklyMonthlyHourly(t *testing.T) {
	utc := time.UTC
	after := time.Date(2026, 10, 2, 10, 15, 0, 0, utc) // a Friday
	cases := []struct {
		s    Spec
		want time.Time
	}{
		{Spec{Kind: Weekly, Weekday: time.Saturday, Hour: 20}, time.Date(2026, 10, 3, 20, 0, 0, 0, utc)},
		{Spec{Kind: Weekly, Weekday: time.Friday, Hour: 10, Minute: 15}, time.Date(2026, 10, 9, 10, 15, 0, 0, utc)},
		{Spec{Kind: Monthly, Day: 31, Hour: 0}, time.Date(2026, 10, 31, 0, 0, 0, 0, utc)},
		{Spec{Kind: Hourly, Minute: 30}, time.Date(2026, 10, 2, 10, 30, 0, 0, utc)},
		{Spec{Kind: Hourly, Minute: 0}, time.Date(2026, 10, 2, 11, 0, 0, 0, utc)},
	}
	for _, c := range cases {
		if got := Next(c.s, after, utc); !got.Equal(c.want) {
			t.Errorf("%v: got %v, want %v", c.s, got, c.want)
		}
	}
	// November has no 31st: the next is in December.
	got := Next(Spec{Kind: Monthly, Day: 31}, time.Date(2026, 10, 31, 0, 0, 0, 0, utc), utc)
	if want := time.Date(2026, 12, 31, 0, 0, 0, 0, utc); !got.Equal(want) {
		t.Errorf("monthly 31: got %v, want %v", got, want)
	}
}

func TestDaylightSaving(t *testing.T) {
	ny := load(t, "America/New_York")
	// 2026-03-08: clocks jump from 02:00 to 03:00. 02:30 runs at 03:00.
	got := Next(Spec{Kind: Daily, Hour: 2, Minute: 30}, time.Date(2026, 3, 8, 0, 0, 0, 0, ny), ny)
	if want := time.Date(2026, 3, 8, 3, 0, 0, 0, ny); !got.Equal(want) {
		t.Fatalf("gap: got %v, want %v", got.In(ny), want)
	}
	// 2026-11-01: clocks go back from 02:00 to 01:00. 01:30 runs once,
	// at its first instant.
	first := Next(Spec{Kind: Daily, Hour: 1, Minute: 30}, time.Date(2026, 11, 1, 0, 0, 0, 0, ny), ny)
	if _, off := first.In(ny).Zone(); off != -4*3600 {
		t.Fatalf("overlap: the first run is at %v, not the first instant", first.In(ny))
	}
	second := Next(Spec{Kind: Daily, Hour: 1, Minute: 30}, first, ny)
	if want := time.Date(2026, 11, 2, 1, 30, 0, 0, ny); !second.Equal(want) {
		t.Fatalf("overlap: after the first run comes %v, want %v", second.In(ny), want)
	}
	// Hourly through the repeated hour runs once at 01:30.
	n := 0
	at := time.Date(2026, 11, 1, 0, 45, 0, 0, ny)
	end := time.Date(2026, 11, 1, 3, 0, 0, 0, ny)
	for {
		at = Next(Spec{Kind: Hourly, Minute: 30}, at, ny)
		if !at.Before(end) {
			break
		}
		n++
	}
	if n != 2 { // 01:30 once, then 02:30
		t.Fatalf("hourly through the repeated hour ran %d times, want 2", n)
	}
}

func TestParse(t *testing.T) {
	if h, m, err := ParseClock("07:05"); err != nil || h != 7 || m != 5 {
		t.Fatal(h, m, err)
	}
	for _, bad := range []string{"7:05", "24:00", "12:60", "ab:cd", ""} {
		if _, _, err := ParseClock(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	if d, err := ParseWeekday("Saturday"); err != nil || d != time.Saturday {
		t.Fatal(d, err)
	}
	if _, err := ParseWeekday("caturday"); err == nil {
		t.Fatal("caturday parsed")
	}
}
