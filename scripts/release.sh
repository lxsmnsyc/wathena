#!/usr/bin/env bash
# Builds waserver, wactl and wabots from the checked-out commit, such
# as a release's tag (decisions 0047 and 0050). It is a build helper for
# owners and developers. The project publishes no binaries: a release
# is a git tag, and owners build from it (strategy section 2.2).
#
#   scripts/release.sh                       every target, into dist/<version>
#   scripts/release.sh linux/amd64 ...       only the targets named
#   scripts/release.sh --check-tools         only check the tools, then
#                                            run go mod verify
#
# Targets: linux/amd64, linux/arm64, darwin/arm64 and windows/amd64.
#
# What it builds:
#   - waserver with the web client built into it, and wactl, both with
#     the procedural tag. Such a binary runs procedural and classic
#     worlds; `wactl world new --mode` picks the mode of a world.
#   - wabots, the load test bots, so owners can measure their own
#     hardware (deploy/README.md, "Measure your own hardware").
#   - All three with the gzextracts tag: they embed gzipped copies of
#     the rAthena extracts, which go generate makes first (review
#     decision 56, reference/rathena/doc.go).
#   - The version is the output of git describe --tags --always --dirty,
#     stamped with -ldflags "-X main.buildVersion=...". waserver version
#     and wactl version print it.
#   - dist/<version>/SHA256SUMS lists the SHA-256 of every file, to
#     compare two builds.
#
# Flags for builds that repeat byte for byte:
#   - -trimpath and -buildvcs=false keep paths and VCS data out.
#   - -buildid= clears the build ID, and -s -w strip symbols.
#   - CGO_CFLAGS and CGO_LDFLAGS are -O2 without -g, Go's default. With
#     -g, the Windows linker writes a PDB whose temporary paths change
#     the GUID and time stamp in the PE headers on each build.
#   - GOAMD64, GOARM64 and GOFLAGS are fixed here too, so the caller's
#     environment does not change the output.
#   - SOURCE_DATE_EPOCH is the commit time, for the client build.
#
# waserver runs Lua through cgo (decision 0002). zig cc compiles the C
# code for every target. The Linux binaries link musl statically. The
# darwin build needs the macOS SDK, so it runs on a Mac with xcrun only.
# wactl and wabots need no cgo.
#
# Tools: Go of go.mod, zig 0.16.0, Node 26 and pnpm of client/package.json
# (scripts/check.sh lists the versions and hashes). Every run checks
# them first, and runs go mod verify. Nothing is signed and there is no
# software bill of materials, since nothing is published.
set -euo pipefail
root=$(git rev-parse --show-toplevel)
cd "$root"

zig_version=0.16.0
node_major=26

# check_tools fails when a tool is missing or is not the version the
# release was written for, and runs go mod verify.
check_tools() {
	local bad=0 tool want have
	for tool in go zig node pnpm; do
		if ! command -v "$tool" >/dev/null 2>&1; then
			echo "release.sh: $tool is not on PATH" >&2
			bad=1
		fi
	done
	if [ "$bad" = 1 ]; then
		return 1
	fi
	# A tool that crashes gives an empty version, so the message below
	# still prints under set -e.
	# Go: at least the go line of go.mod. Only the numbers count: a
	# release candidate "1.25rc1" is 1.25, and "devel go1.28-abc" is 1.28.
	want=$(sed -n 's/^go \([0-9][0-9.]*\).*$/\1/p' go.mod | head -1)
	have=$(go env GOVERSION 2>/dev/null | sed -n 's/.*go\([0-9][0-9]*\.[0-9][0-9.]*\).*/\1/p' | head -1) || have=""
	have=${have%.}
	if [ -z "$want" ] || [ -z "$have" ] || [ "$(printf '%s\n%s\n' "$want" "$have" | sort -V | head -1)" != "$want" ]; then
		echo "release.sh: go ${have:-unknown} is older than go.mod's ${want:-unknown}" >&2
		bad=1
	fi
	# zig: exactly the pinned version.
	have=$(zig version 2>/dev/null) || have=""
	if [ "$have" != "$zig_version" ]; then
		echo "release.sh: zig ${have:-unknown}, but the release needs $zig_version" >&2
		bad=1
	fi
	# Node: the major only.
	have=$(node --version 2>/dev/null | sed 's/^v//; s/\..*//') || have=""
	if [ "$have" != "$node_major" ]; then
		echo "release.sh: node major ${have:-unknown}, but the release needs $node_major" >&2
		bad=1
	fi
	# pnpm: the packageManager of the client. It picks the pinned
	# version inside client/.
	want=$(sed -n 's/.*"packageManager": *"pnpm@\([^"+]*\).*/\1/p' client/package.json | head -1)
	have=$(cd client && pnpm --version 2>/dev/null) || have=""
	if [ -z "$want" ] || [ "$have" != "$want" ]; then
		echo "release.sh: pnpm ${have:-unknown}, but client/package.json needs ${want:-unknown}" >&2
		bad=1
	fi
	if [ "$bad" = 1 ]; then
		return 1
	fi
	if ! go mod verify >/dev/null; then
		echo "release.sh: go mod verify failed" >&2
		return 1
	fi
	echo "tools ok: go $(go env GOVERSION), zig $(zig version), node $(node --version), pnpm $(cd client && pnpm --version)"
}

