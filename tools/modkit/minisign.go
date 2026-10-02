package modkit

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// Signatures are minisign's (https://jedisct1.github.io/minisign/):
//   - A public key is "Ed", an 8-byte key ID and a 32-byte Ed25519 key,
//     in base64, alone or after an "untrusted comment:" line.
//   - A signature file has an untrusted comment, then "Ed" or "ED", the
//     key ID and a 64-byte signature in base64, then a trusted comment,
//     then a 64-byte signature of the signature and the trusted comment.
//     "ED" signs the BLAKE2b-512 hash of the file, as minisign does by
//     default. "Ed" signs the file itself.
//
// wactl only checks signatures. Authors sign with minisign -Sm.

// MinisignPublicKey reads a minisign public key: the key itself, or a
// file that holds it.
func MinisignPublicKey(s string) (id []byte, key ed25519.PublicKey, err error) {
	text := s
	if b, err := os.ReadFile(s); err == nil { // #nosec G304 -- the file the owner named.
		text = string(b)
	}
	var line string
	for l := range strings.SplitSeq(strings.TrimSpace(text), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "untrusted comment:") {
			line = l
			break
		}
	}
	raw, err := base64.StdEncoding.DecodeString(line)
	if err != nil || len(raw) != 42 || string(raw[:2]) != "Ed" {
		return nil, nil, errors.New("not a minisign public key: want the base64 line of a .pub file, or the file")
	}
	return raw[2:10], ed25519.PublicKey(raw[10:]), nil
}

// VerifyMinisign checks a minisign signature of data with a public key.
// It returns the signature's trusted comment.
func VerifyMinisign(data []byte, sig, publicKey string) (string, error) {
	keyID, key, err := MinisignPublicKey(publicKey)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(sig), "\r\n", "\n"), "\n")
	if len(lines) < 4 || !strings.HasPrefix(lines[0], "untrusted comment:") || !strings.HasPrefix(lines[2], "trusted comment: ") {
		return "", errors.New("not a minisign signature file")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[1]))
	if err != nil || len(raw) != 74 {
		return "", errors.New("the signature line is not a minisign signature")
	}
	alg, id, signature := string(raw[:2]), raw[2:10], raw[10:]
	if !bytes.Equal(id, keyID) {
		return "", fmt.Errorf("the file was signed with key %X, not with the key given (%X)", reverse(id), reverse(keyID))
	}
	msg := data
	switch alg {
	case "ED":
		h := blake2b.Sum512(data)
		msg = h[:]
	case "Ed":
	default:
		return "", fmt.Errorf("unknown signature algorithm %q", alg)
	}
	if !ed25519.Verify(key, msg, signature) {
		return "", errors.New("the signature does not match the file")
	}
	comment := strings.TrimPrefix(lines[2], "trusted comment: ")
	global, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[3]))
	if err != nil || len(global) != 64 || !ed25519.Verify(key, append(slices.Clone(signature), comment...), global) {
		return "", errors.New("the trusted comment's signature does not match")
	}
	return comment, nil
}

// reverse shows a key ID as minisign prints it: the bytes from last to
// first.
func reverse(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[len(b)-1-i]
	}
	return out
}
