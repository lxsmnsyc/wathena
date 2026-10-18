package account

import (
	"strings"
	"testing"
)

func TestValidAccountName(t *testing.T) {
	for name, want := range map[string]bool{
		"Alice":                 true,
		"bob_99":                true,
		"abcd":                  true,
		strings.Repeat("a", 24): true,
		"abc":                   false,
		strings.Repeat("a", 25): false,
		"9lives":                false,
		"_under":                false,
		"with space":            false,
		"dash-name":             false,
		"Ålice":                 false,
		"аlice":                 false, // Cyrillic a
		"alice\x00":             false,
		"":                      false,
	} {
		if got := ValidAccountName(name); got != want {
			t.Errorf("ValidAccountName(%q) = %v", name, got)
		}
	}
}

func TestValidCharacterName(t *testing.T) {
	for name, want := range map[string]bool{
		"Alice":                 true,
		"Sir Poring 2":          true,
		"abcd":                  true,
		strings.Repeat("a", 24): true,
		"abc":                   false,
		strings.Repeat("a", 25): false,
		" lead":                 false,
		"trail ":                false,
		"two  spaces":           false,
		"tab\tname":             false,
		"under_score":           false,
		"<script>":              false,
		"Ålice":                 false,
	} {
		if got := ValidCharacterName(name); got != want {
			t.Errorf("ValidCharacterName(%q) = %v", name, got)
		}
	}
}

func TestPasswordProblem(t *testing.T) {
	for _, c := range []struct {
		name, password string
		ok             bool
	}{
		{"alice", "a fine long password", true},
		{"alice", "short", false},
		{"alice", strings.Repeat("x", 256) + "y", false},
		{"alice", strings.Repeat("x", 250) + "yz", true},
		{"LongAccount", "longaccount", false},
		{"alice", "Password1", false},
		{"alice", "RAGNAROK", false},
		{"alice", "12345678", false},
		{"alice", "password123", false},
		{"alice", "QWERTY123", false},
		{"alice", "qwerty123", false},
		{"alice", "iloveyou2", false},
		{"alice", "plum tractor velvet orbit", true},
		{"alice", "Xk7#mQ2v-rTq9", true},
	} {
		if got := PasswordProblem(c.name, c.password) == ""; got != c.ok {
			t.Errorf("PasswordProblem(%q, %q) ok = %v", c.name, c.password, got)
		}
	}
	for p := range commonPasswords() {
		if len(p) < MinPassword || strings.ToLower(p) != p {
			t.Errorf("common password %q is shorter than the minimum or not lowercase", p)
		}
	}
}
