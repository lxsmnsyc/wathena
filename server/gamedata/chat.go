package gamedata

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// Chat channels and the chat filter (proposal 0002, review decision 26):
// core's data/channels.yml and data/chat_filter.yml, which the core mod
// carries, with the mods' records over them. mods/README.md, "Chat
// channels and the chat filter", lists the fields.

// modChat merges the channel and chat filter records.
func modChat(set *mod.Set) (*chat.Data, []*mod.Overlay, error) {
	out := &chat.Data{}
	var errs []error
	co, err := set.Overlay(mod.ChannelKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	for _, r := range co.Records {
		if r.Mod != mod.Core {
			errs = append(errs, fmt.Errorf("%s: channels %s: a mod changes core's channels by name, and makes its own with channels.create", r.Where, r.Name))
			continue
		}
		if modscript.IsChatKind(r.Name) {
			errs = append(errs, fmt.Errorf("%s: channels %s: public, whisper, party and guild are not channel names", r.Where, r.Name))
			continue
		}
		d, err := chat.BuildChannel(r.Name, r.Fields)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: channels %s: %w", r.Where, r.Name, err))
			continue
		}
		out.Channels = append(out.Channels, d)
	}
	fo, err := set.Overlay(mod.ChatFilterKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	var words []chat.Word
	for _, r := range fo.Records {
		word := r.Name
		if r.Mod != mod.Core {
			word = mod.Local(r.Name)
		}
		w, err := chat.BuildWord(word, r.Fields)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: chat filter %s: %w", r.Where, r.Name, err))
			continue
		}
		words = append(words, w)
	}
	out.Filter = chat.NewFilter(words)
	return out, []*mod.Overlay{co, fo}, errors.Join(errs...)
}
