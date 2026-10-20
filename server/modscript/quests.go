package modscript

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/quest"
)

// Quests (proposal 0002, review decision 45):
//   - player:add_quest(quest) is setquest, player:complete_quest(quest)
//     completequest, player:remove_quest(quest) erasequest and
//     player:change_quest(from, to) changequest. Each returns whether
//     the log changed.
//   - player:get_quest_state(quest) returns "none", "active", "inactive"
//     or "complete". player:get_quest_progress(quest, objective) returns
//     an objective's kills and the kills it asks for, as checkquest with
//     HUNTING reads them. player:quest_timed_out(quest) says whether the
//     quest's time limit passed, as checkquest with PLAYTIME.
//   - npc:show_quest_icon(player, icon) is showevent.
//   - A quest is a record of data/quests.yml: its ID, such as 7104, or
//     its name, which is the calling mod's record first, then core's.
//   - The events quest_add, quest_progress and quest_complete follow the
//     changes. They run in mod, NPC and event contexts of the player's
//     map.
//
// The reads need game.read. The changes need no permission.

// QuestHost is the part of a Host that keeps the players' quest logs. A
// host without it has no quests, and the calls raise an error.
type QuestHost interface {
	// AddQuest is quest_add: it returns false when the player has the
	// quest already.
	AddQuest(player int64, q *quest.Record) (bool, error)
	// CompleteQuest is quest_update_status with Q_COMPLETE.
	CompleteQuest(player int64, q *quest.Record) (bool, error)
	// RemoveQuest is quest_delete.
	RemoveQuest(player int64, q *quest.Record) (bool, error)
	// ChangeQuest is quest_change.
	ChangeQuest(player int64, from, to *quest.Record) (bool, error)
	// QuestState is the quest's state, a quest.State name.
	QuestState(player int64, q *quest.Record) (string, error)
	// QuestProgress is an objective's kills, from 0. ok is false when
	// the player lacks the quest.
	QuestProgress(player int64, q *quest.Record, objective int) (count int, ok bool, err error)
	// QuestTimedOut reports whether the quest's time limit passed. It is
	// false for a quest without one, and one the player lacks.
	QuestTimedOut(player int64, q *quest.Record) (bool, error)
	// ShowQuestIcon shows an icon over an NPC to a player, or removes it
	// for "". mark is the minimap mark, yellow, green or purple, or "".
	ShowQuestIcon(npc, player int64, icon, mark string) error
}

// shownByShowevent reports whether showevent shows an icon: rAthena's
// QTYPE_ values 0 to 6 and 8 (script.cpp 21195). It turns the others into
// QTYPE_NONE, which removes the icon.
func shownByShowevent(icon string) bool {
	v, ok := QuestIconNames[icon]
	return ok && v != 7 && v <= 8
}

func (r *Runtime) questHost(what string) (QuestHost, error) {
	h, ok := r.host.(QuestHost)
	if !ok {
		return nil, errors.New(what + ": quests do not exist here")
	}
	return h, nil
}

// questArg reads a quest argument: an ID or a record name.
func (r *Runtime) questArg(what string, v script.Value) (*quest.Record, error) {
	var rec *quest.Record
	switch {
	case v.Kind() == script.KindInt || v.Kind() == script.KindFloat:
		n, ok := v.AsInt()
		if !ok || n <= 0 || n > math.MaxUint32 {
			return nil, fmt.Errorf("%s: there is no quest %v in data/quests.yml", what, v)
		}
		rec = r.opts.Quests.ByID(uint32(n))
		if rec == nil {
			return nil, fmt.Errorf("%s: there is no quest %d in data/quests.yml", what, n)
		}
	default:
		s, ok := v.AsString()
		if !ok || s == "" || len(s) > 160 {
			return nil, errors.New(what + ": want a quest of data/quests.yml: its ID, such as 7104, or its name")
		}
		rec = r.opts.Quests.Resolve(s, r.actor())
		if rec == nil {
			return nil, fmt.Errorf("%s: there is no quest %q in data/quests.yml", what, s)
		}
	}
	return rec, nil
}

// playerQuest reads the player and the quest of a player:*_quest call,
// and checks game.read for a read.
func (r *Runtime) playerQuest(c *script.Call, what string, read bool) (QuestHost, int64, *quest.Record, error) {
	id, err := playerID(c)
	if err != nil {
		return nil, 0, nil, err
	}
	if read {
		if err := r.need(what, "game.read"); err != nil {
			return nil, 0, nil, err
		}
	} else if err := r.mutating(what); err != nil {
		return nil, 0, nil, err
	}
	h, err := r.questHost(what)
	if err != nil {
		return nil, 0, nil, err
	}
	q, err := r.questArg(what, c.Arg(1))
	if err != nil {
		return nil, 0, nil, err
	}
	return h, id, q, nil
}

