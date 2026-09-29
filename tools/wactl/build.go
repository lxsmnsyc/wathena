package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
)

// sourceList collects repeated --source flags.
type sourceList []string

func (s *sourceList) String() string     { return strings.Join(*s, ",") }
func (s *sourceList) Set(v string) error { *s = append(*s, v); return nil }

func assetsBuild(args []string) error {
	fs := newFlags("assets build")
	var sources sourceList
	fs.Var(&sources, "source", "a source, applied in order: placeholder, or grf:<client folder>")
	out := fs.String("out", "", "output folder")
	fast := fs.Bool("fast", false, "use a low Brotli level, for development builds")
	_ = fs.Parse(args)
	if *out == "" || len(sources) == 0 {
		return errors.New("--out and at least one --source are required")
	}

	var srcs []build.Source
	var closers []io.Closer
	defer func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}()
	for _, s := range sources {
		kind, arg, _ := strings.Cut(s, ":")
		switch kind {
		case "placeholder":
			srcs = append(srcs, build.Placeholder{})
		case "grf":
			g := &build.GRF{Dir: arg}
			srcs = append(srcs, g)
			closers = append(closers, g)
		default:
			return fmt.Errorf("unknown source %q", s)
		}
	}
	quality := 11
	if *fast {
		quality = 5
	}
	rep, err := build.Run(build.Options{Out: *out, Sources: srcs, Brotli: quality})
	if err != nil {
		return err
	}
	for _, s := range rep.Sources {
		fmt.Printf("%-40s %6d provided, %6d replaced earlier sources\n", s.Name, s.Provided, s.Replaced)
	}
	for i, r := range rep.Rejected {
		if i == 20 {
			fmt.Printf("... and %d more rejected\n", len(rep.Rejected)-20)
			break
		}
		fmt.Println("rejected:", r)
	}
	fmt.Printf("build %s: %d assets, %d converted, %d from cache, %.1f MB, %s\n",
		rep.BuildID, rep.Assets, rep.Converted, rep.Cached, float64(rep.Bytes)/(1<<20), rep.Duration.Round(time.Millisecond))
	return nil
}
