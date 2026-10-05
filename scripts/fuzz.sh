#!/bin/sh
# Runs every fuzz test in the module for the time given as the first
# argument (10s when it is missing).
#   - It finds the fuzz tests in the test files that go list names for
#     each package, so it does not compile every package to list them.
#   - It runs FUZZ_JOBS tests at once, 4 when it is not set. Each test
#     gets -parallel of the CPU count divided by the jobs, so together
#     they use every CPU.
#   - It prints each test's output when the test ends, and fails when a
#     test failed.
set -eu
fuzztime="${1:-10s}"
jobs="${FUZZ_JOBS:-4}"
cpus=$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)
parallel=$((cpus / jobs))
if [ "$parallel" -lt 1 ]; then
	parallel=1
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# One line per fuzz test: the package, then the test's name.
go list -f '{{.ImportPath}} {{.Dir}} {{join .TestGoFiles " "}} {{join .XTestGoFiles " "}}' ./... |
	while read -r pkg dir files; do
		for f in $files; do
			sed -n 's/^func \(Fuzz[A-Za-z0-9_]*\)([A-Za-z0-9_]* \*testing\.F).*/\1/p' "$dir/$f"
		done | sort -u | while read -r name; do
			echo "$pkg $name"
		done
	done >"$work/targets"
echo "$(wc -l <"$work/targets" | tr -d ' ') fuzz tests, $jobs at once, -parallel $parallel each"

: >"$work/failed"
export fuzztime parallel work
# Each line runs in its own shell. Its output goes to a file, which is
# printed in one piece when the test ends.
xargs -P "$jobs" -L 1 sh -c '
	pkg=$1 name=$2
	log=$(mktemp "$work/log.XXXXXX")
	echo "== $pkg $name" >"$log"
	if ! go test -run "^\$" -fuzz "^${name}\$" -fuzztime "$fuzztime" -fuzzminimizetime 5s -parallel "$parallel" "$pkg" >>"$log" 2>&1; then
		echo "$pkg $name" >>"$work/failed"
	fi
	cat "$log"
' sh <"$work/targets" || true

if [ -s "$work/failed" ]; then
	echo "fuzz tests that failed:"
	cat "$work/failed"
	exit 1
fi
