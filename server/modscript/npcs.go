package modscript

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// NPC contexts and conversations (proposal 0002, "NPC conversations"):
//   - Each placed NPC with a script is an instance: the script's setup
//     function runs once for it, in an NPC context. Its "click" handlers
//     run when a player talks to it, and its "touch" and "untouch"
//     handlers when a player steps into or out of its touch area.
//   - A click starts a conversation: a thread that runs the click
//     handlers. Methods that wait for the player, such as choose, pause
//     the thread. The zone resumes it with the player's answer.
//   - A touch starts one too, for its touch handlers (review decision
//     56). It shows nothing until a handler shows a step.
//   - A conversation that ends without its script, such as when the
//     player leaves, is resumed with "closed": its pending call raises
//     an error that is not logged as a failure.
//   - A reload ends an NPC context. Conversations that run in it finish
//     on the old version: the context keeps running for them, without
//     handlers or timers, and ends with the last one.

// NPCHost is what NPC scripts reach in the zone, beyond Host. A Host
// without it runs no NPC scripts, and these calls raise an error.
type NPCHost interface {
	// NPCField reads a field of an NPC handle: name, map, x or y.
	NPCField(npc int64, field string) (script.Value, error)
	// NPCEmotion shows an emotion over an NPC, as rAthena's emotion.
	NPCEmotion(npc int64, emotion int64) error
	// NPCSay says a line aloud as an NPC, as rAthena's npctalk.
	NPCSay(npc int64, text string) error
	// Converse shows one step of a conversation to its player. A step
	// that waits is answered with ResumeConversation.
	Converse(c *Conversation, s Step) error
	// ConversationEnded tells the host that a conversation's thread
	// finished, by its script or by CloseConversation.
	ConversationEnded(c *Conversation)
	// PayZeny takes zeny from a player. It returns false when the
	// player has too little.
	PayZeny(player, amount int64) (bool, error)
	// GiveZeny gives a player zeny, up to MAX_ZENY.
	GiveZeny(player, amount int64) error
	// TakeItem takes items from a player, as delitem. It returns false,
	// and takes nothing, when the player has too few.
	TakeItem(player int64, item uint32, amount int) (bool, error)
	// CanCarry is checkweight: whether the items fit, by weight and
	// slots.
	CanCarry(player int64, item uint32, amount int) (bool, error)
	// SetSavePoint sets a player's save point. It returns false when
	// the map does not run or the cell is not walkable.
	SetSavePoint(player int64, mapName string, x, y int) (bool, error)
	// MarkMinimap marks a cell on a player's minimap, or removes the
	// mark, as rAthena's viewpoint.
	MarkMinimap(player int64, id int64, x, y int, color uint32, remove bool) error
	// OpenShop opens a shop for a player, by its full record name.
	OpenShop(player int64, shop string) error
	// Variable reads a player's variable as JSON. ok is false when the
	// variable is at its default.
	Variable(player int64, v VariableKey) (value string, ok bool, err error)
	// SetVariable changes a player's variable. "" is the default.
	SetVariable(player int64, v VariableKey, value string) error
	// ItemName is an item's name in the server's language.
	ItemName(item uint32) (string, bool)
	// Announce shows a line to every player of a map, or of the world
	// when mapName is empty. mod is the mod that sends it, which players
	// see with the line, or "" for core (review decision 52).
	Announce(mapName, mod, text string) error
}

// StepKind is what a conversation step shows.
type StepKind int

const (
	// StepSay adds lines to the dialog.
	StepSay StepKind = iota
	// StepNext waits for the Next button.
	StepNext
	// StepMenu waits for a choice among Options.
	StepMenu
	// StepNumber waits for a number from Min to Max.
	StepNumber
	// StepText waits for text of at most MaxLength characters.
	StepText
	// StepClose shows the Close button, and waits for it.
	StepClose
	// StepWait waits Ms milliseconds.
	StepWait
	// StepProgress shows a progress bar and waits Ms milliseconds.
	StepProgress
	// StepImage shows the illustration Image at Position, or hides it
	// when Image is empty.
	StepImage
)

// Waits reports whether a step pauses the conversation.
func (k StepKind) Waits() bool { return k != StepSay && k != StepImage }

