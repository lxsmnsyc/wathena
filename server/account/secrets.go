package account

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
)

// SecretKeyLen is the length of a key that seals TOTP secrets.
const SecretKeyLen = 32

// Sealer encrypts TOTP secrets for storage, with AES-256-GCM:
//   - The key comes from the environment, not the database, so a copy
//     of the database alone does not give the secrets.
//   - Each sealed value names its key by an 8-byte ID, so older keys can
//     still open values sealed before a key change.
//   - The account ID is bound in as additional data, so a sealed value
//     copied to another account's row does not open.
//
// Key changes have limits:
//   - Values sealed under an old key open only while that key is in the
//     old key list.
//   - A value is sealed again under the current key when its player next
//     uses a code. Values of players who never log in again stay under
//     the old key.
//   - Losing every key that sealed a value loses that TOTP secret. The
//     player then logs in with a recovery code, or the server owner
//     turns TOTP off for the account.
type Sealer struct {
	current sealKey
	old     []sealKey
}

type sealKey struct {
	id   [8]byte
	aead cipher.AEAD
}

// The layout of a sealed value: a version byte, the key ID, the nonce,
// then the ciphertext with its 16-byte tag.
const (
	sealVersion  = 1
	sealNonceLen = 12
	sealHeader   = 1 + 8 + sealNonceLen
)

var errSealed = errors.New("account: a sealed secret cannot be opened")

// NewSealer makes a sealer. current seals new values. old keys only open
// values sealed before a key change. Each key is SecretKeyLen bytes.
func NewSealer(current []byte, old ...[]byte) (*Sealer, error) {
	c, err := newSealKey(current)
	if err != nil {
		return nil, err
	}
	s := &Sealer{current: c}
	for _, k := range old {
		o, err := newSealKey(k)
		if err != nil {
			return nil, err
		}
		s.old = append(s.old, o)
	}
	return s, nil
}

func newSealKey(key []byte) (sealKey, error) {
	if len(key) != SecretKeyLen {
		return sealKey{}, errors.New("account: a secret key must be " + strconv.Itoa(SecretKeyLen) + " bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return sealKey{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return sealKey{}, err
	}
	// The ID is an HMAC of a fixed text, so it names the key without
	// revealing anything about it.
	m := hmac.New(sha256.New, key)
	m.Write([]byte("wathena sealing key id"))
	var k sealKey
	copy(k.id[:], m.Sum(nil))
	k.aead = aead
	return k, nil
}

// ParseSecretKey reads a key written as base64, padded or not, standard
// or URL-safe, such as the output of `openssl rand -base64 32`. Its
// errors never quote the value.
func ParseSecretKey(text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimRight(text, "=")
	text = strings.NewReplacer("-", "+", "_", "/").Replace(text)
	key, err := base64.RawStdEncoding.DecodeString(text)
	if err != nil || len(key) != SecretKeyLen {
		return nil, errors.New("account: a secret key must be " + strconv.Itoa(SecretKeyLen) + " random bytes in base64, such as from `openssl rand -base64 32`")
	}
	return key, nil
}

// seal encrypts a value under the current key.
func (s *Sealer) seal(plain, ad []byte) []byte {
	out := make([]byte, sealHeader, sealHeader+len(plain)+s.current.aead.Overhead())
	out[0] = sealVersion
	copy(out[1:9], s.current.id[:])
	// crypto/rand.Read never fails on supported systems. The nonce is
	// random: a key seals far fewer than 2^32 values.
	_, _ = rand.Read(out[9:sealHeader])
	return s.current.aead.Seal(out, out[9:sealHeader], plain, ad)
}

// open decrypts a sealed value. current reports whether the current key
// sealed it, so the caller can seal it again when not.
func (s *Sealer) open(sealed, ad []byte) (plain []byte, current bool, err error) {
	if len(sealed) < sealHeader || sealed[0] != sealVersion {
		return nil, false, errSealed
	}
	id, nonce, body := sealed[1:9], sealed[9:sealHeader], sealed[sealHeader:]
	for i, k := range append([]sealKey{s.current}, s.old...) {
		if !hmac.Equal(id, k.id[:]) {
			continue
		}
		plain, err := k.aead.Open(nil, nonce, body, ad)
		if err != nil {
			return nil, false, errSealed
		}
		return plain, i == 0, nil
	}
	return nil, false, errSealed
}

// totpAD is the additional data that binds a sealed TOTP secret to its
// account.
func totpAD(account int64) []byte {
	return []byte("wathena totp v1 account " + strconv.FormatInt(account, 10))
}
