package quest

import (
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
)

// The quest log rules, as rAthena's quest_add, quest_change,
// quest_delete, quest_update_status, quest_check and
// quest_update_objective. A log holds each quest once, in the order the
// character took them. An entry whose record is gone, such as a removed
// mod's quest, stays in the log and is saved, but no rule reads it, so
// it comes back with the mod.

// States, as the Lua API names them.
const (
	StateNone     = "none"
	StateInactive = "inactive"
	StateActive   = "active"
	StateComplete = "complete"
)

// StateName names a log entry's state.
func StateName(s int) string {
	switch s {
	case character.QuestInactive:
		return StateInactive
	case character.QuestActive:
		return StateActive
	case character.QuestComplete:
		return StateComplete
	}
	return StateNone
}

// Find returns the index of a quest in a log, or -1.
func Find(log []character.Quest, id uint32) int {
	for i := range log {
		if log[i].ID == id {
			return i
		}
	}
	return -1
}

// Add is quest_add: the quest joins the log as active, with its time
// limit from now. It returns false, and changes nothing, when the log
// has the quest already, in any state.
func Add(log *[]character.Quest, r *Record, now time.Time) bool {
	if Find(*log, r.ID) >= 0 {
		return false
	}
	*log = append(*log, character.Quest{ID: r.ID, State: character.QuestActive, End: r.Limit.End(now)})
	return true
}

// Change is quest_change: the quest from, which the log must hold and
// not as complete, becomes the quest to, as a new active quest in its
// place. It returns false, and changes nothing, when the log lacks from,
// holds it as complete, or already holds to.
func Change(log *[]character.Quest, from uint32, to *Record, now time.Time) bool {
	if Find(*log, to.ID) >= 0 {
		return false
	}
	i := Find(*log, from)
	if i < 0 || (*log)[i].State == character.QuestComplete {
		return false
	}
	(*log)[i] = character.Quest{ID: to.ID, State: character.QuestActive, End: to.Limit.End(now)}
	return true
}

// Remove is quest_delete: the quest leaves the log, in any state. It
// returns false when the log lacks it.
func Remove(log *[]character.Quest, id uint32) bool {
	i := Find(*log, id)
	if i < 0 {
		return false
	}
	*log = append((*log)[:i], (*log)[i+1:]...)
	return true
}

// SetState is quest_update_status: a quest that is not complete becomes
// active, inactive or complete. A complete quest stays complete. It
// returns false when the log lacks the quest or holds it as complete.
func SetState(log []character.Quest, id uint32, state int) bool {
	i := Find(log, id)
	if i < 0 || log[i].State == character.QuestComplete {
		return false
	}
	log[i].State = state
	return true
}

// TimedOut reports whether a quest's time limit has passed. A quest
// without a limit never times out.
func TimedOut(q *character.Quest, now time.Time) bool {
	return !q.End.IsZero() && q.End.Before(now)
}

// Done reports whether every hunt objective of a quest has its kills.
func Done(q *character.Quest, r *Record) bool {
	for j, t := range r.Targets {
		if q.Counts[j] < t.Count {
			return false
		}
	}
	return true
}

// Progress is a quest whose counts a kill raised.
type Progress struct {
	// Index is the quest's place in the log.
	Index int
	// Objectives are the objectives that counted the kill, from 0.
	Objectives []int
}

// Gift is an item a quest's drop gives.
type Gift struct {
	Quest uint32
	Item  uint32
	Count int
}

// Count is quest_update_objective for one player: each quest of the log
// that is not complete counts the kill for each of its objectives that
// it passes, up to the objective's count, and rolls its drops. roll(n)
// returns a number from 0 to n-1, as rnd_chance draws it.
func Count(log []character.Quest, t *Table, k Kill, roll func(n int) int) ([]Progress, []Gift) {
	var progress []Progress
	var gifts []Gift
	for i := range log {
		q := &log[i]
		if q.State == character.QuestComplete {
			continue
		}
		r := t.ByID(q.ID)
		if r == nil {
			continue
		}
		var counted []int
		for j := range r.Targets {
			tg := &r.Targets[j]
			if tg.Counts(k) && q.Counts[j] < tg.Count {
				q.Counts[j]++
				counted = append(counted, j)
			}
		}
		if len(counted) > 0 {
			progress = append(progress, Progress{Index: i, Objectives: counted})
		}
		for _, d := range r.Drops {
			if d.Monster != 0 && d.Monster != k.Monster {
				continue
			}
			if d.Rate < 10000 && roll(10000) >= d.Rate {
				continue
			}
			gifts = append(gifts, Gift{Quest: q.ID, Item: d.Item, Count: d.Count})
		}
	}
	return progress, gifts
}

// Check values, as quest_check returns them.
const (
	// CheckHave is HAVEQUEST: -1 without the quest, 1 for an active or
	// inactive quest, 2 for a complete one.
	CheckHave = 0
	// CheckPlaytime is PLAYTIME: -1 without the quest, 2 once its time
	// limit passed, 1 for a complete quest, 0 otherwise.
	CheckPlaytime = 1
	// CheckHunting is HUNTING: -1 without the quest, 2 for an active or
	// inactive quest whose objectives have their kills, 1 for one whose
	// time limit passed, 0 otherwise.
	CheckHunting = 2
)

// Check is quest_check, with rAthena's quirk that a quest without a time
// limit counts as timed out for PLAYTIME and HUNTING: its end time is 0.
// Core's converted scripts read it through player:get_quest_state,
// player:quest_timed_out and player:get_quest_progress, so tests compare
// them with this function.
func Check(log []character.Quest, t *Table, id uint32, kind int, now time.Time) int {
	i := Find(log, id)
	if i < 0 {
		return -1
	}
	q := &log[i]
	expired := q.End.IsZero() || q.End.Before(now)
	switch kind {
	case CheckHave:
		if q.State == character.QuestInactive {
			return 1
		}
		return q.State
	case CheckPlaytime:
		switch {
		case expired:
			return 2
		case q.State == character.QuestComplete:
			return 1
		}
		return 0
	case CheckHunting:
		if q.State == character.QuestComplete {
			return 0
		}
		if r := t.ByID(id); r != nil && Done(q, r) {
			return 2
		}
		if expired {
			return 1
		}
		return 0
	}
	return -1
}
