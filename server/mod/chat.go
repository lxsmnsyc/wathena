package mod

// ChannelKind is data/channels.yml and ChatFilterKind data/chat_filter.yml
// (proposal 0002, review decision 26). Core's own files add the core
// channels and words with plain names. A mod changes core's channels by
// name, and makes its own with channels.create. A mod adds words with
// plain names: they become <mod ID>:<word>, and the word is the part
// after the mod ID. server/gamedata reads the fields.
var (
	ChannelKind    = Kind{Name: "channels", File: "data/channels.yml", CoreNames: true}
	ChatFilterKind = Kind{Name: "chat filter words", File: "data/chat_filter.yml", CoreNames: true, LocalNames: true, Economy: []string{"action", "match"}}
)
