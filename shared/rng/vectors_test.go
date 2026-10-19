package rng

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/rng-v1.json")

const vectorsFile = "testdata/rng-v1.json"

type pathStep struct {
	Label string  `json:"label"`
	Idx   []int64 `json:"idx"`
}

type vectors struct {
	Version string `json:"version"`
	Roots   []struct {
		Secret string `json:"secret_hex"`
		Key    string `json:"key"`
	} `json:"roots"`
	Children []struct {
		Parent  string  `json:"parent"`
		Label   string  `json:"label"`
		Idx     []int64 `json:"idx"`
		Message string  `json:"message"`
		Key     string  `json:"key"`
	} `json:"children"`
	Paths []struct {
		Secret string     `json:"secret_hex"`
		Path   []pathStep `json:"path"`
		Key    string     `json:"key"`
	} `json:"paths"`
	SplitMix64 []struct {
		State   string   `json:"state"`
		Outputs []string `json:"outputs"`
	} `json:"splitmix64"`
	Xoshiro []struct {
		Key     string   `json:"key"`
		Outputs []string `json:"outputs"`
	} `json:"xoshiro"`
	Uint64N []struct {
		Key     string   `json:"key"`
		N       string   `json:"n"`
		Outputs []string `json:"outputs"`
		Reads   int      `json:"reads"`
	} `json:"uint64n"`
	Range []struct {
		Key     string  `json:"key"`
		Lo      int64   `json:"lo"`
		Hi      int64   `json:"hi"`
		Outputs []int64 `json:"outputs"`
	} `json:"range"`
	Shuffle []struct {
		Key    string `json:"key"`
		N      int    `json:"n"`
		Result []int  `json:"result"`
	} `json:"shuffle"`
	Weighted []struct {
		Key     string   `json:"key"`
		Weights []uint32 `json:"weights"`
		Outputs []int    `json:"outputs"`
	} `json:"weighted"`
	Chance []struct {
		Key     string `json:"key"`
		P       Fixed  `json:"p"`
		Outputs []bool `json:"outputs"`
	} `json:"chance"`
	Hash []struct {
		Key   string `json:"key"`
		X     int64  `json:"x"`
		Y     int64  `json:"y"`
		Z     int64  `json:"z"`
		Value string `json:"value"`
	} `json:"hash"`
	Fixed []struct {
		Op     string `json:"op"`
		A      Fixed  `json:"a"`
		B      Fixed  `json:"b"`
		Result Fixed  `json:"result"`
	} `json:"fixed"`
	ChaCha8 []struct {
		Key     string   `json:"key"`
		Outputs []string `json:"outputs"`
	} `json:"chacha8"`
	SecretHash []struct {
		Key   string `json:"key"`
		X     int64  `json:"x"`
		Y     int64  `json:"y"`
		Value string `json:"value"`
	} `json:"secret_hash"`
}

// counter counts reads from a source.
type counter struct {
	src   Source
	reads int
}

func (c *counter) Uint64() uint64 {
	c.reads++
	return c.src.Uint64()
}

func u64hex(v uint64) string { return fmt.Sprintf("%016x", v) }

func keyhex(n Node) string { return hex.EncodeToString(n.key[:]) }

