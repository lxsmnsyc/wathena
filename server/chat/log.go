package chat

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"sync"
	"time"
)

// LineID identifies one line a player said: a UUID of version 7, so IDs
// sort by time. Players report a line by its ID.
type LineID [16]byte

// NewLineID makes the ID of a line said at t.
func NewLineID(t time.Time) LineID {
	var id LineID
	if _, err := rand.Read(id[6:]); err != nil {
		panic(err)
	}
	var ms [8]byte
	binary.BigEndian.PutUint64(ms[:], uint64(t.UnixMilli())) // #nosec G115 -- the clock is after 1970.
	copy(id[:6], ms[2:])
	id[6] = id[6]&0x0f | 0x70 // version 7
	id[8] = id[8]&0x3f | 0x80 // RFC 9562 variant
	return id
}

// IsZero reports whether id is the zero ID.
func (id LineID) IsZero() bool { return id == LineID{} }

// String is the UUID's text form.
func (id LineID) String() string {
	var b [36]byte
	hex.Encode(b[0:8], id[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], id[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], id[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], id[8:10])
	b[23] = '-'
	hex.Encode(b[24:], id[10:])
	return string(b[:])
}

// LineIDFrom reads an ID from the protocol's bytes. ok is false for
// anything but 16 bytes.
func LineIDFrom(b []byte) (LineID, bool) {
	var id LineID
	if len(b) != len(id) {
		return id, false
	}
	copy(id[:], b)
	return id, true
}

// Kinds of chat lines in the log.
const (
	LineLocal   = "local"
	LineWhisper = "whisper"
	LineChannel = "channel"
	LineParty   = "party"
	LineGuild   = "guild"
)

// Line is one line a player said, as the chat log keeps it.
type Line struct {
	ID LineID
	At time.Time
	// Kind is LineLocal, LineWhisper, LineChannel, LineParty or LineGuild.
	Kind string
	// CharacterID is the speaker's database ID, or 0 for a character that
	// only lives in memory.
	CharacterID int64
	Name        string
	// To is the whisper's target, the channel's name with its #, or the
	// party's name.
	To string
	// Map is the speaker's map, such as map/prontera.
	Map  string
	Text string
}

// Report is one player's report of a player or a chat line (strategy
// section 7.10).
type Report struct {
	At         time.Time
	ReporterID int64
	Reporter   string
	// Target is the reported character's name. TargetID is its database
	// ID when the reported line says it, or 0.
	Target   string
	TargetID int64
	// Line is the reported line, or the zero ID for a report of a player.
	Line   LineID
	Reason string
	Map    string
	// Guild is the reported guild's ID for a report of a guild's name or
	// emblem, with Target its name. Emblem is the reported emblem's
	// version and EmblemPNG a copy of it, for a report of an emblem
	// (strategy section 7.10).
	Guild     int64
	Emblem    int
	EmblemPNG []byte
	// Context is the chat the reporter saw in the ContextWindow before
	// the report, oldest first.
	Context []Line
}

// ContextWindow is how far back a report's chat goes (strategy section
// 7.10).
const ContextWindow = 5 * time.Minute

// Log keeps the chat log and the reports. The store's keeps them in the
// database, and MemoryLog in memory. Both calls must not block: they
// run on zone goroutines.
type Log interface {
	// Line logs a line a player said.
	Line(l Line)
	// Report saves a report. Its context lines were logged before.
	Report(r Report)
}

// MemoryLog keeps the last MemoryLines lines and MemoryReports reports
// in memory, for a server without a database. Its reports show at
// /debug/wathena/reports with --dev.
type MemoryLog struct {
	mu      sync.Mutex
	lines   []Line
	reports []Report
}

// Limits of MemoryLog.
const (
	MemoryLines   = 10_000
	MemoryReports = 1_000
)

// Line keeps a line.
func (m *MemoryLog) Line(l Line) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lines = append(m.lines, l)
	if len(m.lines) > MemoryLines {
		m.lines = append(m.lines[:0], m.lines[len(m.lines)-MemoryLines:]...)
	}
}

// Report keeps a report.
func (m *MemoryLog) Report(r Report) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reports = append(m.reports, r)
	if len(m.reports) > MemoryReports {
		m.reports = append(m.reports[:0], m.reports[len(m.reports)-MemoryReports:]...)
	}
}

// Lines returns the kept lines, oldest first.
func (m *MemoryLog) Lines() []Line {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Line(nil), m.lines...)
}

// Reports returns the kept reports, oldest first.
func (m *MemoryLog) Reports() []Report {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Report(nil), m.reports...)
}
