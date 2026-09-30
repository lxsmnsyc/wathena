package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/character"
)

// offlineService makes a service whose pool never connects. It serves
// the pages that need no database.
func offlineService(t *testing.T, publicURL string) *Service {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s, err := New(Config{
		Pool: pool, PublicURL: publicURL,
		ClientOrigins: []string{"https://play.example.com"},
		NewCharacter:  func(string, Look) character.Character { return character.Character{} },
		Hasher:        NewHasher(fastParams, 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

var csrfField = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

func TestSecurityHeaders(t *testing.T) {
	s := offlineService(t, "https://account.example.com")
	for _, path := range []string{"/account/login", "/account/signup", "/account/style.css", "/account/nope"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		h := w.Header()
		csp := h.Get("Content-Security-Policy")
		for _, part := range []string{"default-src 'none'", "script-src 'none'", "frame-ancestors 'none'", "form-action 'self' https://play.example.com", "base-uri 'none'", "style-src 'self'"} {
			if !strings.Contains(csp, part) {
				t.Errorf("%s: CSP %q lacks %q", path, csp, part)
			}
		}
		for k, v := range map[string]string{
			"X-Content-Type-Options":    "nosniff",
			"X-Frame-Options":           "DENY",
			"Referrer-Policy":           "no-referrer",
			"Strict-Transport-Security": "max-age=31536000",
		} {
			if h.Get(k) != v {
				t.Errorf("%s: %s = %q", path, k, h.Get(k))
			}
		}
		if path != "/account/style.css" && h.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: Cache-Control %q", path, h.Get("Cache-Control"))
		}
	}
}

func TestCookieFlags(t *testing.T) {
	for _, c := range []struct {
		url    string
		secure bool
		name   string
	}{
		{"https://account.example.com", true, "__Host-wa_csrf"},
		{"http://localhost:8300", false, "wa_csrf"},
	} {
		s := offlineService(t, c.url)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", "/account/login", nil))
		cookies := w.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("%s: cookies %v", c.url, cookies)
		}
		k := cookies[0]
		if k.Name != c.name || !k.HttpOnly || k.Secure != c.secure || k.SameSite != http.SameSiteStrictMode || k.Path != "/" || k.Domain != "" {
			t.Errorf("%s: cookie %+v", c.url, k)
		}
		if strings.Contains(w.Header().Get("Strict-Transport-Security"), "max-age") != c.secure {
			t.Errorf("%s: HSTS %q", c.url, w.Header().Get("Strict-Transport-Security"))
		}
	}
}

// formPage loads a page and returns its CSRF token and cookies.
func formPage(t *testing.T, s *Service, path string) (string, []*http.Cookie) {
	t.Helper()
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	m := csrfField.FindStringSubmatch(w.Body.String())
	if m == nil {
		t.Fatalf("%s has no CSRF field", path)
	}
	return m[1], w.Result().Cookies()
}

func post(s *Service, path string, form url.Values, cookies []*http.Cookie, header map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range header {
		r.Header.Set(k, v)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestCSRF(t *testing.T) {
	s := offlineService(t, "http://localhost:8300")
	token, cookies := formPage(t, s, "/account/signup")
	// Each case breaks the form in one way. A form that passes the CSRF
	// check reaches the name rules, which refuse "x" with 400.
	good := url.Values{"csrf": {token}, "name": {"x"}}
	wrong := []byte(token)
	wrong[0] ^= 1
	cases := []struct {
		name    string
		form    url.Values
		cookies []*http.Cookie
		header  map[string]string
		status  int
	}{
		{"valid", good, cookies, nil, http.StatusBadRequest},
		{"same-origin fetch metadata", good, cookies, map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusBadRequest},
		{"own origin", good, cookies, map[string]string{"Origin": "http://localhost:8300"}, http.StatusBadRequest},
		{"no token", url.Values{"name": {"x"}}, cookies, nil, http.StatusForbidden},
		{"wrong token", url.Values{"csrf": {string(wrong)}, "name": {"x"}}, cookies, nil, http.StatusForbidden},
		{"no cookie", good, nil, nil, http.StatusForbidden},
		{"cross-site", good, cookies, map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"same-site", good, cookies, map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{"other origin", good, cookies, map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
	}
	for _, c := range cases {
		if w := post(s, "/account/signup", c.form, c.cookies, c.header); w.Code != c.status {
			t.Errorf("%s: status %d, want %d", c.name, w.Code, c.status)
		}
	}
	// A token is bound to the session cookie: a token made without a
	// session fails once a session cookie is present.
	withSession := append([]*http.Cookie{{Name: "wa_session", Value: strings.Repeat("A", 43)}}, cookies...)
	if w := post(s, "/account/signup", good, withSession, nil); w.Code != http.StatusForbidden {
		t.Errorf("token from before login: status %d", w.Code)
	}
}

func TestTicketOrigins(t *testing.T) {
	s := offlineService(t, "https://account.example.com")
	for origin, want := range map[string]int{
		"":                            http.StatusForbidden,
		"https://evil.example":        http.StatusForbidden,
		"http://play.example.com":     http.StatusForbidden, // wrong scheme
		"https://play.example.com":    http.StatusNoContent,
		"https://account.example.com": http.StatusNoContent,
	} {
		r := httptest.NewRequest("OPTIONS", "/account/ticket", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != want {
			t.Errorf("%q: status %d, want %d", origin, w.Code, want)
		}
		allow := w.Header().Get("Access-Control-Allow-Origin")
		if want == http.StatusNoContent && allow != origin || want != http.StatusNoContent && allow != "" {
			t.Errorf("%q: Access-Control-Allow-Origin %q", origin, allow)
		}
	}
}

func TestSafeReturn(t *testing.T) {
	s := offlineService(t, "https://account.example.com")
	for raw, ok := range map[string]bool{
		"https://play.example.com/?server=wss://x/ws":   true,
		"https://play.example.com":                      true,
		"https://play.example.com.evil.example/":        false,
		"https://evil.example/https://play.example.com": false,
		"//play.example.com/":                           false,
		"javascript:alert(1)":                           false,
		"https://user@play.example.com/":                false,
		"/account/characters":                           false,
	} {
		if got := s.safeReturn(raw) != ""; got != ok {
			t.Errorf("safeReturn(%q) = %v", raw, got)
		}
	}
}

func TestClientAddr(t *testing.T) {
	s := offlineService(t, "")
	s.cfg.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	for _, c := range []struct {
		remote, xff, want string
	}{
		{"203.0.113.9:5000", "", "203.0.113.9"},
		{"203.0.113.9:5000", "1.2.3.4", "203.0.113.9"}, // untrusted peer: header ignored
		{"10.0.0.2:5000", "1.2.3.4", "1.2.3.4"},
		{"10.0.0.2:5000", "6.6.6.6, 1.2.3.4, 10.0.0.9", "1.2.3.4"},
		{"10.0.0.2:5000", "junk", "10.0.0.2"},
		{"[::ffff:203.0.113.9]:5000", "", "203.0.113.9"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := s.clientAddr(r).String(); got != c.want {
			t.Errorf("%s %q: %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	pool, _ := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none")
	defer pool.Close()
	nc := func(string, Look) character.Character { return character.Character{} }
	for name, cfg := range map[string]Config{
		"no pool":            {NewCharacter: nc},
		"bad public URL":     {Pool: pool, NewCharacter: nc, PublicURL: "ftp://x"},
		"origin with path":   {Pool: pool, NewCharacter: nc, ClientOrigins: []string{"https://x.example/play"}},
		"untrusted play URL": {Pool: pool, NewCharacter: nc, ClientOrigins: []string{"https://a.example"}, PlayURL: "https://b.example/"},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
