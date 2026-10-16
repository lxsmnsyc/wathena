package gateway

import wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"

// WorldMode is what Welcome tells a client about the world's maps
// (strategy section 8.1, decision 0038). The zero value is a classic
// world.
type WorldMode struct {
	// Procedural is true for a procedural world.
	Procedural bool
	// GeneratorVersion and RNGVersion are a procedural world's map
	// generator version and seed tree spec, such as "rng-v1".
	GeneratorVersion int
	RNGVersion       string
}

// message is the protocol's WorldInfo of the mode.
func (m WorldMode) message() *wathenav1.WorldInfo {
	if !m.Procedural {
		return wathenav1.WorldInfo_builder{Mode: wathenav1.WorldMode_WORLD_MODE_CLASSIC}.Build()
	}
	return wathenav1.WorldInfo_builder{
		Mode:             wathenav1.WorldMode_WORLD_MODE_PROCEDURAL,
		GeneratorVersion: uint32(m.GeneratorVersion), // #nosec G115 -- versions are small and positive.
		RngVersion:       m.RNGVersion,
	}.Build()
}
