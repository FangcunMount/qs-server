package persistence

import (
	"testing"
	"time"

	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestParticipantRequestAdmissionMetricsUseBoundedOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		result string
		err    error
	}{
		{name: "accepted", result: "accepted"},
		{name: "organization", result: "org_daily_budget_exceeded", err: domaingeneration.ErrOrgDailyBudgetExceeded},
		{name: "user", result: "user_daily_budget_exceeded", err: domaingeneration.ErrUserDailyBudgetExceeded},
		{name: "Assessment", result: "assessment_daily_budget_exceeded", err: domaingeneration.ErrAssessmentDailyBudgetExceeded},
		{name: "semantic race", result: "semantic_race_reused", err: domaingeneration.ErrAlreadyExists},
		{name: "error", result: "error", err: domaingeneration.ErrConflict},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			before := testutil.ToFloat64(participantRequestAdmissionTotal.WithLabelValues(testCase.result))
			reservedBefore := testutil.ToFloat64(participantReservedProviderInvocationsTotal)
			observeParticipantRequestAdmission(testCase.err)
			if delta := testutil.ToFloat64(participantRequestAdmissionTotal.WithLabelValues(testCase.result)) - before; delta != 1 {
				t.Fatalf("admission metric delta = %v", delta)
			}
			wantReserved := float64(0)
			if testCase.err == nil {
				wantReserved = 1
			}
			if delta := testutil.ToFloat64(participantReservedProviderInvocationsTotal) - reservedBefore; delta != wantReserved {
				t.Fatalf("reserved invocation metric delta = %v, want %v", delta, wantReserved)
			}
		})
	}
}

func TestParticipantLifecycleMetricsUseBoundedOutcomes(t *testing.T) {
	generatedBefore := testutil.ToFloat64(participantTerminalTotal.WithLabelValues("generated"))
	unknownBefore := testutil.ToFloat64(participantTerminalTotal.WithLabelValues("unknown"))
	now := time.Date(2026, 8, 27, 13, 0, 0, 0, time.UTC)

	observeParticipantExecutionStarted(now, now.Add(2*time.Second))
	observeParticipantTerminal("generated", now, now.Add(5*time.Second))
	observeParticipantTerminal("untrusted-dynamic-outcome", now, now.Add(5*time.Second))

	if delta := testutil.ToFloat64(participantTerminalTotal.WithLabelValues("generated")) - generatedBefore; delta != 1 {
		t.Fatalf("generated terminal metric delta = %v", delta)
	}
	if delta := testutil.ToFloat64(participantTerminalTotal.WithLabelValues("unknown")) - unknownBefore; delta != 1 {
		t.Fatalf("unknown terminal metric delta = %v", delta)
	}
}

func TestParticipantExecutionAdmissionMetricsUseBoundedOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		result string
		err    error
	}{
		{name: "accepted", result: "accepted"},
		{name: "organization", result: "org_active_capacity_exceeded", err: domaingeneration.ErrOrgActiveCapacityExceeded},
		{name: "user", result: "user_active_capacity_exceeded", err: domaingeneration.ErrUserActiveCapacityExceeded},
		{name: "Assessment", result: "assessment_active_capacity_exceeded", err: domaingeneration.ErrAssessmentActiveCapacityExceeded},
		{name: "race", result: "race", err: domaingeneration.ErrConflict},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			before := testutil.ToFloat64(participantExecutionAdmissionTotal.WithLabelValues(testCase.result))
			acquiredBefore := testutil.ToFloat64(participantActiveSlotsAcquiredTotal)
			observeParticipantExecutionAdmission(testCase.err)
			if delta := testutil.ToFloat64(participantExecutionAdmissionTotal.WithLabelValues(testCase.result)) - before; delta != 1 {
				t.Fatalf("execution admission metric delta = %v", delta)
			}
			wantAcquired := float64(0)
			if testCase.err == nil {
				wantAcquired = 1
			}
			if delta := testutil.ToFloat64(participantActiveSlotsAcquiredTotal) - acquiredBefore; delta != wantAcquired {
				t.Fatalf("active slot acquired metric delta = %v, want %v", delta, wantAcquired)
			}
		})
	}
}
