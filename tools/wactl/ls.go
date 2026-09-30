package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
)

func assetsList(args []string) error {
	fs := newFlags("assets ls")
	dir := fs.String("build", "", "build folder to list")
	_ = fs.Parse(args)
	if *dir == "" {
		return errors.New("--build is required")
	}
	entries, err := build.List(*dir)
	if err != nil {
		return err
	}
	prefixes := fs.Args()
	names := make([]string, 0, len(entries))
	for n := range entries {
		if len(prefixes) == 0 || hasAnyPrefix(n, prefixes) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		e := entries[n]
		fmt.Printf("%-8s %9d  %s\n", e.Type, e.Size, n)
	}
	return nil
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
