#!/usr/bin/env bash
# Builds the release binaries of waserver and wactl (decision 0047).
#
#   scripts/release.sh                       every target, into dist/<version>
#   scripts/release.sh linux/amd64 ...       only the targets named
#
# Targets: linux/amd64, linux/arm64, darwin/arm64 and windows/amd64.
#
# What it builds:
#   - waserver with the web client built into it, and wactl, both with
#     the procedural tag. Such a binary runs procedural and classic
#     worlds; `wactl world new --mode` picks the mode of a world.
#   - The version is the output of git describe --tags --always --dirty,
#     stamped with -ldflags "-X main.buildVersion=...". waserver version
#     and wactl version print it.
#   - dist/<version>/SHA256SUMS lists the SHA-256 of every file.
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
# wactl needs no cgo.
#
# Tools: Go of go.mod, zig 0.16.0, Node 26 and pnpm (scripts/check.sh
# lists the versions and hashes). Signing and the software bill of
# materials are not here yet (strategy section 15, phase 8).
set -euo pipefail
root=$(git rev-parse --show-toplevel)
cd "$root"

all_targets="linux/amd64 linux/arm64 darwin/arm64 windows/amd64"
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

for tool in go zig pnpm; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "release.sh: $tool is not on PATH" >&2
		exit 1
	fi
done

version=$(git describe --tags --always --dirty)
out="dist/$version"
rm -rf "$out"
mkdir -p "$out"
echo "version $version, into $out"

export SOURCE_DATE_EPOCH
SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
export GOFLAGS=-mod=readonly GOAMD64=v1 GOARM64=v8.0 CGO_CFLAGS=-O2 CGO_LDFLAGS=-O2
export ZIG_GLOBAL_CACHE_DIR="${ZIG_GLOBAL_CACHE_DIR:-$HOME/.cache/zig}"

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
		-tags procedural,webclient,netgo,osusergo -ldflags "$ldflags $extra" \
		-o "$out/waserver-$goos-$goarch$ext" ./server/cmd/waserver
	CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -buildvcs=false \
		-tags procedural -ldflags "$ldflags" \
		-o "$out/wactl-$goos-$goarch$ext" ./tools/wactl
done

# The binaries of this machine run, as a check of the stamp and the tag.
host="$(go env GOHOSTOS)-$(go env GOHOSTARCH)"
for bin in waserver wactl; do
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
