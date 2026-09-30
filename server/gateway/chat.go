package gateway

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// MaxChat is the most characters a chat line may have, counted as
// Unicode code points. rAthena's CHAT_SIZE_MAX is 255 bytes plus the
// terminating zero.
const MaxChat = 255

// Chat rate limits, per connection. A player may send ChatBurst lines at
// once, and one more every ChatRefill after that. rAthena's
// min_chat_delay does the same with a burst of 1, and is off by default.
const (
	ChatBurst  = 5
	ChatRefill = time.Second
	// ChatStrikes is how many dropped lines, less one for every
	// ChatRefill, end the connection. A person typing fast never gets
	// near it. A bot sending 20 lines a second does in about 3 seconds.
	ChatStrikes = 60
)

// CleanChat checks a chat line and returns the text to show. It returns
// false for text that is not valid UTF-8, that is empty after cleaning,
// or that is longer than MaxChat code points after cleaning. Cleaning
// turns control characters, such as tabs and newlines, into spaces,
// removes the bidirectional formatting characters that could reorder
// the line or the text around it, and trims spaces at both ends.
func CleanChat(text string) (string, bool) {
	if !utf8.ValidString(text) {
		return "", false
	}
	var b strings.Builder
	for _, r := range text {
		switch {
		case bidiControl(r):
		case unicode.IsControl(r):
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" || utf8.RuneCountInString(out) > MaxChat {
		return "", false
	}
	return out, true
}

// bidiControl reports whether r is a bidirectional mark, embedding,
// override or isolate.
func bidiControl(r rune) bool {
	switch {
	case r == '\u061C', r == '\u200E', r == '\u200F':
		return true
	case r >= '\u202A' && r <= '\u202E':
		return true
	case r >= '\u2066' && r <= '\u2069':
		return true
	}
	return false
}

// chatLimit is one connection's chat rate limit: a token bucket of
// ChatBurst lines that refills one line per ChatRefill, and a count of
// dropped lines that also falls by one per ChatRefill.
type chatLimit struct {
	tokens  float64
	strikes float64
	last    time.Time
}

func newChatLimit(now time.Time) *chatLimit {
	return &chatLimit{tokens: ChatBurst, last: now}
}

// allow reports whether a line sent at now may go out. When it may not,
// over reports whether the connection has dropped too many lines.
func (l *chatLimit) allow(now time.Time) (ok, over bool) {
	if d := now.Sub(l.last); d > 0 {
		refill := float64(d) / float64(ChatRefill)
		l.tokens = min(ChatBurst, l.tokens+refill)
		l.strikes = max(0, l.strikes-refill)
		l.last = now
	}
	if l.tokens >= 1 {
		l.tokens--
		return true, false
	}
	l.strikes++
	return false, l.strikes > ChatStrikes
}