// Step is one step of a conversation.
type Step struct {
	Kind      StepKind
	Lines     []string
	Options   []string
	Min, Max  int64
	MaxLength int
	Ms        int64
	Image     string
	// Position is left, center or right.
	Position string
	Color    uint32
}

// Conversation is one player talking to one NPC.
type Conversation struct {
	ID int64
	// Player is the zone's player ID, and NPC the NPC's handle.
	Player, NPC int64
	// Closed is set once conversation:close() was called or the
	// conversation was closed: dialog calls raise an error after it.
	Closed bool
	// Cancelled is set when CloseConversation ended the conversation,
	// rather than its script.
	Cancelled bool
	// Done is set once the thread finished.
	Done bool
	// Touch is set for the conversation of a touch, rather than a
	// click.
	Touch bool

	ctx    *Context
	thread *script.Thread
}

// Limits of conversation steps.
const (
	// MaxLines is the most lines one say call may show.
	MaxLines = 64
	// MaxLine is the longest line, in bytes.
	MaxLine = 1024
	// MaxOptions is the most options choose may offer.
	MaxOptions = 64
	// MaxTextInput is the most characters ask_text may ask for.
	MaxTextInput = 255
	// MaxWait is the longest wait or progress bar, in milliseconds.
	MaxWait = 600_000
)

// NPC is an NPC instance that a zone places: its handle, and its entry
// script as <mod>:<path>.
type NPC struct {
	ID    int64
	Entry string
	ctx   *Context
	rt    *Runtime
}

func (r *Runtime) npcHost() (NPCHost, error) {
	h, ok := r.host.(NPCHost)
	if !ok {
		return nil, errors.New("NPC scripts do not run here")
	}
	return h, nil
}

// StartNPC runs an NPC's entry script for one NPC instance. id is the
// NPC's handle, which the NPCHost's NPCField reads. The NPC keeps its
// context until ReloadNPCs or Close.
func (r *Runtime) StartNPC(id int64, entry string) (*NPC, error) {
	n := &NPC{ID: id, Entry: entry, rt: r}
	if r.OffEntry(entry) {
		// The NPC starts when its mod is turned on (switches.go).
		r.npcs = append(r.npcs, n)
		return n, nil
	}
	if err := r.startNPC(n); err != nil {
		return nil, err
	}
	r.npcs = append(r.npcs, n)
	return n, nil
}

func (r *Runtime) startNPC(n *NPC) error {
	setup, mod, err := r.setup(n.Entry)
	if err != nil {
		return err
	}
	sc := r.scripts[mod]
	c := &Context{ID: r.newID(), Kind: KindNPC, Mod: mod, grants: sc.Grants, timers: map[int64]*timer{}, npc: n.ID}
	c.Instance = script.NewTable(script.KV("npc", script.Int(n.ID)))
	n.ctx = c
	if err := r.runSetup(c, setup); err != nil {
		r.end(c)
		return fmt.Errorf("%s: setup: %w", n.Entry, err)
	}
	return nil
}

// Context returns the NPC's context, or nil when its script failed.
func (n *NPC) Context() *Context {
	if n.ctx == nil || n.ctx.ended || n.ctx.retiring || n.rt != nil && n.rt.off(n.ctx) {
		return nil
	}
	return n.ctx
}

// Clickable reports whether the NPC has a click handler.
func (n *NPC) Clickable() bool {
	c := n.Context()
	if c == nil {
		return false
	}
	for _, h := range c.handlers {
		if h.event == "click" && !h.removed {
			return true
		}
	}
	return false
}

// Click starts a conversation between a player and an NPC: its click
// handlers run in a new thread until they wait for the player or end.
// It returns nil when the NPC has no click handler.
func (r *Runtime) Click(n *NPC, player int64) (*Conversation, error) {
	c := n.Context()
	if c == nil {
		return nil, nil
	}
	var vals []script.Value
	for _, h := range c.handlers {
		if h.event == "click" && !h.removed {
			vals = append(vals, h.fn.Value())
		}
	}
	if len(vals) == 0 {
		return nil, nil
	}
	return r.converse(n, c, player, false, vals)
}

