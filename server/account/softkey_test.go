package account_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
)

// softKey is a passkey authenticator in software, for tests. It does
// what a browser and a platform authenticator do together: it reads the
// server's options, makes a P-256 key, and signs challenges. It stores
// one discoverable credential.
type softKey struct {
	t      *testing.T
	origin string
	key    *ecdsa.PrivateKey
	id     []byte
	rpID   string
	handle []byte
	count  uint32
	// uv is whether it reports that it verified the user.
	uv bool
}

var b64url = base64.RawURLEncoding

func newSoftKey(t *testing.T, origin string) *softKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	_, _ = rand.Read(id)
	return &softKey{t: t, origin: origin, key: k, id: id, uv: true}
}

func (k *softKey) clientData(typ, challenge string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": k.origin, "crossOrigin": false})
	return b
}

func (k *softKey) flags(attested bool) byte {
	f := byte(0x01) // user present
	if k.uv {
		f |= 0x04
	}
	if attested {
		f |= 0x40
	}
	return f
}

func (k *softKey) authData(attested bool) []byte {
	rp := sha256.Sum256([]byte(k.rpID))
	out := append([]byte{}, rp[:]...)
	out = append(out, k.flags(attested))
	out = binary.BigEndian.AppendUint32(out, k.count)
	if attested {
		out = append(out, make([]byte, 16)...) // AAGUID, all zero
		out = binary.BigEndian.AppendUint16(out, uint16(len(k.id)))
		out = append(out, k.id...)
		pub, err := k.key.PublicKey.Bytes() // 0x04 || X || Y
		if err != nil {
			k.t.Fatal(err)
		}
		cose, err := webauthncbor.Marshal(webauthncose.EC2PublicKeyData{
			PublicKeyData: webauthncose.PublicKeyData{KeyType: 2, Algorithm: -7},
			Curve:         1, XCoord: pub[1:33], YCoord: pub[33:],
		})
		if err != nil {
			k.t.Fatal(err)
		}
		out = append(out, cose...)
	}
	return out
}

// create answers registration options (the JSON of begin) with a new
// credential, as navigator.credentials.create would.
func (k *softKey) create(options []byte) []byte {
	k.t.Helper()
	var o struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			RP        struct {
				ID string `json:"id"`
			} `json:"rp"`
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &o); err != nil {
		k.t.Fatalf("options: %v: %s", err, options)
	}
	k.rpID = o.PublicKey.RP.ID
	var err error
	if k.handle, err = b64url.DecodeString(o.PublicKey.User.ID); err != nil {
		k.t.Fatal(err)
	}
	att, err := webauthncbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": k.authData(true)})
	if err != nil {
		k.t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{
		"id": b64url.EncodeToString(k.id), "rawId": b64url.EncodeToString(k.id), "type": "public-key",
		"clientExtensionResults": map[string]any{},
		"response": map[string]any{
			"clientDataJSON":    b64url.EncodeToString(k.clientData("webauthn.create", o.PublicKey.Challenge)),
			"attestationObject": b64url.EncodeToString(att),
			"transports":        []string{"internal"},
		},
	})
	return b
}

// get answers login options with a signed assertion, as
// navigator.credentials.get would. The counter goes up by one each time.
func (k *softKey) get(options []byte) []byte {
	k.t.Helper()
	var o struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &o); err != nil {
		k.t.Fatalf("options: %v: %s", err, options)
	}
	k.count++
	auth := k.authData(false)
	cd := k.clientData("webauthn.get", o.PublicKey.Challenge)
	sum := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, auth...), sum[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.key, digest[:])
	if err != nil {
		k.t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{
		"id": b64url.EncodeToString(k.id), "rawId": b64url.EncodeToString(k.id), "type": "public-key",
		"clientExtensionResults": map[string]any{},
		"response": map[string]any{
			"clientDataJSON":    b64url.EncodeToString(cd),
			"authenticatorData": b64url.EncodeToString(auth),
			"signature":         b64url.EncodeToString(sig),
			"userHandle":        b64url.EncodeToString(k.handle),
		},
	})
	return b
}
