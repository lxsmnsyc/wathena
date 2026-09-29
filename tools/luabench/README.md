# luabench

Compares Lua runtimes on wAthena's script workloads. The results and the decision are in [docs/decisions/0001-lua-runtime.md](../../docs/decisions/0001-lua-runtime.md).

It is a separate Go module, so the runtimes it compares do not become dependencies of wAthena.

Pure Go runtimes:

```text
go run . -json results/out.json
```

With C Lua 5.4 through cgo (macOS with Homebrew `lua@5.4`):

```text
CGO_LDFLAGS="-L/opt/homebrew/opt/lua@5.4/lib" go run -tags clua,lua54,llua . -json results/out.json
```
