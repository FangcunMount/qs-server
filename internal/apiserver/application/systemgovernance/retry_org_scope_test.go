package systemgovernance

import (
	"context"
	"testing"
	"time"
)

type retryGovernanceReaderStub struct {
	orgID int64
	calls int
}

func (s *retryGovernanceReaderStub) ReadRetryGovernance(_ context.Context, orgID int64) (RetryGovernanceSummary, error) {
	s.orgID = orgID
	s.calls++
	return RetryGovernanceSummary{ManualRequired: 2, HeldAutomatic: 1}, nil
}

func TestEventGovernanceRetrySummaryUsesExplicitOrganization(t *testing.T) {
	reader := &retryGovernanceReaderStub{}
	collector := eventGovernanceCollector{retry: reader}
	view, err := collector.Collect(t.Context(), evaluationContext{windowLabel: "5m", evalAt: time.Now()}, 88)
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || reader.orgID != 88 || view.Retry.ManualRequired != 2 || view.Retry.HeldAutomatic != 1 {
		t.Fatalf("reader/view=%#v %#v", reader, view.Retry)
	}
}

func TestOverviewCollectorDoesNotExposeGlobalRetryCounts(t *testing.T) {
	reader := &retryGovernanceReaderStub{}
	collector := eventGovernanceCollector{retry: reader}
	view, err := collector.Collect(t.Context(), evaluationContext{windowLabel: "5m", evalAt: time.Now()}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 0 || view.Retry != (RetryGovernanceSummary{}) {
		t.Fatalf("reader calls=%d retry=%#v", reader.calls, view.Retry)
	}
}
