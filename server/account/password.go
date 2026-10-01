package account

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params are Argon2id parameters.
type Params struct {
	// Memory is in KiB.
	Memory uint32
	// Time is the number of passes.
	Time uint32
	// Threads is the parallelism.
	Threads uint8
	// SaltLen and KeyLen are in bytes.
	SaltLen, KeyLen uint32
}

// DefaultParams take about 50 ms per hash on the development machine
// (Apple M-series, 8 cores, Go 1.27): 64 MiB, 3 passes, 4 threads. They
// are the second recommended option of RFC 9106. Measured on the same
// machine:
//   - m=64 MiB, t=1, p=4: 20 ms.
//   - m=64 MiB, t=2, p=4: 35 ms.
//   - m=64 MiB, t=3, p=4: 53 ms.
//   - m=64 MiB, t=1, p=1: 47 ms.
//
// Server owners measure on their own hardware and raise Time when a
// hash takes much less than 50 ms. Stored hashes keep their own
// parameters, and a login hashes the password again when they differ.
var DefaultParams = Params{Memory: 64 * 1024, Time: 3, Threads: 4, SaltLen: 16, KeyLen: 32}

// Limits on parameters read from a stored hash. They stop a bad row from
// making a login use gigabytes of memory.
const (
	maxMemory  = 1024 * 1024 // 1 GiB
	maxTime    = 64
	maxSaltLen = 64
	maxKeyLen  = 64
	minSaltLen = 8
	minKeyLen  = 16
)

// errBadHash is a stored hash that cannot be read.
var errBadHash = errors.New("account: the stored password hash cannot be read")

// ErrBusy is returned when too many hashes wait to run.
var ErrBusy = errors.New("account: the server is busy")

// Hasher hashes and checks passwords with Argon2id. It runs at most a
// fixed number of hashes at once, since each one uses Params.Memory.
type Hasher struct {
	Params Params
	slots  chan struct{}
}

// NewHasher makes a hasher that runs at most n hashes at once.
func NewHasher(p Params, n int) *Hasher {
	return &Hasher{Params: p, slots: make(chan struct{}, max(n, 1))}
}

// Hash returns the PHC string of a password with a new random salt.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	salt := make([]byte, h.Params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := h.key(ctx, password, salt, h.Params)
	if err != nil {
		return "", err
	}
	return encodeHash(h.Params, salt, key), nil
}

// Verify checks a password against a stored PHC string. rehash is set
// when the password is right and the stored parameters differ from the
// hasher's, so the caller should store a new hash.
func (h *Hasher) Verify(ctx context.Context, encoded, password string) (ok, rehash bool, err error) {
	p, salt, want, err := decodeHash(encoded)
	if err != nil {
		return false, false, err
	}
	got, err := h.key(ctx, password, salt, p)
	if err != nil {
		return false, false, err
	}
	ok = subtle.ConstantTimeCompare(got, want) == 1
	return ok, ok && p != h.Params, nil
}

// Burn runs one hash with the hasher's parameters and throws it away.
// A login for an unknown name calls it, so it takes as long as a login
// for a real account.
func (h *Hasher) Burn(ctx context.Context, password string) error {
	_, err := h.key(ctx, password, make([]byte, h.Params.SaltLen), h.Params)
	return err
}

func (h *Hasher) key(ctx context.Context, password string, salt []byte, p Params) ([]byte, error) {
	select {
	case h.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ErrBusy
	}
	defer func() { <-h.slots }()
	return argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen), nil
}

var b64 = base64.RawStdEncoding

// encodeHash writes the PHC string format that the reference Argon2
// tools use: $argon2id$v=19$m=<KiB>,t=<passes>,p=<threads>$<salt>$<key>.
func encodeHash(p Params, salt, key []byte) string {
	var b strings.Builder
	b.WriteString("$argon2id$v=")
	b.WriteString(strconv.Itoa(argon2.Version))
	b.WriteString("$m=")
	b.WriteString(strconv.FormatUint(uint64(p.Memory), 10))
	b.WriteString(",t=")
	b.WriteString(strconv.FormatUint(uint64(p.Time), 10))
	b.WriteString(",p=")
	b.WriteString(strconv.FormatUint(uint64(p.Threads), 10))
	b.WriteString("$")
	b.WriteString(b64.EncodeToString(salt))
	b.WriteString("$")
	b.WriteString(b64.EncodeToString(key))
	return b.String()
}

// decodeHash reads a PHC string that encodeHash wrote. It rejects other
// algorithms and versions, and parameters outside the limits above.
func decodeHash(s string) (p Params, salt, key []byte, err error) {
	parts := strings.Split(s, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return p, nil, nil, errBadHash
	}
	fields := strings.Split(parts[3], ",")
	if len(fields) != 3 {
		return p, nil, nil, errBadHash
	}
	m, ok1 := param(fields[0], "m=", maxMemory)
	t, ok2 := param(fields[1], "t=", maxTime)
	th, ok3 := param(fields[2], "p=", 255)
	if !ok1 || !ok2 || !ok3 || m < 8*th {
		return p, nil, nil, errBadHash
	}
	salt, err1 := b64.Strict().DecodeString(parts[4])
	key, err2 := b64.Strict().DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(salt) < minSaltLen || len(salt) > maxSaltLen || len(key) < minKeyLen || len(key) > maxKeyLen {
		return p, nil, nil, errBadHash
	}
	// The decoder skips \r and \n even in strict mode, so a stored hash
	// must also encode back to exactly the same text.
	if b64.EncodeToString(salt) != parts[4] || b64.EncodeToString(key) != parts[5] {
		return p, nil, nil, errBadHash
	}
	p = Params{Memory: m, Time: t, Threads: uint8(th), SaltLen: uint32(len(salt)), KeyLen: uint32(len(key))} // #nosec G115 -- th is at most 255, and the lengths at most 64.
	return p, salt, key, nil
}

// param reads "<prefix><n>" with 1 <= n <= limit, in plain digits.
func param(s, prefix string, limit uint32) (uint32, bool) {
	v, ok := strings.CutPrefix(s, prefix)
	if !ok || v == "" || len(v) > 10 || (len(v) > 1 && v[0] == '0') {
		return 0, false
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil || n < 1 || n > uint64(limit) {
		return 0, false
	}
	return uint32(n), true
}
