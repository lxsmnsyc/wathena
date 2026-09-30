package grfimport

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/grf"
)

// Item assets are named by item ID: icon/item/<id>, image/item/<id>,
// image/card/<id>, and sprite/item/<id>. The client's own tables give each
// ID a resource name, which names its files:
//
//	iteminfo.lub, or idnum2itemresnametable.txt for IDs it lacks:
//	  data\texture\유저인터페이스\item\<name>.bmp        icon/item/<id>
//	  data\texture\유저인터페이스\collection\<name>.bmp  image/item/<id>
//	  data\sprite\아이템\<name>.spr and .act             sprite/item/<id>
//	num2cardillustnametable.txt:
//	  data\texture\유저인터페이스\cardbmp\<name>.bmp     image/card/<id>
//
// So game data and the web client refer to items by ID alone, and the
// repository needs no table of Gravity's file names.

const (
	itemInfoPath     = `data\luafiles514\lua files\datainfo\iteminfo.lub`
	itemResNamePath  = `data\idnum2itemresnametable.txt`
	cardIllustPath   = `data\num2cardillustnametable.txt`
	maxItemID        = 1 << 24
	itemTextureDir   = `data\texture\유저인터페이스\`
	itemSpriteFolder = `data\sprite\아이템\`
)

// ItemTexture is an item image and the name it gets.
type ItemTexture struct {
	Name   string
	Source grf.Source
}

// ItemSprite is an item's dropped sprite and the name it gets.
type ItemSprite struct {
	Name     string
	Spr, Act grf.Source
}

// ItemAssets lists the item images and sprites a client has, named by
// item ID. Items whose files are missing are left out.
func ItemAssets(c *grf.Client) ([]ItemTexture, []ItemSprite, error) {
	res, err := itemResources(c)
	if err != nil {
		return nil, nil, err
	}
	cards, err := readIDTable(c, cardIllustPath)
	if err != nil {
		return nil, nil, err
	}
	var textures []ItemTexture
	var sprites []ItemSprite
	image := func(folder, file, name string) {
		for _, ext := range textureFormats {
			if s, ok := c.Lookup(itemTextureDir + folder + `\` + file + ext); ok {
				textures = append(textures, ItemTexture{Name: name, Source: s})
				return
			}
		}
	}
	for id, r := range res {
		n := strconv.Itoa(id)
		image("item", r, "icon/item/"+n)
		image("collection", r, "image/item/"+n)
		spr, ok1 := c.Lookup(itemSpriteFolder + r + ".spr")
		act, ok2 := c.Lookup(itemSpriteFolder + r + ".act")
		if ok1 && ok2 {
			sprites = append(sprites, ItemSprite{Name: "sprite/item/" + n, Spr: spr, Act: act})
		}
	}
	for id, r := range cards {
		image("cardbmp", r, "image/card/"+strconv.Itoa(id))
	}
	return textures, sprites, nil
}

// itemResources reads each item's identified resource name.
func itemResources(c *grf.Client) (map[int]string, error) {
	out, err := readIDTable(c, itemResNamePath)
	if err != nil {
		return nil, err
	}
	s, ok := c.Lookup(itemInfoPath)
	if !ok {
		return out, nil
	}
	b, err := c.ReadFile(s)
	if err != nil {
		return nil, err
	}
	g := map[string]any{}
	if err := evalLub(b, g); err != nil {
		return nil, fmt.Errorf("%s: %w", itemInfoPath, err)
	}
	// iteminfo.lub is newer, so it wins over the text table.
	tbl, _ := g["tbl"].(*LubTable)
	if tbl == nil {
		return out, nil
	}
	for k, v := range tbl.M {
		id, ok := k.(float64)
		e, ok2 := v.(*LubTable)
		if !ok || !ok2 || id < 0 || id >= maxItemID || id != float64(int(id)) {
			continue
		}
		if r, ok := e.M["identifiedResourceName"].(string); ok && r != "" {
			out[int(id)] = decodeCP949(r)
		}
	}
	return out, nil
}

// readIDTable reads a client text table of "id#name#" lines. Lines that
// do not fit, such as comments, are skipped. A missing table is empty.
func readIDTable(c *grf.Client, path string) (map[int]string, error) {
	out := map[int]string{}
	s, ok := c.Lookup(path)
	if !ok {
		return out, nil
	}
	b, err := c.ReadFile(s)
	if err != nil {
		return nil, err
	}
	for line := range strings.SplitSeq(decodeCP949(string(b)), "\n") {
		parts := strings.Split(strings.TrimSpace(line), "#")
		if len(parts) < 2 {
			continue
		}
		id, err := strconv.Atoi(parts[0])
		name := strings.TrimSpace(parts[1])
		if err != nil || id < 0 || id >= maxItemID || name == "" || strings.ContainsAny(name, `\/`) {
			continue
		}
		out[id] = name
	}
	return out, nil
}
