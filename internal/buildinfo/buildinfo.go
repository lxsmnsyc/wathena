// Package buildinfo names the version of a wAthena binary, for the
// version commands of waserver and wactl (decision 0047).
package buildinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/lxsmnsyc/wathena/shared/gen"
)

// Version returns a binary's version:
//
//   - The stamp, when a release build set one with
//     -ldflags "-X main.buildVersion=...". scripts/release.sh stamps the
//     output of git describe, and the Dockerfile its VERSION argument.
//   - Otherwise the module version and the VCS revision built into the
//     binary, or "unknown" without build information.
func Version(stamp string) string {
	if stamp != "" {
		return stamp
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	v := info.Main.Version
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			v += " " + s.Value
		case "vcs.modified":
			if s.Value == "true" {
				v += " (modified)"
			}
		}
	}
	return v
}

// Line is the line a version command prints: the version and the world
// modes of the build, such as "v0.1.0 (modes: classic, procedural)".
// The modes show whether the binary was built with -tags procedural.
func Line(stamp string) string {
	return Version(stamp) + " (modes: " + strings.Join(gen.Modes(), ", ") + ")"
}

// ID names the code of the running binary, for caches of what it
// computes: its build information when that names a VCS revision with
// no changes, else the SHA-256 of its executable. Release builds made
// with -buildvcs=false, such as the container image, take the hash.
func ID() (string, error) {
	if info, ok := debug.ReadBuildInfo(); ok {
		clean := false
		revision := ""
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value
			case "vcs.modified":
				clean = s.Value == "false"
			}
		}
		if revision != "" && clean {
			return "build " + info.String(), nil
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	f, err := os.Open(exe) // #nosec G304 -- the running executable.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "executable " + hex.EncodeToString(h.Sum(nil)), nil
}
