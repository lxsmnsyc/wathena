package botclient

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// smokeEnv turns on TestLoadSmoke. CI's loadtest job sets it.
const smokeEnv = "WATHENA_LOAD_TEST"

// smokeLimits are the budgets of strategy section 3.8 with slack for
// shared CI runners: tick work gets 2.5 times its p99 budget and 4 times
// its maximum. Bandwidth keeps its budgets.
var smokeLimits = Limits{
	WorkP99: 25 * time.Millisecond, WorkMax: 100 * time.Millisecond,
	OutAvg: 8 << 10, OutPeak: 32 << 10, InAvg: 1 << 10,
}

// TestLoadSmoke builds waserver and the placeholder assets, runs the
// server in its own process, and runs the smoke scenario against it: 50
// bots for 20 seconds on the placeholder map. No bot may fail, and the
// server must stay within smokeLimits.
func TestLoadSmoke(t *testing.T) {
	if os.Getenv(smokeEnv) == "" {
		t.Skip("set " + smokeEnv + "=1 to run the load smoke test")
	}
	dir := t.TempDir()
	assets := filepath.Join(dir, "assets")
	if _, err := build.Run(build.Options{Out: assets, Sources: []build.Source{build.Placeholder{}}, Brotli: 1}); err != nil {
		t.Fatal(err)
	}
	grid, err := world.LoadGrid(assets, "placeholder/map")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "waserver")
	gobuild := exec.Command("go", "build", "-o", bin, "github.com/lxsmnsyc/wathena/server/cmd/waserver") // #nosec G204 -- fixed arguments.
	if out, err := gobuild.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	addr, debug := freeAddr(t), freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := exec.CommandContext(ctx, bin, "--assets", assets, "--map", "placeholder/map", "--dev", "--mode", "none", // #nosec G204 -- a binary built above.
		"--addr", addr, "--debug-addr", debug, "--origins", "")
	srv.Cancel = func() error { return srv.Process.Signal(os.Interrupt) }
	srv.WaitDelay = 10 * time.Second
	stderr, err := srv.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	// The server logs "serving" once it listens.
	serving := make(chan struct{})
	logs := make(chan []string, 1)
	go func() {
		var lines []string
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			lines = append(lines, s.Text())
			if strings.Contains(s.Text(), "msg=serving") {
				close(serving)
			}
		}
		logs <- lines
	}()
	select {
	case <-serving:
	case <-time.After(30 * time.Second):
		t.Fatal("the server did not start")
	}
	time.Sleep(200 * time.Millisecond)

	sc := Scenarios["smoke"]
	sc.Name, sc.URL, sc.MetricsURL = "smoke", "ws://"+addr+"/ws", "http://"+debug
	sc.Behaviour.Grids = func(string) *path.Grid { return grid }
	sc.Progress = func(s string) { t.Log(s) }
	rep, err := sc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + rep.Text())
	cancel()
	_ = srv.Wait()
	server := <-logs
	for _, l := range server {
		if strings.Contains(l, "level=ERROR") {
			t.Errorf("server: %s", l)
		}
	}
	if rep.Joined != sc.Bots || rep.Ack.Count == 0 || rep.ChatsHeard == 0 {
		t.Errorf("joined %d of %d, %d acknowledged commands, %.2f chat lines heard/s", rep.Joined, sc.Bots, rep.Ack.Count, rep.ChatsHeard)
	}
	if rep.Server == nil || len(rep.Server.Zones) != 1 || rep.Server.Zones[0].Players != int64(sc.Bots) {
		t.Fatalf("server measurements: %+v", rep.Server)
	}
	// The ticker may skip a few ticks on a busy runner.
	if z := rep.Server.Zones[0]; z.Ticks < uint64(sc.Duration/zone.Tick)*9/10 {
		t.Errorf("the zone ticked %d times in %s", z.Ticks, sc.Duration)
	}
	for _, v := range rep.Check(smokeLimits) {
		t.Error(v)
	}
}

// freeAddr returns a loopback address with a port that was free.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return fmt.Sprint(l.Addr())
}
