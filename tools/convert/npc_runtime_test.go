package convert

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// npcHost is testHost with NPCs: one NPC handle per placed NPC, a player
// with zeny, the last step each conversation waits on, and the monsters
// scripts spawn.
type npcHost struct {
	*testHost
	npcs  map[int64]*NPC
	steps map[int64]modscript.Step
	ended map[int64]bool
	vars  map[modscript.VariableKey]string
	zeny  int64
	// spawned are the monsters that have not died yet, by handle ID.
	spawned     map[int64]uint32
	nextMonster int64
	market      marketHost
	// questLog is the quest log of every player (npc_quest_test.go).
	questLog
}

// marketHost is the MarketHost of the NPC runtime tests: core's
// currencies with balances by name, core's shops by record name with
// the changes scripts make, and the mails scripts send.
type marketHost struct {
	balances map[string]int64
	// shops are the shops by record name, each with its items' stock,
	// -1 for no limit.
	shops map[string]map[uint32]int
	mails []modscript.ScriptMail
	calls []string
}

// Currency finds any currency: core's own and the converted ones are
// core's.
func (m *marketHost) Currency(string, string) (string, bool) { return "core", true }
func (m *marketHost) CurrencyBalance(_ int64, name, _ string) (int64, error) {
	return m.balances[name], nil
}
func (m *marketHost) ChangeCurrency(_ int64, name, _ string, n int64) (bool, error) {
	if m.balances == nil {
		m.balances = map[string]int64{}
	}
	if m.balances[name]+n < 0 {
		return false, nil
	}
	m.balances[name] += n
	return true, nil
}
func (m *marketHost) SendMail(_ int64, mail modscript.ScriptMail) error {
	m.mails = append(m.mails, mail)
	return nil
}
func (m *marketHost) OpenMail(int64) (bool, error) {
	m.calls = append(m.calls, "open_mail")
	return true, nil
}
func (m *marketHost) Autotrade(int64) (bool, error) { return false, nil }
func (m *marketHost) SearchStores(_ int64, uses int, open bool) (bool, error) {
	m.calls = append(m.calls, fmt.Sprintf("search_stores %d %t", uses, open))
	return true, nil
}
func (m *marketHost) OpenBuyingStore(_ int64, slots int) (bool, error) {
	m.calls = append(m.calls, fmt.Sprintf("open_buying_store %d", slots))
	return true, nil
}
func (m *marketHost) ShopFind(name, _ string) (string, bool) {
	name = strings.TrimPrefix(name, "core:")
	if _, ok := m.shops[name]; ok {
		return "core:" + name, true
	}
	return "", false
}
func (m *marketHost) shop(full string) map[uint32]int {
	return m.shops[strings.TrimPrefix(full, "core:")]
}
func (m *marketHost) ShopAdd(shop string, it modscript.ShopItemSpec, _ string) (bool, error) {
	m.shop(shop)[it.Item] = it.Stock
	return true, nil
}
func (m *marketHost) ShopRemove(shop string, item uint32) (bool, error) {
	s := m.shop(shop)
	_, ok := s[item]
	delete(s, item)
	return ok, nil
}
func (m *marketHost) ShopSetStock(shop string, item uint32, n int) (bool, error) {
	s := m.shop(shop)
	if _, ok := s[item]; !ok {
		return false, nil
	}
	s[item] = n
	return true, nil
}

func (h *npcHost) Currency(name, mod string) (string, bool) { return h.market.Currency(name, mod) }
func (h *npcHost) CurrencyBalance(p int64, name, mod string) (int64, error) {
	return h.market.CurrencyBalance(p, name, mod)
}
func (h *npcHost) ChangeCurrency(p int64, name, mod string, n int64) (bool, error) {
	return h.market.ChangeCurrency(p, name, mod, n)
}
func (h *npcHost) SendMail(p int64, m modscript.ScriptMail) error { return h.market.SendMail(p, m) }
func (h *npcHost) OpenMail(p int64) (bool, error)                 { return h.market.OpenMail(p) }
func (h *npcHost) Autotrade(p int64) (bool, error)                { return h.market.Autotrade(p) }
func (h *npcHost) SearchStores(p int64, uses int, open bool) (bool, error) {
	return h.market.SearchStores(p, uses, open)
}
func (h *npcHost) OpenBuyingStore(p int64, slots int) (bool, error) {
	return h.market.OpenBuyingStore(p, slots)
}
func (h *npcHost) ShopFind(name, mod string) (string, bool) { return h.market.ShopFind(name, mod) }
func (h *npcHost) ShopAdd(shop string, it modscript.ShopItemSpec, mod string) (bool, error) {
	return h.market.ShopAdd(shop, it, mod)
}
func (h *npcHost) ShopRemove(shop string, item uint32) (bool, error) {
	return h.market.ShopRemove(shop, item)
}
func (h *npcHost) ShopSetStock(shop string, item uint32, n int) (bool, error) {
	return h.market.ShopSetStock(shop, item, n)
}

