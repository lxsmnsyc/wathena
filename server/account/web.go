package account

import (
	"crypto/hmac"
	"crypto/sha256"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Cookie names. Over HTTPS they take the __Host- prefix, which makes the
// browser refuse them unless they are Secure, have Path=/ and no Domain,
// so a neighbour subdomain cannot set or replace them.
const (
	sessionCookie = "wa_session"
	csrfCookie    = "wa_csrf"
	// pendingCookie holds a login that waits for its second factor.
	pendingCookie = "wa_pending"
	// ceremonyCookie holds a passkey ceremony between its two steps.
	ceremonyCookie = "wa_webauthn"
	hostPrefix     = "__Host-"
)

func (s *Service) cookieName(base string) string {
	if s.secure {
		return hostPrefix + base
	}
	return base
}

// setCookie writes a cookie with the flags every account cookie has:
// HttpOnly, SameSite=Strict, Path=/, and Secure over HTTPS. maxAge < 0
// deletes it.
func (s *Service) setCookie(w http.ResponseWriter, base, value string, maxAge int) {
	// #nosec G124 -- Secure is set whenever PublicURL is https. A plain
	// http PublicURL is for development on localhost.
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName(base),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Service) cookie(r *http.Request, base string) string {
	c, err := r.Cookie(s.cookieName(base))
	if err != nil {
		return ""
	}
	return c.Value
}

// securityHeaders are sent with every response of the account service.
func (s *Service) securityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Security-Policy", s.csp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-site")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
	// Pages hold CSRF tokens and account data, so no cache keeps them.
	h.Set("Cache-Control", "no-store")
	if s.secure {
		h.Set("Strict-Transport-Security", "max-age=31536000")
	}
}

// buildCSP returns the pages' Content-Security-Policy:
//   - No scripts, except a captcha provider's. With self, also the
//     service's own script files, for the pages with passkey buttons.
//     Inline scripts and eval stay off either way.
//   - Styles and images from the service's own origin only.
//   - Forms post to the service, and Play's redirect may go to the
//     trusted client origins.
//   - No framing, no <base>, no plugins.
func buildCSP(clients, captcha []string, self bool) string {
	src := func(list []string) string {
		if len(list) == 0 {
			return ""
		}
		return " " + strings.Join(list, " ")
	}
	var scripts []string
	if self {
		scripts = append(scripts, "'self'")
	}
	scripts = append(scripts, captcha...)
	script := "'none'"
	if len(scripts) > 0 {
		script = strings.Join(scripts, " ")
	}
	return "default-src 'none'; script-src " + script +
		"; frame-src" + orNone(src(captcha)) +
		"; connect-src" + orNone(src(scripts)) +
		"; style-src 'self'; img-src 'self'" +
		"; form-action 'self'" + src(clients) +
		"; frame-ancestors 'none'; base-uri 'none'"
}

func orNone(s string) string {
	if s == "" {
		return " 'none'"
	}
	return s
}

// csrfToken returns the token forms must send back. It is an HMAC of the
// CSRF cookie and the session cookie under a key only the server holds:
//   - A page on another site cannot read the cookie, so it cannot make
//     the token.
//   - Logging in changes the session cookie, so a token from before the
//     login stops working.
func (s *Service) csrfToken(csrf, session string) string {
	m := hmac.New(sha256.New, s.csrfKey)
	m.Write([]byte("wathena-csrf\x00"))
	m.Write([]byte(csrf))
	m.Write([]byte{0})
	m.Write([]byte(session))
	return tokenEncoding.EncodeToString(m.Sum(nil))
}

// ensureCSRF returns the request's CSRF cookie, setting a new one when
// there is none.
func (s *Service) ensureCSRF(w http.ResponseWriter, r *http.Request) string {
	if v := s.cookie(r, csrfCookie); len(v) == TokenLen {
		return v
	}
	v, _ := newToken()
	s.setCookie(w, csrfCookie, v, 0)
	return v
}

// checkCSRF checks a form post:
//   - The browser's Sec-Fetch-Site, when sent, must be same-origin.
//   - Otherwise Origin, when sent, must be the service's own origin.
//   - The form's csrf field must match the cookies, compared in
//     constant time.
func (s *Service) checkCSRF(r *http.Request) bool {
	return s.checkCSRFValue(r, r.PostFormValue("csrf"))
}

// checkCSRFValue checks a post as checkCSRF does, with the token given
// directly, such as from the X-CSRF-Token header of a script's fetch.
func (s *Service) checkCSRFValue(r *http.Request, token string) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		if site != "same-origin" {
			return false
		}
	} else if o := r.Header.Get("Origin"); o != "" && o != s.ownOrigin(r) {
		return false
	}
	csrf := s.cookie(r, csrfCookie)
	if len(csrf) != TokenLen {
		return false
	}
	want := s.csrfToken(csrf, s.cookie(r, sessionCookie))
	return hmac.Equal([]byte(token), []byte(want))
}

// ownOrigin is the origin pages are served from: PublicURL's, or the
// request's host when PublicURL is not set.
func (s *Service) ownOrigin(r *http.Request) string {
	if s.origin != "" {
		return s.origin
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// originOf returns the scheme, host and port of a URL, or "" when it is
// not an absolute http or https URL.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// clientAddr returns the client's address. A request from a trusted
// proxy is followed back through X-Forwarded-For, from the right, past
// every trusted proxy.
func (s *Service) clientAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.IPv4Unspecified()
	}
	addr = addr.Unmap()
	if !s.trusted(addr) {
		return addr
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return addr
		}
		addr = a.Unmap()
		if !s.trusted(addr) {
			return addr
		}
	}
	return addr
}

func (s *Service) trusted(a netip.Addr) bool {
	for _, p := range s.cfg.TrustedProxies {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// userAgent returns the request's User-Agent as valid UTF-8 of at most
// 256 characters, for the session list.
func userAgent(r *http.Request) string {
	ua := strings.ToValidUTF8(r.UserAgent(), "")
	if utf8.RuneCountInString(ua) <= 256 {
		return ua
	}
	return string([]rune(ua)[:256])
}
