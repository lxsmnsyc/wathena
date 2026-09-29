package wspr

import (
	"fmt"
	"slices"
)

// Frame images before packing.
type Source struct {
	Width, Height int
	Pix           []byte
}

// Pack places images of one kind onto pages and returns the pages and
// where each image went, in input order. Images are packed on shelves,
// tallest first, with 1 pixel of space between them so linear filtering
// does not blend neighbors.
func Pack(kind uint8, srcs []Source) ([]Page, []Image, error) {
	bpp := 1
	if kind == RGBA {
		bpp = 4
	}
	area, widest := 0, 0
	for i, s := range srcs {
		if s.Width < 0 || s.Height < 0 || s.Width >= MaxPageSize || s.Height >= MaxPageSize {
			return nil, nil, fmt.Errorf("%w: image %d is %d × %d", ErrFormat, i, s.Width, s.Height)
		}
		if len(s.Pix) != s.Width*s.Height*bpp {
			return nil, nil, fmt.Errorf("%w: image %d has %d bytes", ErrFormat, i, len(s.Pix))
		}
		area += (s.Width + 1) * (s.Height + 1)
		widest = max(widest, s.Width+1)
	}
	// Aim for a square page, rounded up to a power of two.
	width := 64
	for width*width < area && width < MaxPageSize {
		width *= 2
	}
	for width < widest {
		width *= 2
	}
	width = min(width, MaxPageSize)

	order := make([]int, len(srcs))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return srcs[b].Height - srcs[a].Height })

	type spot struct{ page, x, y int }
	spots := make([]spot, len(srcs))
	var heights []int
	page, x, y, shelf := 0, 0, 0, 0
	heights = append(heights, 0)
	for _, i := range order {
		s := srcs[i]
		if s.Width == 0 || s.Height == 0 {
			spots[i] = spot{page: -1}
			continue
		}
		if x+s.Width > width {
			x, y, shelf = 0, y+shelf, 0
		}
		if y+s.Height > MaxPageSize {
			page++
			x, y, shelf = 0, 0, 0
			heights = append(heights, 0)
		}
		spots[i] = spot{page, x, y}
		x += s.Width + 1
		shelf = max(shelf, s.Height+1)
		heights[page] = max(heights[page], y+s.Height)
	}
	if len(heights) > MaxPages {
		return nil, nil, fmt.Errorf("%w: needs %d pages", ErrFormat, len(heights))
	}

	pages := make([]Page, len(heights))
	for p, h := range heights {
		pages[p] = Page{Kind: kind, Width: uint16(width), Height: uint16(h), Pix: make([]byte, width*h*bpp)} // #nosec G115 -- at most MaxPageSize.
	}
	images := make([]Image, len(srcs))
	for i, s := range srcs {
		sp := spots[i]
		if sp.page < 0 {
			continue
		}
		pg := &pages[sp.page]
		for row := range s.Height {
			dst := ((sp.y+row)*width + sp.x) * bpp
			copy(pg.Pix[dst:dst+s.Width*bpp], s.Pix[row*s.Width*bpp:(row+1)*s.Width*bpp])
		}
		images[i] = Image{Page: uint16(sp.page), X: uint16(sp.x), Y: uint16(sp.y), Width: uint16(s.Width), Height: uint16(s.Height)} // #nosec G115 -- at most MaxPageSize.
	}
	return pages, images, nil
}
