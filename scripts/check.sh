#!/usr/bin/env bash
# Runs the checks of .github/workflows/ci.yml on this machine
# (decision 0017). GitHub Actions does not run for this repository, so
# this script is the check before a commit.
#
#   scripts/check.sh              the default steps
#   scripts/check.sh all          the default steps, then compose and browsers
#   scripts/check.sh lint e2e     only the steps named
#
# Steps, with the CI job each one replaces:
#   vet       go vet ./...                                        (test)
#   test      go test without -race on every package but ./e2e,
#             then go test -race ./.... Heavy tests on one
#             goroutine skip under -race (decision 0022).         (test)
#   mods      generated Lua files, the sample mods, a new mod     (mods)
#   lua       vendored Lua, lua.wasm, the WebAssembly build,
#             and the release builds with zig                     (lua, lua-macos)
#   loadtest  the 50-bot smoke test                               (loadtest)
#   schema    buf lint and the generated protocol code            (schema)
#   queries   the generated sqlc code                             (queries)
#   lint      golangci-lint                                       (lint)
#   security  govulncheck, gosec and sqlcheck.sh                  (security)
#   fuzz      every fuzz test for 10 seconds, 4 at once           (fuzz)
#   client    pnpm check, lint, format:check, test and build      (client)
#   tinygo    the shared packages under TinyGo, and path.wasm     (tinygo)
#   e2e       the e2e package alone, with -race. test runs it
#             too, so it is not a default step.
#   compose   the Compose stack smoke test. Only with all.        (compose)
#   browsers  the client in Chromium. Only with all.              (browsers)
#
# Rules:
#   - Tests that need PostgreSQL read WATHENA_TEST_DATABASE_URL. When it
#     is not set and Docker runs, the script starts a postgres:18
#     container for the run and removes it at the end.
#     CHECK_NO_DOCKER=1 turns that off, and those tests skip.
#   - A check of generated files compares them before and after the
#     generator runs, so uncommitted changes do not fail it.
#   - A step whose tool is missing is reported as skipped, with the
#     reason. Skipped is not passed.
#   - The script runs every step, lists each result at the end, and
#     exits with 1 when a step failed.
set -uo pipefail
root=$(git rev-parse --show-toplevel)
cd "$root"

# Versions pinned as in ci.yml.
buf_version=v1.73.0
sqlc_version=v1.31.1
golangci_version=2.14.0
govulncheck_version=v1.8.0
gosec_version=v2.29.0

default_steps="vet test mods lua loadtest schema queries lint security fuzz client tinygo"
all_steps="$default_steps compose browsers"
known_steps="$all_steps e2e"

steps=()
if [ $# -eq 0 ]; then
	read -r -a steps <<<"$default_steps"
elif [ "$1" = all ] && [ $# -eq 1 ]; then
	read -r -a steps <<<"$all_steps"
else
	for s in "$@"; do
		case " $known_steps " in
		*" $s "*) steps+=("$s") ;;
		*)
			echo "check.sh: unknown step $s; the steps are: $known_steps" >&2
			exit 2
			;;
		esac
	done
fi

work=$(mktemp -d)
pg_container=""
cleanup() {
	if [ -n "$pg_container" ]; then
		docker rm -f "$pg_container" >/dev/null 2>&1 || true
	fi
	rm -rf "$work"
}
trap cleanup EXIT

results=()
failed=0

# skip ends a step as skipped, with the reason.
skip() {
	echo "skipped: $*"
	exit 3
}

# have reports whether a command is on PATH.
have() { command -v "$1" >/dev/null 2>&1; }

# unchanged runs a generator and fails when it changes any of the paths.
# Usage: unchanged path... -- command...
unchanged() {
	local paths=() snap
	while [ "$1" != "--" ]; do
		paths+=("$1")
		shift
	done
	shift
	snap=$(mktemp -d "$work/snap.XXXXXX")
	local p
	for p in "${paths[@]}"; do
		mkdir -p "$snap/$(dirname "$p")"
		if [ -e "$p" ]; then
			cp -R "$p" "$snap/$p"
		fi
	done
	"$@" || return 1
	local changed=0
	for p in "${paths[@]}"; do
		if ! diff -r "$snap/$p" "$p" >/dev/null 2>&1; then
			echo "generated files changed: $p"
			diff -r "$snap/$p" "$p" | head -40
			changed=1
		fi
	done
	return "$changed"
}

