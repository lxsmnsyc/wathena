package account

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"html/template"
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/boombuler/barcode/qr"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"
)

// TOTP settings. They are what authenticator apps expect by default
// (RFC 6238): SHA-1, 6 digits, 30-second steps.
const (
	totpPeriod      = 30
	totpDigits      = 6
	totpSecretBytes = 20
	// totpSkew is how many steps before and after the current one are
	// accepted, for clocks that are a little off.
	totpSkew = 1
)

var totpBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// newTOTPSecret returns a new random secret.
func newTOTPSecret() []byte {
	b := make([]byte, totpSecretBytes)
	// crypto/rand.Read never fails on supported systems.
	_, _ = rand.Read(b)
	return b
}

// totpStep is the time step a moment falls in.
func totpStep(t time.Time) int64 { return t.Unix() / totpPeriod }

// totpCode returns the code of a secret for a time step.
func totpCode(secret []byte, step int64) string {
	code, err := hotp.GenerateCodeCustom(totpBase32.EncodeToString(secret), uint64(step), hotp.ValidateOpts{ // #nosec G115 -- steps are after 1970.
		Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		// The secret is always valid base32, since it is encoded here.
		return ""
	}
	return code
}

// isTOTPCode reports whether text is 6 digits.
func isTOTPCode(code string) bool {
	if len(code) != totpDigits {
		return false
	}
	for i := range len(code) {
		if !isDigit(code[i]) {
			return false
		}
	}
	return true
}

// checkTOTP checks a code against the steps around now, with totpSkew
// steps either side. Only steps after lastStep count, so a code that
// was accepted once is refused after. It returns the step whose code
// matched. Every candidate is compared in constant time, and all of them
// are always compared.
func checkTOTP(secret []byte, code string, now time.Time, lastStep int64) (int64, bool) {
	if !isTOTPCode(code) {
		return 0, false
	}
	var matched int64
	found := 0
	cur := totpStep(now)
	for step := cur - totpSkew; step <= cur+totpSkew; step++ {
		eq := subtle.ConstantTimeCompare([]byte(totpCode(secret, step)), []byte(code))
		if eq == 1 && step > lastStep && found == 0 {
			matched, found = step, 1
		}
	}
	return matched, found == 1
}

// normalizeCode takes out the spaces and dashes people type or copy
// with a code.
func normalizeCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' {
			return -1
		}
		return r
	}, strings.TrimSpace(code))
}

// totpURI is the otpauth:// URI that authenticator apps read from the QR
// code.
func totpURI(issuer, account string, secret []byte) (string, error) {
	k, err := totp.Generate(totp.GenerateOpts{
		Issuer: issuer, AccountName: account, Secret: secret,
		Period: totpPeriod, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", err
	}
	return k.URL(), nil
}

// formatKey writes a secret in base32, in groups of 4, for typing into
// an app by hand.
func formatKey(secret []byte) string {
	s := totpBase32.EncodeToString(secret)
	var b strings.Builder
	for i := 0; i < len(s); i += 4 {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s[i:min(i+4, len(s))])
	}
	return b.String()
}

// qrSVG draws text as a QR code in SVG, on the server, so the secret
// never goes to another service and the page needs no script. The SVG is
// made here from the code's modules only, so it is safe to put in the
// page as it is.
func qrSVG(text string) (template.HTML, error) {
	code, err := qr.Encode(text, qr.M, qr.Auto)
	if err != nil {
		return "", err
	}
	b := code.Bounds()
	if b.Dx() <= 0 || b.Dx() > 200 || b.Dy() != b.Dx() {
		return "", errors.New("account: unexpected QR code size")
	}
	const quiet = 4
	size := b.Dx() + 2*quiet
	var out strings.Builder
	n := strconv.Itoa(size)
	out.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" class="qr" role="img" aria-label="QR code for the authenticator app" viewBox="0 0 ` + n + " " + n + `" shape-rendering="crispEdges">`)
	out.WriteString(`<rect width="` + n + `" height="` + n + `" fill="#fff"/><path fill="#000" d="`)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if color.GrayModel.Convert(code.At(x, y)).(color.Gray).Y < 128 {
				out.WriteString("M" + strconv.Itoa(x-b.Min.X+quiet) + " " + strconv.Itoa(y-b.Min.Y+quiet) + "h1v1h-1z")
			}
		}
	}
	out.WriteString(`"/></svg>`)
	return template.HTML(out.String()), nil // #nosec G203 -- built above from numbers only.
}
