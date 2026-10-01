package script

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite csrc/lstrlib_charged.c from lua/src/lstrlib.c and the changes below")

// luaVersion is the pinned Lua release. lua/update.sh pins the tarball's
// SHA-256 and writes lua/SHA256SUMS from it.
const luaVersion = "5.4.9"

// TestVendoredLua checks that lua/src holds exactly the files in
// lua/SHA256SUMS, with those hashes, and that lua.h is the pinned version.
func TestVendoredLua(t *testing.T) {
	sums, err := os.ReadFile("lua/SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		want, name, ok := strings.Cut(sc.Text(), "  ")
		if !ok {
			t.Fatalf("bad line in SHA256SUMS: %q", sc.Text())
		}
		data, err := os.ReadFile(filepath.Join("lua/src", name))
		if err != nil {
			t.Fatal(err)
		}
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != want {
			t.Errorf("lua/src/%s does not match SHA256SUMS", name)
		}
		listed = append(listed, name)
	}
	entries, err := os.ReadDir("lua/src")
	if err != nil {
		t.Fatal(err)
	}
	var present []string
	for _, e := range entries {
		present = append(present, e.Name())
	}
	slices.Sort(listed)
	if !slices.Equal(listed, present) {
		t.Errorf("lua/src has %v, SHA256SUMS lists %v", present, listed)
	}

	h, err := os.ReadFile("lua/src/lua.h")
	if err != nil {
		t.Fatal(err)
	}
	parts := regexp.MustCompile(`#define LUA_VERSION_(MAJOR|MINOR|RELEASE)\s+"(\d+)"`).FindAllSubmatch(h, -1)
	var v []string
	for _, p := range parts {
		v = append(v, string(p[2]))
	}
	if got := strings.Join(v, "."); got != luaVersion {
		t.Errorf("lua.h is Lua %s, want %s", got, luaVersion)
	}
	if LuaVersion != "Lua "+luaVersion {
		t.Errorf("LuaVersion is %q, want Lua %s", LuaVersion, luaVersion)
	}
	script, err := os.ReadFile("lua/update.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(script, []byte("version="+luaVersion+"\n")) {
		t.Errorf("lua/update.sh does not pin version %s", luaVersion)
	}
}

// lstrlibChanges are every change from lua/src/lstrlib.c to csrc/lstrlib_charged.c.
// They charge pattern matching to the CPU budget. Without them a pattern
// such as string.rep("a", 1e5):find(".-.-.-b") runs for hours inside one
// call to string.find, where the count hook never fires.
var lstrlibChanges = []struct{ old, new string }{
	{
		"/*\n** $Id: lstrlib.c $\n",
		"/*\n** wAthena: this copy charges pattern matching to the CPU budget of the\n" +
			"** script call, with glue_charge from glue.h. internal/script/vendor_test.go\n" +
			"** lists every change from the original.\n*/\n\n/*\n** $Id: lstrlib.c $\n",
	},
	{ // Each character a pattern item tests, more for long [sets].
		"    int c = uchar(*s);\n    switch (*p) {\n      case '.': return 1;",
		"    int c = uchar(*s);\n    glue_charge(ms->L, 1 + (ep - p) / 16);\n    switch (*p) {\n      case '.': return 1;",
	},
	{ // Each character %b scans.
		"    while (++s < ms->src_end) {\n      if (*s == e) {",
		"    while (++s < ms->src_end) {\n      glue_charge(ms->L, 1);\n      if (*s == e) {",
	},
	{ // Each back reference such as %1.
		"  len = ms->capture[l].len;\n  if ((size_t)(ms->src_end-s) >= len &&",
		"  len = ms->capture[l].len;\n  glue_charge(ms->L, 1 + (int64_t)(len / 16));\n  if ((size_t)(ms->src_end-s) >= len &&",
	},
	{ // Each step of the matcher, including backtracking.
		"    luaL_error(ms->L, \"pattern too complex\");\n  init:",
		"    luaL_error(ms->L, \"pattern too complex\");\n  glue_charge(ms->L, 1);\n  init:",
	},
	{ // Plain find: the bytes memchr and memcmp look at.
		"static const char *lmemfind (const char *s1, size_t l1,",
		"static const char *lmemfind (lua_State *L, const char *s1, size_t l1,",
	},
	{
		"      init++;   /* 1st char is already checked */\n",
		"      glue_charge(L, 1 + (int64_t)(((size_t)(init - s1) + l2) / 16));\n      init++;   /* 1st char is already checked */\n",
	},
	{
		"lmemfind(s + init, ls - init, p, lp)",
		"lmemfind(L, s + init, ls - init, p, lp)",
	},
}

// TestPatchedLstrlib checks that csrc/lstrlib_charged.c is lua/src/lstrlib.c with
// exactly the changes above. Run go test -run TestPatchedLstrlib -update
// to write it after updating Lua.
func TestPatchedLstrlib(t *testing.T) {
	orig, err := os.ReadFile("lua/src/lstrlib.c")
	if err != nil {
		t.Fatal(err)
	}
	want := string(orig)
	for _, c := range lstrlibChanges {
		if strings.Count(want, c.old) != 1 {
			t.Fatalf("lstrlib.c has %d copies of %q, want 1", strings.Count(want, c.old), c.old)
		}
		want = strings.Replace(want, c.old, c.new, 1)
	}
	if *update {
		if err := os.WriteFile("csrc/lstrlib_charged.c", []byte(want), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile("csrc/lstrlib_charged.c")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Error("csrc/lstrlib_charged.c is not lua/src/lstrlib.c with the changes in vendor_test.go; run go test -run TestPatchedLstrlib -update")
	}
}
