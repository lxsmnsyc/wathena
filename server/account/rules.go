package account

import (
	"bufio"
	_ "embed"
	"strings"
	"sync"
)

// Rules for account names, character names and passwords.
const (
	MinAccountName = 4
	MaxAccountName = 24
	MinCharName    = 4
	MaxCharName    = 24
	// MinPassword and MaxPassword are in bytes. The upper limit stops very
	// long inputs from costing time before hashing.
	MinPassword = 8
	MaxPassword = 256
)

// ValidAccountName reports whether a name follows the signup rules: 4 to
// 24 characters, ASCII letters, digits and underscores, starting with a
// letter. The migration checks the same rule in accounts_name_rules.
func ValidAccountName(name string) bool {
	if len(name) < MinAccountName || len(name) > MaxAccountName || !isLetter(name[0]) {
		return false
	}
	for i := range len(name) {
		c := name[i]
		if !isLetter(c) && !isDigit(c) && c != '_' {
			return false
		}
	}
	return true
}

// ValidCharacterName reports whether a name follows the rules for new
// characters: 4 to 24 characters, ASCII letters, digits and single
// spaces between words. The rule is narrow on purpose: it keeps out
// look-alike letters from other scripts until the name filter of
// strategy section 7.10 exists.
func ValidCharacterName(name string) bool {
	if len(name) < MinCharName || len(name) > MaxCharName || name[0] == ' ' || name[len(name)-1] == ' ' {
		return false
	}
	for i := range len(name) {
		c := name[i]
		switch {
		case isLetter(c), isDigit(c):
		case c == ' ' && name[i-1] != ' ':
		default:
			return false
		}
	}
	return true
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }

// PasswordProblem returns why a new password is refused, or "" when it
// is fine. The checks:
//   - 8 to 256 bytes.
//   - Not the account name, in any case.
//   - Not on the built-in list of common passwords, in any case.
func PasswordProblem(name, password string) string {
	switch {
	case len(password) < MinPassword:
		return "Use at least 8 characters for the password."
	case len(password) > MaxPassword:
		return "Use at most 256 bytes for the password."
	case strings.EqualFold(password, name):
		return "The password cannot be the account name."
	case commonPassword(password):
		return "That password is too common. Choose another one."
	}
	return ""
}

// common-passwords.txt lists common passwords of 8 characters or more,
// one per line, in lowercase. It is small on purpose. A server owner
// who wants a full breach list can check passwords in front of this.
//
//go:embed common-passwords.txt
var commonPasswordsFile string

var commonPasswords = sync.OnceValue(func() map[string]bool {
	m := map[string]bool{}
	s := bufio.NewScanner(strings.NewReader(commonPasswordsFile))
	for s.Scan() {
		if line := strings.TrimSpace(s.Text()); line != "" && line[0] != '#' {
			m[line] = true
		}
	}
	return m
})

func commonPassword(p string) bool { return commonPasswords()[strings.ToLower(p)] }
