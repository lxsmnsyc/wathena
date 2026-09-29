// Package schema holds the protocol version shared by the server and the
// web client. The messages are in proto/ and the generated Go code in gen/go.
package schema

// Version is the protocol version. It increases by 1 for each release that
// changes the protocol. The client sends it in Hello.
const Version = 1

// MinVersion is the oldest client version the server accepts.
const MinVersion = 1

// MaxMessageBytes is the largest WebSocket message either side may send
// (strategy section 3.8). Larger data uses HTTP downloads.
const MaxMessageBytes = 64 << 10

// MaxClientMessageBytes is the largest message the client may send.
// Client messages are commands and chat, so they are small.
const MaxClientMessageBytes = 2 << 10
