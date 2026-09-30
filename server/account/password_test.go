package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fastParams keep tests quick. They are far below DefaultParams.
var fastParams = Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func TestHashAndVerify(t *testing.T) {
	ctx := context.Background()
	h := NewHasher(fastParams, 2)
	enc, err := h.Hash(ctx, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Errorf("encoded %q", enc)
	}
	if again, _ := h.Hash(ctx, "correct horse"); again == enc {
		t.Error("two hashes of one password share a salt")
	}
	ok, rehash, err := h.Verify(ctx, enc, "correct horse")
	if !ok || rehash || err != nil {
		t.Errorf("right password: %v %v %v", ok, rehash, err)
	}
	ok, rehash, err = h.Verify(ctx, enc, "correct horsE")
	if ok || rehash || err != nil {
		t.Errorf("wrong password: %v %v %v", ok, rehash, err)
	}
}

func TestRehashWhenParamsChange(t *testing.T) {
	ctx := context.Background()
	old := NewHasher(fastParams, 1)
	enc, err := old.Hash(ctx, "a password")
	if err != nil {
		t.Fatal(err)
	}
	newer := fastParams
	newer.Time = 2
	h := NewHasher(newer, 1)
	ok, rehash, err := h.Verify(ctx, enc, "a password")
	if !ok || !rehash || err != nil {
		t.Errorf("older parameters: %v %v %v, want a rehash", ok, rehash, err)
	}
	if _, rehash, _ := h.Verify(ctx, enc, "wrong password"); rehash {
		t.Error("a wrong password asks for a rehash")
	}
}

func TestDefaultParamsTakeAbout50ms(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	h := NewHasher(DefaultParams, 1)
	start := time.Now()
	if _, err := h.Hash(context.Background(), "a password"); err != nil {
		t.Fatal(err)
	}
	// Only a check that the parameters were not cut by mistake. CI
	// machines and the race detector vary too much for an upper bound.
	// The measured values are in DefaultParams' comment.
	if d := time.Since(start); d < 5*time.Millisecond {
		t.Errorf("one hash took %v", d)
	}
}

func TestHasherBusy(t *testing.T) {
	h := NewHasher(fastParams, 1)
	h.slots <- struct{}{} // every slot taken
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := h.Hash(ctx, "a password"); !errors.Is(err, ErrBusy) {
		t.Errorf("err %v, want ErrBusy", err)
	}
}

func TestDecodeHashRejects(t *testing.T) {
	good := "$argon2id$v=19$m=64,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$" + strings.Repeat("A", 43)
	if _, _, _, err := decodeHash(good); err != nil {
		t.Fatalf("good hash: %v", err)
	}
	for _, bad := range []string{
		"",
		"$argon2i$v=19$m=64,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$" + strings.Repeat("A", 43),
		"$argon2id$v=16$m=64,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$" + strings.Repeat("A", 43),
		"$argon2id$v=19$m=99999999,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$" + strings.Repeat("A", 43),
		"$argon2id$v=19$m=64,t=0,p=1$c2FsdHNhbHRzYWx0c2FsdA$" + strings.Repeat("A", 43),
		"$argon2id$v=19$m=64,t=1,p=0$c2FsdHNhbHRzYWx0c2FsdA$" + strings.Repeat("A", 43),
		"$argon2id$v=19$m=64,t=01,p=1$c2FsdHNhbHRzYWx0c2FsdA$" + strings.Repeat("A", 43),
		"$argon2id$v=19$m=64,t=1,p=1,x=2$c2FsdHNhbHRzYWx0c2FsdA$" + strings.Repeat("A", 43),
		"$argon2id$v=19$m=64,t=1,p=1$c2FsdA$" + strings.Repeat("A", 43), // salt too short
		"$argon2id$v=19$m=64,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$AAAA",       // key too short
		"$argon2id$v=19$m=64,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA=$" + strings.Repeat("A", 43),
		"$argon2id$v=19$m=8,t=1,p=4$c2FsdHNhbHRzYWx0c2FsdA$" + strings.Repeat("A", 43), // m < 8p
	} {
		if _, _, _, err := decodeHash(bad); err == nil {
			t.Errorf("%q decoded", bad)
		}
	}
	// A stored hash that cannot be read is an error, not a match.
	h := NewHasher(fastParams, 1)
	if ok, _, err := h.Verify(context.Background(), "plain text", "plain text"); ok || err == nil {
		t.Errorf("unreadable hash: %v %v", ok, err)
	}
}

func FuzzDecodeHash(f *testing.F) {
	f.Add("$argon2id$v=19$m=64,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$" + strings.Repeat("A", 43))
	f.Add("$argon2id$v=19$m=65536,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	f.Add("$$$$$")
	f.Fuzz(func(t *testing.T, s string) {
		p, salt, key, err := decodeHash(s)
		if err != nil {
			return
		}
		if p.Memory < 8*uint32(p.Threads) || p.Memory > maxMemory || p.Time < 1 || p.Time > maxTime || p.Threads < 1 {
			t.Fatalf("%q gave parameters %+v", s, p)
		}
		if len(salt) < minSaltLen || len(salt) > maxSaltLen || len(key) < minKeyLen || len(key) > maxKeyLen {
			t.Fatalf("%q gave %d salt and %d key bytes", s, len(salt), len(key))
		}
		// What decodes encodes back to the same text.
		if again := encodeHash(p, salt, key); again != s {
			t.Fatalf("%q encodes back as %q", s, again)
		}
	})
}
