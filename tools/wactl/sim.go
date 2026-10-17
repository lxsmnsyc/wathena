package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/lxsmnsyc/wathena/server/economy"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// simEconomy is `wactl sim economy` (strategy section 8.13, decision
// 0046): it makes a procedural world in memory, as `wactl world new`
// does without a database, and prints each field's exp, zeny and
// supplies per hour for the model player of its band, against the
// band's targets. It starts no server.
func simEconomy(args []string) error {
	fs := flag.NewFlagSet("sim economy", flag.ContinueOnError)
	seedText := fs.String("seed", "", "the world's seed as text, such as test-1; empty draws a random seed")
	config := fs.String("config", "", "waserver.yml, for the mods, procedural.layout and spawns")
	ruleset := fs.String("ruleset", "renewal", "the ruleset: renewal or pre-renewal")
	floors := fs.Bool("floors", false, "also print the dungeon floors, which the checks leave out")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if !gen.Procedural {
		return errors.New("this wactl was built without the procedural tag; sim economy needs procedural worlds")
	}
	g, ok := gen.Latest()
	if !ok {
		return errors.New("this build has no map generator")
	}
	seed := []byte(*seedText)
	if *seedText == "" {
		seed = make([]byte, 32)
		if _, err := rand.Read(seed); err != nil {
			return err
		}
		fmt.Printf("seed: %s (hex)\n", hex.EncodeToString(seed))
	}
	pw, err := proceduralWorld(*config, *ruleset, seed, g.Version)
	if err != nil {
		return err
	}
	return printEconomy(os.Stdout, pw, *floors)
}

// printEconomy writes the economy table of a made world. It returns an
// error when a field misses its band's targets.
func printEconomy(out io.Writer, pw *madeWorld, floors bool) error {
	w, ts := pw.econ, pw.targets
	tol := w.Config.Tolerance.Generator
	lists := map[string]int{}
	for i, p := range pw.spawns {
		lists[p.Map] = i
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintln(tw, "map\tband\tlevel\tkills/h\tbase exp/h\t%\tjob exp/h\tzeny/h\t%\tsupplies/h\tcheck\t")
	missed := 0
	for _, m := range pw.maps {
		if m.Kind != "field" && !floors {
			continue
		}
		i, ok := lists[m.ID]
		if !ok {
			continue
		}
		level := economy.Middle(m.Band)
		r, err := w.Simulate(level, gamedata.EconomyField(m, pw.spawns[i].Spawns))
		if err != nil {
			return err
		}
		t := ts.At(level)
		check := "pass"
		switch {
		case m.Kind != "field":
			check = "floor"
		case len(economy.Check(r, t, tol)) > 0:
			check = "MISS"
			missed++
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d-%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%s\t\n", m.ID, m.Band[0], m.Band[1], level, r.Kills,
			r.BaseExp, economy.Ratio(r.BaseExp, t.BaseExp), r.JobExp, r.Zeny, economy.Ratio(r.Zeny, t.Zeny), r.Supplies, check)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "targets (rAthena's classic fields, the model player of each band's middle level, within %d%%):\n", tol)
	for _, t := range ts {
		_, _ = fmt.Fprintf(out, "  levels %d-%d: base exp %d/h, job exp %d/h, zeny %d/h, supplies %d/h, from %d fields\n",
			t.Levels[0], t.Levels[1], t.BaseExp, t.JobExp, t.Zeny, t.Supplies, t.Fields)
	}
	if missed > 0 {
		return fmt.Errorf("%d fields miss their band's targets by more than %d%%", missed, tol)
	}
	return nil
}
