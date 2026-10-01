package botclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Account uses the account service's pages as a browser does, so a bot
// can sign up, log in, make a character and get a game ticket, instead
// of using --dev sessions.
type Account struct {
	// Base is the server's URL, such as http://127.0.0.1:8300.
	Base string
	// Origin is the web client origin that asks for tickets. It must be
	// one of the server's --origins.
	Origin string

	client *http.Client
}

// NewAccount returns an Account with an empty cookie jar.
func NewAccount(base, origin string) *Account {
	jar, _ := cookiejar.New(nil)
	return &Account{Base: strings.TrimSuffix(base, "/"), Origin: origin, client: &http.Client{
		Jar: jar,
		// Redirects show whether a form worked, so they are not followed.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

var csrfField = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

// submit loads a page for its CSRF token, then posts a form. A form that
// worked answers with a redirect.
func (a *Account) submit(ctx context.Context, from, to string, form url.Values) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.Base+from, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if err != nil {
		return err
	}
	m := csrfField.FindSubmatch(body)
	if m == nil {
		return fmt.Errorf("botclient: %s has no form (status %d)", from, resp.StatusCode)
	}
	form.Set("csrf", string(m[1]))
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, a.Base+to, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = a.client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		return fmt.Errorf("botclient: %s: status %d", to, resp.StatusCode)
	}
	return nil
}

// SignUp makes an account and logs in.
func (a *Account) SignUp(ctx context.Context, name, password string) error {
	return a.submit(ctx, "/account/signup", "/account/signup", url.Values{"name": {name}, "password": {password}, "confirm": {password}})
}

// Login logs in to an account without two-factor login.
func (a *Account) Login(ctx context.Context, name, password string) error {
	return a.submit(ctx, "/account/login", "/account/login", url.Values{"name": {name}, "password": {password}})
}

// MakeCharacter makes a character in a slot of the character list.
func (a *Account) MakeCharacter(ctx context.Context, name string, slot int) error {
	return a.submit(ctx, "/account/characters", "/account/characters", url.Values{
		"name": {name}, "slot": {strconv.Itoa(slot)}, "sex": {"f"}, "hair": {"3"}, "hair_color": {"2"},
	})
}

// Play picks the character in a slot, as the Play button does.
func (a *Account) Play(ctx context.Context, slot int) error {
	return a.submit(ctx, "/account/characters", "/account/play", url.Values{"slot": {strconv.Itoa(slot)}})
}

// Ticket gets a one-time game ticket for the character picked with
// Play, as the web client does. It is the session token for Dial.
func (a *Account) Ticket(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Base+"/account/ticket", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Origin", a.Origin)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Ticket string `json:"ticket"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || body.Ticket == "" {
		return nil, errors.New("botclient: no ticket: " + body.Error)
	}
	return []byte(body.Ticket), nil
}
