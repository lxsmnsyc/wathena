package metrics

import (
	"math"
	"runtime/metrics"
	"sync"
	"time"
)

// Runtime metric names that Process reads.
const (
	mGCPauses    = "/sched/pauses/total/gc:seconds"
	mLatencies   = "/sched/latencies:seconds"
	mGCCycles    = "/gc/cycles/total:gc-cycles"
	mGCCPU       = "/cpu/classes/gc/total:cpu-seconds"
	mHeapObjects = "/memory/classes/heap/objects:bytes"
	mTotalMemory = "/memory/classes/total:bytes"
	mGoroutines  = "/sched/goroutines:goroutines"
	mAllocs      = "/gc/heap/allocs:bytes"
	mMemLimit    = "/gc/gomemlimit:bytes"
)

var processMetrics = []string{mGCPauses, mLatencies, mGCCycles, mGCCPU, mHeapObjects, mTotalMemory, mGoroutines, mAllocs, mMemLimit}

// Process measures the whole process: CPU, memory, goroutines and the
// garbage collector. Counts that grow, such as GC pauses, start again
// at Reset.
type Process struct {
	mu   sync.Mutex
	base processBase
}

type processBase struct {
	at        time.Time
	cpu       time.Duration
	gcCycles  uint64
	gcCPU     float64
	allocs    uint64
	pauses    []uint64
	latencies []uint64
}

func readRuntime() []metrics.Sample {
	s := make([]metrics.Sample, len(processMetrics))
	for i, n := range processMetrics {
		s[i].Name = n
	}
	metrics.Read(s)
	return s
}

func (p *Process) current() processBase {
	s := readRuntime()
	b := processBase{at: time.Now(), cpu: processCPU()}
	for _, v := range s {
		switch v.Name {
		case mGCCycles:
			b.gcCycles = v.Value.Uint64()
		case mGCCPU:
			b.gcCPU = v.Value.Float64()
		case mAllocs:
			b.allocs = v.Value.Uint64()
		case mGCPauses:
			b.pauses = v.Value.Float64Histogram().Counts
		case mLatencies:
			b.latencies = v.Value.Float64Histogram().Counts
		}
	}
	return b
}

// Reset starts the counts that grow again from now.
func (p *Process) Reset() {
	b := p.current()
	p.mu.Lock()
	p.base = b
	p.mu.Unlock()
}

// ProcessSnapshot is the process's measurements since the last reset.
type ProcessSnapshot struct {
	Seconds float64 `json:"seconds"`
	// CPU is the process's user and system CPU time per second of wall
	// time, so 1.0 is one core. It is 0 where the system does not report
	// it.
	CPU float64 `json:"cpu_cores"`
	// GCCPU is the garbage collector's share of CPU, as the runtime
	// estimates it, in cores.
	GCCPU      float64 `json:"gc_cpu_cores"`
	GCCycles   uint64  `json:"gc_cycles"`
	GCPauses   Summary `json:"gc_pauses"`
	SchedDelay Summary `json:"sched_latency"`
	// AllocRate is heap bytes allocated per second.
	AllocRate   float64 `json:"alloc_bytes_per_sec"`
	HeapBytes   uint64  `json:"heap_bytes"`
	TotalBytes  uint64  `json:"total_bytes"`
	MemoryLimit uint64  `json:"memory_limit"`
	Goroutines  uint64  `json:"goroutines"`
}

// Snapshot returns the measurements since the last reset.
func (p *Process) Snapshot() ProcessSnapshot {
	p.mu.Lock()
	base := p.base
	p.mu.Unlock()
	now := p.current()
	s := readRuntime()
	out := ProcessSnapshot{Seconds: now.at.Sub(base.at).Seconds()}
	if base.at.IsZero() {
		out.Seconds = 0
	}
	if out.Seconds > 0 {
		out.CPU = (now.cpu - base.cpu).Seconds() / out.Seconds
		out.GCCPU = (now.gcCPU - base.gcCPU) / out.Seconds
		out.AllocRate = float64(now.allocs-base.allocs) / out.Seconds
	}
	out.GCCycles = now.gcCycles - base.gcCycles
	for _, v := range s {
		switch v.Name {
		case mHeapObjects:
			out.HeapBytes = v.Value.Uint64()
		case mTotalMemory:
			out.TotalBytes = v.Value.Uint64()
		case mGoroutines:
			out.Goroutines = v.Value.Uint64()
		case mMemLimit:
			out.MemoryLimit = v.Value.Uint64()
		case mGCPauses:
			out.GCPauses = histSummary(v.Value.Float64Histogram(), base.pauses)
		case mLatencies:
			out.SchedDelay = histSummary(v.Value.Float64Histogram(), base.latencies)
		}
	}
	return out
}

// histSummary summarizes a runtime histogram less the counts in base. A
// quantile is the upper bound of its bucket. Max is the upper bound of
// the highest bucket with a count, or its lower bound when the bucket
// has no upper bound.
func histSummary(h *metrics.Float64Histogram, base []uint64) Summary {
	counts := make([]uint64, len(h.Counts))
	var n uint64
	var sum float64
	for i, c := range h.Counts {
		if i < len(base) {
			c -= base[i]
		}
		counts[i] = c
		n += c
		sum += float64(c) * mid(h.Buckets[i], h.Buckets[i+1])
	}
	s := Summary{Count: n}
	if n == 0 {
		return s
	}
	bound := func(i int) time.Duration {
		v := h.Buckets[i+1]
		if math.IsInf(v, 1) {
			v = h.Buckets[i]
		}
		return time.Duration(v * float64(time.Second))
	}
	for i := len(counts) - 1; i >= 0; i-- {
		if counts[i] > 0 {
			s.Max = bound(i)
			break
		}
	}
	q := func(f float64) time.Duration {
		rank := min(uint64(f*float64(n)), n-1)
		var seen uint64
		for i, c := range counts {
			seen += c
			if seen > rank {
				return bound(i)
			}
		}
		return s.Max
	}
	s.Mean = time.Duration(sum / float64(n) * float64(time.Second))
	s.P50, s.P99, s.P999 = q(0.5), q(0.99), q(0.999)
	return s
}

func mid(lo, hi float64) float64 {
	switch {
	case math.IsInf(lo, -1):
		return max(hi, 0)
	case math.IsInf(hi, 1):
		return lo
	}
	return (lo + hi) / 2
}
