package modscript

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// The presentation and quest icon API of review decision 58, areas B5
// and B6 of proposal 0002b:
//   - B5: what players see and hear of NPCs and players: lines said
//     aloud (npctalk, unittalk), emotions, effects, titles, names and
//     looks, NPC movement, sounds and music, colored lines, digits and
//     minimap marks of a map.
//   - B6: quest icons that show over an NPC to each player whose
//     condition holds (questinfo), a refresh of them, and the quest log.
//
// A handle of an NPC of another zone takes the changes too: the host
// runs them in that NPC's zone after the current step (decision 0062).

// SendTarget is who gets a line, an effect or a sound: rAthena's send
// targets AREA, ALL_SAMEMAP, ALL_CLIENT and SELF.
type SendTarget struct {
	// To is "area", "map", "world" or "player".
	To string
	// Player is the player of To "player".
	Player int64
}

// NPCLook is what npc:set_look and npc:set_sprite change. A nil field
// stays as it is.
type NPCLook struct {
	Sprite, HairStyle, HairColor, ClothColor, HeadTop, HeadMiddle, HeadBottom *int64
	// Male is the sex: true for male.
	Male *bool
}

// PresentHost is a host that shows the presentation of areas B5 and B6.
// A host without it raises an error for these calls.
type PresentHost interface {
	// SayAloud says a line aloud as an NPC or a player: npctalk and
	// unittalk. color is 0xRRGGBB, or -1 for the channel's own color.
	SayAloud(speaker Entity, text string, to SendTarget, color int64) error
	// PlayerEmotion is emotion on a player.
	PlayerEmotion(player, emotion int64) error
	// ShowEffectTo is specialeffect and specialeffect2 with a send
	// target.
	ShowEffectTo(e Entity, effect int64, to SendTarget) error
	// RemoveEffect is removespecialeffect.
	RemoveEffect(e Entity, effect int64) error
	// SetNPCTitle is setunittitle on an NPC.
	SetNPCTitle(npc int64, title string) error
	// SetNPCDisplayName is setnpcdisplay's name.
	SetNPCDisplayName(npc int64, name string) error
	// SetNPCLook is setnpcdisplay's class and setunitdata's UNPC_ looks.
	SetNPCLook(npc int64, look NPCLook) error
	// MoveNPC is movenpc.
	MoveNPC(npc int64, x, y int) error
	// WalkNPC is npcwalkto and unitwalk on an NPC. It reports whether
	// the NPC can walk there.
	WalkNPC(npc int64, x, y int) (bool, error)
	// SetNPCSpeed is npcspeed.
	SetNPCSpeed(npc int64, ms int) error
	// StopNPC is npcstop and unitstopwalk on an NPC.
	StopNPC(npc int64) error
	// ShowNPCProgress is progressbar_npc's bar over the NPC.
	ShowNPCProgress(npc int64, ms int64, color uint32) error
	// NPCUseSkill is npcskill: the NPC casts a skill on a target with a
	// status of stat in every stat and level npcLevel.
	NPCUseSkill(npc, skill, level int64, target Entity, stat, npcLevel int64) (bool, error)
	// ShowSkillEffect is skilleffect.
	ShowSkillEffect(e Entity, skill, level int64) error
	// PlaySound plays a sound: on a player alone (soundeffect), around
	// an NPC or to its map (soundeffectall), or to a map's players.
	// source is the player or NPC, or a zero Entity with mapName.
	PlaySound(source Entity, mapName, name string, to string, repeat bool) error
	// PlayMusic plays background music to a player, or to a map's
	// players when player is 0 (playBGM, playBGMall).
	PlayMusic(player int64, mapName, name string) error
	// SendColoredMessage is dispbottom with a color.
	SendColoredMessage(player int64, text string, color uint32) error
	// ShowDigits is showdigit. kind is 0 to 3.
	ShowDigits(player int64, kind int, value int64) error
	// MarkMinimapOfMap is viewpointmap: a mark on the minimap of every
	// player of a map, or its removal.
	MarkMinimapOfMap(mapName string, id int64, x, y int, color uint32, remove bool) error
	// QuestIconsChanged tells the host that an NPC's quest icons changed
	// (questinfo), so the players of its map see them again.
	QuestIconsChanged(npc int64) error
	// RefreshQuestIcons is questinfo_refresh.
	RefreshQuestIcons(player int64) error
	// OpenQuestLog is open_quest_ui.
	OpenQuestLog(player int64, quest int64) error
}