// converse runs handlers in a new conversation's thread, with the
// player and the conversation.
func (r *Runtime) converse(n *NPC, c *Context, player int64, touch bool, vals []script.Value) (*Conversation, error) {
	th, err := r.s.NewThread(r.fns.converse.Value())
	if err != nil {
		return nil, err
	}
	conv := &Conversation{ID: r.newID(), Player: player, NPC: n.ID, Touch: touch, ctx: c, thread: th}
	if c.conversations == nil {
		c.conversations = map[int64]*Conversation{}
	}
	c.conversations[conv.ID] = conv
	r.conversations[conv.ID] = conv
	args := append([]script.Value{script.Int(player), script.Int(conv.ID)}, vals...)
	r.resume(conv, args...)
	return conv, nil
}

// Touch runs an NPC's touch or untouch handlers with a player (review
// decisions 55 and 56): event is "touch" when the player steps into the
// NPC's touch area, and "untouch" when it leaves it. They run as a click
// does, since a player triggers them: a mod over its CPU budget still
// runs them.
//
// The touch handlers run in a conversation with the player, as a
// click's, so they may show a dialog as rAthena's OnTouch. Touch returns
// it, or nil when the NPC has no touch handler. The untouch handlers get
// no conversation.
func (r *Runtime) Touch(n *NPC, event string, player int64) (*Conversation, error) {
	c := n.Context()
	if c == nil {
		return nil, nil
	}
	if event == "touch" {
		var vals []script.Value
		for _, h := range r.handlers[event] {
			if h.ctx == c && !h.removed {
				vals = append(vals, h.fn.Value())
			}
		}
		if len(vals) == 0 {
			return nil, nil
		}
		return r.converse(n, c, player, true, vals)
	}
	for _, h := range slices.Clone(r.handlers[event]) {
		if h.ctx != c || h.removed || c.ended {
			continue
		}
		if err := r.run(c, r.fns.invoke.Value(), h.fn.Value(), script.String("player"), script.Int(player)); err != nil {
			r.failed(c, "event "+event, err)
		}
	}
	return nil, nil
}

// ResumeConversation goes on with a paused conversation, with the answer
// to its last step: nothing for next, close, wait and progress, the
// option from 1 for choose, the number, or the text.
func (r *Runtime) ResumeConversation(conv *Conversation, answer script.Value) {
	if conv.Done {
		return
	}
	r.resume(conv, script.String("ok"), answer)
}

// CloseConversation ends a conversation without its script, such as
// when the player leaves or a timeout passes. The pending call raises
// an error, which the script may catch, and the thread runs until it
// ends.
func (r *Runtime) CloseConversation(conv *Conversation) {
	if conv.Done {
		return
	}
	conv.Closed, conv.Cancelled = true, true
	r.resume(conv, script.String("closed"))
	if !conv.Done {
		// A script that caught the error and waits again cannot go on.
		_ = conv.thread.Close()
		r.finishConversation(conv)
	}
}

func (r *Runtime) resume(conv *Conversation, args ...script.Value) {
	prev := r.current
	r.setCurrent(conv.ctx)
	_, done, err := conv.thread.Resume(args...)
	r.setCurrent(prev)
	if err != nil {
		r.afterError()
		if !strings.Contains(err.Error(), r.closed) {
			what := "click"
			if conv.Touch {
				what = "event touch"
			}
			r.failed(conv.ctx, what, err)
		}
	}
	if done || err != nil {
		r.finishConversation(conv)
	}
}

func (r *Runtime) finishConversation(conv *Conversation) {
	if conv.Done {
		return
	}
	conv.Done = true
	conv.Closed = true
	delete(r.conversations, conv.ID)
	c := conv.ctx
	delete(c.conversations, conv.ID)
	if c.retiring && len(c.conversations) == 0 {
		c.retiring = false
		r.end(c)
	}
	if h, ok := r.host.(NPCHost); ok {
		h.ConversationEnded(conv)
	}
}

