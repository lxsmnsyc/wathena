package chat

import (
	"embed"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// CoreFiles are core's data/channels.yml and data/chat_filter.yml, in
// core/. The core mod carries them, and server/gamedata merges the mods'
// records over them.
//
//go:embed core
var CoreFiles embed.FS

// Files of the records.
const (
	ChannelFile = "data/channels.yml"
	FilterFile  = "data/chat_filter.yml"
)

// Join rules of a channel (review decision 26).
const (
	// JoinOpen lets any player join.
	JoinOpen = "open"
	// JoinInvite only takes players that a mod adds with channel:add.
	JoinInvite = "invite"
)

// Channel kinds.
const (
	// KindWorld is one channel for the whole world.
	KindWorld = "world"
	// KindMap is one channel per map, as rAthena's CHAN_TYPE_MAP: members
	// hear the members on their own map, and leave on a map change.
	KindMap = "map"
)

// Limits of channel records.
const (
	// MaxChannelName is the longest channel name without its #, as
	// rAthena's CHAN_NAME_LENGTH of 20 with the #.
	MaxChannelName = 19
	// MaxDelay is the longest delay_milliseconds.
	MaxDelay = 60_000
)

// ChannelDef is one channel: a record of data/channels.yml, or one that
// a mod's channels.create made.
type ChannelDef struct {
	// Name is the channel's name without its #, in lower case.
	Name string
	// DisplayName is what players see before each line, as rAthena's
	// alias, such as [Trade].
	DisplayName string
	// Color is the lines' color, 0xRRGGBB.
	Color uint32
	// Join is JoinOpen or JoinInvite.
	Join string
	// Autojoin puts every player in the channel at login.
	Autojoin bool
	// CanLeave and CanChat are rAthena's leave and chat options.
	CanLeave, CanChat bool
	// Delay is the least time between two lines of one member, in
	// milliseconds, as rAthena's delay.
	Delay int
	// Kind is KindWorld or KindMap.
	Kind string
	// Mod is the mod that made the channel with channels.create, or "" for
	// a record.
	Mod string
}

// Full is the channel's name as players write it, with its #.
func (d *ChannelDef) Full() string { return "#" + d.Name }

// ChannelFields are the fields of a channel record.
var ChannelFields = []string{"display_name", "color", "join", "autojoin", "can_leave", "can_chat", "delay_milliseconds", "kind"}

// NormalizeChannel turns a channel name, with or without its #, into the
// name without it in lower case. ok is false for a name that is not 1 to
// MaxChannelName letters, digits, _ and -.
func NormalizeChannel(name string) (string, bool) {
	n := strings.ToLower(strings.TrimPrefix(name, "#"))
	if n == "" || len(n) > MaxChannelName {
		return "", false
	}
	for _, r := range n {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return "", false
		}
	}
	return n, true
}