func (r *Runtime) presentHost(what string) (PresentHost, error) {
	h, ok := r.host.(PresentHost)
	if !ok {
		return nil, fmt.Errorf("%s: this zone shows no NPC presentation", what)
	}
	return h, nil
}

// Limits of the presentation calls.
const (
	// MaxTitle is the longest NPC title, in bytes: NAME_LENGTH - 1.
	MaxTitle = 23
	// MaxDisplayName is the longest NPC name players see, in bytes:
	// NPC_NAME_LENGTH.
	MaxDisplayName = 50
	// MinNPCSpeed and MaxNPCSpeed are npcspeed's bounds, in
	// milliseconds per cell: MIN_WALK_SPEED and MAX_WALK_SPEED.
	MinNPCSpeed = 20
	MaxNPCSpeed = 1000
	// MaxSkillStat is npcskill's highest stat: max_third_parameter.
	MaxSkillStat = 130
	// MaxSkillNPCLevel is npcskill's highest level: MAX_LEVEL.
	MaxSkillNPCLevel = 275
)

// QuestIconNames are every icon of wa.QuestIcon, by rAthena's QTYPE_
// value. warg and click_me share QTYPE_WARG and QTYPE_CLICKME, and
// warg_face and event3 QTYPE_WARG2 and QTYPE_EVENT3: clients of
// PACKETVER 20170315 and later show the second names.
var QuestIconNames = map[string]int{
	"quest": 0, "quest_question": 1, "job": 2, "job_question": 3, "event": 4, "event_question": 5,
	"warg": 6, "click_me": 6, "daily_quest": 7, "warg_face": 8, "event3": 8, "job_quest": 9, "jumping_poring": 10,
}

// QuestMarks are the minimap marks of quest icons, rAthena's QMARK_
// colors.
var QuestMarks = []string{"yellow", "green", "purple"}

// assetName is a sound or music name: path parts of letters, digits, _
// and -, joined with /.
var assetName = regexp.MustCompile(`^[A-Za-z0-9_\-]+(/[A-Za-z0-9_\-]+)*$`)

// sendTarget reads a send target: a kind, and a player ID for "player".
// allowed lists the kinds the call takes.
func sendTarget(what string, kind, player script.Value, allowed ...string) (SendTarget, error) {
	if kind.IsNil() {
		return SendTarget{To: "area"}, nil
	}
	k, ok := kind.AsString()
	if !ok || !slices.Contains(allowed, k) {
		return SendTarget{}, fmt.Errorf("%s: to is one of %q, or a player handle", what, allowed)
	}
	t := SendTarget{To: k}
	if k == "player" {
		if t.Player, ok = player.AsInt(); !ok {
			return SendTarget{}, fmt.Errorf("%s: to must be a player handle", what)
		}
	}
	return t, nil
}

func checkText(what string, v script.Value, limit int) (string, error) {
	s, ok := v.AsString()
	if !ok || s == "" || len(s) > limit || !utf8.ValidString(s) {
		return "", fmt.Errorf("%s: the text must be 1 to %d bytes of UTF-8", what, limit)
	}
	return s, nil
}

func checkColor(what string, v script.Value) (int64, error) {
	if v.IsNil() {
		return -1, nil
	}
	c, ok := v.AsInt()
	if !ok || c < 0 || c > 0xFFFFFF {
		return 0, fmt.Errorf("%s: the color is 0xRRGGBB", what)
	}
	return c, nil
}

func checkEffect(what string, v script.Value) (int64, error) {
	e, ok := v.AsInt()
	// specialeffect: EF_NONE < effect < EF_MAX.
	if !ok || e < 0 || e > 1<<20 {
		return 0, fmt.Errorf("%s: want an effect from wa.effects", what)
	}
	return e, nil
}