// coreShops are the shops of a converted core, for marketHost: each
// with the items of its line and no stock limit. Their stock is not
// checked.
func coreShops(core *Core) map[string]map[uint32]int {
	out := map[string]map[uint32]int{}
	for _, n := range core.NPCs.NPCs {
		if !n.Shop || n.Status == None {
			continue
		}
		items := map[uint32]int{}
		if ref := core.NPCs.conv.shops[n.Source]; ref != nil {
			for id := range ref.items {
				items[id] = -1
			}
		}
		out[n.Name] = items
	}
	return out
}

func (h *npcHost) SpawnMonsters(_ int64, s modscript.SpawnSpec) ([]int64, error) {
	if h.spawned == nil {
		h.spawned = map[int64]uint32{}
	}
	ids := make([]int64, s.Amount)
	for i := range ids {
		h.nextMonster++
		ids[i] = h.nextMonster
		h.spawned[ids[i]] = s.Monster
	}
	return ids, nil
}
func (h *npcHost) Despawn(int64) {}

// testClock is the clock of the NPC runtime: schedules start from it.
var testClock = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func (h *npcHost) PlayerField(id int64, field string) (script.Value, error) {
	if field == "zeny" {
		return script.Int(h.zeny), nil
	}
	return h.testHost.PlayerField(id, field)
}

func (h *npcHost) NPCField(id int64, field string) (script.Value, error) {
	n := h.npcs[id]
	switch field {
	case "name":
		return script.String(displayName(n.Source)), nil
	case "map":
		return script.String(n.Map), nil
	case "x", "y":
		return script.Int(1), nil
	}
	return script.Nil, fmt.Errorf("npc has no field %q", field)
}
func (h *npcHost) NPCEmotion(int64, int64) error { return nil }
func (h *npcHost) NPCSay(int64, string) error    { return nil }
func (h *npcHost) Converse(c *modscript.Conversation, s modscript.Step) error {
	if s.Kind.Waits() {
		h.steps[c.ID] = s
	}
	return nil
}
func (h *npcHost) ConversationEnded(c *modscript.Conversation) { h.ended[c.ID] = true }
func (h *npcHost) PayZeny(_, amount int64) (bool, error) {
	if h.zeny < amount {
		return false, nil
	}
	h.zeny -= amount
	return true, nil
}
func (h *npcHost) GiveZeny(_, amount int64) error                         { h.zeny += amount; return nil }
func (h *npcHost) TakeItem(int64, uint32, int) (bool, error)              { return true, nil }
func (h *npcHost) CanCarry(int64, uint32, int) (bool, error)              { return true, nil }
func (h *npcHost) SetSavePoint(int64, string, int, int) (bool, error)     { return true, nil }
func (h *npcHost) MarkMinimap(int64, int64, int, int, uint32, bool) error { return nil }
func (h *npcHost) OpenShop(int64, string) error                           { return nil }
func (h *npcHost) Variable(_ int64, v modscript.VariableKey) (string, bool, error) {
	s, ok := h.vars[v]
	return s, ok, nil
}
func (h *npcHost) SetVariable(_ int64, v modscript.VariableKey, value string) error {
	if value == "" {
		delete(h.vars, v)
	} else {
		h.vars[v] = value
	}
	return nil
}
func (h *npcHost) ItemName(id uint32) (string, bool)     { return fmt.Sprintf("Item %d", id), true }
func (h *npcHost) Announce(string, string, string) error { return nil }

// The storage calls: storage opens, and the storages are empty.
func (h *npcHost) OpenStorage(int64, string, string) (bool, error) { return true, nil }
func (h *npcHost) OpenGuildStorageFor(int64) (bool, string, error) { return true, "", nil }
func (h *npcHost) StorageCount(int64, string, string, uint32) (int, bool, error) {
	return 0, true, nil
}
func (h *npcHost) StorageGive(int64, string, string, uint32, int, modscript.GiveRoll, item.Bound) (bool, error) {
	return true, nil
}
func (h *npcHost) StorageTake(int64, string, string, uint32, int) (bool, error) { return false, nil }
func (h *npcHost) GuildCountItem(int64, uint32) (int, error)                    { return 0, nil }

// logRecorder keeps the warnings a runtime logs, which are the errors of
// its handlers.
type logRecorder struct {
	mu   sync.Mutex
	msgs []string
}

func (l *logRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (l *logRecorder) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var err string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "err" {
			err = a.Value.String()
		}
		return true
	})
	l.msgs = append(l.msgs, r.Message+": "+err)
	return nil
}
func (l *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *logRecorder) WithGroup(string) slog.Handler      { return l }

