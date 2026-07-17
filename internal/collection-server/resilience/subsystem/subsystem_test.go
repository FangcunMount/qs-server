package subsystem

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/options"
)

func TestSubsystemOwnsBudgetsAndGates(t *testing.T) {
	opts := options.NewOptions()
	s := New(Options{RateLimit: opts.RateLimit, Concurrency: opts.Concurrency, WaitReport: opts.WaitReport})
	left, ok := s.Budget(BudgetReportEvents)
	if !ok {
		t.Fatal("report events budget unavailable")
	}
	right, _ := s.Budget(BudgetReportEvents)
	if left.Global != right.Global || left.User != right.User {
		t.Fatal("report events callers must share stable limiter proxies")
	}
	if s.Gate(GateQuery) == nil || s.Gate(GateSubmit) == nil || s.Gate(GateWaitReport) == nil {
		t.Fatal("expected process-owned concurrency gates")
	}
	if snapshot := s.Snapshot(time.Now()); len(snapshot.RateLimits) != 8 || snapshot.InstanceID == "" {
		t.Fatalf("Snapshot() = %+v", snapshot)
	}
}
