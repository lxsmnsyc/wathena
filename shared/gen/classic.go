//go:build !procedural

package gen

// Procedural is false: this build contains only the classic world mode.
const Procedural = false

// registry is empty without the procedural tag.
var registry []Generator
