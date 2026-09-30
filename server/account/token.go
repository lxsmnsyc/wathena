package account

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// Session tokens and game tickets are 256-bit random values. They travel
// as 43 characters of unpadded base64url. The database keeps only their
// SHA-256, so a copy of the database cannot be used to log in. The value
// is random, so a plain hash is enough: there is nothing to guess.
const (
	tokenBytes = 32
	// TokenLen is the length of a token's text.
	TokenLen = 43
)

var tokenEncoding = base64.RawURLEncoding

// newToken returns a new random token and its hash.
func newToken() (token string, hash []byte) {
	b := make([]byte, tokenBytes)
	// crypto/rand.Read never fails on supported systems.
	_, _ = rand.Read(b)
	sum := sha256.Sum256(b)
	return tokenEncoding.EncodeToString(b), sum[:]
}

// tokenHash returns the hash of a token's text, or false when the text
// is not a token. Lookups go by this hash through a unique index, so no
// token is compared byte by byte in Go.
func tokenHash(token string) ([]byte, bool) {
	if len(token) != TokenLen {
		return nil, false
	}
	b, err := tokenEncoding.Strict().DecodeString(token)
	if err != nil || len(b) != tokenBytes {
		return nil, false
	}
	sum := sha256.Sum256(b)
	return sum[:], true
}