// ReloadNPCs starts the NPCs of a mod again with its current scripts,
// after Reload. Conversations that run finish on the old version.
func (r *Runtime) reloadNPCs(mod string) error {
	var errs []error
	for _, n := range r.npcs {
		m, _, _ := strings.Cut(n.Entry, ":")
		if m != mod {
			continue
		}
		if n.ctx != nil {
			r.end(n.ctx)
		}
		if err := r.startNPC(n); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// conversation finds the conversation a handle names. It must be a
// conversation of the running context.
func (r *Runtime) conversation(c *script.Call, what string) (*Conversation, NPCHost, error) {
	h, err := r.npcHost()
	if err != nil {
		return nil, nil, err
	}
	conv := r.conversations[c.Int(0)]
	if conv == nil {
		return nil, nil, fmt.Errorf("%s: the conversation has ended", what)
	}
	if conv.Closed {
		return nil, nil, fmt.Errorf("%s: the conversation was closed", what)
	}
	return conv, h, nil
}

// canWait checks that a call may pause the conversation's thread.
func (r *Runtime) canWait(c *script.Call, what string, conv *Conversation) error {
	if !c.CanYield() || r.current != conv.ctx {
		return fmt.Errorf("%s waits for the player, so it runs only in the conversation's click handler, not in a timer or inside table.sort", what)
	}
	return nil
}

// wait shows a step that pauses, and pauses the thread.
func (r *Runtime) wait(c *script.Call, what string, conv *Conversation, h NPCHost, s Step) error {
	if err := r.canWait(c, what, conv); err != nil {
		return err
	}
	if err := h.Converse(conv, s); err != nil {
		return err
	}
	return c.Yield()
}

func checkLine(what string, v script.Value) (string, error) {
	var s string
	if n, ok := v.AsInt(); ok {
		s = fmt.Sprint(n)
	} else if f, ok := v.AsFloat(); ok {
		s = fmt.Sprint(f)
	} else if s, ok = v.AsString(); !ok {
		return "", fmt.Errorf("%s: lines are strings", what)
	}
	if len(s) > MaxLine || !utf8.ValidString(s) {
		return "", fmt.Errorf("%s: a line is at most %d bytes of UTF-8", what, MaxLine)
	}
	return s, nil
}

func (r *Runtime) conversationSay(c *script.Call) error {
	conv, h, err := r.conversation(c, "conversation:say")
	if err != nil {
		return err
	}
	n := c.NArgs() - 1
	if n < 1 || n > MaxLines {
		return fmt.Errorf("conversation:say: give 1 to %d lines", MaxLines)
	}
	lines := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		s, err := checkLine("conversation:say", c.Arg(i))
		if err != nil {
			return err
		}
		lines = append(lines, s)
	}
	return h.Converse(conv, Step{Kind: StepSay, Lines: lines})
}

func (r *Runtime) conversationNext(c *script.Call) error {
	conv, h, err := r.conversation(c, "conversation:wait_for_next")
	if err != nil {
		return err
	}
	return r.wait(c, "conversation:wait_for_next", conv, h, Step{Kind: StepNext})
}

func (r *Runtime) conversationChoose(c *script.Call) error {
	conv, h, err := r.conversation(c, "conversation:choose")
	if err != nil {
		return err
	}
	n := c.NArgs() - 1
	if n < 1 || n > MaxOptions {
		return fmt.Errorf("conversation:choose: give 1 to %d options", MaxOptions)
	}
	opts := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		s, err := checkLine("conversation:choose", c.Arg(i))
		if err != nil {
			return err
		}
		opts = append(opts, s)
	}
	return r.wait(c, "conversation:choose", conv, h, Step{Kind: StepMenu, Options: opts})
}

func (r *Runtime) conversationAskNumber(c *script.Call) error {
	conv, h, err := r.conversation(c, "conversation:ask_number")
	if err != nil {
		return err
	}
	lo, ok1 := c.Arg(1).AsInt()
	hi, ok2 := c.Arg(2).AsInt()
	if !ok1 || !ok2 || lo > hi || lo < -1<<31 || hi > 1<<31-1 {
		return errors.New("conversation:ask_number: want a minimum and a maximum, from -2147483648 to 2147483647")
	}
	return r.wait(c, "conversation:ask_number", conv, h, Step{Kind: StepNumber, Min: lo, Max: hi})
}

