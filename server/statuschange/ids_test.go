package statuschange_test

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// TestIDs checks the status IDs of ids.go against rAthena's
// script_constants.json in both modes.
func TestIDs(t *testing.T) {
	src, err := os.ReadFile("ids.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^\s+SC\w+\s+= (\d+)\s+// (SC_\w+)$`)
	found := re.FindAllStringSubmatch(string(src), -1)
	if len(found) < 100 {
		t.Fatalf("read %d IDs from ids.go", len(found))
	}
	for _, mode := range reference.Modes {
		var d struct {
			Constants map[string]int64 `json:"constants"`
		}
		if err := reference.LoadFile("extracts/"+string(mode)+"/script_constants.json", &d); err != nil {
			t.Fatal(err)
		}
		for _, m := range found {
			v, _ := strconv.ParseInt(m[1], 10, 64)
			want, ok := d.Constants[m[2]]
			if !ok {
				// A status of one mode only keeps its ID in the other.
				continue
			}
			if v != want {
				t.Errorf("%s: %s is %d in ids.go and %d in rAthena", mode, m[2], v, want)
			}
		}
	}
}
