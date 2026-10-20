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
	"Slot Move Card Sales#slo", "Cursed Spirit#nif", "Dancer Kim#fild"}

var goldenFunctions = []string{"f_kafra", "f_kafcart", "f_kafset", "f_kaftele", "f_kafend", "f_kafinfo", "f_kafstor", "f_canopenstorage", "f_setkafcode", "f_entkafcode", "f_checkkafcode", "f_innmaid"}

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

// TestClassicNotesCurrent checks the release notes list of classic mode
// in docs/generated (review decision 55).
func TestClassicNotesCurrent(t *testing.T) {
	path := "../../" + ClassicNotesPath
	got := loadCore(t, reference.Renewal).NPCs.ClassicNotes()
	if *update {
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
		t.Errorf("%s is not current: run go run ./tools/wactl convert classic --report %s", ClassicNotesPath, ClassicNotesPath)
	}
}

// TestParseNPCScripts parses every NPC script and function of both
// modes. Each parses, as rAthena reads each: set on a reference that is
// not a variable, and a comma, a colon or a ) that ends an assignment
// (TestParseRAthenaForms).
func TestParseNPCScripts(t *testing.T) {
	for _, mode := range reference.Modes {
		res := loadCore(t, mode).NPCs
		for _, n := range res.NPCs {
			for _, u := range n.Unsupported {
				if strings.HasPrefix(u, "parse") {
					t.Errorf("%s: %s: %s", mode, n.Source, u)
				}
			}
		}
		for _, f := range res.Functions {
			if strings.Contains(f.Err, "does not parse") {
				t.Errorf("%s: function %s: %s", mode, f.Name, f.Err)
			}
		}
	}
}

// TestShopsConvert checks that every shop line and barter shop becomes
// data: zeny shops in the list form, and the other kinds in the table
// form, which players cannot sell to. Only a barter whose items all fail
// may be left out.
func TestShopsConvert(t *testing.T) {
	for _, mode := range reference.Modes {
		res := loadCore(t, mode).NPCs
		barters := 0
		for _, n := range res.NPCs {
			switch n.Kind {
			case "shop":
				if n.Status != Full || n.record == "" || !strings.Contains(n.record, "shop: [") {
					t.Errorf("%s: shop %s did not convert: %v", mode, n.Name, n.Unsupported)
				}
			case "cashshop", "itemshop", "pointshop", "marketshop", "barter":
				// A barter whose items all take refined items does not
				// convert, with a reason for each item.
				if n.Kind == "barter" && n.Status == None && len(n.Unsupported) > 1 {
					barters++
					continue
				}
				if n.Status == None || !strings.Contains(n.record, "shop: {buys_from_players: false") {
					t.Errorf("%s: %s %s did not convert: %v", mode, n.Kind, n.Name, n.Unsupported)
				}
				if n.Kind == "barter" {
					barters++
				}
			}
		}
		if mode == reference.Renewal && barters == 0 {
			t.Errorf("%s: no barter shops", mode)
		}
	}
}

// TestJobChangeUpper checks rAthena's Job_Change, jobchange <job>,Upper:
// player:change_job's keep_line changes into the job of the player's
// line (review decision 58), so the function converts whole.
func TestJobChangeUpper(t *testing.T) {
	f := loadCore(t, reference.Renewal).NPCs.Functions["job_change"]
	if f == nil {
		t.Fatal("no Job_Change")
	}
	want := "  player:change_job((R.arg(args, 0) or 0), { force = true, keep_line = true })\n"
	if !strings.Contains(f.Lua, want) {
		t.Errorf("Job_Change:\n%s", f.Lua)
	}
	if len(f.Stubs) != 0 {
		t.Errorf("stubs %q", f.Stubs)
	}
}
