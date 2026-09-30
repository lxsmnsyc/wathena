#!/usr/bin/env bash
# Checks that game code only runs the queries sqlc generates, which always
# bind their parameters (strategy section 7.2). In every Go file that
# imports a database package, outside tests and the generated code:
#   - fmt.Sprint, Sprintf and Sprintln are rejected, since they are how
#     SQL gets built from strings.
#   - Raw query calls (Exec, Query, QueryRow, SendBatch, CopyFrom) are
#     rejected. Queries go in server/store/queries and through sqlc.
# It is a text check. It does not follow values across files.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
fail=0
while IFS= read -r f; do
	case "$f" in
	*_test.go | server/store/db/*) continue ;;
	esac
	grep -qE '"(github\.com/jackc/pgx/v5[^"]*|database/sql)"' "$f" || continue
	if grep -nE 'fmt\.Sprint' "$f"; then
		echo "$f: fmt.Sprint in a file that uses the database; use a sqlc query with parameters" >&2
		fail=1
	fi
	if grep -nE '\.(Exec|Query|QueryRow|SendBatch|CopyFrom)\(' "$f"; then
		echo "$f: a raw query; add it to server/store/queries and run sqlc" >&2
		fail=1
	fi
done < <(git ls-files --cached --others --exclude-standard '*.go')
exit "$fail"
