package model

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
)

// This file writes glTF 2.0 binary files with the parts of glTF that the
// assets-v1 model profile allows.

// glTF constants.
const (
	glFloat         = 5126
	glUnsignedShort = 5123
	glUnsignedInt   = 5125
	glArrayBuffer   = 34962
	glElementBuffer = 34963
	glRepeat        = 10497
	glLinear        = 9729
	glLinearMipmap  = 9987
)

type gltfDoc struct {
	Asset       gltfAsset       `json:"asset"`
	Scene       int             `json:"scene"`
	Scenes      []gltfScene     `json:"scenes"`
	Nodes       []gltfNode      `json:"nodes"`
	Meshes      []gltfMesh      `json:"meshes,omitempty"`
	Materials   []gltfMaterial  `json:"materials,omitempty"`
	Textures    []gltfTexture   `json:"textures,omitempty"`
	Images      []gltfImage     `json:"images,omitempty"`
	Samplers    []gltfSampler   `json:"samplers,omitempty"`
	Animations  []gltfAnimation `json:"animations,omitempty"`
	Accessors   []gltfAccessor  `json:"accessors,omitempty"`
	BufferViews []gltfView      `json:"bufferViews,omitempty"`
	Buffers     []gltfBuffer    `json:"buffers,omitempty"`
}

type gltfAsset struct {
	Version   string `json:"version"`
	Generator string `json:"generator"`
}

type gltfScene struct {
	Nodes []int `json:"nodes"`
}

type gltfNode struct {
	Name        string       `json:"name,omitempty"`
	Children    []int        `json:"children,omitempty"`
	Mesh        *int         `json:"mesh,omitempty"`
	Matrix      *[16]float32 `json:"matrix,omitempty"`
	Translation *[3]float32  `json:"translation,omitempty"`
	Rotation    *[4]float32  `json:"rotation,omitempty"`
	Scale       *[3]float32  `json:"scale,omitempty"`
}

type gltfMesh struct {
	Primitives []gltfPrimitive `json:"primitives"`
}

type gltfPrimitive struct {
	Attributes map[string]int `json:"attributes"`
	Indices    int            `json:"indices"`
	Material   int            `json:"material"`
}

type gltfMaterial struct {
	PBR         gltfPBR `json:"pbrMetallicRoughness"`
	AlphaMode   string  `json:"alphaMode,omitempty"`
	AlphaCutoff float32 `json:"alphaCutoff,omitempty"`
	DoubleSided bool    `json:"doubleSided,omitempty"`
}

type gltfPBR struct {
	BaseColorFactor  *[4]float32  `json:"baseColorFactor,omitempty"`
	BaseColorTexture *gltfTexInfo `json:"baseColorTexture,omitempty"`
	MetallicFactor   float32      `json:"metallicFactor"`
	RoughnessFactor  float32      `json:"roughnessFactor"`
}

type gltfTexInfo struct {
	Index int `json:"index"`
}

type gltfTexture struct {
	Sampler int `json:"sampler"`
	Source  int `json:"source"`
}

type gltfImage struct {
	URI string `json:"uri"`
}

type gltfSampler struct {
	MagFilter int `json:"magFilter"`
	MinFilter int `json:"minFilter"`
	WrapS     int `json:"wrapS"`
	WrapT     int `json:"wrapT"`
}

type gltfAnimation struct {
	Channels []gltfChannel    `json:"channels"`
	Samplers []gltfAnimSample `json:"samplers"`
	Extras   map[string]any   `json:"extras,omitempty"`
}

type gltfChannel struct {
	Sampler int        `json:"sampler"`
	Target  gltfTarget `json:"target"`
}

type gltfTarget struct {
	Node int    `json:"node"`
	Path string `json:"path"`
}

type gltfAnimSample struct {
	Input         int    `json:"input"`
	Output        int    `json:"output"`
	Interpolation string `json:"interpolation"`
}

type gltfAccessor struct {
	BufferView    int       `json:"bufferView"`
	ComponentType int       `json:"componentType"`
	Count         int       `json:"count"`
	Type          string    `json:"type"`
	Min           []float32 `json:"min,omitempty"`
	Max           []float32 `json:"max,omitempty"`
}

