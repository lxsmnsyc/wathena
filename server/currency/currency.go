// Package currency holds the currencies of data/currencies.yml (proposal
// 0002, review decision 30): rAthena's cash points and Kafra points, and
// the currencies mods add, such as a pointshop's variable.
//
// A currency belongs to an account, which its characters share, or to a
// character. Amounts are whole numbers from 0 to the record's max, at
// most MaxAmount. They move through server/transfer, as zeny does, and
// their log lines name the currency.
//
// Core's records are files of this package (core/), which the core mod
// also carries. server/gamedata merges the mods' records over them.
package currency

import (
	"embed"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// CoreFiles are core's data/currencies.yml, in core/.
//
//go:embed core
var CoreFiles embed.FS

// File is the data file of currencies.
const File = "data/currencies.yml"

// MaxAmount is the most any currency holds: MAX_ZENY, as pc_getcash caps
// cash points. The logs and the database hold amounts up to it.
const MaxAmount = 2_147_483_647

// Core's records.
const (
	// Cash is rAthena's #CASHPOINTS.
	Cash uint32 = 1
	// Kafra is rAthena's #KAFRAPOINTS.
	Kafra uint32 = 2
)

// Record is one currency.
type Record struct {
	// ID is the currency's ID in the database and the logs. Core's come
	// from its file, from 1. Mod records get IDs from 10,000.
	ID uint32
	// Name is the full name: a plain name for core's, such as cash, and
	// mod:name for a mod's, such as example.arena:tokens.
	Name string
	// Mod is the mod that added the record.
	Mod         string
	DisplayName string
	// Account is set for a currency the account owns, which its
	// characters share. A character's own currency has it unset.
	Account bool
	// Max is the most a player holds, 1 to MaxAmount.
	Max int64
}

// Fields are the fields of a record.
var Fields = []string{"display_name", "owner", "max"}

// Build makes a record from its fields. id is the record's ID.
func Build(name, mod string, id uint32, fields map[string]any) (*Record, error) {
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		if k != "id" && !slices.Contains(Fields, k) {
			return nil, fmt.Errorf("%q is not a field; the fields are %s", k, strings.Join(Fields, ", "))
		}
	}
	r := &Record{ID: id, Name: name, Mod: mod, Account: true, Max: MaxAmount}
	_, local, ok := strings.Cut(name, ":")
	if !ok {
		local = name
	}
	r.DisplayName = local
	if v, ok := fields["display_name"]; ok {
		s, isText := v.(string)
		if !isText || s == "" || utf8.RuneCountInString(s) > 24 {
			return nil, fmt.Errorf("display_name is 1 to 24 characters")
		}
		r.DisplayName = s
	}
	switch fields["owner"] {
	case "account", nil:
	case "character":
		r.Account = false
	default:
		return nil, fmt.Errorf("owner is account or character")
	}
	if v, ok := fields["max"]; ok {
		n, isInt := v.(int)
		if !isInt || n < 1 || n > MaxAmount {
			return nil, fmt.Errorf("max is a whole number from 1 to %d", MaxAmount)
		}
		r.Max = int64(n)
	}
	return r, nil
}

// Table holds the currencies by ID and by name.
type Table struct {
	list   []*Record
	byID   map[uint32]*Record
	byName map[string]*Record
}

// NewTable makes a table of records.
func NewTable(list []*Record) *Table {
	t := &Table{byID: map[uint32]*Record{}, byName: map[string]*Record{}}
	for _, r := range list {
		t.list = append(t.list, r)
		t.byID[r.ID] = r
		t.byName[r.Name] = r
	}
	slices.SortFunc(t.list, func(a, b *Record) int { return int(a.ID) - int(b.ID) })
	return t
}

// All are the records, by ID.
func (t *Table) All() []*Record {
	if t == nil {
		return nil
	}
	return t.list
}

// ByID finds a record, or nil.
func (t *Table) ByID(id uint32) *Record {
	if t == nil {
		return nil
	}
	return t.byID[id]
}

// ByName finds a record by its full name, or nil.
func (t *Table) ByName(name string) *Record {
	if t == nil {
		return nil
	}
	return t.byName[name]
}

// Resolve finds a record by a name a mod gave: a full name, or a plain
// name, which is the mod's own record first, then core's.
func (t *Table) Resolve(name, mod string) *Record {
	if t == nil {
		return nil
	}
	if strings.Contains(name, ":") {
		return t.byName[name]
	}
	if mod != "" && mod != "core" {
		if r := t.byName[mod+":"+name]; r != nil {
			return r
		}
	}
	return t.byName[name]
}

// Core returns core's records alone, for zones that run without mods,
// such as in tests.
func Core() *Table {
	data, err := CoreFiles.ReadFile("core/" + File)
	if err != nil {
		panic(err)
	}
	var list []map[string]any
	if err := yaml.Unmarshal(data, &list); err != nil {
		panic(fmt.Errorf("currency: core %s: %w", File, err))
	}
	var recs []*Record
	for _, m := range list {
		name, _ := m["name"].(string)
		id, _ := m["id"].(int)
		delete(m, "name")
		r, err := Build(name, "core", uint32(id), m) // #nosec G115 -- core's small IDs.
		if err != nil {
			panic(fmt.Errorf("currency: core %s: %w", name, err))
		}
		recs = append(recs, r)
	}
	return NewTable(recs)
}

// Balances are a holder's currencies, by ID. A currency at 0 is not in
// the map.
type Balances map[uint32]int64

// Clone copies the balances.
func (b Balances) Clone() Balances {
	if b == nil {
		return nil
	}
	return maps.Clone(b)
}