// TestRunConvertedNPCs starts every placed NPC of both modes, in a
// runtime for each map, steps into and out of its touch area, and talks
// to it four times: answering each menu with its first option, its second,
// its third, and its last one, each number with its minimum, and text
// with "abc". Then every monster the NPCs spawned dies, killed by the
// player, and every schedule runs once. A part of a function that did
// not convert may fail when reached. Any other error fails the test.
func TestRunConvertedNPCs(t *testing.T) {
	testrace.SkipUnderRace(t, "it converts or runs all of core's scripts on one goroutine")
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			r, base, core := testRuntime(t, mode)
			_ = r
			logs := &logRecorder{}
			h := &npcHost{testHost: base, npcs: map[int64]*NPC{}, steps: map[int64]modscript.Step{}, ended: map[int64]bool{},
				vars: map[modscript.VariableKey]string{}, zeny: 1_000_000, market: marketHost{shops: coreShops(core)}}
			// Each map runs its NPCs in a runtime of its own, as each
			// zone does, so a mod's limits count per map.
			var maps []string
			byMap := map[string][]int{}
			for i, n := range core.NPCs.NPCs {
				if !n.Placed || n.Script == "" {
					continue
				}
				if byMap[n.Map] == nil {
					maps = append(maps, n.Map)
				}
				byMap[n.Map] = append(byMap[n.Map], i)
			}
			sc := coreScripts(t, core)
			quests := coreQuests(t, core)
			var started, clicks, steps, deaths, touches int
			for _, m := range maps {
				rt, err := modscript.New(modscript.Options{Host: h, Mods: []string{"core"}, Constants: core.Constants, Jobs: h.jobs,
					Log: slog.New(logs), Server: variables.NewServer(nil, nil), Quests: quests,
					Clock: func() time.Time { return testClock }, Location: time.UTC}, sc)
				if err != nil {
					t.Fatal(err)
				}
				for _, i := range byMap[m] {
					n := core.NPCs.NPCs[i]
					id := int64(i + 1)
					h.npcs[id] = n
					npc, err := rt.StartNPC(id, "core:"+n.Script)
					if err != nil {
						t.Errorf("%s: %v", n.Name, err)
						continue
					}
					started++
					if n.Touch {
						// The player steps into the touch area and out
						// again.
						touches++
						conv, err := rt.Touch(npc, "touch", 1)
						if err != nil {
							t.Errorf("%s: touch: %v", n.Name, err)
						}
						// A touch that shows a dialog waits in its
						// conversation, which closes as the player leaves.
						if conv != nil && !conv.Done {
							rt.CloseConversation(conv)
						}
						if _, err := rt.Touch(npc, "untouch", 1); err != nil {
							t.Errorf("%s: untouch: %v", n.Name, err)
						}
					}
					if !npc.Clickable() {
						continue
					}
					for _, pick := range []int{1, 2, 3, -1} {
						clicks++
						conv, err := rt.Click(npc, 1)
						if err != nil {
							t.Errorf("%s: click: %v", n.Name, err)
							break
						}
						for range 300 {
							if conv == nil || conv.Done {
								break
							}
							steps++
							s := h.steps[conv.ID]
							var answer script.Value
							switch s.Kind {
							case modscript.StepMenu:
								// Empty options are hidden, so the zone
								// refuses them.
								var shown []int
								for i, o := range s.Options {
									if o != "" {
										shown = append(shown, i+1)
									}
								}
								k := pick
								if k < 0 || k > len(shown) {
									k = len(shown)
								}
								answer = script.Int(int64(shown[k-1]))
							case modscript.StepNumber:
								answer = script.Int(s.Min)
							case modscript.StepText:
								answer = script.String("abc")
							}
							rt.ResumeConversation(conv, answer)
						}
						if conv != nil && !conv.Done {
							rt.CloseConversation(conv)
						}
					}
				}
				// Each spawned monster dies, killed by the player, and each
				// schedule runs once.
				for mapDeaths := 0; len(h.spawned) > 0 && mapDeaths < 10_000; {
					for _, id := range sortedIDs(h.spawned) {
						mob := h.spawned[id]
						delete(h.spawned, id)
						deaths++
						mapDeaths++
						rt.FireDeath(modscript.Death{Victim: id, VictimKind: "monster", Monster: int64(mob), Killer: 1, KillerKind: "player"})
					}
				}
				rt.RunSchedules(testClock.Add(8 * 24 * time.Hour))
				rt.Close()
			}
			bad := 0
			for _, m := range logs.msgs {
				if strings.Contains(m, "which wAthena does not have yet") {
					continue
				}
				bad++
				if bad <= 20 {
					t.Error(m)
				}
			}
			t.Logf("%d NPCs started, %d touched, %d clicks, %d steps, %d monster deaths, %d errors", started, touches, clicks, steps, deaths, bad)
		})
	}
}

func sortedIDs(m map[int64]uint32) []int64 {
	out := make([]int64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