type gltfView struct {
	Buffer     int `json:"buffer"`
	ByteOffset int `json:"byteOffset"`
	ByteLength int `json:"byteLength"`
	Target     int `json:"target,omitempty"`
}

type gltfBuffer struct {
	ByteLength int `json:"byteLength"`
}

// builder collects glTF objects and one binary buffer.
type builder struct {
	doc gltfDoc
	bin bytes.Buffer
}

// view appends data to the buffer, aligned to 4 bytes, and returns its
// buffer view.
func (b *builder) view(data []byte, target int) int {
	for b.bin.Len()%4 != 0 {
		b.bin.WriteByte(0)
	}
	b.doc.BufferViews = append(b.doc.BufferViews, gltfView{ByteOffset: b.bin.Len(), ByteLength: len(data), Target: target})
	b.bin.Write(data)
	return len(b.doc.BufferViews) - 1
}

// floats adds an accessor for float vectors of size n, with bounds if
// bounded, since glTF requires them for positions and animation inputs.
func (b *builder) floats(v []float32, n int, typ string, target int, bounded bool) int {
	data := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(data[4*i:], math.Float32bits(f))
	}
	acc := gltfAccessor{BufferView: b.view(data, target), ComponentType: glFloat, Count: len(v) / n, Type: typ}
	if bounded && len(v) >= n {
		acc.Min = append([]float32(nil), v[:n]...)
		acc.Max = append([]float32(nil), v[:n]...)
		for i, f := range v {
			acc.Min[i%n] = min(acc.Min[i%n], f)
			acc.Max[i%n] = max(acc.Max[i%n], f)
		}
	}
	b.doc.Accessors = append(b.doc.Accessors, acc)
	return len(b.doc.Accessors) - 1
}

// indices adds an index accessor, 16-bit when the vertices allow it.
func (b *builder) indices(idx []uint32, vertices int) int {
	var data []byte
	ct := glUnsignedInt
	if vertices <= 1<<16 {
		ct = glUnsignedShort
		data = make([]byte, 2*len(idx))
		for i, v := range idx {
			binary.LittleEndian.PutUint16(data[2*i:], uint16(v)) // #nosec G115 -- checked above.
		}
	} else {
		data = make([]byte, 4*len(idx))
		for i, v := range idx {
			binary.LittleEndian.PutUint32(data[4*i:], v)
		}
	}
	b.doc.Accessors = append(b.doc.Accessors, gltfAccessor{BufferView: b.view(data, glElementBuffer), ComponentType: ct, Count: len(idx), Type: "SCALAR"})
	return len(b.doc.Accessors) - 1
}

// glb encodes the document and buffer as a GLB file.
func (b *builder) glb() ([]byte, error) {
	for b.bin.Len()%4 != 0 {
		b.bin.WriteByte(0)
	}
	if b.bin.Len() > 0 {
		b.doc.Buffers = []gltfBuffer{{ByteLength: b.bin.Len()}}
	}
	js, err := json.Marshal(b.doc)
	if err != nil {
		return nil, err
	}
	// The JSON chunk is padded with spaces, the binary chunk with zeros.
	for len(js)%4 != 0 {
		js = append(js, ' ')
	}
	total := 12 + 8 + len(js)
	if b.bin.Len() > 0 {
		total += 8 + b.bin.Len()
	}
	out := make([]byte, 0, total)
	out = append(out, "glTF"...)
	out = binary.LittleEndian.AppendUint32(out, 2)
	out = binary.LittleEndian.AppendUint32(out, uint32(total))   // #nosec G115 -- checked by the caller's size limit.
	out = binary.LittleEndian.AppendUint32(out, uint32(len(js))) // #nosec G115 -- as above.
	out = append(out, "JSON"...)
	out = append(out, js...)
	if b.bin.Len() > 0 {
		out = binary.LittleEndian.AppendUint32(out, uint32(b.bin.Len())) // #nosec G115 -- as above.
		out = append(out, "BIN\x00"...)
		out = append(out, b.bin.Bytes()...)
	}
	return out, nil
}
