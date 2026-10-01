// Package secretenv reads secrets from environment variables.
//
// A secret NAME comes from the variable NAME, or from the file that
// NAME_FILE names, as Docker secrets and the postgres image do. Errors
// name the variable or the file, and never quote the value.
package secretenv

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// maxSize is the largest secret file read. Secrets are short.
const maxSize = 64 << 10

// Get returns the secret NAME, or "" when neither NAME nor NAME_FILE is
// set. Setting both is an error. Trailing line breaks in a file are
// removed.
func Get(name string) (string, error) {
	value := os.Getenv(name)
	file := os.Getenv(name + "_FILE")
	switch {
	case file == "":
		return value, nil
	case value != "":
		return "", fmt.Errorf("set %s or %s_FILE, not both", name, name)
	}
	f, err := os.Open(file) // #nosec G304 G703 -- the file the operator named.
	if err != nil {
		return "", fmt.Errorf("%s_FILE: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return "", fmt.Errorf("%s_FILE: %w", name, err)
	}
	if len(data) > maxSize {
		return "", fmt.Errorf("%s_FILE: %s is larger than 64 KiB", name, file)
	}
	s := strings.TrimRight(string(data), "\r\n")
	if s == "" {
		return "", fmt.Errorf("%s_FILE: %s is empty", name, file)
	}
	return s, nil
}
