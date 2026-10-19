#!/usr/bin/env bash
# Runs the project's checks on this machine. The repository has no CI
# (decision 0026), so this script is the check before a commit and
# before a push.
#
#   scripts/check.sh quick        vet, unit and lint: before each commit
#   scripts/check.sh quick client the quick steps and the client's checks
#   scripts/check.sh              the default steps: before a push
#   scripts/check.sh all          the default steps, then compose and browsers
#   scripts/check.sh lint e2e     only the steps named
#
# Steps:
#   vet       go vet ./..., without and with the procedural tag
#   unit      go test without -race on every package but ./e2e, then
#             with the procedural tag on the packages it changes:
#             shared/gen and those that import it (decision 0038).
#             Results of unchanged packages come from Go's test
#             cache.
#   test      unit, then go test -race ./.... Heavy tests on one
#             goroutine skip under -race (decision 0022).
#   mods      generated Lua files, the sample mods with a wactl built
#             with the procedural tag, a new mod
#   lua       vendored Lua, lua.wasm, the WebAssembly build,
#             and the release builds with zig, with the gzipped
#             extracts that go generate makes
#   loadtest  the 50-bot smoke test
#   schema    buf lint and the generated protocol code
#   queries   the generated sqlc code
#   lint      golangci-lint, then with the procedural tag on the
#             packages it changes
#   security  govulncheck, gosec, sqlcheck.sh and pnpm audit of the
#             client
#   fuzz      every fuzz test for 10 seconds, 4 at once, with the
#             procedural tag.
#             scripts/fuzz.sh 5m fuzzes longer.
#   client    pnpm check, lint, format:check, test and build
#   tinygo    the shared packages under TinyGo, without and with the
#             procedural tag, and path.wasm and gen.wasm
#   gen       the map generator's golden checksums, with the WMAP
#             hashes of generated fields, dungeon floors and towns, and
#             their places' hashes, the golden world layouts and
#             spawn lists, and the golden rotations and seasons
#             (decision 0045), in Go
#             on this machine's
#             architecture and on the other one (amd64 under Rosetta on
#             an arm64 Mac), and the maps in gen.wasm in Node. The
#             browsers step checks the maps in Chromium. Only the
#             server makes layouts and spawns.
#   e2e       the e2e package alone, with -race, with the phase 5
#             exit check for 30 minutes (WATHENA_EXIT_MINUTES=30,
#             e2e/README.md), and prints its numbers. test runs
#             the package with a short
#             exit check, so it is not a default step.
#   compose   the Compose stack smoke test. Only with all.
#   browsers  the client in Chromium, with the phase 6 exit check
#             (decision 0035) on desktops against waserver and
#             PostgreSQL, a walk on
#             generated fields (decision 0039), a walk from a field into
#             a town and a dungeon floor (decision 0040), a walk from a
#             home map into a town with the world map (decision 0041),
#             a quest with its window and NPC icon on a desktop
#             (decision 0043), a walk to a Kafra and a quest board
#             in a world that wactl world new made (decision 0044), a
#             rotation that moves a player and the new floors it loads
#             (decision 0045), and the map
#             generator in Chromium. Only with all. Only Chromium is
#             supported (strategy section 2.2).
#             Touch input is frozen as built (decision 0034), so the
#             touch and phone checks are not in this step. Run them with
#             CHECKS=touch,phone pnpm check:browsers in client.
#   economy   the bot runs of the economy simulator (decision 0046):
#             bots of the model player hunt a field of four bands in a
#             world that wactl world new made, with PostgreSQL, for
#             WATHENA_ECONOMY_MINUTES (10 by default), and their exp and
#             zeny per hour must be within 20% of the simulator's. It
#             prints the numbers. Not a default step, and not in all.
#
# Tool versions. The script runs the tools on PATH and does not install
# them. These are the versions the checks were written for:
#   - Go: the version in go.mod. Node 26 and pnpm, for the client.
#   - zig 0.16.0. SHA-256 of the release archives:
#       x86_64-linux   70e49664a74374b48b51e6f3fdfbf437f6395d42509050588bd49abe52ba3d00
#       aarch64-macos  b23d70deaa879b5c2d486ed3316f7eaa53e84acf6fc9cc747de152450d401489
#     The Dockerfile pins the same version.
#   - wasi-sdk 34, x86_64-linux archive:
#       b761e3a0721dbae9c09a0059e5fdb2bf917d1b4a8a7b430fb3b5aafb0984b2c4
#   - TinyGo 0.42.0, amd64 .deb:
#       2082c4762fea6d5cc4cd1f4a243eaacf07b12f576717d4c6b74828bd163cb563
#   - wasmtime 49.0.1, x86_64-linux archive:
#       c71f7e0d30a92e418f0d17db7c6d8f6664c1ad764340a1278678f4209deab534
#   - buf, sqlc, golangci-lint and govulncheck: the variables below.
#     The script runs them with go run at those versions.
#   - gosec: tools/gosec/go.mod. The script builds it from there.
#
# Rules:
#   - Tests that need PostgreSQL read WATHENA_TEST_DATABASE_URL, and so
#     does the browsers step's exit check. When it is not set and Docker
#     runs, the script starts a postgres:18 container for the run and
#     removes it at the end.
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

