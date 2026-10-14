package mod

import "testing"

func TestArtFields(t *testing.T) {
	if n, ok := ConventionName("example.lantern:Paper_Lantern", "icon/item"); !ok || n != "mod/example.lantern/icon/item/Paper_Lantern" {
		t.Errorf("item: %q %v", n, ok)
	}
	if n, ok := ConventionName("example.lantern:Lamplighter#prontera", "sprite/npc"); !ok || n != "mod/example.lantern/sprite/npc/Lamplighter" {
		t.Errorf("npc: %q %v", n, ok)
	}
	for _, record := range []string{"Red_Potion", "example.lantern:a/b"} {
		if n, ok := ConventionName(record, "icon/item"); ok {
			t.Errorf("%s has the name %q", record, n)
		}
	}
	if err := CheckArtName("mod/example.lantern/icon/item/X", "texture"); err != nil {
		t.Error(err)
	}
	for _, bad := range [][2]string{{"mod/example.lantern/icon/item/X", "sprite"}, {"Icon/item/501", "texture"}, {"placeholder/sprite", "sprite"}} {
		if CheckArtName(bad[0], bad[1]) == nil {
			t.Errorf("%s accepted as a %s", bad[0], bad[1])
		}
	}
	sounds, err := ArtTable(map[string]any{"attack": "sound/a", "die": "mod/x/sound/d"}, []string{"attack", "die"}, "sound")
	if err != nil || sounds["die"] != "mod/x/sound/d" {
		t.Errorf("%v %v", sounds, err)
	}
	if _, err := ArtTable(map[string]any{"jump": "sound/a"}, []string{"attack"}, "sound"); err == nil {
		t.Error("an unknown key was accepted")
	}
	if _, err := ArtList([]any{"effect/a", "texture/b"}, "effect"); err == nil {
		t.Error("a texture was accepted as an effect")
	}
	if !IsArtName("sprite/mob/1002") || IsArtName("PORING") {
		t.Error("IsArtName")
	}
}
