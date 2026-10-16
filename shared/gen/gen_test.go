package gen

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

func TestFlags(t *testing.T) {
	for _, c := range []struct{ cell, flags uint8 }{
		{CellWalkable, FlagWalkable | FlagShootable},
		{CellBlocked, 0},
		{CellWater, FlagWalkable | FlagShootable | FlagWater},
		{CellGap, FlagShootable},
		{2, 0},
	} {
		if got := Flags(c.cell); got != c.flags {
			t.Errorf("Flags(%d) = %d, want %d", c.cell, got, c.flags)
		}
	}
}

func TestParamsCheck(t *testing.T) {
	for _, p := range []Params{{Width: 1, Height: 1}, {Width: 1024, Height: 1024}, {Width: 400, Height: 7}} {
		if err := p.Check(); err != nil {
			t.Errorf("%+v: %v", p, err)
		}
	}
	for _, p := range []Params{{Width: 0, Height: 1}, {Width: 1, Height: 0}, {Width: -1, Height: 5}, {Width: 1025, Height: 1}, {Width: 1, Height: 1025}} {
		if err := p.Check(); !errors.Is(err, ErrSize) {
			t.Errorf("%+v: %v, want ErrSize", p, err)
		}
	}
}

// TestChecksum compares the checksum with the encoding in the spec,
// written out byte by byte.
func TestChecksum(t *testing.T) {
	g := Grid{
		Width: 2, Height: 1, WaterLevel: -rng.One / 2,
		Types:   []uint8{CellWalkable, CellWater},
		Heights: []rng.Fixed{1, 2, 3, 4, -1, -2, rng.One, -rng.One},
	}
	msg := []byte("wa-grid-v1")
	msg = append(msg, 0, 2, 0, 1, 0xff, 0xff, 0x80, 0x00)
	msg = append(msg, 0)
	for _, h := range []uint32{1, 2, 3, 4} {
		msg = binary.BigEndian.AppendUint32(msg, h)
	}
	msg = append(msg, 3)
	for _, h := range []uint32{0xffffffff, 0xfffffffe, 0x00010000, 0xffff0000} {
		msg = binary.BigEndian.AppendUint32(msg, h)
	}
	if got, want := g.Checksum(), sha256.Sum256(msg); got != want {
		t.Errorf("checksum %s, want %s", hex.EncodeToString(got[:]), hex.EncodeToString(want[:]))
	}
	// Any change to a cell changes it.
	before := g.Checksum()
	g.Heights[7]++
	if g.Checksum() == before {
		t.Error("a height change kept the checksum")
	}
}

func TestModes(t *testing.T) {
	if !HasMode(ModeClassic) || HasMode("") || HasMode("other") {
		t.Errorf("modes: %v", Modes())
	}
	if Procedural {
		if !slices.Equal(Modes(), []string{ModeClassic, ModeProcedural}) || DefaultMode() != ModeProcedural {
			t.Errorf("procedural build: %v, default %s", Modes(), DefaultMode())
		}
		g, ok := Latest()
		if !ok || g.Version < 1 || g.RNG != rng.Version || len(Versions()) == 0 {
			t.Errorf("latest %+v %v, versions %v", g, ok, Versions())
		}
		return
	}
	if !slices.Equal(Modes(), []string{ModeClassic}) || DefaultMode() != ModeClassic || HasMode(ModeProcedural) {
		t.Errorf("classic build: %v, default %s", Modes(), DefaultMode())
	}
	if _, ok := Latest(); ok || len(Versions()) != 0 {
		t.Errorf("a classic build has generators: %v", Versions())
	}
	if _, ok := Lookup(1); ok {
		t.Error("a classic build has generator 1")
	}
}

// TestVersions: versions are unique and in order, and Lookup finds each.
func TestVersions(t *testing.T) {
	vs := Versions()
	for i, v := range vs {
		if i > 0 && v <= vs[i-1] {
			t.Errorf("versions out of order: %v", vs)
		}
		g, ok := Lookup(v)
		if !ok || g.Version != v || g.Grid == nil || g.RNG == "" {
			t.Errorf("version %d: %+v, %v", v, g, ok)
		}
	}
	if _, ok := Lookup(0); ok {
		t.Error("Lookup(0) found a generator")
	}
}
