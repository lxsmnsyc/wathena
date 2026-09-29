package grfimport

import "testing"

func TestTextureNames(t *testing.T) {
	got := TextureNames([]string{
		`data\texture\프론테라\Ground01.BMP`,
		`data\texture\effect\fire.tga`,
		`data\texture\effect\fire.bmp`,
		`data\texture\effect\glow.png`,
		`data\texture\effect\glow.tga`,
		`data\texture\유저인터페이스\item\빨간포션.bmp`,
		`data\texture\유저인터페이스\collection\빨간포션.bmp`,
		`data\texture\유저인터페이스\cardbmp\포링카드.bmp`,
		`data\texture\유저인터페이스\illust\kafra.bmp`,
		`data\texture\유저인터페이스\login_interface\bg.bmp`,
		`data\texture\유저인터페이스\item.bmp`,
		`data\texture\effect\str.str`,
		`data\sprite\x.bmp`,
	})
	want := map[string]string{
		`data\texture\프론테라\Ground01.BMP`:              "texture/프론테라/ground01",
		`data\texture\effect\fire.tga`:                "texture/effect/fire~tga",
		`data\texture\effect\fire.bmp`:                "texture/effect/fire",
		`data\texture\effect\glow.png`:                "texture/effect/glow~png",
		`data\texture\effect\glow.tga`:                "texture/effect/glow",
		`data\texture\유저인터페이스\item\빨간포션.bmp`:          "icon/item/빨간포션",
		`data\texture\유저인터페이스\collection\빨간포션.bmp`:    "image/item/빨간포션",
		`data\texture\유저인터페이스\cardbmp\포링카드.bmp`:       "image/card/포링카드",
		`data\texture\유저인터페이스\illust\kafra.bmp`:       "image/illust/kafra",
		`data\texture\유저인터페이스\login_interface\bg.bmp`: "ui/login_interface/bg",
		`data\texture\유저인터페이스\item.bmp`:               "ui/item",
	}
	if len(got) != len(want) {
		t.Errorf("got %d names, want %d: %v", len(got), len(want), got)
	}
	for p, w := range want {
		if got[p] != w {
			t.Errorf("%s: %q, want %q", p, got[p], w)
		}
	}
}

func TestTextureResolver(t *testing.T) {
	r := TextureResolver(TextureNames([]string{`data\texture\effect\Fire.tga`, `data\texture\effect\fire.bmp`}))
	for in, want := range map[string]string{
		`effect\FIRE.BMP`: "texture/effect/fire",
		`effect\fire.tga`: "texture/effect/fire~tga",
		`프론테라\gone.bmp`:   "texture/프론테라/gone",
		`..\..\etc\x.bmp`: "texture/missing",
	} {
		if got := r(in); got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
	names := ModelNames([]string{`data\model\a\Tree.rsm`, `data\model\a\tree.rsm2`, `data\model\b.rsm2`, `data\model\c.gr2`})
	mr := ModelResolver(names)
	for in, want := range map[string]string{
		`a\TREE.rsm`:  "model/a/tree",
		`a\tree.rsm2`: "model/a/tree~rsm2",
		`b.rsm2`:      "model/b",
		`c.gr2`:       "model/missing",
		`gone.rsm`:    "model/gone",
	} {
		if got := mr(in); got != want {
			t.Errorf("model %s: %q, want %q", in, got, want)
		}
	}
	if got, ok := MapName(`data\Prontera.rsw`); !ok || got != "map/prontera" || LightmapName(got) != "lightmap/prontera" {
		t.Errorf("MapName = %q, %v", got, ok)
	}
	if _, ok := MapName(`data\sub\x.rsw`); ok {
		t.Error("MapName accepted a subfolder")
	}
	if SoundResolver(`effect\hit.wav`) != "sound/effect/hit" || SoundResolver("") != "sound/missing" {
		t.Error("SoundResolver")
	}
}

func TestEffectName(t *testing.T) {
	for in, want := range map[string][2]string{
		`data\texture\effect\FireWall.str`:        {"effect/firewall", "effect"},
		`data\texture\effect\storm\storm_hit.str`: {"effect/storm/storm_hit", "effect/storm"},
		`data\texture\20th_firework\fiw.str`:      {"effect/texture/20th_firework/fiw", "20th_firework"},
		`DATA\TEXTURE\effect\x.str`:               {"effect/x", "effect"},
	} {
		name, folder, ok := EffectName(in)
		if !ok || name != want[0] || folder != want[1] {
			t.Errorf("%s: %q %q %v", in, name, folder, ok)
		}
	}
	if _, _, ok := EffectName(`data\sprite\x.str`); ok {
		t.Error("EffectName accepted a file outside data\\texture")
	}
}
