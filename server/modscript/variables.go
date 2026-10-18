package modscript

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// Saved variables (proposal 0002, "Variables"). A declaration names a
// scope, a type and a name. Values are kept as JSON: in the zone for a
// player's variables, and in Options.Server for server variables.
//
// Every type has a default, which a variable has until it is set and
// after it is set back: 0, false, "", or nil for a table. A variable at
// its default has no stored value.

// VariableKey names one variable of a player.
type VariableKey struct {
	Mod, Name string
	// Scope is character, character_temporary or account.
	Scope string
}

// Limits of variables.
const (
	// MaxVariableBytes is the most a value may take as JSON.
	MaxVariableBytes = 65536
	// MaxStringVariable is the longest string value, in bytes.
	MaxStringVariable = 4096
	// MaxModVariables is the most variables one mod may declare, over
	// every scope. Declaring a variable again does not count.
	MaxModVariables = 256
	// MaxModPlayerBytes is the most bytes of JSON one mod's variables may
	// hold on one player: its character and account variables together,
	// and its temporary ones apart.
	MaxModPlayerBytes = 1 << 20
)

var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// variableScopes are the scopes of wa.variables, with whether a value
// needs a player.
var variableScopes = map[string]bool{"character": true, "character_temporary": true, "account": true, "server": false}

type variable struct {
	mod, scope, kind, name string
}

func (v *variable) what(method string) string {
	return fmt.Sprintf("variable %s:%s %s", v.scope, v.name, method)
}

// variablesDeclare declares a variable. The running mod owns it.
func (r *Runtime) variablesDeclare(c *script.Call) error {
	scope, kind, name := c.String(0), c.String(1), c.String(2)
	what := "variables." + scope + "_" + kind
	if scope == "instance" {
		return fmt.Errorf("%s: instances do not exist yet", what)
	}
	if _, ok := variableScopes[scope]; !ok {
		return fmt.Errorf("variables: there is no scope %q", scope)
	}
	if err := r.need(what, "storage.mod"); err != nil {
		return err
	}
	if !variableName.MatchString(name) {
		return fmt.Errorf("%s: %q is not a variable name: 1 to 64 letters, digits and _, not starting with a digit", what, name)
	}
	mod := r.actor()
	if mod == "" {
		return fmt.Errorf("%s needs a mod: declare it at the top of a mod's file", what)
	}
	key := mod + " " + scope + " " + name
	if prev, ok := r.declared[key]; ok {
		if prev.kind != kind {
			return fmt.Errorf("%s: %s already declared %s as a %s variable", what, mod, name, prev.kind)
		}
		// The same declaration again, such as in a handler: the same
		// handle.
		c.Return(script.Int(prev.id))
		return nil
	}
	if r.modVariables[mod] >= MaxModVariables {
		return fmt.Errorf("%s: %s has declared %d variables, the most a mod may", what, mod, MaxModVariables)
	}
	r.modVariables[mod]++
	r.variables = append(r.variables, &variable{mod: mod, scope: scope, kind: kind, name: name})
	id := int64(len(r.variables) - 1)
	r.declared[key] = declared{kind: kind, id: id}
	c.Return(script.Int(id))
	return nil
}

// declared is a declaration: its type and its handle.
type declared struct {
	kind string
	id   int64
}

func (r *Runtime) variableOf(c *script.Call) (*variable, error) {
	id, ok := c.Arg(0).AsInt()
	if !ok || id < 0 || id >= int64(len(r.variables)) {
		return nil, errors.New("not a variable: call the method with a colon, such as quest_step:get(player)")
	}
	return r.variables[id], nil
}

// read returns a variable's value as JSON, or "" at its default.
func (r *Runtime) readVariable(v *variable, c *script.Call, method string) (string, int64, error) {
	if v.scope == "server" {
		if r.opts.Server == nil {
			return "", 0, fmt.Errorf("%s: server variables are not set up here", v.what(method))
		}
		s, _ := r.opts.Server.Get(variables.Key{Mod: v.mod, Name: v.name})
		return s, 0, nil
	}
	player, ok := c.Arg(1).AsInt()
	if !ok {
		return "", 0, fmt.Errorf("%s: want a player, such as quest_step:%s(player)", v.what(method), method)
	}
	h, err := r.npcHost()
	if err != nil {
		return "", 0, err
	}
	s, _, err := h.Variable(player, VariableKey{Mod: v.mod, Name: v.name, Scope: v.scope})
	return s, player, err
}

