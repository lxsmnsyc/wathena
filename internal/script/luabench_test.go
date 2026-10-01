package script

import (
	"fmt"
	"strings"
	"testing"
)

// The workloads of tools/luabench, which decision 0002 measured. The Lua
// sources are copied from tools/luabench/scripts.go, which is a separate
// module.

const benchPrelude = `
bLuk = 5
bFlee2 = 50
bBaseAtk = 17
BF_WEAPON = 1
BF_MAGIC = 2
`

const benchWorkloads = `
function static_equip(_)
  wa_add(bLuk, 2)
  wa_add(bFlee2, 1)
  return 0
end

function computed(_)
  local lv = pc_get("base_level")
  return lv > 70 and (lv - 70) // 10 * 10 or 0
end

function hit_taken(flag)
  if flag & (BF_WEAPON | BF_MAGIC) ~= 0 and wa_chance(3, 1000) then
    wa_add(bFlee2, 30)
    return 1
  end
  return 0
end

function compute(n)
  local t = {}
  for i = 1, n do t[i] = (i * 7919) % 1000 end
  table.sort(t)
  local s = 0
  for i = 1, n do s = s + t[i] // 3 end
  local parts = {}
  for i = 1, 20 do parts[#parts + 1] = string.format("%d", t[i]) end
  return s + #table.concat(parts, ",")
end
`

// benchNPC is the prototype's dialog: Lua coroutines that Go drives through
// npc_start and npc_step.
const benchNPC = `
local co
local function mes(_) coroutine.yield(1) end
local function menu(_, _) return coroutine.yield(2) end
local function healer()
  mes("[Healer]")
  mes("Do you want to be healed for 500 zeny?")
  if menu("Yes", "No") == 1 then
    local zeny = pc_get("zeny")
    if zeny < 500 then
      mes("You do not have enough zeny.")
      return
    end
    pc_set("zeny", zeny - 500)
    mes("Done.")
  end
end
local function status(ok, w)
  if not ok then error(w) end
  if coroutine.status(co) == "dead" then return 0 end
  return w
end
function npc_start(_)
  co = coroutine.create(healer)
  return status(coroutine.resume(co))
end
function npc_step(answer)
  return status(coroutine.resume(co, answer))
end
`

// benchNPCHost is the same dialog as the server will run it: a thread that
// Go starts, with mes and menu as host functions that pause it.
const benchNPCHost = `
function healer_host()
  npc_mes("[Healer]")
  npc_mes("Do you want to be healed for 500 zeny?")
  if npc_menu("Yes", "No") == 1 then
    local zeny = pc_get("zeny")
    if zeny < 500 then
      npc_mes("You do not have enough zeny.")
      return
    end
    pc_set("zeny", zeny - 500)
    npc_mes("Done.")
  end
end
`

const benchSandbox = `
function spin(_)
  while true do end
end
function hog(_)
  local t = {}
  for i = 1, 1e9 do t[i] = {i} end
  return 0
end
`

func benchItemScripts(n int) []string {
	stats := []string{"bLuk", "bFlee2", "bBaseAtk"}
	out := make([]string, n)
	for i := range out {
		var b strings.Builder
		fmt.Fprintf(&b, "items = items or {}\nitems[%d] = {\n", 4000+i)
		b.WriteString("  on_equip = function(pc, it, mods)\n")
		for j := range 1 + i%3 {
			fmt.Fprintf(&b, "    wa_add(%s, %d)\n", stats[(i+j)%3], 1+(i*7+j)%10)
		}
		b.WriteString("  end,\n")
		if i%4 == 0 {
			fmt.Fprintf(&b, "  on_use = function(pc, it)\n    if pc_get(\"base_level\") > %d then wa_add(bLuk, 1) end\n  end,\n", i%99)
		}
		b.WriteString("}\n")
		out[i] = b.String()
	}
	return out
}

func benchUtilModule() string {
	var b strings.Builder
	b.WriteString("local M = {}\n")
	b.WriteString("function M.clamp(v, lo, hi) if v < lo then return lo elseif v > hi then return hi end return v end\n")
	b.WriteString("function M.scale(v, lv) return M.clamp(v + math.floor(lv / 10), 1, 99) end\n")
	b.WriteString("function M.percent(v, p) return math.floor(v * p / 100) end\n")
	for i := 1; i <= 48; i++ {
		fmt.Fprintf(&b, "function M.f%d(a, b) return M.clamp(a * %d + b, 0, 1000) end\n", i, i)
	}
	b.WriteString("return M\n")
	return b.String()
}

