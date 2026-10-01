package script

import (
	"errors"
	"strings"
	"testing"
)

func modState(t *testing.T, mods map[string]string) (*State, map[string]int) {
	t.Helper()
	asked := map[string]int{}
	s := newState(t, Options{Modules: func(name string) (string, bool) {
		asked[name]++
		src, ok := mods[name]
		return src, ok
	}})
	return s, asked
}

func TestRequire(t *testing.T) {
	s, asked := modState(t, map[string]string{
		"mymod.util":   `local M = {} M.loads = (M.loads or 0) + 1 function M.twice(x) return 2 * x end return M`,
		"mymod.name":   `return ...`,
		"mymod.nil":    `x_loaded = true`,
		"mymod.a":      `return require("mymod.b")`,
		"mymod.b":      `return require("mymod.a")`,
		"mymod.self":   `return require("mymod.self")`,
		"mymod.broken": `return 1 +`,
		"mymod.fails":  `fails = (fails or 0) + 1 error("module failed")`,
		"mymod.hog":    `local t = {} for i = 1, 1e9 do t[i] = {i} end`,
	})
	t.Run("load once", func(t *testing.T) {
		n := runInt(t, s, `local a = require("mymod.util") local b = require("mymod.util")
			assert(a == b) return a.twice(21) + a.loads - 1`)
		if n != 42 || asked["mymod.util"] != 1 {
			t.Errorf("got %d, asked %d times", n, asked["mymod.util"])
		}
		run(t, s, `assert(require("mymod.util").loads == 1)`)
		if asked["mymod.util"] != 1 {
			t.Errorf("asked %d times", asked["mymod.util"])
		}
	})
	t.Run("name argument", func(t *testing.T) {
		if res := run(t, s, `return require("mymod.name")`); res[0].String() != `"mymod.name"` {
			t.Errorf("got %v", res)
		}
	})
	t.Run("nil becomes true", func(t *testing.T) {
		if res := run(t, s, `return require("mymod.nil"), x_loaded`); !res[0].Truthy() || !res[1].Truthy() {
			t.Errorf("got %v", res)
		}
	})
	for _, tc := range []struct{ name, src, want string }{
		{"missing", `require("other.mod")`, "module 'other.mod' not found"},
		{"cycle", `require("mymod.a")`, "requires itself while it loads"},
		{"self", `require("mymod.self")`, "requires itself while it loads"},
		{"syntax error", `require("mymod.broken")`, "mymod.broken:1:"},
		{"not a string", `require({})`, "bad argument #1 to 'require'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Load(tc.name, tc.src)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
	t.Run("failed module loads again", func(t *testing.T) {
		for range 2 {
			if _, err := s.Load("f", `require("mymod.fails")`); err == nil || !strings.Contains(err.Error(), "module failed") {
				t.Fatalf("got %v", err)
			}
		}
		if n := runInt(t, s, `return fails`); n != 2 {
			t.Errorf("ran %d times", n)
		}
	})
	t.Run("limit inside a module", func(t *testing.T) {
		_, err := s.Load("h", `pcall(require, "mymod.hog") return 1`)
		if !errors.Is(err, ErrMemoryLimit) {
			t.Fatalf("got %v", err)
		}
		// The module is not left marked as loading.
		_, err = s.Load("h", `require("mymod.hog")`)
		if !errors.Is(err, ErrMemoryLimit) {
			t.Fatalf("second time: %v", err)
		}
	})
	t.Run("no modules", func(t *testing.T) {
		s := newState(t, Options{})
		if _, err := s.Load("x", `require("a")`); err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("got %v", err)
		}
	})
}