# Versions of the tools that go run fetches.
buf_version=v1.73.0
sqlc_version=v1.31.1
golangci_version=2.14.0
govulncheck_version=v1.8.0
# gosec is pinned in tools/gosec/go.mod, with a newer golang.org/x/tools
# than its own go.mod asks for.

default_steps="vet test mods lua loadtest schema queries lint security fuzz client tinygo gen"
all_steps="$default_steps compose browsers"
known_steps="$all_steps unit e2e economy"
quick_steps="vet unit lint"

steps=()
if [ $# -eq 0 ]; then
	read -r -a steps <<<"$default_steps"
elif [ "$1" = all ] && [ $# -eq 1 ]; then
	read -r -a steps <<<"$all_steps"
else
	for s in "$@"; do
		if [ "$s" = quick ]; then
			read -r -a quick <<<"$quick_steps"
			steps+=("${quick[@]}")
			continue
		fi
		case " $known_steps " in
		*" $s "*) steps+=("$s") ;;
		*)
			echo "check.sh: unknown step $s; the steps are: quick $known_steps" >&2
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

# procedural_packages lists the folders of the packages that the
# procedural tag changes: shared/gen and every package that imports it.
# Other packages build the same with and without the tag.
procedural_packages() {
	go list -tags procedural -f '{{.Dir}} {{.ImportPath}} {{join .Deps " "}}' ./... |
		awk -v root="$root" -v gen="github.com/lxsmnsyc/wathena/shared/gen" '
			$2 ~ /\/e2e$/ { next }
			{ for (i = 2; i <= NF; i++) if ($i == gen) { sub(root, ".", $1); print $1; next } }'
}

step_vet() {
	go vet ./...
	go vet -tags procedural ./...
}

# The race detector only finds races in code that runs goroutines, and
# it makes code 5 to 20 times slower. So the tests run twice:
#   - Without it, every test of every package but ./e2e runs.
#   - With it, every package runs, ./e2e too. The heavy tests that run
#     on one goroutine, such as the parity and golden tests, skip
#     (internal/testrace).
# ./e2e runs in the second pass only, as before. Beside the busy first
# pass, its duplication tests did too little work in their time.
step_unit() {
	# shellcheck disable=SC2046 # one argument per package.
	go test -timeout 45m $(go list ./... | grep -v '/e2e$')
	# shellcheck disable=SC2046 # one argument per package.
	go test -timeout 45m -tags procedural $(procedural_packages)
}

step_test() {
	step_unit
	go test -race -timeout 45m ./...
}