// The prototype loaded modules through a require written in Lua. Here the
// runtime's require reads them from Options.Modules.
var benchModules = map[string]string{"mymod.util": benchUtilModule()}

const benchModSetup = benchPrelude + `
function equip_item(id) items[id].on_equip(nil, nil, nil) return 0 end
`

func benchModItems(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf(`local util = require("mymod.util")
items = items or {}
items[%d] = {
  on_equip = function(pc, it, mods)
    wa_add(bLuk, util.scale(%d, pc_get("base_level")))
  end,
}
`, 4000+i, 1+i%10)
	}
	return out
}

var (
	benchItems    = benchItemScripts(1000)
	benchModItemN = benchModItems(200)
)

// benchHost is the game state the workloads read and change, as in
// tools/luabench/engine.go.
type benchHost struct {
	mods, zeny, baseLevel, rolls int64
}

func newBenchHost() *benchHost { return &benchHost{zeny: 1 << 40, baseLevel: 95} }

func (h *benchHost) register(t testing.TB, s *State) {
	t.Helper()
	must(t, s.Register("wa_add", func(c *Call) error {
		h.mods += c.Int(1)
		return nil
	}))
	must(t, s.Register("pc_get", func(c *Call) error {
		switch c.String(0) {
		case "zeny":
			c.Return(Int(h.zeny))
		case "base_level":
			c.Return(Int(h.baseLevel))
		default:
			c.Return(Int(0))
		}
		return nil
	}))
	must(t, s.Register("pc_set", func(c *Call) error {
		if c.String(0) == "zeny" {
			h.zeny = c.Int(1)
		}
		return nil
	}))
	must(t, s.Register("wa_chance", func(c *Call) error {
		h.rolls++
		c.Return(Bool(h.rolls%c.Int(1) < c.Int(0)))
		return nil
	}))
	must(t, s.Register("npc_mes", func(c *Call) error {
		return c.Yield(Int(1))
	}))
	must(t, s.Register("npc_menu", func(c *Call) error {
		return c.Yield(Int(2))
	}))
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newBenchState(t testing.TB, h *benchHost) *State {
	t.Helper()
	s, err := New(Options{Modules: func(name string) (string, bool) {
		src, ok := benchModules[name]
		return src, ok
	}})
	must(t, err)
	h.register(t, s)
	return s
}

// setupBench creates a state with the workloads and one mod loaded, as
// luabench's setup does.
func setupBench(t testing.TB, h *benchHost) *State {
	t.Helper()
	s := newBenchState(t, h)
	_, err := s.Load("workloads", benchPrelude+benchWorkloads+benchNPC+benchNPCHost+benchSandbox)
	must(t, err)
	must(t, loadBenchMod(s))
	return s
}

func loadBenchMod(s *State) error {
	if _, err := s.Load("mod", benchModSetup); err != nil {
		return err
	}
	for i, src := range benchModItemN {
		if _, err := s.Load(fmt.Sprintf("moditem%d", i), src); err != nil {
			return err
		}
	}
	return nil
}

func loadBenchItems(s *State) error {
	for i, src := range benchItems {
		if _, err := s.Load(fmt.Sprintf("item%d", i), src); err != nil {
			return err
		}
	}
	return nil
}

func callInt(t testing.TB, s *State, fn Value, arg int64) int64 {
	v, err := s.Call1(fn, Int(arg))
	if err != nil {
		t.Fatal(err)
	}
	n, _ := v.AsInt()
	return n
}

// dialog runs the prototype's dialog to the end, answering 1 to the menu.
func dialog(t testing.TB, s *State, start, step Value) {
	w := callInt(t, s, start, 0)
	for w != 0 {
		w = callInt(t, s, step, 1)
	}
}

// hostDialog runs the same dialog in a thread that host functions pause.
func hostDialog(t testing.TB, s *State, fn Value) {
	th, err := s.NewThread(fn)
	must(t, err)
	vals, done, err := th.Resume()
	for !done {
		must(t, err)
		w, _ := vals[0].AsInt()
		if w == 2 {
			vals, done, err = th.Resume(Int(1))
		} else {
			vals, done, err = th.Resume()
		}
	}
	must(t, err)
}

func global(t testing.TB, s *State, name string) Value {
	t.Helper()
	v, err := s.Global(name)
	must(t, err)
	if v.Kind() != KindFunction {
		t.Fatalf("%s is %s", name, v.Kind())
	}
	r, err := s.Keep(v)
	must(t, err)
	return r.Value()
}

// TestLuabenchResults runs luabench's verify and checks that the results
// match what every engine gave in decision 0002.
func TestLuabenchResults(t *testing.T) {
	h := newBenchHost()
	s := setupBench(t, h)
	defer s.Close()
	var sum int64
	for _, fn := range []struct {
		name string
		arg  int64
	}{{"static_equip", 0}, {"computed", 0}, {"hit_taken", 1}, {"hit_taken", 0}, {"equip_item", 4000}, {"equip_item", 4199}, {"compute", 1000}} {
		f := global(t, s, fn.name)
		for range 2000 {
			sum += callInt(t, s, f, fn.arg)
		}
	}
	start, step := global(t, s, "npc_start"), global(t, s, "npc_step")
	for range 100 {
		dialog(t, s, start, step)
	}
	got := fmt.Sprintf("sum=%d mods=%d zeny=%d", sum, h.mods, h.zeny)
	// From tools/luabench's verify, run with the wazero engine. Decision
	// 0002 found every engine gives the same.
	const want = "sum=332472006 mods=64180 zeny=1099511577776"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}

	// The host-paused dialog changes zeny the same way.
	h2 := newBenchHost()
	s2 := setupBench(t, h2)
	defer s2.Close()
	healer := global(t, s2, "healer_host")
	for range 100 {
		hostDialog(t, s2, healer)
	}
	if h2.zeny != h.zeny {
		t.Errorf("host dialog left zeny %d, want %d", h2.zeny, h.zeny)
	}
}