# database sets WATHENA_TEST_DATABASE_URL, starting a container when
# needed.
database() {
	if [ -n "${WATHENA_TEST_DATABASE_URL:-}" ] || [ -n "$pg_container" ]; then
		return 0
	fi
	if [ "${CHECK_NO_DOCKER:-}" = 1 ] || ! have docker || ! docker info >/dev/null 2>&1; then
		echo "note: no WATHENA_TEST_DATABASE_URL and no Docker; the database tests skip"
		return 0
	fi
	pg_container="wathena-check-pg-$$"
	echo "starting PostgreSQL 18 in the container $pg_container"
	if ! docker run -d --name "$pg_container" -e POSTGRES_PASSWORD=postgres -p 127.0.0.1::5432 postgres:18 >/dev/null; then
		pg_container=""
		echo "note: the PostgreSQL container did not start; the database tests skip"
		return 0
	fi
	local port i
	port=$(docker port "$pg_container" 5432/tcp | head -1 | sed 's/.*://')
	for i in $(seq 60); do
		if docker exec "$pg_container" pg_isready -U postgres -h 127.0.0.1 >/dev/null 2>&1; then
			break
		fi
		sleep 1
	done
	export WATHENA_TEST_DATABASE_URL="postgres://postgres:postgres@127.0.0.1:$port/postgres?sslmode=disable"
}

step_vet() { go vet ./...; }

# The race detector only finds races in code that runs goroutines, and
# it makes code 5 to 20 times slower. So the tests run twice:
#   - Without it, every test of every package but ./e2e runs.
#   - With it, every package runs, ./e2e too. The heavy tests that run
#     on one goroutine, such as the parity and golden tests, skip
#     (internal/testrace).
# ./e2e runs in the second pass only, as before. Beside the busy first
# pass, its duplication tests did too little work in their time.
step_test() {
	# shellcheck disable=SC2046 # one argument per package.
	go test -timeout 45m $(go list ./... | grep -v '/e2e$')
	go test -race -timeout 45m ./...
}

step_e2e() { go test -race -count=1 -timeout 45m ./e2e/; }

