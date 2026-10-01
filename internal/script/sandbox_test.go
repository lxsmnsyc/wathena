package script

import (
	"errors"
	"strings"
	"testing"
)

// TestSandboxGlobals checks which globals scripts can reach.
func TestSandboxGlobals(t *testing.T) {
	s := newState(t, Options{})
	absent := []string{
		"io", "os", "debug", "package", "dofile", "loadfile", "print", "warn",
		"string.dump", "math.random", "math.randomseed", "loadstring", "setfenv",
		"getfenv", "module", "unpack",
	}
	for _, name := range absent {
		res := run(t, s, "return "+name)
		if !res[0].IsNil() {
			t.Errorf("%s is %v, want nil", name, res[0])
		}
	}
	present := []string{
		"assert", "error", "getmetatable", "ipairs", "next", "pairs", "pcall", "rawequal",
		"rawget", "rawlen", "rawset", "select", "setmetatable", "tonumber", "tostring",
		"type", "xpcall", "load", "require", "collectgarbage",
		"coroutine.create", "coroutine.resume", "coroutine.yield", "coroutine.wrap",
		"coroutine.status", "coroutine.close", "coroutine.running", "coroutine.isyieldable",
		"string.format", "string.find", "string.gsub", "string.rep", "string.pack",
		"table.concat", "table.insert", "table.sort", "table.unpack", "table.move",
		"math.floor", "math.maxinteger", "math.tointeger", "math.ult",
		"utf8.char", "utf8.len", "utf8.codes",
	}
	for _, name := range present {
		if res := run(t, s, "return "+name); res[0].IsNil() {
			t.Errorf("%s is nil", name)
		}
	}
}

// TestSandboxEscapes tries known ways out of a Lua sandbox.
func TestSandboxEscapes(t *testing.T) {
	s := newState(t, Options{})
	// A minimal Lua 5.4 binary chunk header. Any bytecode must be refused
	// before it is parsed.
	bytecode := `"\27Lua\84\0\25\147\13\10\26\10\4\8\8\120\86\0\0\0\0\0\0\0\0\0\0\0\40\119\64"`
	for _, tc := range []struct{ name, src, want string }{
		{"load bytecode", `return load(` + bytecode + `)`, "attempt to load a binary chunk"},
		{"load bytecode with mode b", `return load(` + bytecode + `, "x", "b")`, "only text chunks"},
		{"load bytecode with mode bt", `return load(` + bytecode + `, "x", "bt")`, "only text chunks"},
		{"load bytecode from a function", `local done return load(function() if done then return nil end done = true return ` + bytecode + ` end)`, "attempt to load a binary chunk"},
		{"string metatable", `return getmetatable("")`, "false"},
		{"string metatable change", `setmetatable("", {})`, "bad argument"},
		{"string methods are safe", `local ok = pcall(function() ("").dump = 1 end) return ("x"):upper()`, `"X"`},
		{"__gc", `setmetatable({}, {__gc = function() while true do end end})`, "__gc metamethods are not allowed"},
		{"collectgarbage collect", `collectgarbage()`, "only \"count\" is allowed"},
		{"collectgarbage stop", `collectgarbage("stop")`, "only \"count\" is allowed"},
		{"collectgarbage generational", `collectgarbage("generational")`, "only \"count\" is allowed"},
		{"collectgarbage count", `return collectgarbage("count") > 0`, "true"},
		{"require a file", `require("os")`, "module 'os' not found"},
		{"no registry", `return debug`, "nil"},
		{"_ENV is the globals", `return _ENV == _G`, "true"},
		{"load with an env", `return load("return x", "c", "t", {x = 5})()`, "5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := s.Load(tc.name, tc.src)
			var got string
			if err != nil {
				got = err.Error()
			} else {
				var parts []string
				for _, v := range res {
					parts = append(parts, v.String())
				}
				got = strings.Join(parts, " ")
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestLoadOnlyText checks State.Load refuses bytecode too.
func TestLoadOnlyText(t *testing.T) {
	s := newState(t, Options{})
	_, err := s.Load("bin", "\x1bLua\x54\x00")
	var e *Error
	if !errors.As(err, &e) || !e.Syntax || !strings.Contains(e.Message, "binary chunk") {
		t.Fatalf("got %v", err)
	}
}

// TestIntegerRules checks strategy section 5.5.
func TestIntegerRules(t *testing.T) {
	s := newState(t, Options{})
	for _, tc := range []struct{ src, want string }{
		{`return math.maxinteger + 1 == math.mininteger`, "true"},
		{`return math.mininteger - 1 == math.maxinteger`, "true"},
		{`return 7 // 2, -7 // 2, 7 // -2`, "3 -4 -4"},
		{`return 7 / 2, 4 / 2`, "3.5 2"},
		{`return math.type(4 / 2), math.type(4 // 2)`, `"float" "integer"`},
		{`return 7 % -3, -7 % 3`, "-2 2"},
		{`return 1 << 63, (1 << 63) >> 63`, "-9223372036854775808 1"},
		{`return 9007199254740993`, "9007199254740993"},
		{`return math.random`, "nil"},
	} {
		res := run(t, s, tc.src)
		var parts []string
		for _, v := range res {
			parts = append(parts, v.String())
		}
		if got := strings.Join(parts, " "); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
	// Integers pass to and from Go exactly.
	run(t, s, `function id(x) return x end`)
	for _, n := range []int64{0, -1, 1 << 53, 1<<53 + 1, -9223372036854775808, 9223372036854775807} {
		v, err := s.Call1(String("id"), Int(n))
		must(t, err)
		if got, ok := v.AsInt(); !ok || got != n || v.Kind() != KindInt {
			t.Errorf("id(%d) = %v", n, v)
		}
	}
}

// TestStringMetatableShared checks that one script cannot change string
// methods for others through the string metatable.
func TestStringMetatableShared(t *testing.T) {
	s := newState(t, Options{})
	_, err := s.Load("evil", `local mt = getmetatable("") mt.__index = {upper = function() return "pwned" end}`)
	if err == nil {
		t.Fatal("changing the string metatable worked")
	}
	if res := run(t, s, `return ("x"):upper()`); res[0].String() != `"X"` {
		t.Errorf("got %v", res[0])
	}
}
