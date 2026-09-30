# schema

The web client protocol, as Protocol Buffers. The design is in [docs/proposals/0001-protocol.md](../docs/proposals/0001-protocol.md).

| Path | Content |
|---|---|
| `proto/wathena/v1` | The message schema. |
| `gen/go` | Generated Go code. Do not edit. |
| `../client/src/gen` | Generated TypeScript for the web client, with Protobuf-ES. Do not edit. |
| `protocol.go` | The protocol version and message size limits. |

After changing a `.proto` file, run `pnpm install` in `client/` once, then this in this folder, and commit the result:

```text
buf lint
buf generate
```
