package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/tools/modkit"
)

// wactl dev runs a local waserver with one mod and watches the mod's
// files (strategy section 6.9):
//   - The server runs with --dev on loopback addresses, with the mod and
//     the mods of --mods, and approves what they request.
//   - A change to a Lua file or the locale reloads the mod's scripts in
//     every zone, through the server's reload endpoint.
//   - A change to mod.yaml or a data file restarts the server.
//   - Other changes, such as to tests, are shown and change nothing.

// devOptions are the settings of wactl dev.
type devOptions struct {
	modPath, deps, server, assets, startMap, addr, debugAddr string
}

func dev(args []string) error {
	fs := newFlags("dev")
	var o devOptions
	fs.StringVar(&o.deps, "mods", "", "folder of other mods to load with it, such as its dependencies")
	fs.StringVar(&o.server, "server", "", "waserver binary; by default the one next to wactl, then the one on PATH")
	fs.StringVar(&o.assets, "assets", os.Getenv("WATHENA_ASSETS"), "asset build folder; $WATHENA_ASSETS by default")
	fs.StringVar(&o.startMap, "map", "prontera", "map where characters start")
	fs.StringVar(&o.addr, "addr", "127.0.0.1:8300", "loopback address of the game")
	fs.StringVar(&o.debugAddr, "debug-addr", "127.0.0.1:8301", "loopback address of the reload endpoint")
	arg, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if o.modPath, err = modkit.ModPath(arg); err != nil {
		return err
	}
	if fi, err := os.Stat(o.modPath); err != nil || !fi.IsDir() {
		return errors.New("wactl dev runs a mod's folder")
	}
	if o.assets == "" {
		return errors.New("name an asset build with --assets, or set WATHENA_ASSETS")
	}
	if o.server, err = findServer(o.server); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runDev(ctx, o, os.Stdout)
}

// findServer finds the waserver binary.
func findServer(given string) (string, error) {
	if given != "" {
		return given, nil
	}
	if self, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(self), "waserver")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("waserver"); err == nil {
		return p, nil
	}
	return "", errors.New("no waserver binary next to wactl or on PATH; build one with go build ./server/cmd/waserver, or name it with --server")
}

// devFolder makes the server's mods folder: links to the mod and to the
// other mods, and a config that approves what they request.
func devFolder(o devOptions) (dir, config, id string, err error) {
	m, err := mod.Read(o.modPath)
	if err != nil {
		return "", "", "", err
	}
	id = m.ID()
	dir, err = os.MkdirTemp("", "wactl-dev-")
	if err != nil {
		return "", "", "", err
	}
	mods := filepath.Join(dir, "mods")
	if err := os.Mkdir(mods, 0o750); err != nil {
		return "", "", "", err
	}
	list := []*mod.Mod{m}
	if o.deps != "" {
		others, err := mod.Discover(o.deps)
		if err != nil {
			return "", "", "", err
		}
		for _, other := range others {
			if other.ID() != id {
				list = append(list, other)
			}
		}
	}
	var approve strings.Builder
	for _, x := range list {
		abs, err := filepath.Abs(x.Path)
		if err != nil {
			return "", "", "", err
		}
		link := filepath.Join(mods, x.ID()+filepath.Ext(abs))
		if err := os.Symlink(abs, link); err != nil {
			return "", "", "", err
		}
		var perms []string
		for _, p := range x.Manifest.Permissions {
			if perm, _ := mod.LookupPermission(p); perm.Approval {
				perms = append(perms, p)
			}
		}
		if len(perms) > 0 {
			say(&approve, "    %s: [%s]\n", x.ID(), strings.Join(perms, ", "))
		}
	}
	cfg := "# Written by wactl dev. It approves what the mods request, for\n# development only.\nmods:\n  dir: " + mods + "\n"
	if approve.Len() > 0 {
		cfg += "  approve:\n" + approve.String()
	}
	config = filepath.Join(dir, "waserver.yml")
	return dir, config, id, os.WriteFile(config, []byte(cfg), 0o600)
}

