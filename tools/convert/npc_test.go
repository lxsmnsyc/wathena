package convert

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// goldenNPCs are NPCs whose converted output is kept in
// testdata/golden/npcs: a city Kafra and a Kafra with its own dialog, a
// dialog NPC, one with labels and goto, an inn that heals and saves, a
// shop, and two that do not convert yet: one with a clock label and one
// that spawns monsters. goldenFunctions are the functions they call.
var goldenNPCs = []string{"kaf_prontera", "Kafra Service", "Woman#payon", "Wayne", "Inn Employee#Ahlma", "Tool Dealer#prt1",
	"Slot Move Card Sales#slo", "Cursed Spirit#nif"}

var goldenFunctions = []string{"f_kafra", "f_kafset", "f_kaftele", "f_kafend", "f_kafinfo", "f_setkafcode", "f_entkafcode", "f_checkkafcode", "f_innmaid"}

func TestNPCGolden(t *testing.T) {
	res := loadCore(t, reference.Renewal).NPCs
	byName := map[string]*NPC{}
	for _, n := range res.NPCs {
		if byName[n.Source] == nil {
			byName[n.Source] = n
		}
	}
	check := func(name, got string) {
		t.Helper()
		path := filepath.Join("testdata", "golden", "npcs", strings.NewReplacer("#", "_", " ", "_").Replace(name)+".lua")
		if *update {
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
				t.Fatal(err)
			}
			return
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != got {
			t.Errorf("%s changed; run go test ./tools/convert -run TestNPCGolden -update and review the diff:\n%s", path, got)
		}
	}
	for _, name := range goldenNPCs {
		n := byName[name]
		if n == nil {
			t.Errorf("no NPC %s", name)
			continue
		}
		got := "-- status: " + string(n.Status) + "\n-- record: " + n.record + "\n"
		if len(n.Unsupported) > 0 {
			got += "-- unsupported: " + strings.Join(n.Unsupported, "; ") + "\n"
		}
		check(name, got+n.Lua)
	}
	for _, name := range goldenFunctions {
		f := res.Functions[name]
		if f == nil {
			t.Errorf("no function %s", name)
			continue
		}
		got := ""
		if len(f.Stubs) > 0 {
			got = "-- stubs: " + strings.Join(f.Stubs, "; ") + "\n"
		}
		check("function_"+name, got+f.Lua)
	}
}

// TestNPCReportsCurrent checks the NPC coverage reports in
// docs/generated.
func TestNPCReportsCurrent(t *testing.T) {
	for _, mode := range reference.Modes {
		path := "../../docs/generated/npc-scripts-" + string(mode) + ".md"
		got := loadCore(t, mode).NPCs.NPCReport()
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != got {
			t.Errorf("%s is not current: run go run ./tools/wactl convert npcs --mode %s --report %s", path, mode, strings.TrimPrefix(path, "../../"))
		}
	}
}

// jobParseFailures are the job change scripts the parser does not read
// yet, by file: set on getvariableofnpc, a condition as a statement, and
// statements before the first case of a switch.
var jobParseFailures = map[string]bool{
	"npc/jobs/1-1e/taekwon.txt": true, "npc/jobs/2-1/assassin.txt": true, "npc/jobs/2-1/priest.txt": true,
	"npc/jobs/2-2/monk.txt": true, "npc/jobs/2-2e/SoulLinker.txt": true, "npc/re/jobs/novice/academy.txt": true,
}

// TestParseNPCScripts parses every NPC script and function of both
// modes. Only the three scripts rAthena itself reads with warnings fail,
// and the job change scripts of jobParseFailures.
func TestParseNPCScripts(t *testing.T) {
	for _, mode := range reference.Modes {
		res := loadCore(t, mode).NPCs
		bad := 0
		for _, n := range res.NPCs {
			for _, u := range n.Unsupported {
				if strings.HasPrefix(u, "parse") && !jobParseFailures[n.File] {
					bad++
				}
			}
		}
		if bad > 3 {
			t.Errorf("%s: %d scripts do not parse", mode, bad)
		}
	}
}

// TestShopsConvert checks that every zeny shop becomes data, and other
// shop types are reported.
func TestShopsConvert(t *testing.T) {
	for _, mode := range reference.Modes {
		res := loadCore(t, mode).NPCs
		for _, n := range res.NPCs {
			switch n.Kind {
			case "shop":
				if n.Status != Full || n.record == "" || !strings.Contains(n.record, "shop: [") {
					t.Errorf("%s: shop %s did not convert: %v", mode, n.Name, n.Unsupported)
				}
			case "cashshop", "itemshop", "pointshop", "marketshop":
				if n.Status != None || len(n.Unsupported) == 0 {
					t.Errorf("%s: %s %s converted", mode, n.Kind, n.Name)
				}
			}
		}
	}
}
