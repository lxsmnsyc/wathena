// Package client gives Go programs the built web client.
//
// waserver serves it at the root of its public URL. A binary built with
// the webclient build tag embeds client/dist, which `pnpm build` makes
// first. Without the tag, Dist returns nil, so `go build ./...` works
// without Node.js.
package client