func (r *Runtime) variableGet(c *script.Call) error {
	v, err := r.variableOf(c)
	if err != nil {
		return err
	}
	s, _, err := r.readVariable(v, c, "get")
	if err != nil {
		return err
	}
	val, err := decodeVariable(v.kind, s)
	if err != nil {
		return fmt.Errorf("%s: %w", v.what("get"), err)
	}
	c.Return(val)
	return nil
}

func (r *Runtime) writeVariable(v *variable, player int64, value string) error {
	if v.scope == "server" {
		return r.opts.Server.Set(variables.Key{Mod: v.mod, Name: v.name}, value)
	}
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	return h.SetVariable(player, VariableKey{Mod: v.mod, Name: v.name, Scope: v.scope}, value)
}

func (r *Runtime) variableSet(c *script.Call) error {
	v, err := r.variableOf(c)
	if err != nil {
		return err
	}
	if err := r.mutating(v.what("set")); err != nil {
		return err
	}
	_, player, err := r.readVariable(v, c, "set")
	if err != nil {
		return err
	}
	s, err := encodeVariable(v.kind, c.Arg(2))
	if err != nil {
		return fmt.Errorf("%s: %w", v.what("set"), err)
	}
	return r.writeVariable(v, player, s)
}

func (r *Runtime) variableAdd(c *script.Call) error {
	v, err := r.variableOf(c)
	if err != nil {
		return err
	}
	if err := r.mutating(v.what("add")); err != nil {
		return err
	}
	if v.kind != "integer" {
		return fmt.Errorf("%s: only integer variables have add", v.what("add"))
	}
	amount, ok := c.Arg(2).AsInt()
	if !ok {
		return fmt.Errorf("%s: the amount must be an integer", v.what("add"))
	}
	add := func(old string) (string, int64, error) {
		n := int64(0)
		if old != "" {
			var err error
			if n, err = strconv.ParseInt(old, 10, 64); err != nil {
				n = 0
			}
		}
		if amount > 0 && n > math.MaxInt64-amount || amount < 0 && n < math.MinInt64-amount {
			return "", 0, fmt.Errorf("%s: the value would overflow", v.what("add"))
		}
		n += amount
		if n == 0 {
			return "", 0, nil
		}
		return strconv.FormatInt(n, 10), n, nil
	}
	if v.scope == "server" {
		if r.opts.Server == nil {
			return fmt.Errorf("%s: server variables are not set up here", v.what("add"))
		}
		var out int64
		_, err := r.opts.Server.Update(variables.Key{Mod: v.mod, Name: v.name}, func(old string) (string, error) {
			s, n, err := add(old)
			out = n
			return s, err
		})
		if err != nil {
			return err
		}
		c.Return(script.Int(out))
		return nil
	}
	old, player, err := r.readVariable(v, c, "add")
	if err != nil {
		return err
	}
	s, n, err := add(old)
	if err != nil {
		return err
	}
	if err := r.writeVariable(v, player, s); err != nil {
		return err
	}
	c.Return(script.Int(n))
	return nil
}

// encodeVariable turns a value into its JSON, or "" for the default.
func encodeVariable(kind string, v script.Value) (string, error) {
	if v.IsNil() {
		return "", nil
	}
	switch kind {
	case "integer":
		n, ok := v.AsInt()
		if !ok {
			return "", fmt.Errorf("want an integer, got %s", v.Kind())
		}
		if n == 0 {
			return "", nil
		}
		return strconv.FormatInt(n, 10), nil
	case "boolean":
		b, ok := v.AsBool()
		if !ok {
			return "", fmt.Errorf("want a boolean, got %s", v.Kind())
		}
		if !b {
			return "", nil
		}
		return "true", nil
	case "string":
		s, ok := v.AsString()
		if !ok {
			return "", fmt.Errorf("want a string, got %s", v.Kind())
		}
		if len(s) > MaxStringVariable || !utf8.ValidString(s) {
			return "", fmt.Errorf("a string is at most %d bytes of UTF-8", MaxStringVariable)
		}
		if s == "" {
			return "", nil
		}
		b, err := json.Marshal(s)
		return string(b), err
	case "table":
		if _, ok := v.AsTable(); !ok {
			return "", fmt.Errorf("want a table, got %s", v.Kind())
		}
		var buf bytes.Buffer
		if err := writeJSON(&buf, v, 0); err != nil {
			return "", err
		}
		if buf.Len() > MaxVariableBytes {
			return "", fmt.Errorf("the table is larger than %d bytes as JSON", MaxVariableBytes)
		}
		return buf.String(), nil
	}
	return "", fmt.Errorf("unknown type %s", kind)
}

