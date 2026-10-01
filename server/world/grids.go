package world

import (
	"errors"
	"fmt"
	"runtime"
	"sync"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// LoadGrids reads the cells of maps from an asset build, several at once.
// names are asset names, such as map/prontera.
func LoadGrids(dir string, names []string) ([]*path.Grid, error) {
	grids := make([]*path.Grid, len(names))
	errs := make([]error, len(names))
	next := make(chan int)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for i := range next {
				grids[i], errs[i] = LoadGrid(dir, names[i])
			}
		})
	}
	for i := range names {
		next <- i
	}
	close(next)
	wg.Wait()
	return grids, errors.Join(errs...)
}

// LoadGrid reads a map's cells from an asset build. Only the walkable
// and shootable flags are kept.
func LoadGrid(dir, name string) (*path.Grid, error) {
	data, typ, err := build.ReadAsset(dir, name)
	if err != nil {
		return nil, err
	}
	if typ != "map" {
		return nil, fmt.Errorf("%s is a %s, not a map", name, typ)
	}
	w, h, flags, err := wmap.DecodeCells(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	for i := range flags {
		flags[i] &= path.Walkable | path.Shootable
	}
	return &path.Grid{Width: w, Height: h, Cells: flags}, nil
}
