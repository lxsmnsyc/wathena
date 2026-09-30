package grfimport

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/unicode/norm"

	"github.com/lxsmnsyc/wathena/internal/grf"
)

// Item text is what the game client shows for an item: its names, its
// slot count, and its description. It comes from the client's tables:
//
//   - iteminfo.lub: identifiedDisplayName, unidentifiedDisplayName,
//     slotCount, and identifiedDescriptionName, a list of lines.
//   - idnum2itemdisplaynametable.txt, num2itemdisplaynametable.txt,
//     itemslotcounttable.txt, and idnum2itemdesctable.txt, for IDs and
//     fields iteminfo.lub lacks.
//
// The text is in the client's code page, which the build decodes to UTF-8.

const (
	itemNamePath    = `data\idnum2itemdisplaynametable.txt`
	itemUnnamePath  = `data\num2itemdisplaynametable.txt`
	itemSlotsPath   = `data\itemslotcounttable.txt`
	itemDescPath    = `data\idnum2itemdesctable.txt`
	maxItemSlots    = 4
	maxDescLines    = 64
	maxItemTextRune = 1024
)

// ItemTextSources are the client files item text comes from, in the
// order they are read.
var ItemTextSources = []string{itemNamePath, itemUnnamePath, itemSlotsPath, itemDescPath, itemInfoPath}

// ItemText is the text of one item.
type ItemText struct {
	// Name is the identified display name.
	Name string
	// Unidentified is the name before the item is identified.
	Unidentified string
	// Slots is the number of card slots, 0 to 4.
	Slots int
	// Desc is the identified description, one entry per line. Lines keep
	// RO's color codes, "^" and six hex digits.
	Desc []string
}

// CodePages are the code pages client text can be in. The names are
// Windows code page numbers, which is how clients are usually described.
var CodePages = map[string]encoding.Encoding{
	"cp949":  korean.EUCKR, // Korean, a superset of EUC-KR.
	"cp932":  japanese.ShiftJIS,
	"cp936":  simplifiedchinese.GBK,
	"cp950":  traditionalchinese.Big5,
	"cp874":  charmap.Windows874,
	"cp1250": charmap.Windows1250,
	"cp1251": charmap.Windows1251,
	"cp1252": charmap.Windows1252,
	"cp1254": charmap.Windows1254,
	"cp1258": charmap.Windows1258,
	"utf-8":  nil,
}

// DefaultCodePage is the code page of the Korean client.
const DefaultCodePage = "cp949"

// TextDecoder decodes client text in a code page to UTF-8 in form NFC.
// Bytes the code page does not map, and invalid UTF-8 in a utf-8 client,
// are dropped.
type TextDecoder struct {
	enc encoding.Encoding
}

// NewTextDecoder returns the decoder for a code page named in CodePages.
func NewTextDecoder(codePage string) (TextDecoder, error) {
	enc, ok := CodePages[strings.ToLower(codePage)]
	if !ok {
		names := make([]string, 0, len(CodePages))
		for n := range CodePages {
			names = append(names, n)
		}
		sort.Strings(names)
		return TextDecoder{}, fmt.Errorf("unknown code page %q, want one of %s", codePage, strings.Join(names, ", "))
	}
	return TextDecoder{enc: enc}, nil
}

// Decode converts a string of client bytes.
func (d TextDecoder) Decode(s string) string {
	out := s
	if d.enc != nil {
		// A fresh decoder each time, since decoders keep state.
		dec, err := d.enc.NewDecoder().String(s)
		if err == nil {
			out = dec
		}
	}
	out = strings.ToValidUTF8(out, "")
	out = strings.ReplaceAll(out, "�", "")
	return norm.NFC.String(out)
}