func checkCell(what string, x, y script.Value) (int, int, error) {
	cx, ok1 := x.AsInt()
	cy, ok2 := y.AsInt()
	if !ok1 || !ok2 || cx < 0 || cy < 0 || cx > 1023 || cy > 1023 {
		return 0, 0, fmt.Errorf("%s: want a cell, x and y from 0 to 1023", what)
	}
	return int(cx), int(cy), nil
}

func checkAsset(what, kind string, v script.Value) (string, error) {
	s, ok := v.AsString()
	if !ok || len(s) > 128 || !assetName.MatchString(s) {
		return "", fmt.Errorf("%s: want a %s name, such as %s", what, kind, map[string]string{"sound": "effect/ef_hit1", "music": "08"}[kind])
	}
	return s, nil
}

// npcPresent is the presentation methods of NPC handles: Arg(0) is the
// handle, Arg(1) the method, then its arguments.
func (r *Runtime) npcPresent(c *script.Call) error {
	id, err := npcID(c)
	if err != nil {
		return err
	}
	op := c.String(1)
	what := "npc:" + op
	if err := r.mutating(what); err != nil {
		return err
	}
	if op == "say_aloud" && r.host != nil {
		if _, ok := r.host.(PresentHost); !ok {
			// A host without the presentation API says plain lines.
			return r.npcSayPlain(c, id)
		}
	}
	h, err := r.presentHost(what)
	if err != nil {
		return err
	}
	if w, ok := r.host.(NPCWorldHost); ok {
		if err := r.controls(what, w, id); err != nil {
			return err
		}
	}
	arg := func(i int) script.Value { return c.Arg(2 + i) }
	npc := Entity{Kind: "npc", ID: id}
	switch op {
	case "say_aloud":
		text, err := checkText(what, arg(0), MaxMessage)
		if err != nil {
			return err
		}
		to, err := sendTarget(what, arg(1), arg(2), "area", "map", "world", "player")
		if err != nil {
			return err
		}
		color, err := checkColor(what, arg(3))
		if err != nil {
			return err
		}
		err = h.SayAloud(npc, text, to, color)
		return wrap(what, err)
	case "show_effect":
		effect, err := checkEffect(what, arg(0))
		if err != nil {
			return err
		}
		to, err := sendTarget(what, arg(1), arg(2), "area", "map", "player")
		if err != nil {
			return err
		}
		return wrap(what, h.ShowEffectTo(npc, effect, to))
	case "remove_effect":
		effect, err := checkEffect(what, arg(0))
		if err != nil {
			return err
		}
		return wrap(what, h.RemoveEffect(npc, effect))
	case "set_title":
		title, ok := arg(0).AsString()
		if !ok || len(title) > MaxTitle || !utf8.ValidString(title) {
			return fmt.Errorf("%s: the title is at most %d bytes of UTF-8", what, MaxTitle)
		}
		return wrap(what, h.SetNPCTitle(id, title))
	case "set_display_name":
		name, err := checkText(what, arg(0), MaxDisplayName)
		if err != nil {
			return err
		}
		return wrap(what, h.SetNPCDisplayName(id, name))
	case "set_sprite":
		v, ok := arg(0).AsInt()
		if !ok || v < 0 || v > 1<<31-1 {
			return errors.New(what + ": want a sprite ID: an NPC sprite, a monster or a job")
		}
		return wrap(what, h.SetNPCLook(id, NPCLook{Sprite: &v}))
	case "set_look":
		look, err := readNPCLook(what, arg(0))
		if err != nil {
			return err
		}
		return wrap(what, h.SetNPCLook(id, look))
	case "move_to":
		x, y, err := checkCell(what, arg(0), arg(1))
		if err != nil {
			return err
		}
		return wrap(what, h.MoveNPC(id, x, y))
	case "walk_to":
		x, y, err := checkCell(what, arg(0), arg(1))
		if err != nil {
			return err
		}
		ok, err := h.WalkNPC(id, x, y)
		if err != nil {
			return wrap(what, err)
		}
		c.Return(script.Bool(ok))
		return nil
	case "set_speed":
		ms, ok := arg(0).AsInt()
		if !ok || ms < MinNPCSpeed || ms > MaxNPCSpeed {
			return fmt.Errorf("%s: the speed is %d to %d milliseconds per cell", what, MinNPCSpeed, MaxNPCSpeed)
		}
		return wrap(what, h.SetNPCSpeed(id, int(ms)))
	case "stop":
		return wrap(what, h.StopNPC(id))
	case "play_sound":
		name, err := checkAsset(what, "sound", arg(0))
		if err != nil {
			return err
		}
		to := "area"
		if v := arg(1); !v.IsNil() {
			if to, _ = v.AsString(); to != "area" && to != "map" {
				return errors.New(what + `: to is "area" or "map"`)
			}
		}
		return wrap(what, h.PlaySound(npc, "", name, to, arg(2).Truthy()))
	case "use_skill":
		return r.npcUseSkill(c, h, id)
	case "show_progress":
		return r.npcShowProgress(c, h, id)
	case "add_quest_icon":
		return r.npcAddQuestIcon(c, h, id)
	}
	return fmt.Errorf("npc has no method %q", op)
}

