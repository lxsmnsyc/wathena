package main

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/lxsmnsyc/wathena/internal/assets/grfimport"
	"github.com/lxsmnsyc/wathena/internal/grf"
)

// assetsSprites converts every sprite the GRF importer can name and
// reports sizes. It is a check of the importer, not the full build.
func assetsSprites(args []string) error {
	fs := newFlags("assets sprites")
	client := fs.String("client", "", "game client folder with DATA.INI")
	preview := fs.String("preview", "", "folder for PNG previews of the named sprites")
	names := fs.String("names", "", "comma-separated sprite names to preview")
	quality := fs.Int("brotli", 11, "Brotli quality, 0 to 11")
	every := fs.Int("every", 1, "convert only every nth sprite, for quick measurements")
	_ = fs.Parse(args)
	if *client == "" {
		return fmt.Errorf("--client is required")
	}
	c, err := grf.OpenClient(*client)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()

	type pair struct{ spr, act grf.Source }
	pairs := map[string]*pair{}
	for _, s := range c.Files() {
		name, ok := grfimport.SpriteName(s.Entry.Name)
		if !ok {
			continue
		}
		p := pairs[name]
		if p == nil {
			p = &pair{}
			pairs[name] = p
		}
		if strings.HasSuffix(s.Entry.Key, ".spr") {
			p.spr = s
		} else {
			p.act = s
		}
	}
	want := map[string]bool{}
	for _, n := range strings.Split(*names, ",") {
		if n != "" {
			want[n] = true
		}
	}

	start := time.Now()
	var done, failed, missing, rgba atomic.Int64
	var origBytes, outBytes, brBytes atomic.Int64
	var mu sync.Mutex
	var errs []string
	work := make(chan string)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Go(func() {
			for name := range work {
				p := pairs[name]
				if p.spr.Entry.Key == "" || p.act.Entry.Key == "" {
					missing.Add(1)
					continue
				}
				sprData, err1 := c.ReadFile(p.spr)
				actData, err2 := c.ReadFile(p.act)
				if err1 != nil || err2 != nil {
					failed.Add(1)
					continue
				}
				s, err := grfimport.ConvertSprite(sprData, actData, grfimport.EventName)
				var out []byte
				if err == nil {
					out, err = s.Encode()
				}
				if err != nil {
					failed.Add(1)
					mu.Lock()
					errs = append(errs, fmt.Sprintf("%s: %v", name, err))
					mu.Unlock()
					continue
				}
				for _, pg := range s.Pages {
					if pg.Kind == 1 {
						rgba.Add(1)
						break
					}
				}
				var b bytes.Buffer
				bw := brotli.NewWriterLevel(&b, *quality)
				_, _ = bw.Write(out)
				_ = bw.Close()
				origBytes.Add(int64(p.spr.Entry.CompressedSize) + int64(p.act.Entry.CompressedSize))
				outBytes.Add(int64(len(out)))
				brBytes.Add(int64(b.Len()))
				done.Add(1)
				if *preview != "" && want[name] {
					img := s.RenderFrame(0, 0, 256)
					var pb bytes.Buffer
					_ = png.Encode(&pb, img)
					file := filepath.Join(*preview, strings.ReplaceAll(name, "/", "_")+".png")
					if err := os.WriteFile(file, pb.Bytes(), 0o600); err != nil {
						fmt.Fprintln(os.Stderr, err)
					}
				}
			}
		})
	}
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for i, k := range keys {
		if i%max(*every, 1) == 0 {
			work <- k
		}
	}
	close(work)
	wg.Wait()

	slices.Sort(errs)
	for i, e := range errs {
		if i == 20 {
			fmt.Printf("... and %d more\n", len(errs)-20)
			break
		}
		fmt.Println(e)
	}
	mb := func(v int64) float64 { return float64(v) / (1 << 20) }
	fmt.Printf("sprites: %d converted, %d failed, %d without spr or act, %d with RGBA frames\n", done.Load(), failed.Load(), missing.Load(), rgba.Load())
	fmt.Printf("GRF (spr+act, zlib): %.1f MB, WSPR: %.1f MB, WSPR+Brotli: %.1f MB (%.0f%% of GRF)\n",
		mb(origBytes.Load()), mb(outBytes.Load()), mb(brBytes.Load()), 100*float64(brBytes.Load())/float64(max(origBytes.Load(), 1)))
	fmt.Printf("time: %s\n", time.Since(start).Round(time.Millisecond))
	return nil
}
