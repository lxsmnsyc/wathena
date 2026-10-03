package zone

import (
	"sync"
	"sync/atomic"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Messages to connections go out through an outbox. While Run runs, the
// zone collects a tick's messages and hands them, in one batch, to the
// zone's sender goroutine, which puts each on its connection's channel.
// Each wake of a connection goroutine can enter the kernel. Waking 500
// of them one by one stalled the zone goroutine for up to 9 ms in the
// load test (server/README.md, "Load tests and metrics"). Outside Run,
// as in tests that call stepAt directly, messages go out at once.
//
// The sender keeps each connection's messages in order, and runs the
// steps queued with them, such as a map change, in the same order.

// outMsg is one entry of the outbox: a message for a player, a closing
// of the player's messages, or a step to run after the entries before
// it.
type outMsg struct {
	p     *Player
	m     *wathenav1.ServerMessage
	close bool
	then  func()
}

// outClosed is the state of a channel of messages that more than one
// goroutine may close.
type outClosed struct {
	once   sync.Once
	closed atomic.Bool
	// behind is set when the channel closed because it was full.
	behind atomic.Bool
}

func (o *outClosed) close(ch chan *wathenav1.ServerMessage) {
	o.once.Do(func() {
		o.closed.Store(true)
		close(ch)
	})
}

// outState is the closing state of the player's channel: its session's,
// which every zone the player enters shares, or the player's own.
func (p *Player) outState() *outClosed {
	if p.sess != nil {
		return &p.sess.outClosed
	}
	return &p.ownClosed
}

// deliver puts a message on the player's channel. A connection that
// fell behind has its channel closed, so one slow client cannot stall
// the zone.
func (p *Player) deliver(m *wathenav1.ServerMessage) {
	st := p.outState()
	if st.closed.Load() {
		return
	}
	select {
	case p.out <- m:
	default:
		st.behind.Store(true)
		st.close(p.out)
	}
}

// gone reports whether the player's messages are closed, by the zone or
// because its connection fell behind. Only the zone goroutine calls it.
func (p *Player) gone() bool {
	if !p.dropped && p.outState().closed.Load() {
		p.dropped = true
	}
	return p.dropped
}

// queue sends a message to a player's connection.
func (z *Zone) queue(p *Player, m *wathenav1.ServerMessage) {
	if p.gone() {
		return
	}
	if z.sends == nil {
		p.deliver(m)
		p.gone()
		return
	}
	z.outbox = append(z.outbox, outMsg{p: p, m: m})
}

// closeOut closes a player's messages after the ones queued before.
func (z *Zone) closeOut(p *Player) {
	if z.sends == nil {
		p.outState().close(p.out)
		return
	}
	z.outbox = append(z.outbox, outMsg{p: p, close: true})
}

// then runs f once the messages queued before it went out.
func (z *Zone) then(f func()) {
	if z.sends == nil {
		f()
		return
	}
	z.outbox = append(z.outbox, outMsg{then: f})
}

// flush hands the outbox to the sender.
func (z *Zone) flush() {
	if z.sends == nil || len(z.outbox) == 0 {
		return
	}
	z.sends <- z.outbox
	select {
	case b := <-z.spare:
		z.outbox = b
	default:
		z.outbox = make([]outMsg, 0, cap(z.outbox))
	}
}

// startSender starts the sender goroutine. stopSender hands it the last
// messages and waits until it delivered them.
func (z *Zone) startSender() {
	z.sends = make(chan []outMsg, 16)
	z.spare = make(chan []outMsg, 16)
	z.senderDone = make(chan struct{})
	go z.send(z.sends, z.spare, z.senderDone)
}

func (z *Zone) stopSender() {
	z.flush()
	close(z.sends)
	<-z.senderDone
	z.sends = nil
}

func (z *Zone) send(batches <-chan []outMsg, spare chan<- []outMsg, done chan<- struct{}) {
	defer close(done)
	for b := range batches {
		for _, o := range b {
			switch {
			case o.then != nil:
				o.then()
			case o.close:
				o.p.outState().close(o.p.out)
			default:
				o.p.deliver(o.m)
			}
		}
		clear(b)
		select {
		case spare <- b[:0]:
		default:
		}
	}
}