// wrap names the call in a host's error.
func wrap(what string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

// npcSayPlain is npc:say_aloud(text) for a host without PresentHost.
func (r *Runtime) npcSayPlain(c *script.Call, id int64) error {
	const what = "npc:say_aloud"
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	text, err := checkText(what, c.Arg(2), MaxMessage)
	if err != nil {
		return err
	}
	if !c.Arg(3).IsNil() || !c.Arg(5).IsNil() {
		return errors.New(what + ": this zone takes no options")
	}
	return h.NPCSay(id, text)
}

// lookFields are npc:set_look's fields, rAthena's UNPC_ data.
var lookFields = []string{"sprite", "hair_style", "hair_color", "cloth_color", "head_top", "head_middle", "head_bottom", "sex"}

func readNPCLook(what string, v script.Value) (NPCLook, error) {
	t, ok := v.AsTable()
	if !ok {
		return NPCLook{}, fmt.Errorf("%s: want a table of %q", what, lookFields)
	}
	var look NPCLook
	for _, f := range t.Fields {
		key, _ := f.Key.AsString()
		val := f.Value
		if key == "sex" {
			s, ok := val.AsString()
			if !ok || s != "male" && s != "female" {
				return NPCLook{}, fmt.Errorf("%s: sex is \"male\" or \"female\"", what)
			}
			male := s == "male"
			look.Male = &male
			continue
		}
		n, ok := val.AsInt()
		if !ok || n < 0 || n > 1<<31-1 {
			return NPCLook{}, fmt.Errorf("%s: %s must be an integer of 0 or more", what, key)
		}
		switch key {
		case "sprite":
			look.Sprite = &n
		case "hair_style":
			look.HairStyle = &n
		case "hair_color":
			look.HairColor = &n
		case "cloth_color":
			look.ClothColor = &n
		case "head_top":
			look.HeadTop = &n
		case "head_middle":
			look.HeadMiddle = &n
		case "head_bottom":
			look.HeadBottom = &n
		default:
			return NPCLook{}, fmt.Errorf("%s: there is no field %q; the fields are %q", what, key, lookFields)
		}
	}
	return look, nil
}

// npcUseSkill is npc:use_skill(skill, level, target, options): Arg(2)
// is the skill, Arg(3) the level, Arg(4) the target's key, Arg(5) the
// stat and Arg(6) the NPC's level, nil for the default.
func (r *Runtime) npcUseSkill(c *script.Call, h PresentHost, id int64) error {
	const what = "npc:use_skill"
	skill, ok := c.Arg(2).AsInt()
	if !ok || skill <= 0 {
		return errors.New(what + ": want a skill from skills.id")
	}
	level, ok := c.Arg(3).AsInt()
	if !ok || level < 1 || level > 100 {
		return errors.New(what + ": the level is 1 to 100")
	}
	key, _ := c.Arg(4).AsString()
	target, err := parseHandleKey(key)
	if err != nil {
		return fmt.Errorf("%s: the target must be a player, monster or NPC handle", what)
	}
	stat, npcLevel := int64(-1), int64(-1)
	if v := c.Arg(5); !v.IsNil() {
		if stat, ok = v.AsInt(); !ok || stat < 0 || stat > MaxSkillStat {
			return fmt.Errorf("%s: stat is 0 to %d, as max_third_parameter", what, MaxSkillStat)
		}
	}
	if v := c.Arg(6); !v.IsNil() {
		if npcLevel, ok = v.AsInt(); !ok || npcLevel < 0 || npcLevel > MaxSkillNPCLevel {
			return fmt.Errorf("%s: npc_level is 0 to %d, as MAX_LEVEL", what, MaxSkillNPCLevel)
		}
	}
	used, err := h.NPCUseSkill(id, skill, level, target, stat, npcLevel)
	if err != nil {
		return wrap(what, err)
	}
	c.Return(script.Bool(used))
	return nil
}

// npcShowProgress is npc:show_progress(seconds, options): the bar shows
// over the NPC, and the conversation that runs waits until it is full,
// as progressbar_npc's sleep.
func (r *Runtime) npcShowProgress(c *script.Call, h PresentHost, id int64) error {
	const what = "npc:show_progress"
	s, ok := c.Arg(2).AsInt()
	if !ok || s < 0 || s*1000 > MaxWait {
		return fmt.Errorf("%s: seconds are 0 to %d", what, MaxWait/1000)
	}
	color := uint32(0)
	if v := c.Arg(3); !v.IsNil() {
		n, err := checkColor(what, v)
		if err != nil {
			return err
		}
		color = uint32(n) // #nosec G115 -- checked above.
	}
	conv := r.talking
	if conv == nil || conv.Closed || !c.CanYield() || r.current != conv.ctx {
		return errors.New(what + " waits, so it runs only in a conversation's handler, as a click's")
	}
	nh, err := r.npcHost()
	if err != nil {
		return err
	}
	if err := h.ShowNPCProgress(id, s*1000, color); err != nil {
		return wrap(what, err)
	}
	return r.wait(c, what, conv, nh, Step{Kind: StepWait, Ms: s * 1000})
}

// parseHandleKey reads a handle key of the prelude's handle_key:
// "<kind>:<id>".
func parseHandleKey(key string) (Entity, error) {
	var e Entity
	for i := range len(key) {
		if key[i] == ':' {
			e.Kind = key[:i]
			var id int64
			if _, err := fmt.Sscan(key[i+1:], &id); err != nil {
				return Entity{}, err
			}
			e.ID = id
			break
		}
	}
	if e.Kind != "player" && e.Kind != "monster" && e.Kind != "npc" {
		return Entity{}, fmt.Errorf("not a handle key: %q", key)
	}
	return e, nil
}

// playerPresent is the presentation methods of player handles: Arg(0)
// is the handle, Arg(1) the method, then its arguments.
func (r *Runtime) playerPresent(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	op := c.String(1)
	what := "player:" + op
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.presentHost(what)
	if err != nil {
		return err
	}
	arg := func(i int) script.Value { return c.Arg(2 + i) }
	player := Entity{Kind: "player", ID: id}
	switch op {
	case "say_aloud":
		text, err := checkText(what, arg(0), MaxMessage)
		if err != nil {
			return err
		}
		to, err := sendTarget(what, arg(1), arg(2), "area", "map", "world", "player")
		if err != nil {
			return err
		}
		return wrap(what, h.SayAloud(player, text, to, -1))
	case "show_emotion":
		e, ok := arg(0).AsInt()
		if !ok || e < 0 || e > 255 {
			return errors.New(what + ": want an emotion from wa.emotions")
		}
		return wrap(what, h.PlayerEmotion(id, e))
	case "show_effect":
		effect, err := checkEffect(what, arg(0))
		if err != nil {
			return err
		}
		to, err := sendTarget(what, arg(1), arg(2), "area", "map", "player")
		if err != nil {
			return err
		}
		return wrap(what, h.ShowEffectTo(player, effect, to))
	case "remove_effect":
		effect, err := checkEffect(what, arg(0))
		if err != nil {
			return err
		}
		return wrap(what, h.RemoveEffect(player, effect))
	case "play_sound":
		name, err := checkAsset(what, "sound", arg(0))
		if err != nil {
			return err
		}
		return wrap(what, h.PlaySound(player, "", name, "player", arg(1).Truthy()))
	case "play_music":
		name, err := checkAsset(what, "music", arg(0))
		if err != nil {
			return err
		}
		return wrap(what, h.PlayMusic(id, "", name))
	case "send_message":
		text, err := checkText(what, arg(0), MaxMessage)
		if err != nil {
			return err
		}
		color, err := checkColor(what, arg(1))
		if err != nil {
			return err
		}
		if color < 0 {
			return r.host.SendMessage(id, text)
		}
		return wrap(what, h.SendColoredMessage(id, text, uint32(color))) // #nosec G115 -- checked above.
	case "show_digits":
		value, ok := arg(0).AsInt()
		if !ok || value < -(1<<31) || value > 1<<31-1 {
			return errors.New(what + ": the value is a 32-bit integer of seconds")
		}
		kind := 0
		if v := arg(1); !v.IsNil() {
			s, _ := v.AsString()
			if kind = slices.Index(DigitKinds, s); kind < 0 {
				return fmt.Errorf("%s: the kind is one of %q", what, DigitKinds)
			}
		}
		return wrap(what, h.ShowDigits(id, kind, value))
	case "open_quest_log":
		var quest int64
		if v := arg(0); !v.IsNil() {
			rec, err := r.questArg(what, v)
			if err != nil {
				return err
			}
			quest = int64(rec.ID)
		}
		return wrap(what, h.OpenQuestLog(id, quest))
	case "refresh_quest_icons":
		return wrap(what, h.RefreshQuestIcons(id))
	}
	return fmt.Errorf("player has no method %q", op)
}

// DigitKinds are player:show_digits's kinds, showdigit's types 0 to 3.
var DigitKinds = []string{"show", "count_up", "count_down", "count_down_to_zero"}

// mapsPresent is maps.play_music, maps.play_sound, maps.mark_minimap and
// maps.unmark_minimap: Arg(0) is the function, Arg(1) the map.
func (r *Runtime) mapsPresent(c *script.Call) error {
	op := c.String(0)
	what := "maps." + op
	if err := r.mutating(what); err != nil {
		return err
	}
	m, ok := c.Arg(1).AsString()
	if !ok || m == "" || len(m) > 64 {
		return errors.New(what + ": want a map name, such as prontera")
	}
	h, err := r.presentHost(what)
	if err != nil {
		return err
	}
	arg := func(i int) script.Value { return c.Arg(2 + i) }
	switch op {
	case "play_music":
		name, err := checkAsset(what, "music", arg(0))
		if err != nil {
			return err
		}
		return wrap(what, h.PlayMusic(0, m, name))
	case "play_sound":
		name, err := checkAsset(what, "sound", arg(0))
		if err != nil {
			return err
		}
		return wrap(what, h.PlaySound(Entity{}, m, name, "map", arg(1).Truthy()))
	case "mark_minimap", "unmark_minimap":
		mark, ok := arg(0).AsInt()
		if !ok || mark < 0 || mark > 1<<16 {
			return errors.New(what + ": the mark ID must be 0 to 65536")
		}
		if op == "unmark_minimap" {
			return wrap(what, h.MarkMinimapOfMap(m, mark, 0, 0, 0, true))
		}
		x, y, err := checkCell(what, arg(1), arg(2))
		if err != nil {
			return err
		}
		color := int64(0xFFFFFF)
		if v := arg(3); !v.IsNil() {
			if color, err = checkColor(what, v); err != nil {
				return err
			}
		}
		return wrap(what, h.MarkMinimapOfMap(m, mark, x, y, uint32(color), false)) // #nosec G115 -- checked above.
	}
	return fmt.Errorf("maps has no function %q", op)
}

// entitySkillEffect is entity:show_skill_effect(skill, level), as
// skilleffect: Arg(0) and Arg(1) are the entity, Arg(2) the skill and
// Arg(3) the level.
func (r *Runtime) entitySkillEffect(c *script.Call) error {
	e, err := entityArg(c, 0)
	if err != nil {
		return err
	}
	what := e.Kind + ":show_skill_effect"
	if err := r.mutating(what); err != nil {
		return err
	}
	skill, ok := c.Arg(2).AsInt()
	if !ok || skill <= 0 {
		return errors.New(what + ": want a skill from skills.id")
	}
	level, ok := c.Arg(3).AsInt()
	if !ok || level < 0 || level > 100 {
		return errors.New(what + ": the level is 0 to 100")
	}
	h, err := r.presentHost(what)
	if err != nil {
		return err
	}
	return wrap(what, h.ShowSkillEffect(e, skill, level))
}

// questIcon is one entry of an NPC's quest icons, as rAthena's
// s_questinfo: the icon, its minimap mark, and the condition, a
// function of the player, or nil.
type questIcon struct {
	ctx        *Context
	npc        int64
	icon, mark string
	cond       *script.Ref
}

// npcAddQuestIcon is npc:add_quest_icon(icon, options): Arg(2) is the
// icon, Arg(3) the mark and Arg(4) the condition.
func (r *Runtime) npcAddQuestIcon(c *script.Call, h PresentHost, id int64) error {
	const what = "npc:add_quest_icon"
	ctx, err := r.needContext(what)
	if err != nil {
		return err
	}
	icon, ok := c.Arg(2).AsString()
	if _, known := QuestIconNames[icon]; !ok || !known {
		return fmt.Errorf("%s: the icon is one of wa.QuestIcon, such as quest", what)
	}
	mark := ""
	if v := c.Arg(3); !v.IsNil() {
		if mark, ok = v.AsString(); !ok || !slices.Contains(QuestMarks, mark) {
			return fmt.Errorf("%s: the mark is one of %q, or nil", what, QuestMarks)
		}
	}
	if err := h.QuestIconsChanged(id); err != nil {
		return wrap(what, err)
	}
	qi := &questIcon{ctx: ctx, npc: id, icon: icon, mark: mark}
	if v := c.Arg(4); !v.IsNil() {
		if qi.cond, err = c.State().Keep(v); err != nil {
			return err
		}
	}
	if r.questIcons == nil {
		r.questIcons = map[int64][]*questIcon{}
	}
	r.questIcons[id] = append(r.questIcons[id], qi)
	ctx.questIcons = append(ctx.questIcons, qi)
	return nil
}

// dropQuestIcons forgets a context's quest icons, when it ends.
func (r *Runtime) dropQuestIcons(c *Context) {
	for _, qi := range c.questIcons {
		list := r.questIcons[qi.npc]
		if i := slices.Index(list, qi); i >= 0 {
			list = slices.Delete(list, i, i+1)
		}
		if len(list) == 0 {
			delete(r.questIcons, qi.npc)
		} else {
			r.questIcons[qi.npc] = list
		}
		if qi.cond != nil {
			qi.cond.Release()
		}
		if h, ok := r.host.(PresentHost); ok {
			_ = h.QuestIconsChanged(qi.npc)
		}
	}
	c.questIcons = nil
}

// HasQuestIcons reports whether an NPC has quest icons.
func (r *Runtime) HasQuestIcons(npc int64) bool { return len(r.questIcons[npc]) > 0 }

// QuestIcon is pc_show_questinfo's check of one NPC for one player
// (pc.cpp 6088's caller): the first of the NPC's quest icons whose
// condition holds. ok is false when none holds. A condition runs as a
// hook does: calls that change the game raise an error. A condition
// that fails counts as false, as achievement_check_condition does.
func (r *Runtime) QuestIcon(npc, player int64) (icon, mark string, ok bool) {
	for _, qi := range slices.Clone(r.questIcons[npc]) {
		if qi.ctx.ended || r.off(qi.ctx) {
			continue
		}
		if qi.cond == nil {
			return qi.icon, qi.mark, true
		}
		held, err := r.questCondition(qi, player)
		if err != nil {
			r.failed(qi.ctx, "quest icon condition", err)
			continue
		}
		if held {
			return qi.icon, qi.mark, true
		}
	}
	return "", "", false
}

func (r *Runtime) questCondition(qi *questIcon, player int64) (bool, error) {
	prev := r.computing
	r.computing = true
	defer func() { r.computing = prev }()
	var held bool
	err := r.runFn(qi.ctx, func() error {
		v, err := r.s.Call1(r.fns.invoke.Value(), qi.cond.Value(), script.String("player"), script.Int(player))
		held = v.Truthy()
		return err
	})
	return held, err
}
