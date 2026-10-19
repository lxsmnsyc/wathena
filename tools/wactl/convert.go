package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// convertItems converts rAthena's item scripts from the extracts in
// reference/rathena. waserver does the same when it starts. This command
// writes the files for reading, and the coverage report.
func convertItems(args []string) error {
	fs := newFlags("convert items")
	mode := fs.String("mode", "renewal", "rAthena data mode: renewal or pre-renewal")
	out := fs.String("out", "", "folder to write data/items.yml and scripts/items/*.lua into")
	report := fs.String("report", "", "file to write the coverage report into, such as docs/generated/item-scripts-renewal.md")
	_ = fs.Parse(args)
	m := reference.Mode(*mode)
	if m != reference.Renewal && m != reference.PreRenewal {
		return fmt.Errorf("--mode must be renewal or pre-renewal")
	}
	res, err := convert.Reference(m)
	if err != nil {
		return err
	}
	if *out != "" {
		files := res.Files()
		paths := make([]string, 0, len(files))
		for p := range files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			full := filepath.Join(*out, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
				return err
			}
			if err := os.WriteFile(full, []byte(files[p]), 0o600); err != nil {
				return err
			}
		}
		fmt.Printf("wrote %d files to %s\n", len(paths), *out)
	}
	if *report != "" {
		if err := os.WriteFile(*report, []byte(res.Report()), 0o600); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", *report)
	}
	if *out == "" && *report == "" {
		fmt.Print(res.Summary())
	}
	return nil
}

// convertNPCs converts rAthena's NPC scripts and shops from the extracts
// in reference/rathena, as waserver does when it starts. This command
// writes the files for reading, and the coverage report.
func convertNPCs(args []string) error {
	fs := newFlags("convert npcs")
	mode := fs.String("mode", "renewal", "rAthena data mode: renewal or pre-renewal")
	out := fs.String("out", "", "folder to write data/npcs.yml and scripts/npcs, scripts/functions and scripts/lib into")
	report := fs.String("report", "", "file to write the coverage report into, such as docs/generated/npc-scripts-renewal.md")
	_ = fs.Parse(args)
	m := reference.Mode(*mode)
	if m != reference.Renewal && m != reference.PreRenewal {
		return fmt.Errorf("--mode must be renewal or pre-renewal")
	}
	core, err := convert.LoadCore(m)
	if err != nil {
		return err
	}
	res := core.NPCs
	if *out != "" {
		files := res.Files()
		paths := make([]string, 0, len(files))
		for p := range files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			full := filepath.Join(*out, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
				return err
			}
			if err := os.WriteFile(full, []byte(files[p]), 0o600); err != nil {
				return err
			}
		}
		fmt.Printf("wrote %d files to %s\n", len(paths), *out)
	}
	if *report != "" {
		if err := os.WriteFile(*report, []byte(res.NPCReport()), 0o600); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", *report)
	}
	if *out == "" && *report == "" {
		fmt.Println(res.NPCSummary())
	}
	return nil
}

// convertClassic writes the release notes list of classic mode (review
// decision 55): what does not convert in a classic world with mode
// renewal, from the converter's NPC coverage. Classic ships with
// renewal, so the list is renewal's.
func convertClassic(args []string) error {
	fs := newFlags("convert classic")
	report := fs.String("report", "", "file to write the list into, such as "+convert.ClassicNotesPath)
	_ = fs.Parse(args)
	core, err := convert.LoadCore(reference.Renewal)
	if err != nil {
		return err
	}
	notes := core.NPCs.ClassicNotes()
	if *report == "" {
		fmt.Print(notes)
		return nil
	}
	if err := os.WriteFile(*report, []byte(notes), 0o600); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", *report)
	return nil
}

