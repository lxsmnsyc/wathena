//go:build unix

package metrics

import (
	"syscall"
	"time"
)

// processCPU is the process's user and system CPU time.
func processCPU() time.Duration {
	var u syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &u) != nil {
		return 0
	}
	return time.Duration(u.Utime.Nano() + u.Stime.Nano())
}
