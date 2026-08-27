package evaluation

import (
	"testing"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPromptEvaluationStartAdmissionMetricsUseBoundedOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		result string
		err    error
	}{
		{name: "accepted", result: "accepted"},
		{name: "daily budget", result: "daily_budget_exceeded", err: domainevaluation.ErrDailyBudgetExceeded},
		{name: "organization concurrency", result: "org_concurrency_exceeded", err: domainevaluation.ErrOrgConcurrencyExceeded},
		{name: "internal error", result: "error", err: domainevaluation.ErrConflict},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			before := testutil.ToFloat64(promptEvaluationStartAdmissionTotal.WithLabelValues(testCase.result))
			reservedBefore := testutil.ToFloat64(promptEvaluationReservedProviderInvocationsTotal)
			observePromptEvaluationStartAdmission(testCase.err)
			if delta := testutil.ToFloat64(promptEvaluationStartAdmissionTotal.WithLabelValues(testCase.result)) - before; delta != 1 {
				t.Fatalf("admission metric delta = %v", delta)
			}
			wantReserved := float64(0)
			if testCase.err == nil {
				wantReserved = MaxProviderInvocationsV1
			}
			if delta := testutil.ToFloat64(promptEvaluationReservedProviderInvocationsTotal) - reservedBefore; delta != wantReserved {
				t.Fatalf("reserved invocation metric delta = %v, want %v", delta, wantReserved)
			}
		})
	}
}
