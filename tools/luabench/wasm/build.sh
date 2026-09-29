#!/bin/sh
# Builds lua.wasm from the Lua 5.4.9 source and glue/glue.c.
# The output is committed, so running the benchmark does not need a C compiler.
#
# Needs wasi-sdk 34 or later. Set WASI_SDK to its folder.
#
# Lua's errors use setjmp and longjmp. wasi-sdk compiles them to WebAssembly
# exception handling, which wazero supports as an experimental feature.
set -eu
cd "$(dirname "$0")"
: "${WASI_SDK:?set WASI_SDK to the wasi-sdk folder}"
version=5.4.9
sha256=2335b6c582a52654f94612bf10d2f4672805d05329aa6568b1d8cd9e5c6fb8e6
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl -fsSLo "$work/lua.tar.gz" "https://www.lua.org/ftp/lua-$version.tar.gz"
echo "$sha256  $work/lua.tar.gz" | shasum -a 256 -c -
tar -xzf "$work/lua.tar.gz" -C "$work"
src="$work/lua-$version/src"

# The interpreter, the compiler, and the io, os, and package libraries are
# left out. The glue opens only the libraries scripts may use.
files=""
for f in lapi lcode lctype ldebug ldo ldump lfunc lgc llex lmem lobject \
	lopcodes lparser lstate lstring ltable ltm lundump lvm lzio lauxlib \
	lbaselib lcorolib lmathlib lstrlib ltablib lutf8lib; do
	files="$files $src/$f.c"
done

# WASI has no signals, so Lua's signal-safe type becomes a plain int.
# ltablib.c uses clock(), which WASI only has as an emulation.
"$WASI_SDK/bin/clang" --target=wasm32-wasip1 --sysroot="$WASI_SDK/share/wasi-sysroot" \
	-mexec-model=reactor -O2 -s \
	-mllvm -wasm-enable-sjlj -mllvm -wasm-use-legacy-eh=false \
	-Dl_signalT=int -D_WASI_EMULATED_PROCESS_CLOCKS \
	-I "$src" \
	-o lua.wasm $files ../glue/glue.c \
	-lsetjmp -lwasi-emulated-process-clocks
ls -l lua.wasm