func mustKey(t *testing.T, s string) Node {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		t.Fatalf("bad key %q", s)
	}
	return FromKey([32]byte(b))
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustU64(t *testing.T, s string) uint64 {
	t.Helper()
	var v uint64
	if _, err := fmt.Sscanf(s, "%x", &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func outputs(src Source, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = u64hex(src.Uint64())
	}
	return out
}

// compute fills the expected values from the inputs in v.
func compute(t *testing.T, v *vectors) {
	v.Version = Version
	for i := range v.Roots {
		root, err := NewRoot(mustHex(t, v.Roots[i].Secret))
		if err != nil {
			t.Fatal(err)
		}
		v.Roots[i].Key = keyhex(root)
	}
	for i := range v.Children {
		c := &v.Children[i]
		c.Message = hex.EncodeToString(childMessage(c.Label, c.Idx))
		c.Key = keyhex(mustKey(t, c.Parent).Child(c.Label, c.Idx...))
	}
	for i := range v.Paths {
		p := &v.Paths[i]
		n, err := NewRoot(mustHex(t, p.Secret))
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range p.Path {
			n = n.Child(step.Label, step.Idx...)
		}
		p.Key = keyhex(n)
	}
	for i := range v.SplitMix64 {
		s := &v.SplitMix64[i]
		sm := &SplitMix64{state: mustU64(t, s.State)}
		s.Outputs = outputs(sm, 4)
	}
	for i := range v.Xoshiro {
		x := &v.Xoshiro[i]
		x.Outputs = outputs(NewXoshiro(mustKey(t, x.Key).key), 8)
	}
	for i := range v.Uint64N {
		u := &v.Uint64N[i]
		c := &counter{src: mustKey(t, u.Key).Stream()}
		r := New(c)
		n := mustU64(t, u.N)
		u.Outputs = make([]string, 8)
		for j := range u.Outputs {
			u.Outputs[j] = u64hex(r.Uint64N(n))
		}
		u.Reads = c.reads
	}
	for i := range v.Range {
		g := &v.Range[i]
		r := mustKey(t, g.Key).Stream()
		g.Outputs = make([]int64, 8)
		for j := range g.Outputs {
			g.Outputs[j] = r.Range(g.Lo, g.Hi)
		}
	}
	for i := range v.Shuffle {
		s := &v.Shuffle[i]
		s.Result = make([]int, s.N)
		for j := range s.Result {
			s.Result[j] = j
		}
		mustKey(t, s.Key).Stream().Shuffle(s.N, func(a, b int) {
			s.Result[a], s.Result[b] = s.Result[b], s.Result[a]
		})
	}
	for i := range v.Weighted {
		w := &v.Weighted[i]
		r := mustKey(t, w.Key).Stream()
		w.Outputs = make([]int, 16)
		for j := range w.Outputs {
			w.Outputs[j] = r.Weighted(w.Weights)
		}
	}
	for i := range v.Chance {
		c := &v.Chance[i]
		r := mustKey(t, c.Key).Stream()
		c.Outputs = make([]bool, 16)
		for j := range c.Outputs {
			c.Outputs[j] = r.Chance(c.P)
		}
	}
	for i := range v.Hash {
		h := &v.Hash[i]
		h.Value = u64hex(mustKey(t, h.Key).Hash(h.X, h.Y, h.Z))
	}
	for i := range v.Fixed {
		f := &v.Fixed[i]
		switch f.Op {
		case "mul":
			f.Result = f.A.Mul(f.B)
		case "div":
			f.Result = f.A.Div(f.B)
		default:
			t.Fatalf("unknown fixed op %q", f.Op)
		}
	}
	computeSecret(t, v)
}

func loadVectors(t *testing.T) *vectors {
	t.Helper()
	data, err := os.ReadFile(vectorsFile)
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return &v
}

// TestVectors checks the implementation against testdata/rng-v1.json.
// Run with -update after adding inputs to rewrite the expected values.
// Never update the expected values of an existing input: that breaks rng-v1.
func TestVectors(t *testing.T) {
	want := loadVectors(t)
	got := loadVectors(t)
	compute(t, got)
	if *update {
		data, err := json.MarshalIndent(got, "", "\t")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(vectorsFile, append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	gv, wv := reflect.ValueOf(got).Elem(), reflect.ValueOf(want).Elem()
	for i := range gv.NumField() {
		name := gv.Type().Field(i).Name
		if !reflect.DeepEqual(gv.Field(i).Interface(), wv.Field(i).Interface()) {
			g, _ := json.Marshal(gv.Field(i).Interface())
			w, _ := json.Marshal(wv.Field(i).Interface())
			t.Errorf("%s:\n got %s\nwant %s", name, g, w)
		}
	}
}

// Published outputs from the algorithm authors, checked apart from the
// vector file so a bad update cannot hide an error.
func TestPublishedOutputs(t *testing.T) {
	// SplitMix64 with state 0.
	var sm SplitMix64
	if got := sm.Uint64(); got != 0xE220A8397B1DCDAF {
		t.Errorf("SplitMix64 first output %#x", got)
	}
	// xoshiro256** with state {1, 2, 3, 4}.
	x := &Xoshiro{s: [4]uint64{1, 2, 3, 4}}
	for _, want := range []uint64{11520, 0, 1509978240, 1215971899390074240} {
		if got := x.Uint64(); got != want {
			t.Errorf("xoshiro256** output %d, want %d", got, want)
		}
	}
}
