//go:build tinygo

// Command wasm is the path package for the web client, built with TinyGo:
//
//	tinygo build -o client/src/world/path.wasm -target=wasm-unknown -opt=2 -no-debug ./shared/path/wasm
//
// The client writes cell flags into the buffer grid returns, then calls
// search and reads the steps from the buffer steps returns.
package main

import "github.com/lxsmnsyc/wathena/shared/path"

var (
	grid  path.Grid
	steps [path.MaxWalkPath]int8
)

// setGrid sizes the map and returns its cell buffer, width × height flags
// row by row, or 0 if the size is invalid.
//
//export grid
func setGrid(width, height int32) *uint8 {
	grid = path.Grid{Width: int(width), Height: int(height)}
	if width <= 0 || height <= 0 || width > 1024 || height > 1024 {
		grid = path.Grid{}
		return nil
	}
	grid.Cells = make([]uint8, grid.Width*grid.Height)
	return &grid.Cells[0]
}

// stepsBuffer returns the buffer search writes its steps to.
//
//export steps
func stepsBuffer() *int8 { return &steps[0] }

// search finds a path and returns its number of steps, or -1.
//
//export search
func search(x0, y0, x1, y1, easy int32) int32 {
	if !grid.Valid() {
		return -1
	}
	found, ok := path.Search(&grid, int(x0), int(y0), int(x1), int(y1), easy != 0)
	if !ok {
		return -1
	}
	for i, d := range found {
		steps[i] = int8(d)
	}
	return int32(len(found)) // #nosec G115 -- at most MaxWalkPath.
}

// shoot returns 1 if a projectile can fly between two cells.
//
//export shoot
func shoot(x0, y0, x1, y1 int32) int32 {
	if !grid.Valid() {
		return 0
	}
	if clear, _ := path.Shoot(&grid, int(x0), int(y0), int(x1), int(y1)); clear {
		return 1
	}
	return 0
}

func main() {}
