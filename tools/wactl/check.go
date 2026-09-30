package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	clientdata "github.com/lxsmnsyc/wathena/client/data"
	"github.com/lxsmnsyc/wathena/internal/assets/build"
)

func assetsCheck(args []string) error {
	fs := newFlags("assets check")
	dir := fs.String("build", "", "build folder to check")
	list := fs.Bool("list", false, "list every missing name")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	_ = fs.Parse(args)
	if *dir == "" {
		return errors.New("--build is required")
	}

	// Client data tables refer to assets too.
	tableRefs, err := clientdata.References()
	if err != nil {
		return err
	}
	refs := make([]build.Reference, 0, len(tableRefs))
	for _, r := range tableRefs {
		refs = append(refs, build.Reference{From: r.Table, Name: r.Name})
	}

	rep, err := build.Check(*dir, refs)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "\t")
		if err := enc.Encode(rep); err != nil {
			return err
		}
	} else {
		printCheck(rep, *list)
	}
	if len(rep.Broken) > 0 {
		return fmt.Errorf("%d broken files", len(rep.Broken))
	}
	return nil
}

func printCheck(rep *build.CheckReport, list bool) {
	fmt.Printf("build %s: %d assets in %d files\n", rep.Build, rep.Assets, rep.Files)
	fmt.Printf("broken files: %d\n", len(rep.Broken))
	for _, b := range rep.Broken {
		fmt.Println("  " + b)
	}
	// Missing names are grouped by kind, such as sprite/body or texture,
	// since one missing texture can be named by many maps.
	groups := map[string][]string{}
	for name := range rep.Missing {
		groups[kindOf(name)] = append(groups[kindOf(name)], name)
	}
	kinds := make([]string, 0, len(groups))
	for k := range groups {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	fmt.Printf("missing names: %d. The client shows placeholders for them.\n", len(rep.Missing))
	for _, k := range kinds {
		names := groups[k]
		sort.Strings(names)
		fmt.Printf("  %-16s %6d\n", k, len(names))
		show := names
		if !list {
			show = names[:min(3, len(names))]
		}
		for _, n := range show {
			from := rep.Missing[n]
			more := ""
			if len(from) > 1 {
				more = fmt.Sprintf(" and %d more", len(from)-1)
			}
			fmt.Printf("    %s, from %s%s\n", n, from[0], more)
		}
	}
}

// kindOf returns the first segment of a name, or the first two for
// sprites and palettes.
func kindOf(name string) string {
	parts := strings.SplitN(name, "/", 3)
	if len(parts) > 1 && (parts[0] == "sprite" || parts[0] == "palette") {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

func assetsGC(args []string) error {
	fs := newFlags("assets gc")
	dir := fs.String("build", "", "build folder to clean")
	dryRun := fs.Bool("dry-run", false, "only report what would be removed")
	_ = fs.Parse(args)
	if *dir == "" {
		return errors.New("--build is required")
	}
	rep, err := build.GC(*dir, *dryRun)
	if err != nil {
		return err
	}
	verb := "removed"
	if *dryRun {
		verb = "would remove"
	}
	fmt.Printf("%s %d files, %.1f MB, and %d cache entries\n", verb, rep.Files, float64(rep.Bytes)/(1<<20), rep.CacheEntries)
	return nil
}
