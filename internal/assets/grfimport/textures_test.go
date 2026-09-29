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
	if got, ok := ModelName(`data\model\프론테라\Fountain.rsm`); !ok || got != "model/프론테라/fountain" {
		t.Errorf("ModelName = %q, %v", got, ok)
	}
	if _, ok := ModelName(`data\model\x.rsm2`); ok {
		t.Error("ModelName accepted rsm2")
	}
}
