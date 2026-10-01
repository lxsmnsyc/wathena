#!/bin/sh
# Builds lua.wasm from ../csrc/all.c: the vendored Lua 5.4 sources and the
# same glue.c that the cgo build compiles. engine_wasm.go runs it in wazero
# when cgo is off. The output is committed, so builds need no C compiler.
#
# Needs wasi-sdk 34. Set WASI_SDK to its folder.
#
# Lua's errors use setjmp and longjmp. wasi-sdk compiles them to WebAssembly
# exception handling, which wazero supports as an experimental feature.
set -eu
cd "$(dirname "$0")/.."
: "${WASI_SDK:?set WASI_SDK to the wasi-sdk folder}"

# Flags:
#   - WASI has no signals, so Lua's signal-safe type becomes a plain int.
#   - ltablib.c calls clock(), which WASI only has as an emulation.
#   - The stack is 1 MB. The default 64 KB is too small for Lua's 200
#     nested C calls, and an overflow in WebAssembly corrupts memory.
#   - clang runs wasm-opt when it finds one on PATH. --no-wasm-opt keeps
#     the output the same on every machine, so CI can check it.
"$WASI_SDK/bin/clang" --target=wasm32-wasip1 --sysroot="$WASI_SDK/share/wasi-sysroot" \
	-mexec-model=reactor -O2 -s --no-wasm-opt \
	-mllvm -wasm-enable-sjlj -mllvm -wasm-use-legacy-eh=false \
	-Dl_signalT=int -D_WASI_EMULATED_PROCESS_CLOCKS \
	-Wl,-z,stack-size=1048576 \
	-I lua/src -I csrc \
	-o wasm/lua.wasm csrc/all.c \
	-lsetjmp -lwasi-emulated-process-clocks
ls -l wasm/lua.wasm
