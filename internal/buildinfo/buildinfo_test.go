package buildinfo

import (
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/shared/gen"
)

// TestVersion: a stamp wins, and the line names the build's modes.
func TestVersion(t *testing.T) {
	if got := Version("v0.1.0-3-gabc1234"); got != "v0.1.0-3-gabc1234" {
		t.Errorf("stamped version %q", got)
	}
	if Version("") == "" {
		t.Error("no version without a stamp")
	}
	line := Line("v0.1.0")
	if !strings.HasPrefix(line, "v0.1.0 (modes: classic") || strings.Contains(line, "procedural") != gen.Procedural {
		t.Errorf("version line %q", line)
	}
}
