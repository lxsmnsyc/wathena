package store_test

import (
	"context"
	"testing"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/db"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// TestVariablesAndSavePoint saves a character's variables, its account's,
// its save point and its job change fields with the character, and reads
// them back: changed
// values, deleted ones, and an account variable another character of
// the account sees.
func TestVariablesAndSavePoint(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	id, err := s.DevCharacter(ctx, newCharacter("Vera"))
	if err != nil {
		t.Fatal(err)
	}
	c, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if c.SaveMap != "" || len(c.Variables) != 0 {
		t.Fatalf("a new character has a save point %q and variables %v", c.SaveMap, c.Variables)
	}
	quest := character.Variable{Mod: "example.town", Name: "quest_step"}
	seen := character.Variable{Mod: "example.town", Name: "seen", Account: true}
	bag := character.Variable{Mod: "core", Name: "bag"}
	next := *c
	next.SaveMap, next.SaveX, next.SaveY = "map/prontera", 157, 327
	// The fields of job changes (migration 00012).
	next.Job, next.JobChangeLevel, next.RebirthJob, next.Options = 4008, 44, 7, 0x20
	// The record names of companions and mounts (migration 00014).
	next.Companion, next.Mount = "example.familiars:Owl", "peco_peco"
	next.Variables = map[character.Variable]string{quest: "2", seen: "true", bag: `{"items":[1,2]}`}
	sess.Save(&next, false)
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	// A change, a deletion, and an unchanged value, in the same session.
	third := next
	third.Variables = map[character.Variable]string{quest: "3", seen: "true"}
	sess.Save(&third, true)
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(&third) {
		t.Fatalf("loaded %+v\nwant %+v", got, &third)
	}
	// Another character of the account sees the account variable only.
	q := db.New(admin)
	other, err := q.CreateCharacter(ctx, db.CreateCharacterParams{
		AccountID: got.AccountID, Slot: 1, Name: "Vera2", BaseLevel: 1, JobLevel: 1,
		Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1, Hp: 40, Sp: 11, Map: "map/prontera", InventorySize: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	c2, _, err := s.Load(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if len(c2.Variables) != 1 || c2.Variables[seen] != "true" {
		t.Errorf("the other character's variables %v", c2.Variables)
	}
}

func TestServerVariables(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	vs := store.NewServerVariables(game)
	sv := variables.NewServer(vs, nil)
	if err := sv.Load(ctx); err != nil {
		t.Fatal(err)
	}
	kills := variables.Key{Mod: "example.boss", Name: "kills"}
	name := variables.Key{Mod: "example.boss", Name: "last"}
	sv.Set(kills, "41")
	if _, err := sv.Update(kills, func(old string) (string, error) { return "42", nil }); err != nil {
		t.Fatal(err)
	}
	sv.Set(name, `"Ann"`)
	sv.Set(name, "")
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		sv.Run(runCtx)
	}()
	cancel()
	<-done
	again := variables.NewServer(vs, nil)
	if err := again.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if v, ok := again.Get(kills); v != "42" || !ok {
		t.Errorf("kills %q", v)
	}
	if _, ok := again.Get(name); ok {
		t.Error("a deleted variable came back")
	}
	// The account role cannot read mods' variables.
	account := storetest.As(t, admin, "wathena_account")
	if _, err := account.Exec(ctx, "SELECT * FROM mod_character_variables"); !isCode(err, "42501") {
		t.Errorf("the account role read character variables: %v", err)
	}
}
