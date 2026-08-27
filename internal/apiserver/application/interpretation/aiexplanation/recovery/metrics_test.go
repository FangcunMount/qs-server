package recovery

import (
	"errors"
	"strings"
	"testing"

	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestParticipantRetryAuthorizationMetricUsesCanonicalAIExplanationName(t *testing.T) {
	description := participantRetryAuthorizationTotal.WithLabelValues("created").Desc().String()
	if !strings.Contains(description, `fqName: "qs_ai_explanation_participant_retry_authorizations_total"`) {
		t.Fatalf("retry authorization metric description = %s", description)
	}
}

func TestParticipantRetryAuthorizationMetricsUseBoundedOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		result *Result
		err    error
		label  string
	}{
		{name: "created", result: &Result{Created: true}, label: "created"},
		{name: "reused", result: &Result{}, label: "reused"},
		{name: "capacity", err: domaingeneration.ErrOrgDailyBudgetExceeded, label: "capacity_rejected"},
		{name: "not allowed", err: domainrun.ErrRetryNotAllowed, label: "not_allowed"},
		{name: "conflict", err: domainrun.ErrConflict, label: "conflict"},
		{name: "error", err: errors.New("persistence unavailable"), label: "error"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			before := testutil.ToFloat64(participantRetryAuthorizationTotal.WithLabelValues(testCase.label))
			observeParticipantRetryAuthorization(testCase.result, testCase.err)
			if delta := testutil.ToFloat64(participantRetryAuthorizationTotal.WithLabelValues(testCase.label)) - before; delta != 1 {
				t.Fatalf("retry authorization metric delta = %v", delta)
			}
		})
	}
}