# The e2e package takes about 16 minutes with -race and a database.
# The exit check adds 30 minutes of moves and its restart.
step_e2e() {
	local code=0
	WATHENA_EXIT_MINUTES=30 WATHENA_EXIT_REPORT="$work/exit-report.txt" go test -race -count=1 -timeout 90m ./e2e/ || code=$?
	if [ -f "$work/exit-report.txt" ]; then
		echo "the exit check's numbers:"
		cat "$work/exit-report.txt"
	fi
	return "$code"
}

# wactl is built with the procedural tag, so the Market Town sample's
# tests run on the test worlds' generated maps (decision 0040).
step_mods() {
	local wactl="$work/wactl"
	go build -tags procedural -o "$wactl" ./tools/wactl
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
	# The release builds embed the gzipped extracts (review decision 56).
	go generate ./reference/rathena
	go test -count=1 -tags gzextracts ./reference/rathena
	local static="-linkmode external -extldflags '-static -s -Wl,-z,stack-size=8388608'"
	build() {
		echo "== $1/$2"
		CGO_ENABLED=1 GOOS=$1 GOARCH=$2 CC="zig cc -target $3" go build -trimpath -tags netgo,osusergo,gzextracts \
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
			go build -trimpath -tags netgo,osusergo,gzextracts -ldflags "-s -w" -o "$work/waserver-darwin-arm64" ./server/cmd/waserver
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
	local lint=(go run "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$golangci_version")
	if have golangci-lint && golangci-lint version 2>/dev/null | grep -q "version $golangci_version "; then
		lint=(golangci-lint)
	fi
	"${lint[@]}" run ./...
	# shellcheck disable=SC2046 # one argument per package.
	"${lint[@]}" run --build-tags procedural $(procedural_packages)
}

step_security() {
	go run "golang.org/x/vuln/cmd/govulncheck@$govulncheck_version" ./...
	go build -C tools/gosec -o "$work/gosec" github.com/securego/gosec/v2/cmd/gosec
	"$work/gosec" -exclude-generated ./...
	scripts/sqlcheck.sh
	# The client's dependencies, build tools too (review 2026-10). It
	# needs the network, as govulncheck does.
	(cd client && pnpm audit --audit-level high)
}

step_fuzz() { scripts/fuzz.sh 10s; }

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
	tinygo test -tags procedural ./shared/...
	if have wasmtime; then
		tinygo test -target=wasip1 ./shared/...
		tinygo test -target=wasip1 -tags procedural ./shared/...
	else
		echo "part skipped: the WebAssembly tests need wasmtime"
	fi
	unchanged client/src/world/path.wasm -- \
		tinygo build -o client/src/world/path.wasm -target=wasm-unknown -opt=2 -no-debug ./shared/path/wasm
	unchanged client/src/world/gen.wasm -- \
		tinygo build -o client/src/world/gen.wasm -target=wasm-unknown -opt=2 -no-debug -tags procedural ./shared/gen/wasm
}

# The map generator must give the same grid on every platform (strategy
# section 8.4). Each part generates the golden maps of
# shared/gen/testdata and compares the checksums, and for fields the
# WMAP's SHA-256 too. The field and f32 tests run on both architectures.
# layout_tests are the world layout's golden layouts and rules (decision
# 0041), which only the server runs, so no client checks them.
layout_tests='^TestGoldenLayout$|^TestLayoutRules$|^TestLayoutDeterministic$'
# spawn_tests are the golden spawn lists of new worlds (decision 0042),
# which only wactl picks, with the economy checks' lists (decision
# 0046).
spawn_tests='^TestGoldenSpawns$|^TestSpawnDeterministic$|^TestGoldenEconomySpawns$'
rotation_tests='^TestGoldenRotation$|^TestRotationRules$'
gen_tests='^TestGolden$|^TestGridV1Order$|^TestFieldV2$|^TestFieldV2Retries$|^TestF32Bits$|^TestDungeonV3$|^TestTownV3$|^TestFieldV3Places$|^TestMapV3Retries$'

