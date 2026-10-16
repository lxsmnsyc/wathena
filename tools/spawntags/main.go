// Command spawntags writes core's spawn tags, server/spawns/core/data/
// spawn_tags.yml, from rAthena's spawn files as reference/rathena
// extracted them (strategy section 8.11, proposal 0002, review decision
// 44, decision 0042):
//
//	go run ./tools/spawntags          rewrites the file
//	go run ./tools/spawntags -check   fails when the file differs
//
// Each rAthena map family maps to a biome of wAthena (families.go). A
// monster's weight in a biome is its share of the monsters that
// rAthena spawns on that biome's maps, per 1,000. Its role comes from
// its spawn lines, and from its class for bosses. The densities are
// the median monsters per 10,000 walkable cells of the field maps and
// of the dungeon maps.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
)

// Out is the file the command writes, from the repository's root.
const Out = "server/spawns/core/data/spawn_tags.yml"

func main() {
	check := flag.Bool("check", false, "fail when the file differs instead of writing it")
	flag.Parse()
	if err := run(*check); err != nil {
		fmt.Fprintln(os.Stderr, "spawntags:", err)
		os.Exit(1)
	}
}

func run(check bool) error {
	d, err := Load()
	if err != nil {
		return err
	}
	b := d.YAML()
	if check {
		old, err := os.ReadFile(Out)
		if err != nil {
			return err
		}
		if !bytes.Equal(old, b) {
			return fmt.Errorf("%s differs from rAthena's spawn files; run go run ./tools/spawntags", Out)
		}
		return nil
	}
	return os.WriteFile(Out, b, 0o600)
}