// convertMonsters converts rAthena's monster skills from the extracts in
// reference/rathena, as waserver does when it starts. This command writes
// core's data/monsters.yml for reading, and the coverage report.
func convertMonsters(args []string) error {
	fs := newFlags("convert monsters")
	mode := fs.String("mode", "renewal", "rAthena data mode: renewal or pre-renewal")
	out := fs.String("out", "", "folder to write data/monsters.yml into")
	report := fs.String("report", "", "file to write the coverage report into, such as docs/generated/monster-skills-renewal.md")
	_ = fs.Parse(args)
	m := reference.Mode(*mode)
	if m != reference.Renewal && m != reference.PreRenewal {
		return fmt.Errorf("--mode must be renewal or pre-renewal")
	}
	core, err := convert.LoadCore(m)
	if err != nil {
		return err
	}
	res := core.MonsterSkills
	if *out != "" {
		for p, content := range res.Files() {
			full := filepath.Join(*out, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
				return err
			}
			if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
				return err
			}
		}
		fmt.Printf("wrote data/monsters.yml to %s\n", *out)
	}
	if *report != "" {
		if err := os.WriteFile(*report, []byte(res.MonsterReport()), 0o600); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", *report)
	}
	if *out == "" && *report == "" {
		fmt.Println(res.MonsterSummary())
	}
	return nil
}

// convertRandomOptions converts rAthena's random options and their groups
// from the extracts in reference/rathena, as waserver does when it
// starts. This command writes core's data/random_options.yml and
// data/random_option_groups.yml for reading, and the coverage report.
func convertRandomOptions(args []string) error {
	fs := newFlags("convert random-options")
	mode := fs.String("mode", "renewal", "rAthena data mode: renewal or pre-renewal")
	out := fs.String("out", "", "folder to write data/random_options.yml and data/random_option_groups.yml into")
	report := fs.String("report", "", "file to write the coverage report into, such as docs/generated/random-options-renewal.md")
	_ = fs.Parse(args)
	m := reference.Mode(*mode)
	if m != reference.Renewal && m != reference.PreRenewal {
		return fmt.Errorf("--mode must be renewal or pre-renewal")
	}
	core, err := convert.LoadCore(m)
	if err != nil {
		return err
	}
	res := core.RandomOptions
	if *out != "" {
		for p, content := range res.Files() {
			full := filepath.Join(*out, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
				return err
			}
			if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
				return err
			}
		}
		fmt.Printf("wrote the random options to %s\n", *out)
	}
	if *report != "" {
		if err := os.WriteFile(*report, []byte(res.Report(string(m))), 0o600); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", *report)
	}
	if *out == "" && *report == "" {
		fmt.Printf("%d options converted, %d not\n", len(res.Options), len(res.Failed))
	}
	return nil
}

// convertQuests converts rAthena's quests from the extracts in
// reference/rathena, as waserver does when it starts. This command writes
// core's data/quests.yml for reading, and the coverage report.
func convertQuests(args []string) error {
	fs := newFlags("convert quests")
	mode := fs.String("mode", "renewal", "rAthena data mode: renewal or pre-renewal")
	out := fs.String("out", "", "folder to write data/quests.yml into")
	report := fs.String("report", "", "file to write the coverage report into, such as docs/generated/quests-renewal.md")
	_ = fs.Parse(args)
	m := reference.Mode(*mode)
	if m != reference.Renewal && m != reference.PreRenewal {
		return fmt.Errorf("--mode must be renewal or pre-renewal")
	}
	core, err := convert.LoadCore(m)
	if err != nil {
		return err
	}
	res := core.Quests
	if *out != "" {
		for p, content := range res.Files() {
			full := filepath.Join(*out, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
				return err
			}
			if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
				return err
			}
		}
		fmt.Printf("wrote data/quests.yml to %s\n", *out)
	}
	if *report != "" {
		if err := os.WriteFile(*report, []byte(res.Report()), 0o600); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", *report)
	}
	if *out == "" && *report == "" {
		fmt.Printf("%d quests converted, %d left out\n", len(res.Records), len(res.Skipped))
	}
	return nil
}