func (r *Runtime) conversationAskText(c *script.Call) error {
	conv, h, err := r.conversation(c, "conversation:ask_text")
	if err != nil {
		return err
	}
	n, ok := c.Arg(1).AsInt()
	if !ok || n < 1 || n > MaxTextInput {
		return fmt.Errorf("conversation:ask_text: the maximum length is 1 to %d characters", MaxTextInput)
	}
	return r.wait(c, "conversation:ask_text", conv, h, Step{Kind: StepText, MaxLength: int(n)})
}

func (r *Runtime) conversationClose(c *script.Call) error {
	conv, h, err := r.conversation(c, "conversation:close")
	if err != nil {
		return err
	}
	if err := r.canWait(c, "conversation:close", conv); err != nil {
		return err
	}
	// The dialog is over once the player presses Close: later dialog
	// calls fail, and the script goes on.
	conv.Closed = true
	return r.wait(c, "conversation:close", conv, h, Step{Kind: StepClose})
}

func (r *Runtime) conversationWait(c *script.Call) error {
	conv, h, err := r.conversation(c, "conversation:wait")
	if err != nil {
		return err
	}
	ms, ok := c.Arg(1).AsInt()
	if !ok || ms < 0 || ms > MaxWait {
		return fmt.Errorf("conversation:wait: milliseconds are 0 to %d", MaxWait)
	}
	return r.wait(c, "conversation:wait", conv, h, Step{Kind: StepWait, Ms: ms})
}

func (r *Runtime) conversationProgress(c *script.Call) error {
	conv, h, err := r.conversation(c, "conversation:show_progress")
	if err != nil {
		return err
	}
	s, ok := c.Arg(1).AsInt()
	if !ok || s < 1 || s*1000 > MaxWait {
		return fmt.Errorf("conversation:show_progress: seconds are 1 to %d", MaxWait/1000)
	}
	return r.wait(c, "conversation:show_progress", conv, h, Step{Kind: StepProgress, Ms: s * 1000})
}

// imagePositions are the positions of show_image.
var imagePositions = map[string]bool{"left": true, "center": true, "right": true}

func (r *Runtime) conversationImage(c *script.Call) error {
	conv, h, err := r.conversation(c, "conversation:show_image")
	if err != nil {
		return err
	}
	name, ok := c.Arg(1).AsString()
	pos, ok2 := c.Arg(2).AsString()
	if !ok || !ok2 || !imagePositions[pos] {
		return errors.New("conversation:show_image: want an image name, such as kafra_01, and left, center or right")
	}
	if len(name) > 64 || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("conversation:show_image: %q is not an image name", name)
	}
	return h.Converse(conv, Step{Kind: StepImage, Image: name, Position: pos})
}

// npcID reads an NPC handle.
func npcID(c *script.Call) (int64, error) {
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return 0, errors.New("not an NPC: call the method with a colon, such as npc:say_aloud(text)")
	}
	return id, nil
}

func (r *Runtime) npcField(c *script.Call) error {
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	id, err := npcID(c)
	if err != nil {
		return err
	}
	v, err := h.NPCField(id, c.String(1))
	if err != nil {
		return err
	}
	c.Return(v)
	return nil
}

func (r *Runtime) npcEmotion(c *script.Call) error {
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	id, err := npcID(c)
	if err != nil {
		return err
	}
	if err := r.mutating("npc:show_emotion"); err != nil {
		return err
	}
	e, ok := c.Arg(1).AsInt()
	if !ok || e < 0 || e > 255 {
		return errors.New("npc:show_emotion: want an emotion from wa.emotions")
	}
	return h.NPCEmotion(id, e)
}

func (r *Runtime) npcSay(c *script.Call) error {
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	id, err := npcID(c)
	if err != nil {
		return err
	}
	if err := r.mutating("npc:say_aloud"); err != nil {
		return err
	}
	text, ok := c.Arg(1).AsString()
	if !ok || text == "" || len(text) > MaxMessage || !utf8.ValidString(text) {
		return fmt.Errorf("npc:say_aloud: the text must be 1 to %d bytes", MaxMessage)
	}
	return h.NPCSay(id, text)
}

// Conversations returns how many conversations have not ended, for
// tests.
func (r *Runtime) Conversations() int { return len(r.conversations) }
