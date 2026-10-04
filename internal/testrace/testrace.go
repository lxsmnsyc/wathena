// Package testrace tells tests whether the race detector is on.
//
// The race detector only finds races in code that runs goroutines, and
// it makes code 5 to 20 times slower. scripts/check.sh runs the tests
// twice: once without the race detector and once with it (decision
// 0022). A heavy test that runs on one goroutine, such as a parity or
// golden test, calls SkipUnderRace. It then runs in the first pass only.
package testrace

import "testing"

// SkipUnderRace skips the test when the race detector is on. why says
// why the test does not need it.
func SkipUnderRace(t testing.TB, why string) {
	t.Helper()
	if Enabled {
		t.Skip("skipped under the race detector, since " + why + "; go test without -race runs it")
	}
}
