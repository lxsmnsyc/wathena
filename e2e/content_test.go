package e2e

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// contentConfig is a small world layout for TestProceduralContent: 2 × 2
// fields of 64 cells, two towns and a dungeon.
const contentConfig = `procedural:
  field_size: 64
  layout: {grid: [2, 2], towns: 2, dungeons: 1}
`

// worldQuest is a row of world_quests.
type worldQuest struct {
	id                   uint32
	town, purpose, guild string
	stage, kind, title   string
	step                 int
	monster, item        string
	count                int
	zeny                 int64
}

// worldQuests reads the world's quests.
func worldQuests(t *testing.T, srv *Server) []worldQuest {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rows, err := srv.DB.Query(ctx, `SELECT id, town, purpose, guild, stage, step, kind, title, monster, item, count, zeny
		FROM world_quests ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []worldQuest
	for rows.Next() {
		var q worldQuest
		if err := rows.Scan(&q.id, &q.town, &q.purpose, &q.guild, &q.stage, &q.step, &q.kind, &q.title, &q.monster, &q.item, &q.count, &q.zeny); err != nil {
			t.Fatal(err)
		}
		out = append(out, q)
	}
	return out
}

// doQuest meets a hunt's or a collect quest's objective: the fixture
// spawns its monsters next to the player, who kills them, or gives its
// items. label is the hunt objective's label in the quest log.
func doQuest(p *Player, q worldQuest, label string) {
	p.t.Helper()
	switch q.kind {
	case "hunt":
		for range q.count {
			p.MustCommand("!spawn " + q.monster)
			p.Kill(p.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, label))
		}
	case "collect":
		p.MustCommand(fmt.Sprintf("!give %s %d", q.item, q.count))
	}
}

// TestProceduralContent: a new character in a procedural world takes a
// quest of its first town's quest board, completes it for its reward,
// and passes the Swordman guild's test to become a Swordman (decision
// 0044). Between the two, the test raises the character to job level 10
// with Basic Skill level 9 in the database, as a Novice reaches them by
// play, with a base level and stats to win the test's fights. The
// fixture spawns the monsters of the hunts next to the player and gives
// the items of the collect quests. Needs PostgreSQL.
func TestProceduralContent(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true, Procedural: true, Config: contentConfig, StartJob: "0:1:1"})
	quests := worldQuests(t, srv)
	// A collect quest of the first town's board when it has one, as a
	// fresh Novice gathers its items without a fight, else a hunt.
	var board *worldQuest
	tests := map[string]worldQuest{}
	for _, kind := range []string{"collect", "hunt"} {
		for i, q := range quests {
			if q.purpose == "board" && q.town == "gen:town:t1" && board == nil && q.kind == kind {
				board = &quests[i]
			}
		}
	}
	for _, q := range quests {
		if q.guild == "swordman" {
			tests[fmt.Sprintf("%s%d", q.stage, q.step)] = q
		}
	}
	if board == nil || len(tests) != 4 {
		t.Fatalf("the world's quests %+v", quests)
	}
	first1, first2 := tests["first1"], tests["first2"]
	t1 := readMap(t, srv, "gen:town:t1")
	home := readMap(t, srv, "gen:home:village")

	p := srv.Join(t, "Novice")
	px, py := home.place(t, "portal")
	p.Until("on the home map", func(v View, _ Game) bool { return v.Map == "gen:home:village" })
	route(t, srv, p, px, py, 0, true)
	p.MapChange("gen:town:t1")

	// The quest board: take the quest, meet its objective, turn it in.
	bx, by := t1.place(t, "quest_board")
	route(t, srv, p, bx, by, 2, false)
	c := p.Talk("Quest Board")
	c.Says("Quests of " + t1.DisplayName)
	c.Next()
	mark := p.Mark()
	c.Choose(board.title)
	c.Next()
	c.Choose("Take it")
	c.Says("You took the quest")
	c.Close()
	info := p.questUpdate(mark, "the board's quest", func(q *wathenav1.QuestInfo) bool { return q.GetId() == board.id })
	label := ""
	if o := info.GetObjectives(); len(o) > 0 {
		label = o[0].GetLabel()
	}
	// A fresh Novice fights a hunt's monsters with its fists, which may
	// take longer than a step's usual wait, such as for a Moss Poring.
	p.Timeout = 2 * time.Minute
	doQuest(p, *board, label)
	p.Timeout = DefaultTimeout
	before := p.Stat(wathenav1.StatId_STAT_ID_ZENY)
	c = p.Talk("Quest Board")
	c.Next()
	c.Choose(board.title + " (taken)")
	c.Says("Quest complete")
	c.Close()
	p.Until("paid the reward", func(_ View, g Game) bool { return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == before+board.zeny })

	// Job level 10 and Basic Skill level 9, saved while offline, with the
	// levels and stats to win the test's fights.
	_ = p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var id int64
	for range 100 {
		if err := srv.DB.QueryRow(ctx, `SELECT id FROM characters WHERE name = 'Novice' AND map = 'gen:town:t1'`).Scan(&id); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if id == 0 {
		t.Fatal("the character was not saved in the town")
	}
	if _, err := srv.DB.Exec(ctx, `UPDATE characters SET job_level = 10, skill_points = 0, base_level = 50,
		str = 80, agi = 60, vit = 60, dex = 60 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(ctx, `INSERT INTO character_skills (character_id, skill_id, level) VALUES ($1, 1, 9)
		ON CONFLICT (character_id, skill_id) DO UPDATE SET level = 9`, id); err != nil {
		t.Fatal(err)
	}

	// The Swordman guild's test: a hunt, a collect quest, and a talk.
	p = srv.Join(t, "Novice")
	p.Until("back in the town", func(v View, _ Game) bool { return v.Map == "gen:town:t1" })
	gx, gy := t1.place(t, "guild:swordman")
	route(t, srv, p, gx, gy, 2, false)
	master := "Swordman Guild Master"
	c = p.Talk(master)
	c.Says("part 1 of 2")
	c.Next()
	mark = p.Mark()
	c.Choose("Begin")
	c.Close()
	info = p.questUpdate(mark, "the hunt of the test", func(q *wathenav1.QuestInfo) bool { return q.GetId() == first1.id })
	label = ""
	if o := info.GetObjectives(); len(o) > 0 {
		label = o[0].GetLabel()
	}
	doQuest(p, first1, label)
	c = p.Talk(master)
	c.Says("Well done")
	c.Next()
	c.Says("part 2 of 2")
	c.Next()
	c.Choose("Begin")
	c.Close()
	doQuest(p, first2, "")
	c = p.Talk(master)
	c.Says("Well done")
	c.Next()
	c.Says("You passed the Swordman test")
	c.Next()
	c.Choose("Become a Swordman")
	c.Says("You are now a Swordman")
	c.Close()
	_ = p.Close()

	var job, jobLevel int
	var done []int64
	for range 100 {
		done = done[:0]
		_ = srv.DB.QueryRow(ctx, `SELECT job, job_level FROM characters WHERE id = $1`, id).Scan(&job, &jobLevel)
		rows, err := srv.DB.Query(ctx, `SELECT quest_id FROM character_quests WHERE character_id = $1 AND state = 2 ORDER BY quest_id`, id)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var q int64
			if err := rows.Scan(&q); err != nil {
				t.Fatal(err)
			}
			done = append(done, q)
		}
		rows.Close()
		if job == 1 && len(done) == 3 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if job != 1 || jobLevel != 1 || !slices.Equal(done, []int64{int64(board.id), int64(first1.id), int64(first2.id)}) {
		t.Errorf("saved: job %d at job level %d, complete quests %v", job, jobLevel, done)
	}
	srv.CheckLogs()
}
