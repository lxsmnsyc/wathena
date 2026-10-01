package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"
)

// countdown warns every player before the server stops: at the start,
// then at 30, 10, 5, 3, 2 and 1 seconds left. A signal ends it early.
func countdown(d time.Duration, announce func(text string), signals <-chan os.Signal, log *slog.Logger) {
	if d <= 0 {
		return
	}
	log.Info("shutting down after a countdown", "countdown", d)
	end := time.Now().Add(d)
	say := func(left time.Duration) {
		s := int(left.Round(time.Second) / time.Second)
		unit := "seconds"
		if s == 1 {
			unit = "second"
		}
		announce(fmt.Sprintf("The server will restart in %d %s.", s, unit))
	}
	say(d)
	for _, mark := range []time.Duration{30 * time.Second, 10 * time.Second, 5 * time.Second, 3 * time.Second, 2 * time.Second, time.Second} {
		if mark > d-time.Second/2 {
			continue
		}
		t := time.NewTimer(time.Until(end.Add(-mark)))
		select {
		case <-t.C:
			say(mark)
		case <-signals:
			t.Stop()
			return
		}
	}
	t := time.NewTimer(time.Until(end))
	defer t.Stop()
	select {
	case <-t.C:
	case <-signals:
	}
}
