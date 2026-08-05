package catalogreconcile

import (
	"context"
	"testing"
	"time"
)

type countingReconcileService struct {
	calls  int
	counts DriftCounts
}

func (s *countingReconcileService) ReconcileOnce(context.Context, Filter) (DriftCounts, error) {
	s.calls++
	return s.counts, nil
}

func TestScheduledAuditorDefersStartupAndThrottlesLaterTicks(t *testing.T) {
	service := &countingReconcileService{counts: DriftCounts{Dangling: 2}}
	now := time.Unix(1000, 0)
	auditor := newScheduledAuditor(service, 10*time.Minute, func() time.Time { return now })

	count, err := auditor.AuditOnce(context.Background(), 100)
	if err != nil || count != 0 || service.calls != 0 {
		t.Fatalf("startup audit count=%d calls=%d err=%v", count, service.calls, err)
	}
	if count, err = auditor.AuditOnce(context.Background(), 100); err != nil || count != 0 || service.calls != 0 {
		t.Fatalf("startup throttled audit count=%d calls=%d err=%v", count, service.calls, err)
	}
	now = now.Add(10 * time.Minute)
	if count, err = auditor.AuditOnce(context.Background(), 100); err != nil || count != 2 || service.calls != 1 {
		t.Fatalf("first scheduled audit count=%d calls=%d err=%v", count, service.calls, err)
	}
	if count, err = auditor.AuditOnce(context.Background(), 100); err != nil || count != 0 || service.calls != 1 {
		t.Fatalf("later throttled audit count=%d calls=%d err=%v", count, service.calls, err)
	}
	now = now.Add(10 * time.Minute)
	if _, err = auditor.AuditOnce(context.Background(), 100); err != nil || service.calls != 2 {
		t.Fatalf("next scheduled interval calls=%d err=%v", service.calls, err)
	}
}
