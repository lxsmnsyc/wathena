# luabench

Compares Lua runtimes on wAthena's script workloads. The results and the decision are in [docs/decisions/0002-lua-runtime-c-lua.md](../../docs/decisions/0002-lua-runtime-c-lua.md).

It is a separate Go module, so the runtimes it compares do not become dependencies of wAthena.

## Engines

| Engine | File | Needs |
|---|---|---|
| arnodel/golua | `golua.go` | Nothing. |
| yuin/gopher-lua | `gopher.go` | Nothing. |
| C Lua on wazero | `wasm.go`, `wasm/lua.wasm` | Nothing. `lua.wasm` is committed. |
| C Lua, own cgo binding | `cluaown.go`, `glue/` | cgo and Lua 5.4. |
| C Lua, aarzilli/golua | `clua.go` | cgo and Lua 5.4. |

`glue/glue.c` is the binding for both the cgo build and `lua.wasm`.

## Running

Pure Go engines only:

```text
go run . -json results/out.json
```

All engines, on macOS with Homebrew `lua@5.4` and `pkgconf`:

```text
PKG_CONFIG_PATH=/opt/homebrew/opt/lua@5.4/lib/pkgconfig \
CGO_LDFLAGS="-L/opt/homebrew/opt/lua@5.4/lib" \
go run -tags clua,lua54,llua . -json results/out.json
```

Use `-engine <text>` to run only engines whose name contains the text.

## Rebuilding lua.wasm

Needs [wasi-sdk](https://github.com/WebAssembly/wasi-sdk) 34 or later:

```text
WASI_SDK=/path/to/wasi-sdk wasm/build.sh
```
