// Command luabench compares Lua runtimes on wAthena's script workloads.
//
// Pure Go runtimes:
//
//	go run .
//
// With the cgo engines, see README.md.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

const itemCount = 1000

// Result is one benchmark result.
type Result struct {
	Engine      string  `json:"engine"`
	Workload    string  `json:"workload"`
	NsPerOp     float64 `json:"ns_per_op"`
	AllocsPerOp int64   `json:"allocs_per_op"`
	BytesPerOp  int64   `json:"bytes_per_op"`
}

// Check is one sandbox or memory check.
type Check struct {
	Engine string `json:"engine"`
	Check  string `json:"check"`
	Result string `json:"result"`
}

type workload struct {
	name string
	// run does one operation on a loaded engine.
	run func(e Engine) error
	// fresh workloads create their own engines.
	fresh func(d EngineDef) error
}

func callN(fn string, arg int64) func(Engine) error {
	return func(e Engine) error {
		_, err := e.Call(fn, arg)
		return err
	}
}

// dialog runs the Healer dialog to the end, answering 1 to the menu.
func dialog(e Engine) error {
	w, err := e.Call("npc_start", 0)
	for err == nil && w != 0 {
		w, err = e.Call("npc_step", 1)
	}
	return err
}

// setup creates an engine with the workloads and one mod loaded.
func setup(d EngineDef, h *Host) (Engine, error) {
	e, err := d.New(h, d.Limits)
	if err != nil {
		return nil, err
	}
	err = e.Load("workloads", Script(d.Dialect))
	if err == nil {
		err = loadMod(e)
	}
	if err != nil {
		e.Close()
		return nil, err
	}
	return e, nil
}

const modItemCount = 200

var (
	items    = ItemScripts(itemCount)
	modItems = ModItems(modItemCount)
)

// loadItems loads the 1,000 plain item scripts.
func loadItems(e Engine) error {
	for i, src := range items {
		if err := e.Load(fmt.Sprintf("item%d", i), src); err != nil {
			return err
		}
	}
	return nil
}

// loadMod loads one mod: its setup and 200 items that require its module.
func loadMod(e Engine) error {
	if err := e.Load("mod", ModSetup()); err != nil {
		return err
	}
	for i, src := range modItems {
		if err := e.Load(fmt.Sprintf("moditem%d", i), src); err != nil {
			return err
		}
	}
	return nil
}

var workloads = []workload{
	{name: "static_equip", run: callN("static_equip", 0)},
	{name: "computed", run: callN("computed", 0)},
	{name: "hit_taken", run: callN("hit_taken", 1)},
	{name: "npc_dialog", run: dialog},
	{name: "equip_with_require", run: callN("equip_item", 4000)},
	{name: "compute_1000", run: callN("compute", 1000)},
	{name: "new_state", fresh: func(d EngineDef) error {
		e, err := setup(d, newHost())
		if err == nil {
			e.Close()
		}
		return err
	}},
	{name: "load_1000_items", fresh: fresh(loadItems)},
	{name: "mod_state_200_items", fresh: fresh(loadMod)},
}

// fresh returns a workload that creates an engine and loads into it.
func fresh(load func(Engine) error) func(EngineDef) error {
	return func(d EngineDef) error {
		e, err := d.New(newHost(), d.Limits)
		if err != nil {
			return err
		}
		defer e.Close()
		return load(e)
	}
}

// verify runs each workload a fixed number of times and returns a checksum
// of the results and the host state, so engines can be compared.
func verify(d EngineDef) (string, error) {
	h := newHost()
	e, err := setup(d, h)
	if err != nil {
		return "", err
	}
	defer e.Close()
	var sum int64
	for _, fn := range []struct {
		name string
		arg  int64
	}{{"static_equip", 0}, {"computed", 0}, {"hit_taken", 1}, {"hit_taken", 0}, {"equip_item", 4000}, {"equip_item", 4199}, {"compute", 1000}} {
		for range 2000 {
			v, err := e.Call(fn.name, fn.arg)
			if err != nil {
				return "", fmt.Errorf("%s: %w", fn.name, err)
			}
			sum += v
		}
	}
	for range 100 {
		if err := dialog(e); err != nil {
			return "", fmt.Errorf("npc_dialog: %w", err)
		}
	}
	return fmt.Sprintf("sum=%d mods=%d zeny=%d", sum, h.Mods, h.Zeny), nil
}

func bench(d EngineDef, w workload) (Result, error) {
	var ferr error
	res := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		if w.fresh != nil {
			for b.Loop() {
				if err := w.fresh(d); err != nil {
					ferr = err
					b.FailNow()
				}
			}
			return
		}
		e, err := setup(d, newHost())
		if err != nil {
			ferr = err
			b.FailNow()
		}
		defer e.Close()
		for b.Loop() {
			if err := w.run(e); err != nil {
				ferr = err
				b.FailNow()
			}
		}
	})
	if ferr != nil {
		return Result{}, fmt.Errorf("%s %s: %w", d.Name, w.name, ferr)
	}
	return Result{
		Engine:      d.Name,
		Workload:    w.name,
		NsPerOp:     float64(res.T.Nanoseconds()) / float64(res.N),
		AllocsPerOp: res.AllocsPerOp(),
		BytesPerOp:  res.AllocedBytesPerOp(),
	}, nil
}

