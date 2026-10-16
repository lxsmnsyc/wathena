package convert

import (
	"fmt"
	"strings"
)

// Quest commands of NPC scripts, converted to the quest API of proposal
// 0002, review decision 45:
//
//   - setquest, completequest, erasequest and changequest become
//     player:add_quest, player:complete_quest, player:remove_quest and
//     player:change_quest. A quest is named by its rAthena ID, which is
//     core's record name.
//   - checkquest and isbegin_quest become R.checkquest and
//     R.isbegin_quest (rathena.lua), which read player:get_quest_state,
//     player:quest_timed_out and player:get_quest_progress and return
//     rAthena's numbers.
//   - rAthena counts a quest without a time limit as timed out for
//     PLAYTIME and HUNTING, as its end time is 0. The converter knows
//     which of the mode's quests have one, and passes it to
//     R.checkquest. For a computed ID it passes nil, and the quest's own
//     limit decides.
//   - A fixed ID that the mode's quest_db does not have converts to what
//     rAthena does with it: the commands only log an error, checkquest is
//     -1 and isbegin_quest is 0.
//   - showevent becomes npc:show_quest_icon with the icon's name.
//   - A command for another character does not convert.

// questIcons are the names of showevent's icons, by rAthena's QTYPE_
// value. PACKETVER 20211103 shows 0 to 6 and 8 (QTYPE_WARG2, the warg's
// face). QTYPE_DAILYQUEST (7), QTYPE_NONE (9999) and any other value
// remove the icon, as buildin_showevent.
var questIcons = map[int64]string{
	0: "quest", 1: "quest_question", 2: "job", 3: "job_question",
	4: "event", 5: "event_question", 6: "warg", 8: "warg_face",
}

// knownQuest reads a quest ID argument. known is false for a fixed ID
// that the mode's quests lack.
func (b *body) questID(name string, e Expr) (v val, known bool, err error) {
	v, err = b.expr(e)
	if err != nil {
		return v, false, err
	}
	if v.str != nil {
		return v, false, unsupported(name, "a quest named by text")
	}
	if v.static == nil {
		return v, true, nil
	}
	return v, b.n.c.quests.Has(*v.static), nil
}

// questPlayer checks a command's optional character ID argument.
func (b *body) questPlayer(name string, args []Expr, at int) error {
	if len(args) <= at {
		return nil
	}
	if len(args) > at+1 {
		return unsupported(name, "arguments")
	}
	who, err := b.expr(args[at])
	if err != nil {
		return err
	}
	if !isPlayerID(who) {
		return unsupported(name, "for another character")
	}
	return nil
}

// questCommand converts the quest commands written as statements. ok is
// false for another command.
func (b *body) questCommand(name string, args []Expr) (ok bool, err error) {
	switch name {
	case "setquest", "completequest", "erasequest":
		if len(args) == 0 {
			return true, unsupported(name, "arguments")
		}
		if err := b.questPlayer(name, args, 1); err != nil {
			return true, err
		}
		id, known, err := b.questID(name, args[0])
		if err != nil {
			return true, err
		}
		if !known {
			b.line("-- %s %s: this mode's quest_db has no such quest, so rAthena only logs an error.", name, id.code)
			return true, nil
		}
		method := map[string]string{"setquest": "add_quest", "completequest": "complete_quest", "erasequest": "remove_quest"}[name]
		b.line("player:%s(%s)", method, id.asInt())
		return true, nil
	case "changequest":
		if len(args) < 2 {
			return true, unsupported(name, "arguments")
		}
		if err := b.questPlayer(name, args, 2); err != nil {
			return true, err
		}
		from, knownFrom, err := b.questID(name, args[0])
		if err != nil {
			return true, err
		}
		to, knownTo, err := b.questID(name, args[1])
		if err != nil {
			return true, err
		}
		if !knownFrom || !knownTo {
			b.line("-- changequest %s, %s: this mode's quest_db lacks a quest, so rAthena only logs an error.", from.code, to.code)
			return true, nil
		}
		b.line("player:change_quest(%s, %s)", from.asInt(), to.asInt())
		return true, nil
	case "showevent":
		return true, b.showevent(args)
	case "checkquest", "isbegin_quest":
		// As a statement, the result is not read.
		if _, err := b.questRead(name, args); err != nil {
			return true, err
		}
		return true, nil
	}
	return false, nil
}