// BuildChannel checks a channel record's fields and makes its channel.
// name is the record's name, such as trade.
func BuildChannel(name string, fields map[string]any) (ChannelDef, error) {
	n, ok := NormalizeChannel(name)
	if !ok || n != name {
		return ChannelDef{}, fmt.Errorf("the name must be 1 to %d lower case letters, digits, _ and -, without the #", MaxChannelName)
	}
	d := ChannelDef{Name: n, DisplayName: "[" + n + "]", Color: 0xFFFFFF, Join: JoinOpen, CanLeave: true, CanChat: true, Delay: 1000, Kind: KindWorld}
	var errs []string
	bad := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	for _, f := range slices.Sorted(maps.Keys(fields)) {
		v := fields[f]
		switch f {
		case "display_name":
			s, ok := v.(string)
			if !ok || s == "" || utf8.RuneCountInString(s) > 24 {
				bad("display_name must be 1 to 24 characters")
				continue
			}
			d.DisplayName = s
		case "color":
			c, ok := v.(int)
			if !ok || c < 0 || c > 0xFFFFFF {
				bad("color must be a color written 0xRRGGBB")
				continue
			}
			d.Color = uint32(c) // #nosec G115 -- checked above.
		case "join":
			s, _ := v.(string)
			if s != JoinOpen && s != JoinInvite {
				bad("join must be open or invite")
				continue
			}
			d.Join = s
		case "autojoin", "can_leave", "can_chat":
			b, ok := v.(bool)
			if !ok {
				bad("%s must be true or false", f)
				continue
			}
			switch f {
			case "autojoin":
				d.Autojoin = b
			case "can_leave":
				d.CanLeave = b
			default:
				d.CanChat = b
			}
		case "delay_milliseconds":
			ms, ok := v.(int)
			if !ok || ms < 0 || ms > MaxDelay {
				bad("delay_milliseconds must be 0 to %d", MaxDelay)
				continue
			}
			d.Delay = ms
		case "kind":
			s, _ := v.(string)
			if s != KindWorld && s != KindMap {
				bad("kind must be world or map")
				continue
			}
			d.Kind = s
		default:
			bad("%q is not a field; channels have %s", f, strings.Join(ChannelFields, ", "))
		}
	}
	if d.Autojoin && d.Join == JoinInvite {
		bad("autojoin needs join: open")
	}
	if len(errs) > 0 {
		return ChannelDef{}, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return d, nil
}

// FilterFields are the fields of a chat filter record.
var FilterFields = []string{"action", "match"}

// BuildWord checks a chat filter record's fields. word is the record's
// name without a mod's prefix.
func BuildWord(word string, fields map[string]any) (Word, error) {
	w := Word{Word: word, Action: Replace}
	if strings.TrimSpace(word) == "" || utf8.RuneCountInString(word) > 64 {
		return Word{}, fmt.Errorf("the word must be 1 to 64 characters")
	}
	var errs []string
	for _, f := range slices.Sorted(maps.Keys(fields)) {
		v := fields[f]
		switch f {
		case "action":
			s, _ := v.(string)
			switch Action(s) {
			case Replace, Reject, Reserve:
				w.Action = Action(s)
			default:
				errs = append(errs, "action must be replace, reject or reserve")
			}
		case "match":
			s, _ := v.(string)
			switch s {
			case "word":
			case "part":
				w.Part = true
			default:
				errs = append(errs, "match must be word or part")
			}
		default:
			errs = append(errs, fmt.Sprintf("%q is not a field; chat filter words have %s", f, strings.Join(FilterFields, ", ")))
		}
	}
	if len(errs) > 0 {
		return Word{}, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return w, nil
}

// Data is the channels and the filter of a world.
type Data struct {
	Channels []ChannelDef
	Filter   *Filter
}

var (
	coreOnce sync.Once
	coreData *Data
	coreErr  error
)

// Core returns core's channels and filter alone, for a world that runs
// without mods, such as in tests. The result is shared: do not change
// it.
func Core() *Data {
	coreOnce.Do(func() { coreData, coreErr = parseCore() })
	if coreErr != nil {
		panic(coreErr)
	}
	return coreData
}

func parseCore() (*Data, error) {
	d := &Data{}
	recs, err := CoreRecords(ChannelFile)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		c, err := BuildChannel(r.Name, r.Fields)
		if err != nil {
			return nil, fmt.Errorf("chat: core channel %s: %w", r.Name, err)
		}
		d.Channels = append(d.Channels, c)
	}
	recs, err = CoreRecords(FilterFile)
	if err != nil {
		return nil, err
	}
	var words []Word
	for _, r := range recs {
		w, err := BuildWord(r.Name, r.Fields)
		if err != nil {
			return nil, fmt.Errorf("chat: core chat filter %s: %w", r.Name, err)
		}
		words = append(words, w)
	}
	d.Filter = NewFilter(words)
	return d, nil
}

// CoreRecord is one record of core's files.
type CoreRecord struct {
	Name   string
	Fields map[string]any
}

// CoreRecords reads one of core's files.
func CoreRecords(file string) ([]CoreRecord, error) {
	data, err := CoreFiles.ReadFile("core/" + file)
	if err != nil {
		return nil, err
	}
	var list []map[string]any
	if err := yaml.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("chat: core %s: %w", file, err)
	}
	out := make([]CoreRecord, 0, len(list))
	for _, m := range list {
		name, _ := m["name"].(string)
		delete(m, "name")
		out = append(out, CoreRecord{Name: name, Fields: m})
	}
	return out, nil
}