all_targets="linux/amd64 linux/arm64 darwin/arm64 windows/amd64"
if [ "${1:-}" = --check-tools ] && [ $# -eq 1 ]; then
	check_tools
	exit
fi
targets=("$@")
if [ ${#targets[@]} -eq 0 ]; then
	read -r -a targets <<<"$all_targets"
fi
for t in "${targets[@]}"; do
	case " $all_targets " in
	*" $t "*) ;;
	*)
		echo "release.sh: unknown target $t; the targets are: $all_targets" >&2
		exit 2
		;;
	esac
done

check_tools || exit 1

version=$(git describe --tags --always --dirty)
out="dist/$version"
rm -rf "$out"
mkdir -p "$out"
echo "version $version, into $out"

export SOURCE_DATE_EPOCH
SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
export GOFLAGS=-mod=readonly GOAMD64=v1 GOARM64=v8.0 CGO_CFLAGS=-O2 CGO_LDFLAGS=-O2
export ZIG_GLOBAL_CACHE_DIR="${ZIG_GLOBAL_CACHE_DIR:-$HOME/.cache/zig}"

echo "== the gzipped extracts"
go generate ./reference/rathena

echo "== the web client"
(
	cd client
	pnpm install --frozen-lockfile
	pnpm build
)

ldflags="-s -w -buildid= -X main.buildVersion=$version"
# musl's default thread stack is 128 KB. Threads that run Lua need about
# 512 KB of C stack, so the Linux builds set 8 MB, as the Dockerfile does.
static="-linkmode external -extldflags '-static -s -Wl,-z,stack-size=8388608'"

for t in "${targets[@]}"; do
	goos=${t%/*}
	goarch=${t#*/}
	ext=""
	if [ "$goos" = windows ]; then
		ext=.exe
	fi
	extra=""
	case "$t" in
	linux/amd64)
		cc="zig cc -target x86_64-linux-musl"
		extra=$static
		;;
	linux/arm64)
		cc="zig cc -target aarch64-linux-musl"
		extra=$static
		;;
	windows/amd64)
		cc="zig cc -target x86_64-windows-gnu"
		;;
	darwin/arm64)
		if [ "$(uname -s)" != Darwin ] || ! command -v xcrun >/dev/null 2>&1; then
			echo "release.sh: darwin/arm64 needs a Mac with xcrun for the macOS SDK" >&2
			exit 1
		fi
		sdk=$(xcrun --show-sdk-path)
		cc="zig cc -target aarch64-macos --sysroot $sdk -L/usr/lib -F$sdk/System/Library/Frameworks"
		;;
	esac
	echo "== $t"
	CGO_ENABLED=1 GOOS=$goos GOARCH=$goarch CC="$cc" go build -trimpath -buildvcs=false \
		-tags procedural,gzextracts,webclient,netgo,osusergo -ldflags "$ldflags $extra" \
		-o "$out/waserver-$goos-$goarch$ext" ./server/cmd/waserver
	CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -buildvcs=false \
		-tags procedural,gzextracts -ldflags "$ldflags" \
		-o "$out/wactl-$goos-$goarch$ext" ./tools/wactl
	CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -buildvcs=false \
		-tags procedural,gzextracts -ldflags "$ldflags" \
		-o "$out/wabots-$goos-$goarch$ext" ./tools/wabots
done

# The binaries of this machine run, as a check of the stamp and the tag.
host="$(go env GOHOSTOS)-$(go env GOHOSTARCH)"
for bin in waserver wactl wabots; do
	if [ -x "$out/$bin-$host" ]; then
		"$out/$bin-$host" version
	fi
done

(
	cd "$out"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum -- *-* >SHA256SUMS
	else
		shasum -a 256 -- *-* >SHA256SUMS
	fi
	cat SHA256SUMS
)
