#!/bin/sh
# Builds webp.wasm, libwebp's lossless encoder with glue.c, which
# ../webp.go runs in wazero. The output is committed, so building wAthena
# does not need a C compiler.
#
# Needs wasi-sdk 34 or later. Set WASI_SDK to its folder.
set -eu
cd "$(dirname "$0")"
: "${WASI_SDK:?set WASI_SDK to the wasi-sdk folder}"
version=1.6.0
sha256=e4ab7009bf0629fd11982d4c2aa83964cf244cffba7347ecd39019a9e38c4564
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl -fsSLo "$work/libwebp.tar.gz" "https://storage.googleapis.com/downloads.webmproject.org/releases/webp/libwebp-$version.tar.gz"
echo "$sha256  $work/libwebp.tar.gz" | shasum -a 256 -c -
tar -xzf "$work/libwebp.tar.gz" -C "$work"
src="$work/libwebp-$version"

# The encoder, its DSP functions, and sharpyuv, which the encoder links to.
# The decoder, mux, and demux are left out.
files=$(ls "$src"/src/enc/*.c "$src"/src/dsp/*.c "$src"/src/utils/*.c "$src"/sharpyuv/*.c)

# Threads are off, since WASI has none. The encoder is plain C here, without
# SIMD, so the output is the same on every host.
"$WASI_SDK/bin/clang" --target=wasm32-wasip1 --sysroot="$WASI_SDK/share/wasi-sysroot" \
	-mexec-model=reactor -O2 -s \
	-I "$src" \
	-o webp.wasm $files glue.c
ls -l webp.wasm
