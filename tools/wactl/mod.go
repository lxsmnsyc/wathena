package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/reference/rathena"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/tools/modkit"
)

// The mod commands (strategy section 6.9, mods/README.md, "Tools"). They
// read the reference data built into wactl, as every command does, so
// they work outside the repository.

// errFailed is the error of a command that printed its problems.
var errFailed = errors.New("failed")

func useEmbeddedReference() { reference.UseFS(rathena.Files) }

// parseArgs parses flags before and after one optional path argument.
func parseArgs(fs *flag.FlagSet, args []string) (string, error) {
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	var path string
	if fs.NArg() > 0 {
		path = fs.Arg(0)
		if err := fs.Parse(fs.Args()[1:]); err != nil {
			return "", err
		}
		if fs.NArg() > 0 {
			return "", fmt.Errorf("unexpected argument %q", fs.Arg(0))
		}
	}
	return path, nil
}

func modeFlag(fs *flag.FlagSet) *string {
	return fs.String("mode", "renewal", "rAthena data mode to check against: renewal or pre-renewal")
}

func loadEnv(mode, assets string) (*modkit.Env, error) {
	m := reference.Mode(mode)
	if m != reference.Renewal && m != reference.PreRenewal {
		return nil, errors.New("--mode must be renewal or pre-renewal")
	}
	useEmbeddedReference()
	return modkit.LoadEnv(m, assets)
}

func loadConstants() (*constants.Data, error) {
	useEmbeddedReference()
	data, err := reference.ReadFile("extracts/renewal/script_constants.json")
	if err != nil {
		return nil, err
	}
	return constants.Read(bytes.NewReader(data))
}

// modBase is the ruleset base of a data mode.
func modBase(mode string) string {
	if b, ok := mod.ModeBase(mode); ok {
		return b
	}
	return mod.Renewal
}

// modNew is wactl mod new <id>.
func modNew(args []string) error {
	fs := newFlags("mod new")
	dir := fs.String("dir", "", "folder to make the mod in; the mod's ID by default")
	id, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if id == "" {
		return errors.New("usage: wactl mod new <id> [--dir <folder>], such as wactl mod new example.halloween")
	}
	if *dir == "" {
		*dir = id
	}
	consts, err := loadConstants()
	if err != nil {
		return err
	}
	files, err := modkit.New(id, *dir, consts)
	if err != nil {
		return err
	}
	fmt.Printf("made %s in %s:\n", id, *dir)
	for _, f := range files {
		fmt.Println("  " + f)
	}
	fmt.Printf("next: cd %s && wactl mod validate && wactl mod test\n", *dir)
	return nil
}

// modValidate is wactl mod validate [path].
func modValidate(args []string) error {
	fs := newFlags("mod validate")
	deps := fs.String("mods", "", "folder of other mods, where the mod's dependencies are")
	show := fs.Bool("show", false, "print the final value of every record the mod adds or changes, and which mod set each field")
	mode := modeFlag(fs)
	arg, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	p, err := modkit.ModPath(arg)
	if err != nil {
		return err
	}
	env, err := loadEnv(*mode, "")
	if err != nil {
		return err
	}
	rep := env.Validate(context.Background(), p, *deps)
	return printReport(p, rep, *show)
}

func printReport(p string, rep *modkit.Report, show bool) error {
	if m := rep.Manifest; m != nil {
		fmt.Printf("%s %s (%s)\n", m.ID, m.Version, p)
		if len(rep.Rulesets) > 0 {
			fmt.Printf("loads on: %s\n", strings.Join(rep.Rulesets, ", "))
		}
		fmt.Printf("checked: %d Lua files, %d test cases\n", rep.Files, rep.Cases)
		modkit.PrintPermissions(os.Stdout, m.ID, rep.Permissions)
	}
	for _, n := range rep.Notes {
		fmt.Println("note: " + n)
	}
	if show {
		printRecords(rep.Records)
	}
	if len(rep.Problems) > 0 {
		fmt.Fprintf(os.Stderr, "%d problems:\n", len(rep.Problems))
		for _, pr := range rep.Problems {
			fmt.Fprintln(os.Stderr, "  "+pr)
		}
		return errFailed
	}
	fmt.Println("ok")
	return nil
}

func printRecords(records []modkit.Record) {
	if len(records) == 0 {
		fmt.Println("records: none")
		return
	}
	fmt.Println("records:")
	for _, r := range records {
		fmt.Printf("  %s %s (added by %s, %s)\n", r.Kind, r.Name, r.AddedBy, r.Where)
		for _, f := range r.Fields {
			by := f.SetBy
			if by == "" {
				by = "default"
			}
			fmt.Printf("    %-14s %-40s %s\n", f.Name+":", value(f.Value), by)
		}
	}
}

