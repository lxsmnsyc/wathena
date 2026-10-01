package metrics

import (
	"testing"
	"time"
)

func TestBuckets(t *testing.T) {
	prev := int64(-1)
	for i := range nBuckets {
		u := upper(i)
		if u <= prev {
			t.Fatalf("bucket %d ends at %d, not after %d", i, u, prev)
		}
		if bucket(u) != i || bucket(prev+1) != i {
			t.Fatalf("bucket %d holds %d to %d, but bucket() gives %d and %d", i, prev+1, u, bucket(prev+1), bucket(u))
		}
		prev = u
	}
}

func TestSummary(t *testing.T) {
	var h Histogram
	for i := 1; i <= 1000; i++ {
		h.Record(time.Duration(i) * time.Microsecond)
	}
	s := h.Summary()
	if s.Count != 1000 || s.Max != time.Millisecond {
		t.Fatalf("count %d, max %v", s.Count, s.Max)
	}
	check := func(name string, got, want time.Duration) {
		t.Helper()
		if got < want || float64(got) > float64(want)*1.125 {
			t.Errorf("%s is %v, want %v to 12.5%% more", name, got, want)
		}
	}
	check("p50", s.P50, 500*time.Microsecond)
	check("p99", s.P99, 990*time.Microsecond)
	check("mean", s.Mean, 500*time.Microsecond)
	h.Reset()
	if s := h.Summary(); s.Count != 0 || s.Max != 0 {
		t.Errorf("after Reset: %+v", s)
	}
}
