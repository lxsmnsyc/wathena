package account

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

// Recovery codes let a player log in without their authenticator app.
// Each works once:
//   - A code is 80 random bits, written as 16 base32 letters and digits
//     in groups of 4, such as abcd-efgh-2345-67ab.
//   - The database keeps only the SHA-256 of each code. The codes are
//     random and long, so a plain hash is enough, as for session tokens.
//   - The player sees them once, when TOTP is turned on or when they ask
//     for new ones. New codes replace all older ones.
const (
	RecoveryCodes     = 10
	recoveryCodeBytes = 10
	recoveryCodeLen   = 16
)

var recoveryEncoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// newRecoveryCodes returns RecoveryCodes new codes, as shown to the
// player, and their hashes, as stored.
func newRecoveryCodes() (codes []string, hashes [][]byte) {
	for range RecoveryCodes {
		b := make([]byte, recoveryCodeBytes)
		// crypto/rand.Read never fails on supported systems.
		_, _ = rand.Read(b)
		s := recoveryEncoding.EncodeToString(b)
		codes = append(codes, s[0:4]+"-"+s[4:8]+"-"+s[8:12]+"-"+s[12:16])
		sum := sha256.Sum256([]byte(s))
		hashes = append(hashes, sum[:])
	}
	return codes, hashes
}

// recoveryHash returns the hash of a recovery code as the player typed
// it, in any case and with or without dashes and spaces. It returns
// false for text that cannot be a recovery code.
func recoveryHash(input string) ([]byte, bool) {
	s := strings.ToLower(normalizeCode(input))
	if len(s) != recoveryCodeLen {
		return nil, false
	}
	if _, err := recoveryEncoding.DecodeString(s); err != nil {
		return nil, false
	}
	sum := sha256.Sum256([]byte(s))
	return sum[:], true
}
