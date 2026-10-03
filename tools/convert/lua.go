package convert

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Converted expressions are AST nodes after substitution: variables are
// replaced by their values, constants by Const, and game reads by Read.
// An expression is static when it holds only Num, Str and Const.

// Const is a resolved rAthena constant. Its name is kept for messages.
type Const struct {
	Name  string
	Value int64
}

// Read is a value the script reads from the game, such as the refine
// level. Lua is its code, and Bool marks a condition rather than a
// number. NonNeg marks values that are never negative, so integer
// division can use //.
type Read struct {
	Lua    string
	Bool   bool
	NonNeg bool
	// Job marks a job ID, so constants compared with it are written as
	// jobs.<name>.
	Job bool
	// Grade marks an enchant grade, so constants compared with it carry
	// their rAthena name.
	Grade bool
	// Weapon marks a weapon type, so constants compared with it are
	// written as weapon_types.<name>.
	Weapon bool
}

func (*Const) expr() {}
func (*Read) expr()  {}

// static returns the integer value of a static expression.
func static(e Expr) (int64, bool) {
	switch e := e.(type) {
	case *Num:
		return e.Value, true
	case *Const:
		return e.Value, true
	}
	return 0, false
}

func staticStr(e Expr) (string, bool) {
	s, ok := e.(*Str)
	if !ok {
		return "", false
	}
	return s.Value, true
}

// luaFile builds one generated Lua file: its requires, the records it
// looks up when it loads, and its body.
type luaFile struct {
	modules  map[string]bool
	records  []string
	byRecord map[string]string
	names    map[string]bool
	body     strings.Builder
	indent   int
}

func newLuaFile() *luaFile {
	return &luaFile{modules: map[string]bool{}, byRecord: map[string]string{}, names: map[string]bool{}}
}

// moduleOrder is the order of requires at the top of a file.
var moduleOrder = []string{
	"bonuses", "context", "events", "random", "timers", "integers", "items", "skills", "monsters",
	"races", "monster_groups", "elements", "sizes", "monster_classes", "weapon_types", "statuses",
	"effects", "headgear_effects", "jobs", "companions", "mounts", "schedules",
}

// need marks an API module as used and returns its local name.
func (f *luaFile) need(module string) string {
	f.modules[module] = true
	return module
}

// record returns the local that holds a record looked up by name, such
// as local RED_POTION = items.id "Red_Potion".
func (f *luaFile) record(lookup, name string) string {
	key := lookup + " " + name
	if l, ok := f.byRecord[key]; ok {
		return l
	}
	module := strings.SplitN(lookup, ".", 2)[0]
	f.need(module)
	local := constName(name)
	for i := 2; f.names[local] || luaKeywords[strings.ToLower(local)]; i++ {
		local = constName(name) + "_" + strconv.Itoa(i)
	}
	f.names[local] = true
	f.byRecord[key] = local
	f.records = append(f.records, fmt.Sprintf("local %s = %s %s", local, lookup, luaString(name)))
	return local
}

// constName makes a Lua name in capitals from a record name.
func constName(name string) string {
	var b strings.Builder
	for _, c := range strings.ToUpper(name) {
		if c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" || s[0] >= '0' && s[0] <= '9' {
		s = "N" + s
	}
	return s
}

var luaKeywords = map[string]bool{
	"and": true, "break": true, "do": true, "else": true, "elseif": true, "end": true, "false": true,
	"for": true, "function": true, "goto": true, "if": true, "in": true, "local": true, "nil": true,
	"not": true, "or": true, "repeat": true, "return": true, "then": true, "true": true, "until": true, "while": true,
}

func (f *luaFile) line(format string, args ...any) {
	f.body.WriteString(strings.Repeat("  ", f.indent))
	fmt.Fprintf(&f.body, format, args...)
	f.body.WriteByte('\n')
}

// String returns the file: a header comment, the requires, the records,
// and the body.
func (f *luaFile) String(header []string) string {
	var b strings.Builder
	for _, h := range header {
		b.WriteString("-- " + h + "\n")
	}
	// A module is required when the code uses it. Parts that did not
	// convert may have asked for modules that the file does not use.
	code := strings.Join(f.records, "\n") + "\n" + f.body.String()
	var mods []string
	for m := range f.modules {
		if regexp.MustCompile(`(^|[^\w.])` + m + `[.:]`).MatchString(code) {
			mods = append(mods, m)
		}
	}
	// Modules that moduleOrder does not list come last, by name.
	sort.Strings(mods)
	sort.SliceStable(mods, func(i, j int) bool { return indexOf(moduleOrder, mods[i]) < indexOf(moduleOrder, mods[j]) })
	for _, m := range mods {
		fmt.Fprintf(&b, "local %s = require \"wa.%s\"\n", m, m)
	}
	if len(f.records) > 0 {
		b.WriteByte('\n')
		for _, r := range f.records {
			b.WriteString(r + "\n")
		}
	}
	b.WriteByte('\n')
	b.WriteString(f.body.String())
	return b.String()
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return len(list) + 1
}

// luaString quotes a string for Lua.
func luaString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range []byte(s) {
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c < 32 || c == 127:
			fmt.Fprintf(&b, "\\%03d", c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}
