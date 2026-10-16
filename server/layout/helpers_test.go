//go:build procedural

package layout_test

import (
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/fields"
	fieldscore "github.com/lxsmnsyc/wathena/server/fields/core"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/layout/core"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// records reads one of core's record files.
func records(t testing.TB, b []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := yaml.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// coreInputs are a world's inputs with core's records, from a secret.
func coreInputs(t testing.TB, secret string) layout.Inputs {
	t.Helper()
	g, ok := gen.Latest()
	if !ok {
		t.Fatal("no generator")
	}
	root, err := rng.NewRoot([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	in := layout.Inputs{Generator: g, Public: gen.PublicBranch(root)}
	b, _ := fieldscore.Files.ReadFile("data/town_templates.yml")
	for _, r := range records(t, b) {
		name, _ := r["name"].(string)
		delete(r, "name")
		tp, err := fields.ParseTemplate(r)
		if err != nil {
			t.Fatal(err)
		}
		in.Towns = append(in.Towns, layout.TownRecord{Name: name, Template: tp})
	}
	b, _ = fieldscore.Files.ReadFile("data/dungeon_styles.yml")
	for _, r := range records(t, b) {
		name, _ := r["name"].(string)
		delete(r, "name")
		st, err := fields.ParseStyle(r)
		if err != nil {
			t.Fatal(err)
		}
		in.Styles = append(in.Styles, layout.StyleRecord{Name: name, Style: st})
	}
	in.Naming = coreNaming(t)
	words, err := chat.CoreRecords(chat.FilterFile)
	if err != nil {
		t.Fatal(err)
	}
	var list []chat.Word
	for _, w := range words {
		cw, err := chat.BuildWord(w.Name, w.Fields)
		if err != nil {
			t.Fatal(err)
		}
		list = append(list, cw)
	}
	filter := chat.NewFilter(list)
	in.Blocked = func(name string) bool { return filter.CheckName(name) != nil }
	return in
}

// coreNaming is core's naming styles.
func coreNaming(t testing.TB) *layout.Naming {
	t.Helper()
	b, err := core.Files.ReadFile("data/naming_styles.yml")
	if err != nil {
		t.Fatal(err)
	}
	var styles []layout.NamingStyle
	for _, r := range records(t, b) {
		name, _ := r["name"].(string)
		delete(r, "name")
		s, err := layout.ParseNamingStyle(name, r)
		if err != nil {
			t.Fatalf("naming style %s: %v", name, err)
		}
		styles = append(styles, s)
	}
	return layout.NewNaming(styles)
}
