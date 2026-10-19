// Package rathena embeds the reference files that waserver reads at
// startup, so a release binary needs no copy of this folder.
//
// waserver imports it, and wactl for its mod commands, which run
// outside the repository. Tests and other tools read the folder on disk
// through internal/reference.
//
// The extracts stay JSON in git (review decision 56). How they go into
// a binary depends on the build:
//   - A plain go build or go test embeds the JSON files (embed.go), so
//     a clean checkout builds with no extra step.
//   - The gzextracts tag embeds gzipped copies from the gz folder
//     instead (embed_gz.go), and Files decompresses a file when it is
//     read. go generate makes the gz folder, which git ignores.
//     scripts/release.sh and the Dockerfile run it before they build
//     with the tag.
package rathena

//go:generate go run ./internal/gzextracts -out gz
