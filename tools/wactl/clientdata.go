package main

import (
	"errors"
	"fmt"
	"os"

	clientdata "github.com/lxsmnsyc/wathena/client/data"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modassets"
	"github.com/lxsmnsyc/wathena/tools/modkit"
)

// clientDataValidate is wactl client-data validate [<mod folder>...]:
// it checks core's client data tables, merged with each named mod's
// client/*.yml in the order given, against their schema (strategy
// section 10.5, docs/client-data.md).
func clientDataValidate(args []string) error {
	fs := newFlags("client-data validate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dirs := fs.Args()
	var overrides []clientdata.Override
	for _, d := range dirs {
		p, err := modkit.ModPath(d)
		if err != nil {
			return err
		}
		m, err := mod.Read(p)
		if err != nil {
			return err
		}
		fsys, closeFS, err := m.Open()
		if err != nil {
			return err
		}
		tables, err := modassets.ClientTables(fsys, m.ID())
		_ = closeFS()
		if err != nil {
			return err
		}
		for _, t := range tables {
			fmt.Printf("%s: client/%s\n", m.ID(), t.File)
		}
		overrides = append(overrides, tables...)
	}
	if _, err := clientdata.Merge(overrides); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return errFailed
	}
	if len(dirs) == 0 {
		fmt.Println("core's client data tables are valid")
		return nil
	}
	if len(overrides) == 0 {
		return errors.New("the mods have no client/*.yml tables")
	}
	fmt.Printf("%d tables valid over core's\n", len(overrides))
	return nil
}
