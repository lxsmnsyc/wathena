package zone

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestCallIsAllOrNothing checks the zone calls of other goroutines
// (review 2026-10): a call whose caller gives up before the zone starts
// it never runs, and a call the zone started is waited for and reported,
// even when the caller's context ends meanwhile.
func TestCallIsAllOrNothing(t *testing.T) {
	z := New("map/test", open(10), rng.FromKey([32]byte{7}).Stream(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go z.Run(ctx)
	wait, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()

	// The zone is busy with a call that waits for release.
	release := make(chan struct{})
	busy := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		busy <- z.do(wait, func() error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	// A call queued behind it, whose caller gives up, never runs.
	var ran atomic.Bool
	short, cancelShort := context.WithTimeout(wait, 50*time.Millisecond)
	defer cancelShort()
	queued := make(chan error, 1)
	go func() {
		queued <- z.do(short, func() error {
			ran.Store(true)
			return nil
		})
	}()
	<-short.Done()
	if err := <-queued; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the queued call answered %v", err)
	}

	// A call that started is waited for past its caller's deadline.
	close(release)
	if err := <-busy; err != nil {
		t.Fatal(err)
	}
	slow, cancelSlow := context.WithTimeout(wait, 50*time.Millisecond)
	defer cancelSlow()
	want := errors.New("the call's own error")
	var finished atomic.Bool
	err := z.do(slow, func() error {
		time.Sleep(200 * time.Millisecond)
		finished.Store(true)
		return want
	})
	if !errors.Is(err, want) || !finished.Load() {
		t.Fatalf("the started call answered %v, finished %v", err, finished.Load())
	}

	// The zone runs calls in order, and skipped the abandoned one.
	if _, err := z.Summary(wait); err != nil {
		t.Fatal(err)
	}
	if ran.Load() {
		t.Fatal("the zone ran a call whose caller gave up before it started")
	}
}
