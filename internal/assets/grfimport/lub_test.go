package grfimport

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/grf"
)

// chunk builds a Lua 5.1 chunk with one main function. Real clients use
// luac output. The tests build chunks by hand, since the repository has
// no Lua 5.1 compiler.
func chunk(code []uint32, consts []any) []byte {
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.Write([]byte{0x1b, 'L', 'u', 'a', 0x51, 0, 1, 4, 4, 4, 8, 0})
	w(uint32(0))                 // source name
	w([]uint32{0, 0})            // line defined, last line defined
	b.Write([]byte{0, 0, 2, 10}) // upvalues, params, vararg, max stack
	w(uint32(len(code)))
	w(code)
	w(uint32(len(consts)))
	for _, c := range consts {
		switch v := c.(type) {
		case string:
			b.WriteByte(4)
			w(uint32(len(v) + 1))
			b.WriteString(v)
			b.WriteByte(0)
		case float64:
			b.WriteByte(3)
			w(math.Float64bits(v))
		}
	}
	w([]uint32{0, 0, 0, 0}) // protos, line info, locals, upvalue names
	return b.Bytes()
}

func iABC(op, a, b, c uint32) uint32 { return op | a<<6 | c<<14 | b<<23 }
func iABx(op, a, bx uint32) uint32   { return op | a<<6 | bx<<14 }

const rkK = 0x100 // marks a constant operand

func TestEvalLub(t *testing.T) {
	// T = { [1] = "_고글" in CP949, name = 7 }; U = T[1]
	goggle := "_\xb0\xed\xb1\xdb"
	b := chunk([]uint32{
		iABC(opNewTable, 0, 0, 0),
		iABC(opSetTable, 0, rkK|1, rkK|2),
		iABC(opSetTable, 0, rkK|3, rkK|4),
		iABx(opSetGlob, 0, 0),
		iABx(opGetGlob, 1, 0),
		iABC(opGetTable, 2, 1, rkK|1),
		iABx(opSetGlob, 2, 5),
		iABC(opReturn, 0, 1, 0),
	}, []any{"T", 1.0, goggle, "name", 7.0, "U"})
	g := map[string]any{}
	if err := evalLub(b, g); err != nil {
		t.Fatal(err)
	}
	tb := g["T"].(*LubTable)
	if tb.M[1.0] != goggle || tb.M["name"] != 7.0 || g["U"] != goggle {
		t.Errorf("globals %v, table %v", g, tb.M)
	}
	if decodeCP949(goggle) != "_고글" {
		t.Errorf("decoded %q", decodeCP949(goggle))
	}
}

func TestEvalLubRejectsCode(t *testing.T) {
	const opCall, opJmp = 28, 22
	for name, code := range map[string][]uint32{
		"call": {iABx(opGetGlob, 0, 0), iABC(opCall, 0, 1, 1)},
		"jump": {iABx(opJmp, 0, 131071-1)},
	} {
		if err := evalLub(chunk(code, []any{"print"}), map[string]any{}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func FuzzEvalLub(f *testing.F) {
	f.Add(chunk([]uint32{iABC(opNewTable, 0, 0, 0), iABx(opSetGlob, 0, 0), iABC(opReturn, 0, 1, 0)}, []any{"T"}))
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = evalLub(b, map[string]any{})
	})
}

// TestRealTables reads the tables of a real client when WATHENA_CLIENT
// names a client folder.
func TestRealTables(t *testing.T) {
	dir := os.Getenv("WATHENA_CLIENT")
	if dir == "" {
		t.Skip("set WATHENA_CLIENT to a client folder to run")
	}
	c, err := grf.OpenClient(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	tb, err := LoadTables(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ids := tb.headgear["남/남_고글"]; len(ids) == 0 {
		t.Error("no view ID for the goggles")
	}
	t.Logf("%d headgear sprite names", len(tb.headgear))
}
