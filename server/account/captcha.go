package account

import (
	"context"
	"html/template"
	"net/netip"
	"net/url"
)

// Captcha checks that a person, not a script, sends a form:
//   - The login page asks for it after repeated failures
//     (Limits.AccountCaptcha and Limits.IPCaptcha).
//   - The signup page always asks for it.
//
// A real provider, such as hCaptcha, Turnstile or a self-hosted proof
// of work, implements this interface. It is out of scope for now.
type Captcha interface {
	// Field returns the HTML to show inside the login form.
	Field() template.HTML
	// Verify checks the answer in a submitted form. addr is the
	// client's address, which some providers want.
	Verify(ctx context.Context, form url.Values, addr netip.Addr) (bool, error)
	// Sources lists origins the provider's script and frame load from.
	// They are added to the pages' Content-Security-Policy.
	Sources() []string
}

// NoCaptcha asks nothing and passes every form. The rate limits and the
// waits after failures still apply.
type NoCaptcha struct{}

func (NoCaptcha) Field() template.HTML { return "" }
func (NoCaptcha) Verify(context.Context, url.Values, netip.Addr) (bool, error) {
	return true, nil
}
func (NoCaptcha) Sources() []string { return nil }

// DevCaptcha is a checkbox, for development and tests. It shows where a
// real captcha goes and that the form is refused without it. It stops
// no script.
type DevCaptcha struct{}

func (DevCaptcha) Field() template.HTML {
	return `<label class="captcha"><input type="checkbox" name="dev_captcha" value="yes" required> I am a person (development check)</label>`
}

func (DevCaptcha) Verify(_ context.Context, form url.Values, _ netip.Addr) (bool, error) {
	return form.Get("dev_captcha") == "yes", nil
}

func (DevCaptcha) Sources() []string { return nil }