// heapPerState measures Go heap growth for 20 engines after load.
// It does not see memory that C Lua allocates outside the Go heap.
func heapPerState(d EngineDef, load func(Engine) error) (uint64, error) {
	const n = 20
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	var es []Engine
	for range n {
		e, err := d.New(newHost(), d.Limits)
		if err != nil {
			return 0, err
		}
		if err := load(e); err != nil {
			return 0, err
		}
		es = append(es, e)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	for _, e := range es {
		e.Close()
	}
	return (after.HeapAlloc - before.HeapAlloc) / n, nil
}

// luaMemory reads collectgarbage("count") after load.
func luaMemory(d EngineDef, load func(Engine) error) (int64, error) {
	e, err := d.New(newHost(), d.Limits)
	if err != nil {
		return 0, err
	}
	defer e.Close()
	if err := load(e); err != nil {
		return 0, err
	}
	if err := e.Load("mem", `function mem_kb(_) collectgarbage() return math.floor(collectgarbage("count")) end`); err != nil {
		return 0, err
	}
	return e.Call("mem_kb", 0)
}

// stops calls fn under limits and reports whether and when it was stopped.
func stops(d EngineDef, fn string) string {
	if !d.Limits.On {
		return "no limits"
	}
	e, err := setup(d, newHost())
	if err != nil {
		return "setup failed: " + err.Error()
	}
	defer e.Close()
	start := time.Now()
	_, err = e.Call(fn, 0)
	if err == nil {
		return "not stopped"
	}
	return fmt.Sprintf("stopped after %s: %s", time.Since(start).Round(time.Millisecond), short(err.Error()))
}

// short returns the first line of an error message, cut to 80 bytes.
func short(msg string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	if len(msg) > 80 {
		msg = msg[:80]
	}
	return msg
}

func main() {
	jsonOut := flag.String("json", "", "write results to this JSON file")
	only := flag.String("engine", "", "run only engines whose name contains this text")
	flag.Parse()

	var results []Result
	var checks []Check
	var want string
	for _, d := range engines {
		if !strings.Contains(d.Name, *only) {
			continue
		}
		sum, err := verify(d)
		switch {
		case err != nil:
			checks = append(checks, Check{d.Name, "verify", "failed: " + short(err.Error())})
		case want == "":
			want = sum
		case sum != want:
			fmt.Fprintf(os.Stderr, "%s gives %s, want %s\n", d.Name, sum, want)
			os.Exit(1)
		}
		for _, w := range workloads {
			r, err := bench(d, w)
			if err != nil {
				checks = append(checks, Check{d.Name, w.name, "failed: " + short(err.Error())})
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			results = append(results, r)
			fmt.Fprintf(os.Stderr, "%-28s %-16s %12.0f ns/op %6d allocs/op\n", r.Engine, r.Workload, r.NsPerOp, r.AllocsPerOp)
		}
		for _, m := range []struct {
			name string
			load func(Engine) error
		}{{"1000_items", loadItems}, {"mod_200_items", loadMod}} {
			if heap, err := heapPerState(d, m.load); err == nil {
				checks = append(checks, Check{d.Name, "go_heap_" + m.name, fmt.Sprintf("%d KB", heap/1024)})
			}
			if kb, err := luaMemory(d, m.load); err == nil {
				checks = append(checks, Check{d.Name, "lua_count_" + m.name, fmt.Sprintf("%d KB", kb)})
			}
		}
		checks = append(checks, Check{d.Name, "infinite_loop", stops(d, "spin")})
		// Only golua has a memory quota. The others would use all memory.
		if d.Limits.On && d.MemoryQuota {
			checks = append(checks, Check{d.Name, "memory_hog", stops(d, "hog")})
		}
	}

	printMarkdown(results, checks)
	if *jsonOut != "" {
		data, _ := json.MarshalIndent(map[string]any{
			"go":      runtime.Version(),
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
			"cpus":    runtime.NumCPU(),
			"results": results,
			"checks":  checks,
		}, "", "\t")
		if err := os.WriteFile(*jsonOut, append(data, '\n'), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func printMarkdown(results []Result, checks []Check) {
	var names []string
	seen := map[string]bool{}
	for _, r := range results {
		if !seen[r.Engine] {
			seen[r.Engine] = true
			names = append(names, r.Engine)
		}
	}
	fmt.Print("| Workload |")
	for _, n := range names {
		fmt.Printf(" %s |", n)
	}
	fmt.Print("\n|---|")
	for range names {
		fmt.Print("---:|")
	}
	fmt.Println()
	for _, w := range workloads {
		fmt.Printf("| %s |", w.name)
		for _, n := range names {
			cell := "failed"
			for _, r := range results {
				if r.Engine == n && r.Workload == w.name {
					cell = formatNs(r.NsPerOp)
				}
			}
			fmt.Printf(" %s |", cell)
		}
		fmt.Println()
	}
	fmt.Println()
	fmt.Println("| Engine | Check | Result |")
	fmt.Println("|---|---|---|")
	for _, c := range checks {
		fmt.Printf("| %s | %s | %s |\n", c.Engine, c.Check, c.Result)
	}
}

func formatNs(ns float64) string {
	switch {
	case ns >= 1e6:
		return fmt.Sprintf("%.2f ms", ns/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.2f µs", ns/1e3)
	}
	return fmt.Sprintf("%.0f ns", ns)
}
