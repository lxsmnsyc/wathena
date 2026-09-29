#!/bin/sh
# Builds opus.wasm, libopus's encoder and the speexdsp resampler with
# glue.c, which ../opus.go runs in wazero. The output is committed, so
# building wAthena does not need a C compiler.
#
# Needs wasi-sdk 34 or later. Set WASI_SDK to its folder.
set -eu
cd "$(dirname "$0")"
: "${WASI_SDK:?set WASI_SDK to the wasi-sdk folder}"
opus=1.6.1
opus_sha256=6ffcb593207be92584df15b32466ed64bbec99109f007c82205f0194572411a1
speexdsp=1.2.1
speexdsp_sha256=8c777343e4a6399569c72abc38a95b24db56882c83dbdb6c6424a5f4aeb54d3d
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl -fsSLo "$work/opus.tar.gz" "https://downloads.xiph.org/releases/opus/opus-$opus.tar.gz"
curl -fsSLo "$work/speexdsp.tar.gz" "https://downloads.xiph.org/releases/speex/speexdsp-$speexdsp.tar.gz"
echo "$opus_sha256  $work/opus.tar.gz" | shasum -a 256 -c -
echo "$speexdsp_sha256  $work/speexdsp.tar.gz" | shasum -a 256 -c -
tar -xzf "$work/opus.tar.gz" -C "$work"
tar -xzf "$work/speexdsp.tar.gz" -C "$work"
o="$work/opus-$opus"
s="$work/speexdsp-$speexdsp"

# sources prints the files in a make variable from libopus's .mk lists.
sources() {
	awk -v name="$2" '
		$1 == name && $2 == "=" { on = 1; next }
		on {
			more = /\\$/
			gsub(/\\/, "")
			for (i = 1; i <= NF; i++) print $i
			if (!more) on = 0
		}
	' "$o/$1" | sed "s|^|$o/|"
}
files="$(sources celt_sources.mk CELT_SOURCES) $(sources silk_sources.mk SILK_SOURCES)
$(sources silk_sources.mk SILK_SOURCES_FLOAT) $(sources opus_sources.mk OPUS_SOURCES)
$(sources opus_sources.mk OPUS_SOURCES_FLOAT)"

# Floating point, without SIMD or run-time CPU detection. WebAssembly
# floating point gives the same results on every host, so builds make the
# same bytes everywhere. The resampler is built on its own, outside speexdsp.
# libopus puts large buffers on the stack with alloca, which can overflow
# the default 64 KiB WebAssembly stack, so the stack is 1 MiB.
#
# The code is built at -O1. With wasi-sdk 34, -O2 and -Os builds trap with
# an out-of-bounds access in silk_NSQ_del_dec_c on mono input, while -O0,
# -O1, and -O2 with UBSan all give the same bytes. ../sound_test.go pins
# the output, so a build that changes it fails the tests.
"$WASI_SDK/bin/clang" --target=wasm32-wasip1 --sysroot="$WASI_SDK/share/wasi-sysroot" \
	-mexec-model=reactor -O1 -s -Wl,-z,stack-size=1048576 \
	-DOPUS_BUILD -DUSE_ALLOCA -DHAVE_LRINTF -DHAVE_LRINT \
	-DOUTSIDE_SPEEX -DFLOATING_POINT -DRANDOM_PREFIX=wa -DEXPORT= \
	-I "$o/include" -I "$o/celt" -I "$o/silk" -I "$o/silk/float" -I "$o" \
	-I "$s/include/speex" \
	-o opus.wasm $files "$s/libspeexdsp/resample.c" glue.c
ls -l opus.wasm
