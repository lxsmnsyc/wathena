# schema

The web client protocol, as Protocol Buffers. The design is in [docs/proposals/0001-protocol.md](../docs/proposals/0001-protocol.md).

| Path | Content |
|---|---|
| `proto/wathena/v1` | The message schema. |
| `gen/go` | Generated Go code. Do not edit. |
| `protocol.go` | The protocol version and message size limits. |

After changing a `.proto` file, run this in this folder and commit the result:

```text
buf lint
buf generate
```
