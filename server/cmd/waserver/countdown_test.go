package main

import (
	"log/slog"
	"os"
	"slices"
	"syscall"
	"testing"
	"time"
)

func TestCountdown(t *testing.T) {
	var got []string
	began := time.Now()
	countdown(2100*time.Millisecond, func(text string) { got = append(got, text) }, make(chan os.Signal), slog.New(slog.DiscardHandler))
	if took := time.Since(began); took < 2*time.Second || took > 4*time.Second {
		t.Errorf("the countdown took %v", took)
	}
	want := []string{"The server will restart in 2 seconds.", "The server will restart in 1 second."}
	if !slices.Equal(got, want) {
		t.Errorf("announced %q", got)
	}
	// A second signal ends it at once.
	signals := make(chan os.Signal, 1)
	signals <- syscall.SIGTERM
	began = time.Now()
	countdown(time.Minute, func(string) {}, signals, slog.New(slog.DiscardHandler))
	if time.Since(began) > time.Second {
		t.Error("a signal did not end the countdown")
	}
}