step_mods() {
	local wactl="$work/wactl"
	go build -o "$wactl" ./tools/wactl
	go test -count=1 -run TestGeneratedFiles ./server/modscript/luaapi
	local m
	for m in mods/examples/*/; do
		"$wactl" mod validate "$m" --mods mods/examples
		"$wactl" mod test "$m" --mods mods/examples
	done
	(
		cd "$work"
		./wactl mod new example.ci
		./wactl mod validate example.ci
		./wactl mod test example.ci
		./wactl mod pack example.ci --out dist
		mkdir installed
		./wactl mod install dist/example.ci-0.1.0.zip --mods-dir installed
	)
}

step_lua() {
	if have curl; then
		internal/script/lua/update.sh --check
	else
		echo "part skipped: the vendored Lua check needs curl"
	fi
	if [ -n "${WASI_SDK:-}" ]; then
		unchanged internal/script/wasm/lua.wasm -- internal/script/wasm/build.sh
	else
		echo "part skipped: the lua.wasm check needs WASI_SDK, the path of wasi-sdk 34"
	fi
	CGO_ENABLED=0 go test -count=1 ./internal/script/...
	go test -count=1 -tags luawasm ./internal/script/...
	if ! have zig; then
		echo "part skipped: the static musl test and the release builds need zig"
		return 0
	fi
	if [ "$(uname -s)" = Linux ] && [ "$(uname -m)" = x86_64 ]; then
		CGO_ENABLED=1 CC="zig cc -target x86_64-linux-musl" go test -c -o "$work/script.test" -tags netgo,osusergo \
			-ldflags "-linkmode external -extldflags '-static -Wl,-z,stack-size=8388608'" ./internal/script
		(cd internal/script && "$work/script.test" -test.count=1)
	else
		echo "part skipped: the static musl test runs on Linux x86_64 only"
	fi
	local static="-linkmode external -extldflags '-static -s -Wl,-z,stack-size=8388608'"
	build() {
		echo "== $1/$2"
		CGO_ENABLED=1 GOOS=$1 GOARCH=$2 CC="zig cc -target $3" go build -trimpath -tags netgo,osusergo \
			-ldflags "-s -w $4" -o "$work/waserver-$1-$2" ./server/cmd/waserver
	}
	build linux amd64 x86_64-linux-musl "$static"
	build linux arm64 aarch64-linux-musl "$static"
	build windows amd64 x86_64-windows-gnu ""
	if [ "$(uname -s)" = Darwin ] && have xcrun; then
		local sdk
		sdk=$(xcrun --show-sdk-path)
		CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
			CC="zig cc -target aarch64-macos --sysroot $sdk -L/usr/lib -F$sdk/System/Library/Frameworks" \
			go build -trimpath -tags netgo,osusergo -ldflags "-s -w" -o "$work/waserver-darwin-arm64" ./server/cmd/waserver
		if [ "$(uname -m)" = arm64 ]; then
			"$work/waserver-darwin-arm64" version
		fi
	fi
	if have file; then
		file "$work"/waserver-*
	fi
	if [ "$(uname -s)" = Linux ] && [ "$(uname -m)" = x86_64 ]; then
		"$work/waserver-linux-amd64" version
	fi
}

step_loadtest() { WATHENA_LOAD_TEST=1 go test -count=1 -run TestLoadSmoke -v ./tools/botclient; }

# client_install installs the client's packages as the lock file pins
# them.
client_install() {
	have pnpm || skip "the client needs pnpm"
	(cd client && pnpm install --frozen-lockfile)
}

step_schema() {
	client_install
	(cd schema && go run "github.com/bufbuild/buf/cmd/buf@$buf_version" lint)
	unchanged schema/gen client/src/gen -- sh -c "cd schema && go run github.com/bufbuild/buf/cmd/buf@$buf_version generate"
}

step_queries() {
	unchanged server/store/db -- sh -c "cd server/store && go run github.com/sqlc-dev/sqlc/cmd/sqlc@$sqlc_version generate"
}

step_lint() {
	if have golangci-lint && golangci-lint version 2>/dev/null | grep -q "version $golangci_version "; then
		golangci-lint run ./...
	else
		go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$golangci_version" run ./...
	fi
}

step_security() {
	go run "golang.org/x/vuln/cmd/govulncheck@$govulncheck_version" ./...
	go run "github.com/securego/gosec/v2/cmd/gosec@$gosec_version" -exclude-generated ./...
	.github/scripts/sqlcheck.sh
}

step_fuzz() { .github/scripts/fuzz.sh 10s; }

step_client() {
	client_install
	(
		cd client
		pnpm check
		pnpm lint
		pnpm format:check
		pnpm test
		pnpm build
	)
}

step_tinygo() {
	have tinygo || skip "the shared packages need tinygo 0.42"
	tinygo test ./shared/...
	if have wasmtime; then
		tinygo test -target=wasip1 ./shared/...
	else
		echo "part skipped: the WebAssembly tests need wasmtime"
	fi
	unchanged client/src/world/path.wasm -- \
		tinygo build -o client/src/world/path.wasm -target=wasm-unknown -opt=2 -no-debug ./shared/path/wasm
}

step_compose() {
	have docker || skip "the Compose test needs Docker"
	(cd deploy/compose && docker compose --env-file .env.example --profile tools config --quiet)
	.github/scripts/compose-smoke.sh
}

step_browsers() {
	client_install
	(cd client && pnpm check:browsers)
}

# The database starts here, not in a step's subshell, so the cleanup
# sees its container.
case " ${steps[*]} " in
*" test "* | *" e2e "*) database ;;
esac

for s in "${steps[@]}"; do
	echo
	echo "==== $s"
	began=$(date +%s)
	(
		set -e
		"step_$s"
	)
	code=$?
	took=$(($(date +%s) - began))
	case $code in
	0) results+=("passed   $s (${took}s)") ;;
	3) results+=("skipped  $s (${took}s)") ;;
	*)
		results+=("FAILED   $s (${took}s)")
		failed=1
		;;
	esac
done

echo
echo "==== results"
for r in "${results[@]}"; do
	echo "$r"
done
exit "$failed"
