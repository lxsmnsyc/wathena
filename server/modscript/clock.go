package modscript

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// The clock and server settings of review decision 58, area B10 of
// proposal 0002b: wa.clock is rAthena's gettimetick, gettime and
// gettimestr, and rules.get reads the server settings that
// getbattleflag reads.

// ClockFields are clock.get's fields, gettime's DT_ types (script.cpp
// 10908, date.cpp 107), in the server's time zone.
var ClockFields = []string{"second", "minute", "hour", "day_of_week", "day_of_month", "month", "year", "day_of_year", "date"}

// MaxClockFormat bounds clock.format's format, in bytes.
const MaxClockFormat = 255

// SettingsHost is a host that has server settings for rules.get, which
// rAthena's getbattleflag reads (review decision 58).
type SettingsHost interface {
	// Setting reads a server setting by name. ok is false for a name it
	// does not have.
	Setting(name string) (int, bool)
}

// SettingNames are the server settings of rules.get: rAthena's battle
// flags of conf/battle/exp.conf and client.conf.
var SettingNames = []string{"base_exp_rate", "job_exp_rate", "quest_exp_rate", "max_hair_style", "max_hair_color", "max_cloth_color"}

// DefaultSettings are rAthena's values of SettingNames in conf/battle:
// exp.conf 13, 16 and 57, and client.conf 18, 20 and 22.
var DefaultSettings = map[string]int{"base_exp_rate": 100, "job_exp_rate": 100, "quest_exp_rate": 100,
	"max_hair_style": 42, "max_hair_color": 8, "max_cloth_color": 4}

// now is the server clock, or an error without one.
func (r *Runtime) clockNow(what string) (time.Time, error) {
	if r.opts.Clock == nil {
		return time.Time{}, errors.New(what + ": this zone has no server clock")
	}
	t := r.opts.Clock()
	if r.opts.Location != nil {
		t = t.In(r.opts.Location)
	} else {
		t = t.UTC()
	}
	return t, nil
}

// clockCall is wa.clock: Arg(0) is the function, then its arguments.
func (r *Runtime) clockCall(c *script.Call) error {
	op := c.String(0)
	what := "clock." + op
	t, err := r.clockNow(what)
	if err != nil {
		return err
	}
	switch op {
	case "now":
		c.Return(script.Int(t.Unix()))
	case "milliseconds":
		c.Return(script.Int(t.UnixMilli()))
	case "get":
		name, _ := c.Arg(1).AsString()
		v, ok := clockField(t, name)
		if !ok {
			return fmt.Errorf("%s: the field is one of %s", what, strings.Join(ClockFields, ", "))
		}
		c.Return(script.Int(int64(v)))
	case "format":
		f, ok := c.Arg(1).AsString()
		if !ok || len(f) > MaxClockFormat {
			return fmt.Errorf("%s: the format is a string of at most %d bytes", what, MaxClockFormat)
		}
		if v := c.Arg(2); !v.IsNil() {
			s, ok := v.AsInt()
			if !ok || s < 0 || s > 1<<40 {
				return errors.New(what + ": the time is Unix seconds of 0 or more")
			}
			t = time.Unix(s, 0).In(t.Location())
		}
		out, err := Strftime(f, t)
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		c.Return(script.String(out))
	default:
		return fmt.Errorf("clock has no function %q", op)
	}
	return nil
}

// clockField is one of gettime's values.
func clockField(t time.Time, name string) (int, bool) {
	switch name {
	case "second":
		return t.Second(), true
	case "minute":
		return t.Minute(), true
	case "hour":
		return t.Hour(), true
	case "day_of_week":
		// tm_wday: 0 is Sunday.
		return int(t.Weekday()), true
	case "day_of_month":
		return t.Day(), true
	case "month":
		return int(t.Month()), true
	case "year":
		return t.Year(), true
	case "day_of_year":
		// tm_yday: 0 is January 1st.
		return t.YearDay() - 1, true
	case "date":
		return t.Year()*10000 + int(t.Month())*100 + t.Day(), true
	}
	return 0, false
}

