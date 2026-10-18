//go:build cgo && !luawasm && (amd64 || arm64)

// Lua and the binding, compiled by cgo. See csrc/all.c.
//
// The Go build cache only sees files in this directory, so the line below
// holds a digest of csrc/ and lua/src/. Changing a C source changes this
// file, and cgo compiles again. TestSourcesDigest checks it; run
// go test -run TestSourcesDigest -update after editing C sources.
//
// sources sha256: cfbd681cc9ccc0900574f3ee022bbd9ff580fdd7139f7de530ba3fe184e75318
#include "csrc/all.c"
