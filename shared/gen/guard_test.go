//go:build !tinygo

package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// guarded are the packages whose results must match on every platform:
// the generators and the seed tree they use.
var guarded = []string{".", "wasm", "../rng"}

// banned are imports that make results depend on the machine or the
// clock. math/rand/v2 is allowed for rng's ChaCha8, which the server alone
// runs.
var banned = map[string]string{
	"math/rand": "its algorithm is not documented",
	"time":      "results must not depend on the clock",
	"unsafe":    "memory layout differs between platforms",
	"os":        "results must not depend on the machine",
}

// TestGuard checks the rules of strategy section 8.4 on the source of the
// guarded packages, without their tests:
//
//   - No floating-point types or literals, and from package math only its
//     integer limits.
//   - No map types, so no code ranges over a map.
//   - No imports of the clock, the machine, or math/rand.
func TestGuard(t *testing.T) {
	checked := 0
	for _, dir := range guarded {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			checked++
			for _, p := range guardFile(t, file) {
				t.Error(p)
			}
		}
	}
	if checked < 5 {
		t.Errorf("checked %d files", checked)
	}
}

// TestGuardFinds: the guard reports each kind of problem.
func TestGuardFinds(t *testing.T) {
	src := `package x

import (
	"math"
	"time"
)

var a float64 = 1.5
var b = map[int]int{}
var c = math.Sqrt(2)
var d = math.MaxInt32
var e = time.Now
var f complex64
`
	file := filepath.Join(t.TempDir(), "x.go")
	if err := os.WriteFile(file, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	got := guardFile(t, file)
	want := []string{"time", "float64", "1.5", "map type", "math.Sqrt", "complex64"}
	if len(got) != len(want) {
		t.Fatalf("problems %q, want %d", got, len(want))
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) {
			t.Errorf("problem %d: %q, want %q", i, got[i], w)
		}
	}
}

// guardFile returns the problems of one file, in source order.
func guardFile(t *testing.T, file string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var problems []string
	report := func(n ast.Node, what string) {
		problems = append(problems, fset.Position(n.Pos()).String()+": "+what)
	}
	mathName := ""
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if why, ok := banned[path]; ok {
			report(imp, "imports "+path+": "+why)
		}
		if path == "math" {
			mathName = "math"
			if imp.Name != nil {
				mathName = imp.Name.Name
			}
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Ident:
			switch n.Name {
			case "float32", "float64", "complex64", "complex128":
				report(n, "uses "+n.Name+": generation uses fixed point")
			}
		case *ast.BasicLit:
			if n.Kind == token.FLOAT || n.Kind == token.IMAG {
				report(n, "has the literal "+n.Value+": generation uses fixed point")
			}
		case *ast.MapType:
			report(n, "has a map type: Go map order is random, use sorted slices")
		case *ast.SelectorExpr:
			if x, ok := n.X.(*ast.Ident); ok && mathName != "" && x.Name == mathName &&
				!strings.HasPrefix(n.Sel.Name, "Max") && !strings.HasPrefix(n.Sel.Name, "Min") {
				report(n, "uses math."+n.Sel.Name+": only math's integer limits are allowed")
			}
		}
		return true
	})
	return problems
}
