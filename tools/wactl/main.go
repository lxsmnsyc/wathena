// Command wactl manages wAthena worlds, mods, maps, and assets.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/lxsmnsyc/wathena/internal/buildinfo"
)

// buildVersion is set by release builds, such as scripts/release.sh and
// the container image, with -ldflags "-X main.buildVersion=...".
var buildVersion string

const usage = `usage: wactl <command> [arguments]

commands:
  assets build     build assets from sources into a folder
  assets check     verify a build's files and list missing names
  assets gc        remove files a build no longer uses
  assets ls        list a build's names, optionally by prefix
  assets serve     serve a build over HTTP for development
  assets scan      list what a game client folder contains
  assets sprites   convert monster and NPC sprites and report sizes
  client-data validate check client data tables against their schema
  convert items    convert rAthena item scripts to Lua and item data
  convert npcs     convert rAthena NPC scripts and shops to Lua and NPC data
  convert monsters convert rAthena monster skills to monster data
  convert random-options convert rAthena random options and groups to data
  convert quests         convert rAthena quests to data
  dev              run a local server with a mod, and reload it when files change
  map validate     check map folders with the map generator's checks
  mod new          make a mod from a template
  mod validate     check a mod's manifest, data, permissions, scripts and tests
  mod test         run a mod's tests in a headless world
  mod pack         build a mod's .zip and checksum file, with its art built
  mod build-art    build a folder mod's art/ into its assets/
  mod install      check a mod from Git, a .zip or a folder, and install it
  mod types        write the Lua Language Server definitions into a mod
  migrate status   list the database migrations and which are applied
  migrate up       apply pending database migrations
  sim economy      print a new procedural world's exp and zeny per hour by field
  version          print the version and the world modes of the build
  world new        apply migrations, make the world and its first account
  world backup     write the database to a file with pg_dump
  world restore    load a backup into an empty database with pg_restore
`

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "dev" {
		exit(dev(os.Args[2:]))
	}
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(buildinfo.Line(buildVersion))
		return
	}
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
	case "convert monsters":
		err = convertMonsters(os.Args[3:])
	case "convert random-options":
		err = convertRandomOptions(os.Args[3:])
	case "convert quests":
		err = convertQuests(os.Args[3:])
	case "migrate status":
		err = migrate(os.Args[3:], false)
	case "migrate up":
		err = migrate(os.Args[3:], true)
	case "sim economy":
		err = simEconomy(os.Args[3:])
	case "world new":
		err = worldNew(os.Args[3:])
	case "world backup":
		err = worldBackup(os.Args[3:])
	case "world restore":
		err = worldRestore(os.Args[3:])
	case "mod new":
		err = modNew(os.Args[3:])
	case "mod validate":
		err = modValidate(os.Args[3:])
	case "mod test":
		err = modTest(os.Args[3:])
	case "mod pack":
		err = modPack(os.Args[3:])
	case "mod build-art":
		err = modBuildArt(os.Args[3:])
	case "client-data validate":
		err = clientDataValidate(os.Args[3:])
	case "map validate":
		err = mapValidate(os.Args[3:])
	case "mod install":
		err = modInstall(os.Args[3:])
	case "mod types":
		err = modTypes(os.Args[3:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	exit(err)
}

// exit ends wactl with status 1 on an error, and prints it unless the
// command printed its problems already.
func exit(err error) {
	if err == nil {
		os.Exit(0)
	}
	if !errors.Is(err, errFailed) {
		fmt.Fprintln(os.Stderr, "wactl:", err)
	}
	os.Exit(1)
}

func newFlags(name string) *flag.FlagSet {
	return flag.NewFlagSet("wactl "+name, flag.ExitOnError)
}
