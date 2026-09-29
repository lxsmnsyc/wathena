package schema

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	v1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Per-message overhead on the wire: a WebSocket frame header from the server
// (2 to 4 bytes) and a TLS 1.3 record (5 byte header, 1 byte type, 16 byte tag).
const wireOverhead = 4 + 22

// scenario is a view of the world, with event rates per second.
type scenario struct {
	name         string
	movesPerSec  float64
	steps        int
	appearPerSec float64
	chatPerSec   float64
	chatLen      int
	damagePerSec float64
	castsPerSec  float64
	statsPerSec  float64
	// framesPerSec is how many ticks per second have at least one event.
	framesPerSec float64
	budget       float64
}

func steps(n int) []byte {
	b := make([]byte, (n+1)/2)
	for i := range b {
		b[i] = 0x32
	}
	if n%2 == 1 {
		b[len(b)-1] |= 0xF0
	}
	return b
}

func sampleEvents() map[string]*v1.Event {
	look := v1.Look_builder{Body: 4012, Hair: 12, HairColor: 3, ClothColor: 1, Weapon: 1201, Shield: 2101, HeadTop: 5010, HeadMid: 2202, HeadBottom: 2203, Sex: 1}.Build()
	return map[string]*v1.Event{
		"appear": v1.Event_builder{Appear: v1.EntityAppear_builder{
			Id: 48213, Kind: v1.EntityKind_ENTITY_KIND_PLAYER, X: 156, Y: 187, Dir: 3,
			Look: look, Name: "SomePlayerName", WalkMs: 150, HpPermille: 1000,
		}.Build()}.Build(),
		"vanish": v1.Event_builder{Vanish: v1.EntityVanish_builder{Id: 48213, Reason: v1.VanishReason_VANISH_REASON_OUT_OF_SIGHT}.Build()}.Build(),
		"move10": v1.Event_builder{Move: v1.EntityMove_builder{
			Id: 48213, FromX: 156, FromY: 187, Steps: steps(10), StartMs: 20, WalkMs: 150,
		}.Build()}.Build(),
		"chat": v1.Event_builder{Chat: v1.ChatMessage_builder{
			Channel: v1.ChatChannel_CHAT_CHANNEL_LOCAL, From: 48213, FromName: "SomePlayerName",
			Text: strings.Repeat("x", 40),
		}.Build()}.Build(),
		"damage": v1.Event_builder{Damage: v1.Damage_builder{
			Source: 48213, Target: 50122, Amount: 1834, Hits: 1, Kind: v1.DamageKind_DAMAGE_KIND_NORMAL, DelayMs: 30,
		}.Build()}.Build(),
		"cast": v1.Event_builder{SkillCast: v1.SkillCast_builder{
			Source: 48213, SkillId: 5, Level: 10, CastMs: 0, Entity: proto.Uint32(50122),
		}.Build()}.Build(),
		"stats": v1.Event_builder{Stats: v1.StatUpdate_builder{Stats: []*v1.Stat{
			v1.Stat_builder{Id: v1.StatId_STAT_ID_HP, Value: 8123}.Build(),
			v1.Stat_builder{Id: v1.StatId_STAT_ID_SP, Value: 402}.Build(),
		}}.Build()}.Build(),
	}
}

func size(t *testing.T, m proto.Message) int {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return len(b)
}

// TestBandwidth estimates server-to-client bytes per second for typical
// views and checks them against the budgets in strategy section 3.8.
func TestBandwidth(t *testing.T) {
	ev := sampleEvents()
	sz := map[string]int{}
	for name, e := range ev {
		// Size of the event inside Frame.events: the event plus its tag and length.
		frame := v1.Frame_builder{Events: []*v1.Event{e}}.Build()
		sz[name] = size(t, frame)
	}
	sz["move"] = sz["move10"]
	emptyFrame := size(t, v1.ServerMessage_builder{Frame: v1.Frame_builder{Tick: 1 << 20, AckSeq: 5000}.Build()}.Build())
	t.Logf("event sizes in bytes: %v, empty frame %d", sz, emptyFrame)

	for _, s := range []scenario{
		// 100 players in view, each starting a walk every 3 seconds.
		{name: "busy town", movesPerSec: 100.0 / 3, steps: 10, appearPerSec: 2, chatPerSec: 2, chatLen: 40, statsPerSec: 0.5, framesPerSec: 20, budget: 8 << 10},
		// A crowded event: 300 players in view.
		{name: "town peak", movesPerSec: 300.0 / 2, steps: 10, appearPerSec: 10, chatPerSec: 10, chatLen: 40, statsPerSec: 1, framesPerSec: 20, budget: 32 << 10},
		// Fighting 15 monsters in view with a party of 3.
		{name: "field", movesPerSec: 18.0 / 4, steps: 10, appearPerSec: 0.5, chatPerSec: 0.2, chatLen: 40, damagePerSec: 6, castsPerSec: 1, statsPerSec: 3, framesPerSec: 10, budget: 2 << 10},
	} {
		bytes := s.movesPerSec*float64(sz["move"]) +
			s.appearPerSec*float64(sz["appear"]+sz["vanish"]) +
			s.chatPerSec*float64(sz["chat"]) +
			s.damagePerSec*float64(sz["damage"]) +
			s.castsPerSec*float64(sz["cast"]) +
			s.statsPerSec*float64(sz["stats"]) +
			s.framesPerSec*float64(emptyFrame+wireOverhead)
		t.Logf("%s: %.0f bytes/s, budget %.0f", s.name, bytes, s.budget)
		if bytes > s.budget {
			t.Errorf("%s: %.0f bytes/s is over the budget of %.0f", s.name, bytes, s.budget)
		}
	}
}

func TestSizeLimits(t *testing.T) {
	// The longest chat line the client may send must fit in a client message.
	m := v1.ClientMessage_builder{Seq: 1 << 30, Command: v1.Command_builder{Chat: v1.Chat_builder{
		Channel: v1.ChatChannel_CHAT_CHANNEL_WHISPER, To: strings.Repeat("n", 24), Text: strings.Repeat("é", 255),
	}.Build()}.Build()}.Build()
	if n := size(t, m); n > MaxClientMessageBytes {
		t.Errorf("longest chat message is %d bytes, over %d", n, MaxClientMessageBytes)
	}
}
