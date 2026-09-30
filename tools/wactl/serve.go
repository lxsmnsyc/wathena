package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/lxsmnsyc/wathena/internal/assets/serve"
)

func assetsServe(args []string) error {
	fs := newFlags("assets serve")
	dir := fs.String("build", "", "build folder to serve")
	addr := fs.String("addr", "127.0.0.1:8200", "address to listen on")
	_ = fs.Parse(args)
	if *dir == "" {
		return errors.New("--build is required")
	}
	h, err := serve.New(*dir)
	if err != nil {
		return err
	}
	defer func() { _ = h.Close() }()
	srv := &http.Server{
		Addr:              *addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Printf("serving %s on http://%s\n", *dir, *addr)
	return srv.ListenAndServe()
}
