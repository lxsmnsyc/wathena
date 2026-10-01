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
  assets ls        list a build's names, optionally by prefix
  assets serve     serve a build over HTTP for development
  assets scan      list what a game client folder contains
  assets sprites   convert monster and NPC sprites and report sizes
  convert items    convert rAthena item scripts to Lua and item data
  convert npcs     convert rAthena NPC scripts and shops to Lua and NPC data
  migrate status   list the database migrations and which are applied
  migrate up       apply pending database migrations
  world new        apply migrations, make the world and its first staff account
  world backup     write the database to a file with pg_dump
  world restore    load a backup into an empty database with pg_restore
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
	case "assets ls":
		err = assetsList(os.Args[3:])
	case "assets gc":
		err = assetsGC(os.Args[3:])
	case "assets serve":
		err = assetsServe(os.Args[3:])
	case "assets scan":
		err = assetsScan(os.Args[3:])
	case "assets sprites":
		err = assetsSprites(os.Args[3:])
	case "convert items":
		err = convertItems(os.Args[3:])
	case "convert npcs":
		err = convertNPCs(os.Args[3:])
	case "migrate status":
		err = migrate(os.Args[3:], false)
	case "migrate up":
		err = migrate(os.Args[3:], true)
	case "world new":
		err = worldNew(os.Args[3:])
	case "world backup":
		err = worldBackup(os.Args[3:])
	case "world restore":
		err = worldRestore(os.Args[3:])
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
