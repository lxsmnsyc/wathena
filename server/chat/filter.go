// Package chat holds what chat and moderation share across the zones of
// a world (strategy section 7.10, proposal 0002 review decision 26):
//   - the chat filter of data/chat_filter.yml, for chat lines and names,
//   - the chat channels of data/channels.yml and of mods,
//   - the hub, which finds online players by name for whispers, keeps
//     their block lists, and runs the channels,
//   - the rate limits of chat, which are server settings,
//   - the chat log and the reports, which the store or memory keep.
//
// docs/decisions/0018-chat-and-moderation.md records the choices.
package chat

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// Action is what the chat filter does with a word.
type Action string

// The filter's actions.
const (
	// Replace turns the word into asterisks, one per character.
	Replace Action = "replace"
	// Reject refuses the whole line.
	Reject Action = "reject"
	// Reserve only applies to names: a character name may not have the
	// word, such as a staff title. Chat may.
	Reserve Action = "reserve"
)

// Word is one entry of the chat filter.
type Word struct {
	// Word is the text to find. Case does not matter.
	Word   string
	Action Action
	// Part matches the word inside longer words too. Without it, the
	// word must stand alone: the characters around it are not letters or
	// digits.
	Part bool
}

// Filter is the server's chat filter. It is safe for use by many
// goroutines, since it never changes after NewFilter.
type Filter struct {
	words []filterWord
}

type filterWord struct {
	Word
	runes []rune
}

// NewFilter makes a filter of words. Longer words are tried first, so a
// phrase wins over a word inside it.
func NewFilter(words []Word) *Filter {
	f := &Filter{}
	for _, w := range words {
		r := lowerRunes(w.Word)
		if len(r) == 0 {
			continue
		}
		f.words = append(f.words, filterWord{w, r})
	}
	slices.SortStableFunc(f.words, func(a, b filterWord) int { return len(b.runes) - len(a.runes) })
	return f
}

// Len is the number of words.
func (f *Filter) Len() int {
	if f == nil {
		return 0
	}
	return len(f.words)
}

// Check applies the filter to a chat line. It returns the line with
// replaced words, and false when a word rejects the line. Reserved words
// do not apply to chat.
func (f *Filter) Check(text string) (string, bool) {
	if f.Len() == 0 {
		return text, true
	}
	orig := []rune(text)
	low := make([]rune, len(orig))
	for i, r := range orig {
		low[i] = unicode.ToLower(r)
	}
	changed := false
	for _, w := range f.words {
		if w.Action == Reserve {
			continue
		}
		for i := 0; i+len(w.runes) <= len(low); i++ {
			if !w.matchAt(low, i) {
				continue
			}
			if w.Action == Reject {
				return "", false
			}
			for j := i; j < i+len(w.runes); j++ {
				orig[j], low[j] = '*', '*'
			}
			changed = true
			i += len(w.runes) - 1
		}
	}
	if !changed {
		return text, true
	}
	return string(orig), true
}

// Errors of CheckName.
var (
	ErrNameFiltered = errors.New("the name has a word the server does not allow")
	ErrNameReserved = errors.New("the name has a reserved word, such as a staff title")
)

// CheckName checks a character name: no word of the filter may be in
// it, whatever its action. It returns ErrNameReserved for a reserved
// word and ErrNameFiltered for the others, wrapped with the word.
func (f *Filter) CheckName(name string) error {
	if f.Len() == 0 {
		return nil
	}
	low := lowerRunes(name)
	for _, w := range f.words {
		for i := 0; i+len(w.runes) <= len(low); i++ {
			if !w.matchAt(low, i) {
				continue
			}
			if w.Action == Reserve {
				return fmt.Errorf("%w: %s", ErrNameReserved, w.Word.Word)
			}
			return ErrNameFiltered
		}
	}
	return nil
}

// matchAt reports whether the word is at i of the lower case text.
func (w *filterWord) matchAt(low []rune, i int) bool {
	if !slices.Equal(low[i:i+len(w.runes)], w.runes) {
		return false
	}
	if w.Part {
		return true
	}
	end := i + len(w.runes)
	return (i == 0 || !wordRune(low[i-1])) && (end == len(low) || !wordRune(low[end]))
}

func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func lowerRunes(s string) []rune {
	r := []rune(strings.TrimSpace(s))
	for i, c := range r {
		r[i] = unicode.ToLower(c)
	}
	return r
}
