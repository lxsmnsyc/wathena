package main

import (
	"testing"
	"time"
)

// The example in server/README.md, "Bots".
const example = `name: market
bots: 200
ramp: 20
warmup: 10s
duration: 2m
chat_rate: 4
behaviour:
  walk_every: 5s
  walk_radius: 6
  stay: true
`

func TestParseScenario(t *testing.T) {
	sc, err := parseScenario([]byte(example))
	if err != nil {
		t.Fatal(err)
	}
	b := sc.Behaviour
	if sc.Name != "market" || sc.Bots != 200 || sc.Ramp != 20 || sc.Warmup != 10*time.Second || sc.Duration != 2*time.Minute || sc.ChatRate != 4 ||
		b.WalkEvery != 5*time.Second || b.WalkRadius != 6 || !b.Stay {
		t.Errorf("%+v", sc)
	}
	if _, err := parseScenario([]byte("bots: 2\nspeed: 3\n")); err == nil {
		t.Error("an unknown field was accepted")
	}
}

func TestSpreadBands(t *testing.T) {
	var fields []worldMap
	for i := range 30 {
		lo := 1 + (29-i)*3
		fields = append(fields, worldMap{id: string(rune('a' + i)), kind: "field", band: [2]int{lo, lo + 9}})
	}
	got := spreadBands(fields, 8)
	if len(got) != 8 || got[0].band[0] != 1 || got[7].band[0] != 88 {
		t.Fatalf("%v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i].band[0] <= got[i-1].band[0] {
			t.Errorf("bands not rising: %v", got)
		}
	}
	if got := spreadBands(fields[:3], 8); len(got) != 3 {
		t.Errorf("fewer fields than asked: %v", got)
	}
}

func TestHTTPBase(t *testing.T) {
	for in, want := range map[string]string{
		"ws://127.0.0.1:8300/ws":    "http://127.0.0.1:8300",
		"wss://game.example.com/ws": "https://game.example.com",
	} {
		if got, err := httpBase(in); err != nil || got != want {
			t.Errorf("%s: %q, %v", in, got, err)
		}
	}
	if _, err := httpBase("http://127.0.0.1/ws"); err == nil {
		t.Error("an http URL was taken")
	}
}
