package convert

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/quest"
	"github.com/lxsmnsyc/wathena/server/variables"
	"go.yaml.in/yaml/v3"
)

// questLog is a QuestHost for the NPC tests: one log for every player,
// at the test clock, with icons recorded.
type questLog struct {
	log   []character.Quest
	now   time.Time
	icons []string
	// said are the messages sent to players.
	said []string
}

// SendMessage records a message to a player.
func (h *npcHost) SendMessage(_ int64, text string) error {
	h.said = append(h.said, text)
	return nil
}

func (h *questLog) at() time.Time {
	if h.now.IsZero() {
		return testClock
	}
	return h.now
}

func (h *questLog) AddQuest(_ int64, r *quest.Record) (bool, error) {
	return quest.Add(&h.log, r, h.at()), nil
}

func (h *questLog) CompleteQuest(_ int64, r *quest.Record) (bool, error) {
	return quest.SetState(h.log, r.ID, character.QuestComplete), nil
}

func (h *questLog) RemoveQuest(_ int64, r *quest.Record) (bool, error) {
	return quest.Remove(&h.log, r.ID), nil
}

func (h *questLog) ChangeQuest(_ int64, from, to *quest.Record) (bool, error) {
	return quest.Change(&h.log, from.ID, to, h.at()), nil
}

func (h *questLog) QuestState(_ int64, r *quest.Record) (string, error) {
	if i := quest.Find(h.log, r.ID); i >= 0 {
		return quest.StateName(h.log[i].State), nil
	}
	return quest.StateNone, nil
}

func (h *questLog) QuestProgress(_ int64, r *quest.Record, j int) (int, bool, error) {
	i := quest.Find(h.log, r.ID)
	if i < 0 {
		return 0, false, nil
	}
	return h.log[i].Counts[j], true, nil
}

func (h *questLog) QuestTimedOut(_ int64, r *quest.Record) (bool, error) {
	i := quest.Find(h.log, r.ID)
	return i >= 0 && quest.TimedOut(&h.log[i], h.at()), nil
}

func (h *questLog) ShowQuestIcon(_, _ int64, icon, mark string) error {
	if mark != "" {
		icon += "/" + mark
	}
	h.icons = append(h.icons, icon)
	return nil
}

// coreQuests reads core's data/quests.yml of a mode into a table, with
// every name as the server's gamedata resolves it.
func coreQuests(t testing.TB, core *Core) *quest.Table {
	t.Helper()
	tab, err := questTable(core)
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

func questTable(core *Core) (*quest.Table, error) {
	var items struct {
		Items []struct {
			ID        uint32 `json:"id"`
			AegisName string `json:"aegis_name"`
			Stackable bool   `json:"stackable"`
		} `json:"items"`
	}
	if err := reference.LoadFile("extracts/"+core.Quests.Mode+"/items.json", &items); err != nil {
		return nil, err
	}
	type itemInfo struct {
		id    uint32
		stack bool
	}
	byName := map[string]itemInfo{}
	for _, it := range items.Items {
		byName[it.AegisName] = itemInfo{it.ID, it.Stackable}
	}
	modules := core.Constants.Modules()
	names := quest.Names{
		Monster: func(name string) (uint32, bool) {
			id, ok := core.Monsters[strings.ToUpper(name)]
			return uint32(id), ok // #nosec G115 -- monster IDs are small.
		},
		Item: func(name string) (uint32, bool, bool) {
			it, ok := byName[name]
			return it.id, it.stack, ok
		},
		Constant: func(module, name string) (int64, bool) {
			v, ok := modules[module].Values[name]
			return v, ok
		},
	}
	var list []map[string]any
	if err := yaml.Unmarshal([]byte(core.Quests.Files()[quest.File]), &list); err != nil {
		return nil, err
	}
	var recs []*quest.Record
	for _, m := range list {
		name, _ := m["name"].(string)
		delete(m, "name")
		id, err := strconv.ParseUint(name, 10, 32)
		if err != nil {
			return nil, err
		}
		r, err := quest.Build(name, mod.Core, uint32(id), m, names)
		if err != nil {
			return nil, &questError{name, err}
		}
		recs = append(recs, r)
	}
	return quest.NewTable(recs), nil
}

type questError struct {
	name string
	err  error
}

func (e *questError) Error() string { return "quest " + e.name + ": " + e.err.Error() }

// TestQuestsConvert converts both modes' quest_db and builds every record
// as the server does, and checks a few records' text.
func TestQuestsConvert(t *testing.T) {
	for _, mode := range []reference.Mode{reference.Renewal, reference.PreRenewal} {
		core := loadCore(t, mode)
		var d QuestData
		if err := reference.LoadFile("extracts/"+string(mode)+"/quests.json", &d); err != nil {
			t.Fatal(err)
		}
		tab := coreQuests(t, core)
		if len(tab.All()) != len(d.Quests) || len(core.Quests.Records) != len(d.Quests) {
			t.Errorf("%s: %d records, %d built, want %d", mode, len(core.Quests.Records), len(tab.All()), len(d.Quests))
		}
		for _, q := range d.Quests {
			if (q.TimeLimit != nil) != core.Quests.Timed[q.ID] {
				t.Errorf("%s: quest %d: timed %v", mode, q.ID, core.Quests.Timed[q.ID])
			}
			r := tab.ByID(q.ID)
			if r == nil || r.Title != q.Title || len(r.Targets) != len(q.Targets) || len(r.Drops) != len(q.Drops) {
				t.Errorf("%s: quest %d: %+v", mode, q.ID, r)
			}
		}
	}
	text := loadCore(t, reference.Renewal).Quests.Files()[quest.File]
	for _, want := range []string{
		"- name: \"1100\"\n  title: Solo in the Sphinx Dungeon!\n  targets:\n    - monster: ZEROM\n      count: 20\n",
		"- name: \"62238\"\n  title: Midgardian Mercenary timer\n  time_limit: 3h\n",
		"  time_limit:\n    at: \"04:00\"\n    days: 7\n",
		"      min_level: 140\n      race: demon\n",
		"      map: ice_d03_i\n      monsters: [ILL_GAZETI, ILL_SNOWIER, ILL_ICE_TITAN, ILL_ICEICLE]\n",
		"  drops:\n    - monster: TIYANAK\n      item: Lost_Belongings\n      rate: 3000\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("data/quests.yml lacks %q", want)
		}
	}
}

