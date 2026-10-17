package e2e

import (
	"strings"
	"testing"
)

// TestStartFailsAtOnce: a server with a database whose start fails after
// its server variables began to run stops with the error. Here a world
// event of the fixture names e2e_field, which does not run. Before
// decision 0045 such a start waited for a signal, since runAll's wait
// for the variables came before the context ended.
func TestStartFailsAtOnce(t *testing.T) {
	t.Parallel()
	log := StartFailing(t, Options{Database: true, Maps: StartMap})
	if !strings.Contains(log, "the map e2e_field does not run in this world") {
		t.Errorf("the log does not say why:\n%s", log[max(len(log)-2000, 0):])
	}
}
