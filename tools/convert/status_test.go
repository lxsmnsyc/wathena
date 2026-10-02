package convert

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/bonus"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

func loadStatusDB(t *testing.T, mode reference.Mode) *sc.DB {
	t.Helper()
	var d sc.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/status.json", &d); err != nil {
		t.Fatal(err)
	}
	db, err := sc.Load(&d)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// TestStatusScripts converts the scripts of status.yml in both modes,
// and keeps the templates of every status in testdata/golden.
func TestStatusScripts(t *testing.T) {
	for _, mode := range reference.Modes {
		core := loadCore(t, mode)
		db := loadStatusDB(t, mode)
		var out strings.Builder
		converted, total := 0, 0
		for _, s := range core.Result.StatusBonuses(db) {
			total++
			if s.Err != nil {
				fmt.Fprintf(&out, "%s: not converted: %v\n", s.Status, s.Err)
				continue
			}
			converted++
			for _, b := range s.Bonuses {
				var args []string
				for _, a := range b.Args {
					args = append(args, a.String())
				}
				fmt.Fprintf(&out, "%s: %s(%s)", s.Status, b.Name, strings.Join(args, ", "))
				if opts := describeOptions(&b.Options); opts != "" {
					fmt.Fprintf(&out, " %s", opts)
				}
				if b.Unless >= 0 {
					fmt.Fprintf(&out, " unless %d", b.Unless)
				}
				out.WriteString("\n")
			}
		}
		t.Logf("%s: %d of %d status scripts convert", mode, converted, total)
		path := "testdata/golden/status_scripts_" + string(mode) + ".txt"
		if *update {
			if err := os.WriteFile(path, []byte(out.String()), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil || string(want) != out.String() {
			t.Errorf("%s is not current; run go test ./tools/convert -run TestStatusScripts -update", path)
		}
	}
}

// describeOptions writes the options a template sets.
func describeOptions(o *bonus.Options) string {
	var parts []string
	for _, k := range slices.Sorted(maps.Keys(o.Set)) {
		var v any
		switch k {
		case "percent":
			v = o.Percent
		case "kind":
			v = o.Kind
		case "range":
			v = o.Range
		case "attack":
			v = o.Attack
		case "stacking":
			v = o.Stacking
		case "element":
			v = o.Element
		case "skill":
			v = o.Skill
		case "chance":
			v = o.Chance.Value
		case "on":
			v = o.On
		case "against":
			v = *o.Against
		case "from":
			v = *o.From
		default:
			v = "set"
		}
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, " ")
}