// questResult converts NPC lines of the test with the renewal quests.
func questResult(t *testing.T) *NPCResult {
	t.Helper()
	read := func(name string) []byte {
		b, err := reference.ReadFile("extracts/renewal/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	quests, err := ConvertQuests(bytes.NewReader(read("quests")))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"mode": "renewal", "npcs": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := ConvertNPCs(NPCInput{NPCs: bytes.NewReader(data), Constants: bytes.NewReader(read("script_constants")),
		ClientTables: bytes.NewReader(read("client_tables")), Quests: quests})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestQuestCommandsConvert checks the converted text of each quest
// command (review decision 45).
func TestQuestCommandsConvert(t *testing.T) {
	res := questResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ setquest 1100; end; }`, []string{"player:add_quest(1100)"}},
		{`{ setquest 1100,getcharid(0); end; }`, []string{"player:add_quest(1100)"}},
		{`{ setquest 999999; end; }`, []string{"-- setquest 999999: this mode's quest_db has no such quest, so rAthena only logs an error."}},
		{`{ completequest 1100; erasequest 1100; end; }`, []string{"player:complete_quest(1100)", "player:remove_quest(1100)"}},
		{`{ changequest 1100,1101; end; }`, []string{"player:change_quest(1100, 1101)"}},
		{`{ changequest 1100,999999; end; }`, []string{"-- changequest 1100, 999999: this mode's quest_db lacks a quest"}},
		{`{ .@q = 1100; setquest .@q; end; }`, []string{`R.quest(player, "add_quest", q)`}},
		{`{ .@q = 1100; erasequest .@q; changequest .@q, 1101; end; }`, []string{`R.quest(player, "remove_quest", q)`, `R.quest(player, "change_quest", q, 1101)`}},
		{`{ if (checkquest(1100) == -1) end; end; }`, []string{"R.checkquest(player, 1100, 0) == -1"}},
		{`{ if (checkquest(1100,HAVEQUEST) == 2) end; end; }`, []string{"R.checkquest(player, 1100, 0) == 2"}},
		{`{ if (checkquest(62238,PLAYTIME) == 2) end; end; }`, []string{"R.checkquest(player, 62238, 1, true) == 2"}},
		{`{ if (checkquest(1100,HUNTING) == 2) end; end; }`, []string{"R.checkquest(player, 1100, 2, false) == 2"}},
		{`{ .@q = 1100; if (checkquest(.@q,HUNTING) == 2) end; end; }`, []string{"R.checkquest(player, q, 2, nil) == 2"}},
		{`{ .@r = checkquest(999999); end; }`, []string{"r = -1"}},
		{`{ if (isbegin_quest(1100) == 1) end; end; }`, []string{"R.isbegin_quest(player, 1100) == 1"}},
		{`{ .@r = isbegin_quest(999999); end; }`, []string{"r = 0"}},
		{`{ showevent QTYPE_QUEST; end; }`, []string{`npc:show_quest_icon(player, "quest")`}},
		{`{ showevent QTYPE_QUEST2,QMARK_NONE; end; }`, []string{`npc:show_quest_icon(player, "quest_question")`}},
		{`{ showevent QTYPE_WARG2; end; }`, []string{`npc:show_quest_icon(player, "warg_face")`}},
		{`{ showevent QTYPE_QUEST,QMARK_YELLOW; end; }`, []string{`npc:show_quest_icon(player, "quest", { mark = "yellow" })`}},
		{`{ showevent QTYPE_DAILYQUEST; end; }`, []string{`npc:show_quest_icon(player, nil)`}},
		{`{ showevent QTYPE_NONE; end; }`, []string{`npc:show_quest_icon(player, nil)`}},
		{`{ showevent 7; end; }`, []string{`npc:show_quest_icon(player, nil)`}},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		lua, err := n.npcScript()
		if err != nil {
			t.Errorf("%s: %v", c.script, err)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(lua, w) {
				t.Errorf("%s: got\n%s\nwant %q", c.script, lua, w)
			}
		}
	}
	for _, c := range []struct{ script, want string }{
		{`{ setquest 1100,150001; end; }`, "setquest: for another character"},
		{`{ if (checkquest(1100,PLAYTIME,150001)) end; end; }`, "checkquest: for another character"},
		{`{ .@k = 1; if (checkquest(1100,.@k)) end; end; }`, "checkquest: a check other than HAVEQUEST, PLAYTIME and HUNTING"},
		{`{ showevent QTYPE_QUEST,QMARK_YELLOW,.@i; end; }`, "showevent: for another character"},
		{`{ showevent QTYPE_QUEST,.@i; end; }`, "showevent: a computed mark color"},
		{`{ showevent .@i; end; }`, "showevent: a computed icon"},
		{`{ setquest "x"; end; }`, "setquest: a quest named by text"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}

// TestRunQuestCommands runs converted quest commands against a quest log
// and checks rAthena's numbers, with the PLAYTIME and HUNTING values of a
// quest without a time limit.
func TestRunQuestCommands(t *testing.T) {
	res := questResult(t)
	// 1100 hunts 20 Zeroms without a time limit. 62238 has 3 hours.
	n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "Run#t", Map: "prontera", Script: `{
	dispbottom "a " + checkquest(1100) + " " + isbegin_quest(1100);
	setquest 1100;
	setquest 62238;
	dispbottom "b " + checkquest(1100) + " " + checkquest(1100,PLAYTIME) + " " + checkquest(1100,HUNTING) + " " + isbegin_quest(1100);
	dispbottom "c " + checkquest(62238,PLAYTIME) + " " + checkquest(62238,HUNTING);
	completequest 62238;
	dispbottom "d " + checkquest(62238) + " " + checkquest(62238,PLAYTIME) + " " + checkquest(62238,HUNTING) + " " + isbegin_quest(62238);
	changequest 1100,1101;
	dispbottom "e " + checkquest(1100) + " " + checkquest(1101);
	erasequest 1101;
	dispbottom "f " + checkquest(1101);
	showevent QTYPE_QUEST;
	// A computed ID that quest_db lacks only logs an error in rAthena.
	.@q = 999999;
	setquest .@q;
	completequest .@q;
	changequest .@q, 1100;
	erasequest .@q;
	end;
}`})
	lua, err := n.npcScript()
	if err != nil {
		t.Fatal(err)
	}
	_, base, core := testRuntime(t, reference.Renewal)
	h := &npcHost{testHost: base, npcs: map[int64]*NPC{1: {Source: "Run#t", Map: "prontera"}}, steps: map[int64]modscript.Step{},
		ended: map[int64]bool{}, vars: map[modscript.VariableKey]string{}}
	sc := &mod.Scripts{Mod: mod.Core, Files: map[string]string{"scripts/npcs/Run_t.lua": lua, RathenaLib: rathenaLua}, Grants: mod.Grants{}, Visible: []string{mod.Core}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	logs := &logRecorder{}
	rt, err := modscript.New(modscript.Options{Host: h, Mods: []string{mod.Core}, Constants: core.Constants, Jobs: base.jobs,
		Log: slog.New(logs), Server: variables.NewServer(nil, nil), Clock: func() time.Time { return testClock }, Location: time.UTC,
		Quests: coreQuests(t, core)}, []*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	npc, err := rt.StartNPC(1, "core:scripts/npcs/Run_t.lua")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Click(npc, 1); err != nil {
		t.Fatal(err)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
	got := strings.Join(h.said, "|")
	// rAthena: a quest without a limit is timed out for PLAYTIME (2), and
	// HUNTING is 1 while its kills are missing. A quest without
	// objectives has all its kills, so HUNTING is 2.
	want := "a -1 0|b 1 2 1 1|c 0 2|d 2 1 0 2|e -1 1|f -1"
	if got != want {
		t.Errorf("said %q, want %q", got, want)
	}
	if len(h.icons) != 1 || h.icons[0] != "quest" {
		t.Errorf("icons %v", h.icons)
	}
}

// TestQuestReportsCurrent checks docs/generated/quests-<mode>.md.
func TestQuestReportsCurrent(t *testing.T) {
	for _, mode := range reference.Modes {
		path := "../../docs/generated/quests-" + string(mode) + ".md"
		got := loadCore(t, mode).Quests.Report()
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
			t.Errorf("%s is not current: run go run ./tools/wactl convert quests --mode %s --report %s", path, mode, strings.TrimPrefix(path, "../../"))
		}
	}
}