// writeJSON writes plain data: numbers, booleans, strings, and tables
// whose keys are all strings, or 1 to n.
func writeJSON(b *bytes.Buffer, v script.Value, depth int) error {
	if depth > 15 {
		return errors.New("the table is nested too deeply")
	}
	switch v.Kind() {
	case script.KindBool:
		x, _ := v.AsBool()
		b.WriteString(strconv.FormatBool(x))
	case script.KindInt:
		n, _ := v.AsInt()
		b.WriteString(strconv.FormatInt(n, 10))
	case script.KindFloat:
		f, _ := v.AsFloat()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return errors.New("a table cannot hold NaN or infinity")
		}
		b.WriteString(strconv.FormatFloat(f, 'g', -1, 64))
	case script.KindString:
		s, _ := v.AsString()
		if !utf8.ValidString(s) {
			return errors.New("a string in the table is not UTF-8")
		}
		enc, _ := json.Marshal(s)
		b.Write(enc)
	case script.KindTable:
		t, _ := v.AsTable()
		return writeTable(b, t, depth)
	default:
		return fmt.Errorf("a table can hold numbers, booleans, strings and tables, not %s", v.Kind())
	}
	return nil
}

func writeTable(b *bytes.Buffer, t *script.Table, depth int) error {
	n := len(t.Fields)
	array := n > 0
	seen := make([]bool, n)
	for _, f := range t.Fields {
		k, ok := f.Key.AsInt()
		if f.Key.Kind() != script.KindInt || !ok || k < 1 || k > int64(n) || seen[k-1] {
			array = false
			break
		}
		seen[k-1] = true
	}
	if array || n == 0 {
		vals := make([]script.Value, n)
		for _, f := range t.Fields {
			k, _ := f.Key.AsInt()
			vals[k-1] = f.Value
		}
		b.WriteByte('[')
		for i, v := range vals {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeJSON(b, v, depth+1); err != nil {
				return err
			}
		}
		b.WriteByte(']')
		return nil
	}
	fields := slices.Clone(t.Fields)
	for _, f := range fields {
		if f.Key.Kind() != script.KindString {
			return errors.New("a table's keys are all strings, or 1 to n as a list")
		}
	}
	slices.SortFunc(fields, func(a, c script.Field) int {
		x, _ := a.Key.AsString()
		y, _ := c.Key.AsString()
		return compareStrings(x, y)
	})
	b.WriteByte('{')
	for i, f := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := f.Key.AsString()
		enc, _ := json.Marshal(k)
		b.Write(enc)
		b.WriteByte(':')
		if err := writeJSON(b, f.Value, depth+1); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// decodeVariable turns stored JSON into a value. A value of another type
// than the declaration's, such as after a mod changed it, reads as the
// default.
func decodeVariable(kind, s string) (script.Value, error) {
	if s == "" {
		return defaultVariable(kind), nil
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(s)))
	dec.UseNumber()
	var x any
	if err := dec.Decode(&x); err != nil {
		return script.Nil, err
	}
	v := fromJSON(x)
	switch kind {
	case "integer":
		if _, ok := v.AsInt(); ok && v.Kind() == script.KindInt {
			return v, nil
		}
	case "boolean":
		if v.Kind() == script.KindBool {
			return v, nil
		}
	case "string":
		if v.Kind() == script.KindString {
			return v, nil
		}
	case "table":
		if v.Kind() == script.KindTable {
			return v, nil
		}
	}
	return defaultVariable(kind), nil
}

func defaultVariable(kind string) script.Value {
	switch kind {
	case "integer":
		return script.Int(0)
	case "boolean":
		return script.Bool(false)
	case "string":
		return script.String("")
	}
	return script.Nil
}

func fromJSON(x any) script.Value {
	switch x := x.(type) {
	case bool:
		return script.Bool(x)
	case string:
		return script.String(x)
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return script.Int(n)
		}
		f, _ := x.Float64()
		return script.Float(f)
	case []any:
		vals := make([]script.Value, len(x))
		for i, e := range x {
			vals[i] = fromJSON(e)
		}
		return script.List(vals...)
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		fields := make([]script.Field, 0, len(x))
		for _, k := range keys {
			fields = append(fields, script.KV(k, fromJSON(x[k])))
		}
		return script.NewTable(fields...)
	}
	return script.Nil
}