var (
	shortDays   = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	shortMonths = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
)

// Strftime is C's strftime in the C locale, as gettimestr calls it. It
// takes the conversions of C99, and refuses another one.
func Strftime(format string, t time.Time) (string, error) {
	var b strings.Builder
	pad := func(n, width int) { fmt.Fprintf(&b, "%0*d", width, n) }
	hour12 := func() int {
		h := t.Hour() % 12
		if h == 0 {
			h = 12
		}
		return h
	}
	for i := 0; i < len(format); i++ {
		ch := format[i]
		if ch != '%' {
			b.WriteByte(ch)
			continue
		}
		i++
		if i >= len(format) {
			return "", errors.New("the format ends with %")
		}
		// %E and %O modify a conversion in another locale; the C locale
		// ignores them.
		if (format[i] == 'E' || format[i] == 'O') && i+1 < len(format) {
			i++
		}
		switch format[i] {
		case 'a':
			b.WriteString(shortDays[t.Weekday()])
		case 'A':
			b.WriteString(t.Weekday().String())
		case 'b', 'h':
			b.WriteString(shortMonths[t.Month()-1])
		case 'B':
			b.WriteString(t.Month().String())
		case 'c':
			fmt.Fprintf(&b, "%s %s %2d %02d:%02d:%02d %d", shortDays[t.Weekday()], shortMonths[t.Month()-1], t.Day(),
				t.Hour(), t.Minute(), t.Second(), t.Year())
		case 'C':
			pad(t.Year()/100, 2)
		case 'd':
			pad(t.Day(), 2)
		case 'D':
			fmt.Fprintf(&b, "%02d/%02d/%02d", int(t.Month()), t.Day(), t.Year()%100)
		case 'e':
			fmt.Fprintf(&b, "%2d", t.Day())
		case 'F':
			fmt.Fprintf(&b, "%d-%02d-%02d", t.Year(), int(t.Month()), t.Day())
		case 'g', 'G':
			y, _ := t.ISOWeek()
			if format[i] == 'g' {
				pad(y%100, 2)
			} else {
				b.WriteString(strconv.Itoa(y))
			}
		case 'H':
			pad(t.Hour(), 2)
		case 'I':
			pad(hour12(), 2)
		case 'j':
			pad(t.YearDay(), 3)
		case 'm':
			pad(int(t.Month()), 2)
		case 'M':
			pad(t.Minute(), 2)
		case 'n':
			b.WriteByte('\n')
		case 'p':
			if t.Hour() < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case 'r':
			ap := "AM"
			if t.Hour() >= 12 {
				ap = "PM"
			}
			fmt.Fprintf(&b, "%02d:%02d:%02d %s", hour12(), t.Minute(), t.Second(), ap)
		case 'R':
			fmt.Fprintf(&b, "%02d:%02d", t.Hour(), t.Minute())
		case 'S':
			pad(t.Second(), 2)
		case 't':
			b.WriteByte('\t')
		case 'T', 'X':
			fmt.Fprintf(&b, "%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second())
		case 'u':
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			b.WriteString(strconv.Itoa(wd))
		case 'U':
			pad((t.YearDay()+6-int(t.Weekday()))/7, 2)
		case 'V':
			_, w := t.ISOWeek()
			pad(w, 2)
		case 'w':
			b.WriteString(strconv.Itoa(int(t.Weekday())))
		case 'W':
			pad((t.YearDay()+6-(int(t.Weekday())+6)%7)/7, 2)
		case 'x':
			fmt.Fprintf(&b, "%02d/%02d/%02d", int(t.Month()), t.Day(), t.Year()%100)
		case 'y':
			pad(t.Year()%100, 2)
		case 'Y':
			b.WriteString(strconv.Itoa(t.Year()))
		case 'z':
			b.WriteString(t.Format("-0700"))
		case 'Z':
			name, _ := t.Zone()
			b.WriteString(name)
		case '%':
			b.WriteByte('%')
		default:
			return "", fmt.Errorf("%%%c is not a conversion of strftime", format[i])
		}
	}
	return b.String(), nil
}
