#!/bin/sh
# Vendors the Lua sources that internal/script compiles, from the official
# release tarball. The version and the tarball's SHA-256 are pinned here.
#
#   update.sh          download, check, and copy the sources into src/
#   update.sh --check  download, check, and compare with src/ (used by CI)
#
# To update Lua:
#   1. Change version and sha256. The SHA-256 is on https://www.lua.org/ftp/.
#   2. Run update.sh. It rewrites src/ and SHA256SUMS.
#   3. Copy the new src/lstrlib.c over ../csrc/lstrlib_charged.c and apply the
#      changes listed in ../vendor_test.go again. go test checks them.
#   4. Rebuild ../wasm/lua.wasm with ../wasm/build.sh.
set -eu
cd "$(dirname "$0")"
version=5.4.9
sha256=2335b6c582a52654f94612bf10d2f4672805d05329aa6568b1d8cd9e5c6fb8e6

# The interpreter, the compiler, and the debug, io, os, and package
# libraries are left out. Scripts cannot use them.
skip="lua.c luac.c ldblib.c liolib.c loadlib.c loslib.c linit.c"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl -fsSLo "$work/lua.tar.gz" "https://www.lua.org/ftp/lua-$version.tar.gz"
echo "$sha256  $work/lua.tar.gz" | shasum -a 256 -c -
tar -xzf "$work/lua.tar.gz" -C "$work"
mkdir "$work/out"
for f in "$work/lua-$version/src/"*.c "$work/lua-$version/src/"*.h; do
	name=$(basename "$f")
	case " $skip " in *" $name "*) continue ;; esac
	cp "$f" "$work/out/$name"
done
(cd "$work/out" && shasum -a 256 *.c *.h) > "$work/SHA256SUMS"

if [ "${1:-}" = "--check" ]; then
	diff -r "$work/out" src
	diff "$work/SHA256SUMS" SHA256SUMS
	echo "src/ matches lua-$version"
	exit 0
fi
rm -rf src
cp -R "$work/out" src
cp "$work/SHA256SUMS" SHA256SUMS
echo "vendored lua-$version"
