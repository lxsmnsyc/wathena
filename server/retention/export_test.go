package retention

import "time"

// SetWallClock replaces the process clock, and returns what puts it back.
func SetWallClock(f func() time.Time) (restore func()) {
	old := wallClock
	wallClock = f
	return func() { wallClock = old }
}
