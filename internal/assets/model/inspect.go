package model

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
)

// Inspect checks that a GLB file follows the assets-v1 model profile and
// that its indexes and buffer ranges are in bounds. It returns the asset
// names of the textures it refers to.
func Inspect(glb []byte) ([]string, error) {
	if len(glb) > MaxBytes {
		return nil, fmt.Errorf("model: %d bytes is over the size limit", len(glb))
	}
	if len(glb) < 20 || string(glb[:4]) != "glTF" || binary.LittleEndian.Uint32(glb[4:]) != 2 || int(binary.LittleEndian.Uint32(glb[8:])) != len(glb) {
		return nil, fmt.Errorf("model: glb header")
	}
	g, err := readGLB(glb)
	if err != nil {
		return nil, err
	}
	// The profile leaves out skins, morph targets, cameras, lights, and
	// extensions other than KHR_materials_unlit.
	var raw map[string]json.RawMessage
	n := int(binary.LittleEndian.Uint32(glb[12:]))
	if err := json.Unmarshal(glb[20:20+n], &raw); err != nil {
		return nil, err
	}
	for _, key := range []string{"skins", "cameras"} {
		if _, ok := raw[key]; ok {
			return nil, fmt.Errorf("model: %s are not allowed", key)
		}
	}
	if used, ok := raw["extensionsUsed"]; ok {
		var list []string
		_ = json.Unmarshal(used, &list)
		for _, e := range list {
			if e != "KHR_materials_unlit" {
				return nil, fmt.Errorf("model: extension %s is not allowed", e)
			}
		}
	}
	d := &g.doc
	for i, v := range d.BufferViews {
		if v.Buffer != 0 || v.ByteOffset < 0 || v.ByteLength < 0 || v.ByteOffset+v.ByteLength > len(g.bin) {
			return nil, fmt.Errorf("model: buffer view %d is out of range", i)
		}
	}
	sizes := map[string]int{"SCALAR": 1, "VEC2": 2, "VEC3": 3, "VEC4": 4}
	for i, a := range d.Accessors {
		comp := map[int]int{glFloat: 4, glUnsignedShort: 2, glUnsignedInt: 4}[a.ComponentType]
		if a.BufferView < 0 || a.BufferView >= len(d.BufferViews) || comp == 0 || sizes[a.Type] == 0 || a.Count < 0 ||
			a.Count*sizes[a.Type]*comp > d.BufferViews[a.BufferView].ByteLength {
			return nil, fmt.Errorf("model: accessor %d is out of range", i)
		}
	}
	accessor := func(i int) bool { return i >= 0 && i < len(d.Accessors) }
	for mi, m := range d.Meshes {
		for _, p := range m.Primitives {
			if !accessor(p.Indices) || p.Material < 0 || p.Material >= len(d.Materials) {
				return nil, fmt.Errorf("model: mesh %d primitive", mi)
			}
			pos, ok := p.Attributes["POSITION"]
			if !ok || !accessor(pos) || d.Accessors[pos].Count > MaxPrimitiveVertices {
				return nil, fmt.Errorf("model: mesh %d positions", mi)
			}
			for _, a := range p.Attributes {
				if !accessor(a) || d.Accessors[a].Count != d.Accessors[pos].Count {
					return nil, fmt.Errorf("model: mesh %d attributes", mi)
				}
			}
			idx, err := g.indices(p.Indices)
			if err != nil {
				return nil, err
			}
			for _, v := range idx {
				if v >= d.Accessors[pos].Count {
					return nil, fmt.Errorf("model: mesh %d index %d is out of range", mi, v)
				}
			}
		}
	}
	for i, node := range d.Nodes {
		if node.Mesh != nil && (*node.Mesh < 0 || *node.Mesh >= len(d.Meshes)) {
			return nil, fmt.Errorf("model: node %d mesh", i)
		}
		for _, c := range node.Children {
			if c <= i || c >= len(d.Nodes) {
				// The converter writes children after their parents, which
				// also rules out loops.
				return nil, fmt.Errorf("model: node %d child %d", i, c)
			}
		}
	}
	if d.Scene < 0 || d.Scene >= len(d.Scenes) {
		return nil, fmt.Errorf("model: scene")
	}
	for _, t := range d.Textures {
		if t.Source < 0 || t.Source >= len(d.Images) || t.Sampler < 0 || t.Sampler >= len(d.Samplers) {
			return nil, fmt.Errorf("model: texture")
		}
	}
	for i, m := range d.Materials {
		if bt := m.PBR.BaseColorTexture; bt != nil && (bt.Index < 0 || bt.Index >= len(d.Textures)) {
			return nil, fmt.Errorf("model: material %d texture", i)
		}
	}
	for i, a := range d.Animations {
		for _, ch := range a.Channels {
			if ch.Sampler < 0 || ch.Sampler >= len(a.Samplers) || ch.Target.Node < 0 || ch.Target.Node >= len(d.Nodes) {
				return nil, fmt.Errorf("model: animation %d channel", i)
			}
		}
		for _, s := range a.Samplers {
			if !accessor(s.Input) || !accessor(s.Output) {
				return nil, fmt.Errorf("model: animation %d sampler", i)
			}
		}
	}
	var names []string
	for _, img := range d.Images {
		name, ok := strings.CutPrefix(unescapeURI(img.URI), "asset:")
		if !ok {
			return nil, fmt.Errorf("model: image uri %q is not an asset name", img.URI)
		}
		names = append(names, name)
	}
	return names, nil
}