step_gen() {
	local host other runner=() bin
	host=$(go env GOARCH)
	echo "== Go on $(go env GOOS)/$host"
	go test -count=1 -tags procedural -run "$gen_tests" -v ./shared/gen
	go test -count=1 -tags procedural -run "$layout_tests" -v ./server/layout
	go test -count=1 -tags procedural -run "$spawn_tests" -v ./server/spawns
	go test -count=1 -tags procedural -run "$rotation_tests" -v ./server/rotation
	case "$host" in
	arm64) other=amd64 ;;
	amd64) other=arm64 ;;
	*) other="" ;;
	esac
	if [ -n "$other" ]; then
		echo "== Go on $(go env GOOS)/$other"
		bin="$work/gen-$other.test"
		GOARCH=$other go test -c -tags procedural -o "$bin" ./shared/gen
		GOARCH=$other go test -c -tags procedural -o "$work/layout-$other.test" ./server/layout
		GOARCH=$other go test -c -tags procedural -o "$work/spawns-$other.test" ./server/spawns
		GOARCH=$other go test -c -tags procedural -o "$work/rotation-$other.test" ./server/rotation
		if [ "$(uname -s)" = Darwin ] && [ "$other" = amd64 ]; then
			runner=(arch -x86_64)
		fi
		if ${runner[@]+"${runner[@]}"} "$bin" -test.list XXX >/dev/null 2>&1; then
			(cd shared/gen && ${runner[@]+"${runner[@]}"} "$bin" -test.count=1 -test.run "$gen_tests" -test.v)
			(cd server/layout && ${runner[@]+"${runner[@]}"} "$work/layout-$other.test" -test.count=1 -test.run "$layout_tests" -test.v)
			(cd server/spawns && ${runner[@]+"${runner[@]}"} "$work/spawns-$other.test" -test.count=1 -test.run "$spawn_tests" -test.v)
			(cd server/rotation && ${runner[@]+"${runner[@]}"} "$work/rotation-$other.test" -test.count=1 -test.run "$rotation_tests" -test.v)
		elif [ "$(uname -s)" = Darwin ]; then
			echo "part skipped: the $other test binary does not run; an arm64 Mac needs Rosetta (softwareupdate --install-rosetta)"
		else
			echo "part skipped: this machine cannot run $other binaries"
		fi
	fi
	echo "== gen.wasm in Node"
	if have pnpm; then
		client_install
		(cd client && pnpm exec vitest run src/world/generator.test.ts)
	else
		echo "part skipped: the Node check needs pnpm"
	fi
}

# The bot runs take about 3 minutes to make the world and start the
# server, then the minutes the bots hunt.
step_economy() {
	local code=0
	WATHENA_ECONOMY_MINUTES="${WATHENA_ECONOMY_MINUTES:-10}" WATHENA_ECONOMY_REPORT="$work/economy-report.txt" \
		go test -count=1 -timeout 60m -run '^TestEconomyBots$' -v ./e2e/ | grep -E 'economy_test.go|^(---|ok|FAIL|PASS)' || code=$?
	if [ -f "$work/economy-report.txt" ]; then
		echo "the bot runs' numbers:"
		cat "$work/economy-report.txt"
	fi
	return "$code"
}

step_compose() {
	have docker || skip "the Compose test needs Docker"
	(cd deploy/compose && docker compose --env-file .env.example --profile tools config --quiet)
	scripts/compose-smoke.sh
}

step_browsers() {
	client_install
	(cd client && pnpm check:browsers)
}

# The database starts here, not in a step's subshell, so the cleanup
# sees its container.
case " ${steps[*]} " in
*" test "* | *" e2e "* | *" browsers "* | *" economy "*) database ;;
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
