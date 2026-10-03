//go:build race

package e2e

// raceEnabled is set when the tests run under the race detector. The
// server is then built with -race too.
const raceEnabled = true
