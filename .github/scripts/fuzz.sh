#!/bin/sh
# Runs every fuzz test in the module for the time given as the first argument.
set -eu
fuzztime="${1:-10s}"
for pkg in $(go list ./...); do
	for name in $(go test -list '^Fuzz' "$pkg" | grep '^Fuzz' || true); do
		echo "== $pkg $name"
		go test -run '^$' -fuzz "^${name}\$" -fuzztime "$fuzztime" "$pkg"
	done
done