func runDev(ctx context.Context, o devOptions, out io.Writer) error {
	dir, config, id, err := devFolder(o)
	if dir != "" {
		defer func() { _ = os.RemoveAll(dir) }()
	}
	if err != nil {
		return err
	}
	say(out, "wactl dev: %s from %s, server %s\n", id, o.modPath, o.server)
	start := func() *server {
		cmd := exec.Command(o.server, "all", "--config", config, "--dev", "--addr", o.addr, "--debug-addr", o.debugAddr, // #nosec G204 -- the binary the developer named.
			"--assets", o.assets, "--map", o.startMap, "--shutdown-countdown", "0")
		cmd.Stdout, cmd.Stderr = out, out
		if err := cmd.Start(); err != nil {
			say(out, "wactl dev: the server did not start: %v\n", err)
			return nil
		}
		s := &server{cmd: cmd, exited: make(chan struct{})}
		go func() {
			_ = cmd.Wait()
			close(s.exited)
		}()
		if s.waitReady(ctx, "http://"+o.addr+"/readyz") {
			say(out, "wactl dev: ready at http://%s; editing %s reloads it\n", o.addr, o.modPath)
		} else if ctx.Err() == nil {
			say(out, "%s\n", "wactl dev: the server stopped; fix the mod, and a change starts it again")
		}
		return s
	}
	srv := start()
	defer func() { srv.stop() }()
	files := snapshot(o.modPath)
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			say(out, "%s\n", "wactl dev: stopping")
			return nil
		case <-tick.C:
		}
		now := snapshot(o.modPath)
		changed := changes(files, now)
		if len(changed) == 0 {
			continue
		}
		// Editors write files in several steps: wait until it settles.
		time.Sleep(200 * time.Millisecond)
		now = snapshot(o.modPath)
		changed = changes(files, now)
		files = now
		action := "ignored"
		for _, c := range changed {
			switch {
			case c == mod.ManifestFile || strings.HasPrefix(c, "data/"):
				action = "restart"
			case (strings.HasPrefix(c, "scripts/") || c == mod.LocaleFile) && action != "restart":
				action = "reload"
			}
		}
		say(out, "wactl dev: changed %s\n", strings.Join(changed, ", "))
		switch action {
		case "restart":
			say(out, "%s\n", "wactl dev: the manifest or data changed: restarting the server")
			srv.stop()
			srv = start()
		case "reload":
			if !srv.running() {
				say(out, "%s\n", "wactl dev: the server is not running: starting it")
				srv = start()
				continue
			}
			if problems := syntaxErrors(o.modPath, changed); len(problems) > 0 {
				say(out, "wactl dev: not reloaded, the Lua does not compile:\n  %s\n", strings.Join(problems, "\n  "))
				continue
			}
			say(out, "%s\n", "wactl dev: "+reload(ctx, o.debugAddr, id))
		default:
			say(out, "%s\n", "wactl dev: nothing the server reads changed")
		}
	}
}

// server is a running waserver.
type server struct {
	cmd *exec.Cmd
	// exited closes when the process has exited.
	exited chan struct{}
}

func (s *server) running() bool {
	if s == nil {
		return false
	}
	select {
	case <-s.exited:
		return false
	default:
		return true
	}
}

// syntaxErrors compiles the changed Lua files.
func syntaxErrors(dir string, changed []string) []string {
	files := map[string]string{}
	for _, c := range changed {
		if strings.HasSuffix(c, ".lua") {
			if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(c))); err == nil { // #nosec G304 -- a file of the mod.
				files[c] = string(b)
			}
		}
	}
	problems, err := modkit.CheckSyntax(files)
	if err != nil {
		return []string{err.Error()}
	}
	return problems
}

// waitReady waits until the server answers /readyz. It returns false
// when the server exits first, or after a minute.
func (s *server) waitReady(ctx context.Context, url string) bool {
	for range 600 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return false
		}
		if resp, err := http.DefaultClient.Do(req); err == nil { // #nosec G107 G704 -- the loopback address of the dev server.
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		select {
		case <-s.exited:
			return false
		case <-ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
	return false
}

// reload asks the server to reload a mod's scripts, and says what it
// answered.
func reload(ctx context.Context, debugAddr, id string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+debugAddr+"/debug/wathena/mods/reload?mod="+id, nil)
	if err != nil {
		return "reload failed: " + err.Error()
	}
	resp, err := http.DefaultClient.Do(req) // #nosec G107 G704 -- the loopback address of the dev server.
	if err != nil {
		return "reload failed: " + err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	text := strings.TrimSpace(string(body))
	if resp.StatusCode != http.StatusOK {
		return "reload failed: " + text
	}
	return text
}

// stop stops the server with an interrupt, and kills it when it takes
// more than 10 seconds.
func (s *server) stop() {
	if !s.running() {
		return
	}
	_ = s.cmd.Process.Signal(os.Interrupt)
	select {
	case <-s.exited:
	case <-time.After(10 * time.Second):
		_ = s.cmd.Process.Kill()
		<-s.exited
	}
}

// fileState is what a change to a file changes.
type fileState struct {
	size int64
	mod  time.Time
}

// snapshot reads the size and time of every file of a mod, by path.
// Names that start with "." are skipped.
func snapshot(dir string) map[string]fileState {
	out := map[string]fileState{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p != dir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = fileState{info.Size(), info.ModTime()}
		return nil
	})
	return out
}

// changes lists the files added, changed or removed between snapshots.
func changes(before, after map[string]fileState) []string {
	var out []string
	for p, s := range after {
		if b, ok := before[p]; !ok || b != s {
			out = append(out, p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out
}

// say writes to a writer of messages, where a failed write has no one
// to tell.
func say(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