// ItemTexts reads the text of every item the client describes.
func ItemTexts(c *grf.Client, d TextDecoder) (map[int]*ItemText, error) {
	out := map[int]*ItemText{}
	get := func(id int) *ItemText {
		t := out[id]
		if t == nil {
			t = &ItemText{}
			out[id] = t
		}
		return t
	}
	read := func(path string) (string, bool, error) {
		s, ok := c.Lookup(path)
		if !ok {
			return "", false, nil
		}
		b, err := c.ReadFile(s)
		if err != nil {
			return "", false, err
		}
		return string(b), true, nil
	}

	// The text tables come first, and iteminfo.lub replaces what it has.
	for _, tbl := range []struct {
		path string
		set  func(t *ItemText, v string)
	}{
		{itemNamePath, func(t *ItemText, v string) { t.Name = v }},
		{itemUnnamePath, func(t *ItemText, v string) { t.Unidentified = v }},
		{itemSlotsPath, func(t *ItemText, v string) {
			if n, err := strconv.Atoi(v); err == nil {
				t.Slots = n
			}
		}},
	} {
		b, ok, err := read(tbl.path)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		for id, v := range parseTextTable(b) {
			// The text tables write spaces as "_", as the client reads them.
			// They are replaced after decoding, since in some code pages
			// the byte of "_" can be part of a character.
			if v = cleanLine(strings.ReplaceAll(d.Decode(v), "_", " ")); v != "" {
				tbl.set(get(id), v)
			}
		}
	}
	if b, ok, err := read(itemDescPath); err != nil {
		return nil, err
	} else if ok {
		for id, lines := range parseDescTable(b) {
			get(id).Desc = decodeLines(d, lines)
		}
	}
	if b, ok, err := read(itemInfoPath); err != nil {
		return nil, err
	} else if ok {
		g := map[string]any{}
		if err := evalLub([]byte(b), g); err != nil {
			return nil, fmt.Errorf("%s: %w", itemInfoPath, err)
		}
		if tbl, _ := g["tbl"].(*LubTable); tbl != nil {
			for k, v := range tbl.M {
				id, ok1 := k.(float64)
				e, ok2 := v.(*LubTable)
				if !ok1 || !ok2 || id < 0 || id >= maxItemID || id != float64(int(id)) {
					continue
				}
				lubItemText(get(int(id)), e, d)
			}
		}
	}
	for id, t := range out {
		t.Slots = max(0, min(t.Slots, maxItemSlots))
		if t.Name == "" && t.Unidentified == "" && len(t.Desc) == 0 {
			delete(out, id)
		}
	}
	return out, nil
}

// lubItemText sets the fields an iteminfo.lub entry has.
func lubItemText(t *ItemText, e *LubTable, d TextDecoder) {
	if s, ok := e.M["identifiedDisplayName"].(string); ok {
		if v := cleanLine(d.Decode(s)); v != "" {
			t.Name = v
		}
	}
	if s, ok := e.M["unidentifiedDisplayName"].(string); ok {
		if v := cleanLine(d.Decode(s)); v != "" {
			t.Unidentified = v
		}
	}
	if n, ok := e.M["slotCount"].(float64); ok && n == float64(int(n)) {
		t.Slots = int(n)
	}
	if lt, ok := e.M["identifiedDescriptionName"].(*LubTable); ok {
		var lines []string
		for i := 1; i <= len(lt.M) && i <= maxDescLines; i++ {
			s, ok := lt.M[float64(i)].(string)
			if !ok {
				break
			}
			lines = append(lines, s)
		}
		if len(lines) > 0 {
			t.Desc = decodeLines(d, lines)
		}
	}
}

// parseTextTable reads "id#value#" lines. Other lines, such as comments,
// are skipped.
func parseTextTable(b string) map[int]string {
	out := map[int]string{}
	for line := range strings.SplitSeq(b, "\n") {
		parts := strings.Split(strings.TrimSpace(line), "#")
		if len(parts) < 2 {
			continue
		}
		id, err := strconv.Atoi(parts[0])
		if err != nil || id < 0 || id >= maxItemID {
			continue
		}
		out[id] = parts[1]
	}
	return out
}

// parseDescTable reads idnum2itemdesctable.txt, where an entry is a line
// "id#", then its lines, then a line that is "#".
func parseDescTable(b string) map[int][]string {
	out := map[int][]string{}
	id := -1
	var lines []string
	for line := range strings.SplitSeq(b, "\n") {
		line = strings.TrimRight(line, "\r")
		if id < 0 {
			head, ok := strings.CutSuffix(strings.TrimSpace(line), "#")
			n, err := strconv.Atoi(head)
			if ok && err == nil && n >= 0 && n < maxItemID {
				id, lines = n, nil
			}
			continue
		}
		if strings.TrimSpace(line) == "#" {
			out[id] = lines
			id = -1
			continue
		}
		if len(lines) < maxDescLines {
			lines = append(lines, line)
		}
	}
	return out
}

// decodeLines decodes description lines and drops trailing empty ones.
func decodeLines(d TextDecoder, lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, cleanLine(d.Decode(l)))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return slices.Clip(out)
}

// cleanLine turns control characters into spaces, trims the ends, and
// cuts a line at maxItemTextRune characters.
func cleanLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxItemTextRune {
		s = string([]rune(s)[:maxItemTextRune])
	}
	return s
}
