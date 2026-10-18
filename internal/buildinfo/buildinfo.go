// Package buildinfo names the version of a wAthena binary, for the
// version commands of waserver and wactl (decision 0047).
package buildinfo

import (
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