// value shows a field's value on one line.
func value(v any) string {
	s := fmt.Sprint(v)
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

// modTest is wactl mod test [path].
func modTest(args []string) error {
	fs := newFlags("mod test")
	deps := fs.String("mods", "", "folder of other mods, where the mod's dependencies are")
	run := fs.String("run", "", "run only the cases whose name contains this text")
	assets := fs.String("assets", "", "asset build whose map cells to use; without it, every cell is walkable")
	mode := modeFlag(fs)
	arg, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	p, err := modkit.ModPath(arg)
	if err != nil {
		return err
	}
	began := time.Now()
	env, err := loadEnv(*mode, *assets)
	if err != nil {
		return err
	}
	ctx := context.Background()
	l, err := env.Load(ctx, p, *deps, modBase(*mode), nil)
	if err != nil {
		return fmt.Errorf("the mod does not load; run wactl mod validate:\n%w", err)
	}
	file := ""
	results, err := env.RunTests(ctx, l, *run, func(r modkit.CaseResult) {
		if r.File != file {
			file = r.File
			fmt.Println(file)
		}
		status := "ok  "
		if r.Failed() {
			status = "FAIL"
		}
		fmt.Printf("  %s %s (%d ms)\n", status, r.Name, r.Took.Milliseconds())
		if r.Err != nil {
			for line := range strings.SplitSeq(r.Err.Error(), "\n") {
				fmt.Printf("       %s\n", line)
			}
		}
		for _, l := range r.Logs {
			fmt.Printf("       server log: %s\n", l)
		}
	})
	if err != nil {
		return err
	}
	failed := 0
	for _, r := range results {
		if r.Failed() {
			failed++
		}
	}
	if len(results) == 0 {
		fmt.Printf("no test cases in %s\n", filepath.Join(p, modkit.TestPattern))
		return nil
	}
	fmt.Printf("%d cases: %d passed, %d failed (%.1f s)\n", len(results), len(results)-failed, failed, time.Since(began).Seconds())
	if failed > 0 {
		return errFailed
	}
	return nil
}

// modPack is wactl mod pack [path].
func modPack(args []string) error {
	fs := newFlags("mod pack")
	out := fs.String("out", "dist", "folder to write the .zip and its checksum file into")
	deps := fs.String("mods", "", "folder of other mods, where the mod's dependencies are")
	arg, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	p, err := modkit.ModPath(arg)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() { // #nosec G703 -- a path the user gave on the command line.
		return errors.New("wactl mod pack packs a mod's folder")
	}
	env, err := loadEnv("renewal", "")
	if err != nil {
		return err
	}
	if rep := env.Validate(context.Background(), p, *deps); len(rep.Problems) > 0 {
		_ = printReport(p, rep, false)
		return errors.New("the mod does not pass wactl mod validate, so it is not packed")
	}
	zipPath, sumPath, sum, err := modkit.Pack(p, *out)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s\nwrote %s\nSHA-256 %s\n", zipPath, sumPath, sum)
	return nil
}

// modInstall is wactl mod install <source>.
func modInstall(args []string) error {
	fs := newFlags("mod install")
	var o modkit.InstallOptions
	fs.StringVar(&o.ModsDir, "mods-dir", "", "the server's mods folder (mods.dir)")
	fs.StringVar(&o.Ref, "ref", "", "tag, branch or commit to check out of a Git source")
	fs.StringVar(&o.SHA256, "sha256", "", "expected SHA-256 of a .zip file; without it, <file>.sha256 is used when it exists")
	fs.StringVar(&o.PublicKey, "public-key", "", "minisign public key, or a file with one; <file>.minisig must then match")
	fs.BoolVar(&o.Replace, "replace", false, "replace an installed mod of the same ID")
	src, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if src == "" {
		return errors.New("usage: wactl mod install <git URL | .zip file | folder> --mods-dir <folder>")
	}
	o.Source = src
	env, err := loadEnv("renewal", "")
	if err != nil {
		return err
	}
	return env.Install(context.Background(), o, os.Stdout)
}

// modTypes is wactl mod types [path].
func modTypes(args []string) error {
	fs := newFlags("mod types")
	arg, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	p, err := modkit.ModPath(arg)
	if err != nil {
		return err
	}
	consts, err := loadConstants()
	if err != nil {
		return err
	}
	n, err := modkit.WriteTypes(p, consts)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %d files to %s, and %s if it was missing\n", n, filepath.Join(p, modkit.TypesDir), modkit.LuaRCFile)
	return nil
}

// modBuildArt is wactl mod build-art [<folder>]: it builds a folder
// mod's art/ into its assets/, which the server reads (decision 0033).
func modBuildArt(args []string) error {
	fs := newFlags("mod build-art")
	out := fs.String("out", "", "folder to build into; the mod's assets/ by default")
	fast := fs.Bool("fast", false, "use a low Brotli level, for development builds")
	arg, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	p, err := modkit.ModPath(arg)
	if err != nil {
		return err
	}
	quality := 11
	if *fast {
		quality = 5
	}
	rep, err := modkit.BuildArt(p, *out, quality)
	if rep != nil {
		for _, r := range rep.Rejected {
			fmt.Println("rejected:", r)
		}
		fmt.Printf("build %s: %d assets, %d converted, %d from cache, %.1f MB, %s\n",
			rep.BuildID, rep.Assets, rep.Converted, rep.Cached, float64(rep.Bytes)/(1<<20), rep.Duration.Round(time.Millisecond))
	}
	return err
}