// questRead converts checkquest and isbegin_quest in an expression. ok
// is false for another function.
func (b *body) questRead(name string, args []Expr) (val, error) {
	switch name {
	case "checkquest":
		if len(args) == 0 {
			return val{}, unsupported(name, "arguments")
		}
		if err := b.questPlayer(name, args, 2); err != nil {
			return val{}, err
		}
		kind := int64(0)
		if len(args) >= 2 {
			k, err := b.expr(args[1])
			if err != nil {
				return val{}, err
			}
			if k.static == nil || *k.static < 0 || *k.static > 2 {
				return val{}, unsupported(name, "a check other than HAVEQUEST, PLAYTIME and HUNTING")
			}
			kind = *k.static
		}
		id, known, err := b.questID(name, args[0])
		if err != nil {
			return val{}, err
		}
		if !known {
			return intVal(-1), nil
		}
		if kind == 0 {
			return val{code: fmt.Sprintf("R.checkquest(player, %s, 0)", id.asInt()), t: tInt}, nil
		}
		timed := "nil"
		if id.static != nil {
			timed = fmt.Sprint(b.n.c.quests.Timed[uint32(*id.static)]) // #nosec G115 -- Has checked the ID.
		}
		return val{code: fmt.Sprintf("R.checkquest(player, %s, %d, %s)", id.asInt(), kind, timed), t: tInt}, nil
	case "isbegin_quest":
		if len(args) == 0 {
			return val{}, unsupported(name, "arguments")
		}
		if err := b.questPlayer(name, args, 1); err != nil {
			return val{}, err
		}
		id, known, err := b.questID(name, args[0])
		if err != nil {
			return val{}, err
		}
		if !known {
			return intVal(0), nil
		}
		return val{code: "R.isbegin_quest(player, " + id.asInt() + ")", t: tInt}, nil
	}
	return val{}, errNotQuest
}

// errNotQuest is questRead's answer for another function.
var errNotQuest = fmt.Errorf("not a quest read")

// showevent converts showevent <icon>{,<mark color>{,<char id>}}. A mark
// color other than QMARK_NONE does not convert, since the API has no
// minimap mark for it.
func (b *body) showevent(args []Expr) error {
	const name = "showevent"
	if len(args) == 0 || len(args) > 3 {
		return unsupported(name, "arguments")
	}
	if err := b.questPlayer(name, args, 2); err != nil {
		return err
	}
	if len(args) >= 2 {
		c, err := b.expr(args[1])
		if err != nil {
			return err
		}
		if c.static == nil || *c.static != 0 {
			return unsupported(name, "a minimap mark color, which the API does not show")
		}
	}
	icon, err := b.expr(args[0])
	if err != nil {
		return err
	}
	if icon.static == nil {
		return unsupported(name, "a computed icon")
	}
	if n, ok := questIcons[*icon.static]; ok {
		b.line("npc:show_quest_icon(player, %s)", luaString(n))
		return nil
	}
	b.line("npc:show_quest_icon(player, nil)")
	return nil
}

// Has reports whether the mode has a quest.
func (r *QuestResult) Has(id int64) bool {
	if r == nil || id <= 0 || id > int64(^uint32(0)) {
		return false
	}
	if r.ids == nil {
		r.ids = make(map[uint32]bool, len(r.Records))
		for _, q := range r.Records {
			var n uint32
			_, _ = fmt.Sscan(strings.TrimSpace(q.Name), &n)
			r.ids[n] = true
		}
	}
	return r.ids[uint32(id)] // #nosec G115 -- checked above.
}
