package main

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"text/tabwriter"
	"time"

	"github.com/lxsmnsyc/wathena/internal/grf"
)

// assetGroup sorts a file into the groups in strategy section 10.1.
func assetGroup(key string) string {
	switch path.Ext(key) {
	case ".spr", ".act", ".pal", ".imf":
		return "sprites"
	case ".bmp", ".tga", ".png", ".jpg":
		return "textures"
	case ".gnd", ".gat", ".rsw":
		return "maps"
	case ".rsm", ".rsm2", ".gr2":
		return "models"
	case ".wav", ".mp3":
		return "sounds"
	case ".str":
		return "effects"
	}
	return "other"
}

type groupStats struct {
	files      int
	compressed int64
	size       int64
}

func assetsScan(args []string) error {
	fs := newFlags("assets scan")
	client := fs.String("client", "", "game client folder with DATA.INI")
	read := fs.Bool("read", false, "read and decompress every file, to time a full pass")
	_ = fs.Parse(args)
	if *client == "" {
		return fmt.Errorf("--client is required")
	}

	start := time.Now()
	c, err := grf.OpenClient(*client)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	files := c.Files()
	opened := time.Since(start)

	stats := map[string]*groupStats{}
	for _, s := range files {
		g := assetGroup(s.Entry.Key)
		st := stats[g]
		if st == nil {
			st = &groupStats{}
			stats[g] = st
		}
		st.files++
		st.compressed += int64(s.Entry.CompressedSize)
		st.size += int64(s.Entry.Size)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintf(w, "group\tfiles\tcompressed MB\tsize MB\t\n")
	names := make([]string, 0, len(stats))
	for g := range stats {
		names = append(names, g)
	}
	slices.Sort(names)
	for _, g := range names {
		st := stats[g]
		_, _ = fmt.Fprintf(w, "%s\t%d\t%.0f\t%.0f\t\n", g, st.files, float64(st.compressed)/(1<<20), float64(st.size)/(1<<20))
	}
	_ = w.Flush()
	fmt.Printf("\narchives: %v\nfiles: %d\nopened in %s\n", c.Archives(), len(files), opened.Round(time.Millisecond))

	if !*read {
		return nil
	}
	start = time.Now()
	var bytesRead, failed atomic.Int64
	work := make(chan grf.Source)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Go(func() {
			for s := range work {
				data, err := c.ReadFile(s)
				if err != nil {
					failed.Add(1)
					fmt.Fprintf(os.Stderr, "%s: %v\n", s.Entry.Name, err)
					continue
				}
				bytesRead.Add(int64(len(data)))
			}
		})
	}
	for _, s := range files {
		work <- s
	}
	close(work)
	wg.Wait()
	d := time.Since(start)
	fmt.Printf("read %.0f MB in %s (%.0f MB/s), %d errors, %d entries with a wrong recorded size\n",
		float64(bytesRead.Load())/(1<<20), d.Round(time.Millisecond), float64(bytesRead.Load())/(1<<20)/d.Seconds(), failed.Load(), c.Repaired())
	return nil
}
