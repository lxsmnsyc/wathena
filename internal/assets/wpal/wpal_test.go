package wpal

import "testing"

func TestRoundTrip(t *testing.T) {
	ro := make([]byte, 1024)
	ro[0], ro[4], ro[5], ro[6] = 255, 10, 20, 30
	p, err := FromRO(ro)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(p.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if got[1] != [4]byte{10, 20, 30, 255} || got[0][3] != 0 {
		t.Errorf("palette %v %v", got[0], got[1])
	}
	if _, err := FromRO(ro[:10]); err == nil {
		t.Error("short .pal accepted")
	}
}

func FuzzDecode(f *testing.F) {
	var p Palette
	f.Add(p.Encode())
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := Decode(b)
		if err != nil {
			return
		}
		if string(p.Encode()) != string(b) {
			t.Fatal("round trip differs")
		}
	})
}