func benchCall(b *testing.B, name string, arg int64) {
	s := setupBench(b, newBenchHost())
	defer s.Close()
	f := global(b, s, name)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.Call1(f, Int(arg)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStaticEquip(b *testing.B)      { benchCall(b, "static_equip", 0) }
func BenchmarkComputed(b *testing.B)         { benchCall(b, "computed", 0) }
func BenchmarkHitTaken(b *testing.B)         { benchCall(b, "hit_taken", 1) }
func BenchmarkEquipWithRequire(b *testing.B) { benchCall(b, "equip_item", 4000) }
func BenchmarkCompute1000(b *testing.B)      { benchCall(b, "compute", 1000) }

func BenchmarkNPCDialog(b *testing.B) {
	s := setupBench(b, newBenchHost())
	defer s.Close()
	start, step := global(b, s, "npc_start"), global(b, s, "npc_step")
	b.ReportAllocs()
	for b.Loop() {
		dialog(b, s, start, step)
	}
}

func BenchmarkNPCDialogHost(b *testing.B) {
	s := setupBench(b, newBenchHost())
	defer s.Close()
	healer := global(b, s, "healer_host")
	b.ReportAllocs()
	for b.Loop() {
		hostDialog(b, s, healer)
	}
}

func BenchmarkNewState(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		setupBench(b, newBenchHost()).Close()
	}
}

func BenchmarkLoad1000Items(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		s := newBenchState(b, newBenchHost())
		must(b, loadBenchItems(s))
		s.Close()
	}
}

func BenchmarkModState200Items(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		s := newBenchState(b, newBenchHost())
		must(b, loadBenchMod(s))
		s.Close()
	}
}

// BenchmarkCallEmpty and BenchmarkHostCall measure the binding alone.
func BenchmarkCallEmpty(b *testing.B) {
	s := newBenchState(b, newBenchHost())
	defer s.Close()
	_, err := s.Load("empty", "function empty() end")
	must(b, err)
	f := global(b, s, "empty")
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.Call1(f); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHostCall(b *testing.B) {
	s := newBenchState(b, newBenchHost())
	defer s.Close()
	must(b, s.Register("nop", func(*Call) error { return nil }))
	_, err := s.Load("loop", "function loop(n) for _ = 1, n do nop() end end")
	must(b, err)
	f := global(b, s, "loop")
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.Call1(f, Int(1000)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/1000, "ns/hostcall")
}

// TestMemoryAfterLoad reports the state's memory after loading, as
// decision 0002 did.
func TestMemoryAfterLoad(t *testing.T) {
	for _, tc := range []struct {
		name string
		load func(*State) error
	}{{"1000_items", loadBenchItems}, {"mod_200_items", loadBenchMod}} {
		s := newBenchState(t, newBenchHost())
		must(t, tc.load(s))
		must(t, s.GC())
		t.Logf("%s: %d KB", tc.name, s.Memory()/1024)
		s.Close()
	}
}
