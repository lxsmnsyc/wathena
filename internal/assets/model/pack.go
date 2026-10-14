package model

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
)

// RenameTextures checks a GLB file from a pack against the profile, and
// passes each texture name of its image URIs through rename, such as to
// give a mod's own textures their mod/<id>/ prefix. Images must already
// point to textures by name, with asset: URIs: the profile allows no
// embedded images. The result is checked again.
func RenameTextures(glb []byte, rename func(string) string) ([]byte, error) {
	if _, err := Inspect(glb); err != nil {
		return nil, err
	}
	n := int(binary.LittleEndian.Uint32(glb[12:]))
	dec := json.NewDecoder(bytes.NewReader(glb[20 : 20+n]))
	// Numbers stay as written, so nothing but the URIs changes.
	dec.UseNumber()
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	images, _ := doc["images"].([]any)
	changed := false
	for _, im := range images {
		m, ok := im.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("model: image is not an object")
		}
		uri, _ := m["uri"].(string)
		name, ok := strings.CutPrefix(unescapeURI(uri), "asset:")
		if !ok {
			return nil, fmt.Errorf("model: image uri %q is not an asset name", uri)
		}
		if to := rename(name); to != name {
			m["uri"] = TextureURI(to)
			changed = true
		}
	}
	if !changed {
		return glb, nil
	}
	js, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	for len(js)%4 != 0 {
		js = append(js, ' ')
	}
	rest := glb[20+n:]
	var out bytes.Buffer
	out.WriteString("glTF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(2))
	_ = binary.Write(&out, binary.LittleEndian, uint32(12+8+len(js)+len(rest))) // #nosec G115 -- at most MaxBytes plus the new names.
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(js)))                // #nosec G115 -- as above.
	out.WriteString("JSON")
	out.Write(js)
	out.Write(rest)
	data := out.Bytes()
	if _, err := Inspect(data); err != nil {
		return nil, err
	}
	return data, nil
}
