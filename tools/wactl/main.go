// Command wactl manages wAthena worlds, mods, maps, and assets.
package main

import (
	"flag"
	"fmt"
	"os"
)

const usage = `usage: wactl <command> [arguments]

commands:
  assets build     build assets from sources into a folder
  assets check     verify a build's files and list missing names
  assets gc        remove files a build no longer uses
  assets scan      list what a game client folder contains
  assets sprites   convert monster and NPC sprites and report sizes
`

func main() {
	if len(os.Args) < 3 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] + " " + os.Args[2] {
	case "assets build":
		err = assetsBuild(os.Args[3:])
	case "assets check":
		err = assetsCheck(os.Args[3:])
	case "assets gc":
		err = assetsGC(os.Args[3:])
	case "assets scan":
		err = assetsScan(os.Args[3:])
	case "assets sprites":
		err = assetsSprites(os.Args[3:])
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
