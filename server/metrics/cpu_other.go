//go:build !unix

package metrics

import "time"

// processCPU is not measured on this system.
func processCPU() time.Duration { return 0 }
