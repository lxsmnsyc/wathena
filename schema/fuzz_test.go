package schema

import (
	"testing"

	"google.golang.org/protobuf/proto"

	v1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Decoding any bytes must not panic, and a decoded message must encode again.
func FuzzClientMessage(f *testing.F) {
	seed, _ := proto.Marshal(v1.ClientMessage_builder{Seq: 1, Command: v1.Command_builder{
		WalkTo: v1.WalkTo_builder{X: 150, Y: 180}.Build(),
	}.Build()}.Build())
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		var m v1.ClientMessage
		if err := proto.Unmarshal(data, &m); err != nil {
			return
		}
		if _, err := proto.Marshal(&m); err != nil {
			t.Fatal(err)
		}
	})
}
