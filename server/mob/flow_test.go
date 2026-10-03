package mob

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

// parseFlow reads a YAML flow mapping, as the monster overlay reads a
// skills entry.
func parseFlow(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	return m
}