// playerQuestCall is the player's quest methods: Arg(0) is the player,
// Arg(1) the quest, the last argument the method's name.
func (r *Runtime) playerQuestCall(c *script.Call) error {
	method := c.String(c.NArgs() - 1)
	what := "player:" + method
	switch method {
	case "add_quest", "complete_quest", "remove_quest":
		h, id, q, err := r.playerQuest(c, what, false)
		if err != nil {
			return err
		}
		var ok bool
		switch method {
		case "add_quest":
			ok, err = h.AddQuest(id, q)
		case "complete_quest":
			ok, err = h.CompleteQuest(id, q)
		default:
			ok, err = h.RemoveQuest(id, q)
		}
		if err != nil {
			return err
		}
		c.Return(script.Bool(ok))
	case "change_quest":
		h, id, from, err := r.playerQuest(c, what, false)
		if err != nil {
			return err
		}
		to, err := r.questArg(what, c.Arg(2))
		if err != nil {
			return err
		}
		ok, err := h.ChangeQuest(id, from, to)
		if err != nil {
			return err
		}
		c.Return(script.Bool(ok))
	case "get_quest_state":
		h, id, q, err := r.playerQuest(c, what, true)
		if err != nil {
			return err
		}
		s, err := h.QuestState(id, q)
		if err != nil {
			return err
		}
		c.Return(script.String(s))
	case "get_quest_progress":
		h, id, q, err := r.playerQuest(c, what, true)
		if err != nil {
			return err
		}
		n, ok := c.Arg(2).AsInt()
		if !ok || n < 1 {
			return errors.New(what + ": the objective is a number from 1")
		}
		if n > int64(len(q.Targets)) {
			return nil
		}
		count, has, err := h.QuestProgress(id, q, int(n-1))
		if err != nil || !has {
			return err
		}
		c.Return(script.Int(int64(count)), script.Int(int64(q.Targets[n-1].Count)))
	case "quest_timed_out":
		h, id, q, err := r.playerQuest(c, what, true)
		if err != nil {
			return err
		}
		v, err := h.QuestTimedOut(id, q)
		if err != nil {
			return err
		}
		c.Return(script.Bool(v))
	default:
		return fmt.Errorf("player has no method %q", method)
	}
	return nil
}

// npcShowQuestIcon is npc:show_quest_icon(player, icon).
func (r *Runtime) npcShowQuestIcon(c *script.Call) error {
	const what = "npc:show_quest_icon"
	id, err := npcID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	player, ok := c.Arg(1).AsInt()
	if !ok {
		return errors.New(what + ": want a player")
	}
	icon := ""
	if v := c.Arg(2); !v.IsNil() {
		s, ok := v.AsString()
		if _, known := QuestIconNames[s]; !ok || !known {
			return fmt.Errorf("%s: the icon is nil or one of wa.QuestIcon, such as quest", what)
		}
		icon = s
	}
	mark := ""
	if v := c.Arg(3); !v.IsNil() {
		s, ok := v.AsString()
		if !ok || !slices.Contains(QuestMarks, s) {
			return fmt.Errorf("%s: the mark is one of %q, or nil", what, QuestMarks)
		}
		mark = s
	}
	if icon != "" && !shownByShowevent(icon) {
		// showevent turns daily_quest, job_quest and jumping_poring into
		// QTYPE_NONE, which removes the icon. The mark still goes out.
		icon = ""
	}
	h, err := r.questHost(what)
	if err != nil {
		return err
	}
	return h.ShowQuestIcon(id, player, icon, mark)
}

// QuestEvent is a quest_add, quest_progress or quest_complete event.
type QuestEvent struct {
	// Kind is the event's name.
	Kind string
	// Player is the player's handle ID.
	Player int64
	Quest  *quest.Record
	// Objective, Count and Goal are set for quest_progress: the
	// objective from 1, its kills, and the kills it asks for.
	Objective, Count, Goal int
}

// QuestEventNames are the quest events.
var QuestEventNames = []string{"quest_add", "quest_progress", "quest_complete"}

// FireQuest runs the handlers of a quest event. The object is { player,
// quest, id }, and quest_progress adds objective, count and goal.
func (r *Runtime) FireQuest(e QuestEvent) {
	fields := []script.Field{
		script.KV("player", script.Int(e.Player)),
		script.KV("quest", script.String(e.Quest.Name)),
		script.KV("id", script.Int(int64(e.Quest.ID))),
	}
	if e.Kind == "quest_progress" {
		fields = append(fields, script.KV("objective", script.Int(int64(e.Objective))),
			script.KV("count", script.Int(int64(e.Count))), script.KV("goal", script.Int(int64(e.Goal))))
	}
	r.Fire(e.Kind, Arg{"quest_event", script.NewTable(fields...)})
}
