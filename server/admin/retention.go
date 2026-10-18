package admin

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/retention"
)

// GET /retention (world.read) shows the retention periods in effect and
// what the retention job did in this process (decision 0049). It is
// read-only, and writes no audit row.

// Retention is what the API needs from the retention job.
// *retention.Job is one.
type Retention interface {
	Status() retention.Status
}

func (h *Handler) retention(_ context.Context, w http.ResponseWriter, _ *http.Request, _ caller) {
	if h.cfg.Retention == nil {
		h.fail(w, http.StatusServiceUnavailable, "unavailable", "The retention job does not run in this process.")
		return
	}
	h.write(w, http.StatusOK, RetentionStatus(h.cfg.Retention.Status()))
}

// RetentionStatus is GET /retention's answer for a job's status.
func RetentionStatus(s retention.Status) *adminv1.RetentionStatus {
	b := adminv1.RetentionStatus_builder{LastError: s.LastError, Skipped: s.Skipped,
		PartitionsDropped: int32(min(s.Partitions, 1<<30)), Warnings: s.Warnings, // #nosec G115 -- bounded.
		LastRun: stamp(s.LastRun), LastSuccess: stamp(s.LastSuccess), NextRun: stamp(s.NextRun)}
	for _, k := range retention.Kinds {
		b.Kinds = append(b.Kinds, adminv1.RetentionKind_builder{
			Kind: k, Days: int32(s.Periods.Days(k)), DefaultDays: int32(retention.Defaults.Days(k)), // #nosec G115 -- at most 3650.
			LastDeleted: s.Last[k], DeletedSinceStart: s.Total[k],
		}.Build())
	}
	return b.Build()
}

// stamp is a time as a timestamp, or nil for the zero time.
func stamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
