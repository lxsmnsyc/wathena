package modscript

import (
	"errors"
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Tests of wactl mod test (mods/README.md, "Testing a mod"). With
// Options.Testing, the runtime has wa.testing. A test file is loaded in
// its mod's environment and returns a function that registers cases.
// Each case runs in a thread. Its world methods pause the thread, and
// the TestWorld does the work in Go: a host function cannot call into
// its own Lua state, and the work runs mod scripts.

// TestWorld does what a test case's world asks for: new_player, talk,
// use_item, equip, advance, spawn_monster and kill. args are the values the
// world method passed. A returned error fails the case at the line that
// called the method.
type TestWorld interface {
	Do(op string, args []script.Value) ([]script.Value, error)
}

// testingWorld pauses the test thread with a world request.
func testingWorld(c *script.Call) error {
	if !c.CanYield() {
		return errors.New("world methods only run inside a test case")
	}
	args := make([]script.Value, c.NArgs())
	for i := range args {
		args[i] = c.Arg(i)
	}
	return c.Yield(args...)
}

// LoadTests loads a test file in a mod's environment and registers its
// cases. path names the file in errors, such as tests/greeter_test.lua.
// It returns the names of the cases, in registration order.
func (r *Runtime) LoadTests(modID, path, source string) ([]string, error) {
	if !r.opts.Testing {
		return nil, errors.New("modscript: the runtime was made without Options.Testing")
	}
	if r.scripts[modID] == nil {
		return nil, fmt.Errorf("modscript: %s has no scripts in this runtime", modID)
	}
	r.setLoading(modID)
	v, err := r.s.Call1(r.fns.loadTests.Value(), script.String(modID), script.String(path), script.String(source))
	r.setLoading("")
	if err != nil {
		r.afterError()
		return nil, err
	}
	t, ok := v.AsTable()
	if !ok {
		return nil, errors.New("modscript: load_tests returned no list")
	}
	var names []string
	for i := int64(1); ; i++ {
		s, ok := t.Index(i).AsString()
		if !ok {
			break
		}
		names = append(names, s)
	}
	return names, nil
}

// RunCase runs case i, from 0, of the test file that loaded last. The
// case's code acts as the mod: its calls need the mod's permissions. It
// returns the error that failed the case, or nil.
func (r *Runtime) RunCase(modID string, i int, w TestWorld) error {
	th, err := r.s.NewThread(r.fns.runCase.Value())
	if err != nil {
		return err
	}
	resume := func(args ...script.Value) ([]script.Value, bool, error) {
		prev := r.loading
		r.setLoading(modID)
		vals, done, err := th.Resume(args...)
		r.setLoading(prev)
		return vals, done, err
	}
	vals, done, err := resume(script.Int(int64(i + 1)))
	for !done && err == nil {
		if len(vals) == 0 {
			_ = th.Close()
			return errors.New("the test case paused without a world request; do not call coroutine.yield in a test case")
		}
		op, _ := vals[0].AsString()
		// Values from a call are valid until the next call: copy what
		// the world reads before it runs scripts.
		res, opErr := w.Do(op, slices.Clone(vals[1:]))
		if opErr != nil {
			vals, done, err = resume(script.String("error"), script.String(opErr.Error()))
			continue
		}
		vals, done, err = resume(append([]script.Value{script.String("ok")}, res...)...)
	}
	if err != nil {
		r.afterError()
	}
	return err
}

// Describe lists the API as the runtime has it: the keys of each module,
// and the methods of each handle type, by name. The test of the API
// definitions compares them.
func (r *Runtime) Describe() (modules, handles map[string][]string, err error) {
	v, err := r.s.Call1(r.fns.describe.Value())
	if err != nil {
		return nil, nil, err
	}
	t, ok := v.AsTable()
	if !ok {
		return nil, nil, errors.New("modscript: describe returned no table")
	}
	read := func(key string) map[string][]string {
		out := map[string][]string{}
		inner, _ := t.Get(key).AsTable()
		if inner == nil {
			return out
		}
		for _, f := range inner.Fields {
			name, _ := f.Key.AsString()
			list, _ := f.Value.AsTable()
			var keys []string
			for i := int64(1); list != nil; i++ {
				s, ok := list.Index(i).AsString()
				if !ok {
					break
				}
				keys = append(keys, s)
			}
			slices.Sort(keys)
			out[name] = keys
		}
		return out
	}
	return read("modules"), read("handles"), nil
}

// CheckEntry loads an NPC or item entry script, as <mod>:<path>, and
// checks that it returns a setup function. Its setup does not run.
func (r *Runtime) CheckEntry(entry string) error {
	_, _, err := r.setup(entry)
	return err
}
