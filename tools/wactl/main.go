// Command wactl manages wAthena worlds, mods, maps, and assets.
package main

import (
	"flag"
	"fmt"
	"os"
)

const usage = `usage: wactl <command> [arguments]

commands:
  assets scan   list what a game client folder contains
`

func main() {
	if len(os.Args) < 3 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] + " " + os.Args[2] {
	case "assets scan":
		err = assetsScan(os.Args[3:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wactl:", err)
		os.Exit(1)
	}
}

func newFlags(name string) *flag.FlagSet {
	return flag.NewFlagSet("wactl "+name, flag.ExitOnError)
}
