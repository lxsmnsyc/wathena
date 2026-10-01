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
