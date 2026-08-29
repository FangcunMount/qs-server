package evaluation

import (
	"testing"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPromptEvaluationAttemptFailureMetricsUseBoundedStageAndCode(t *testing.T) {
	tests := []struct {
		name      string
		failure   domainevaluation.AttemptFailure
		wantStage string
		wantCode  string
	}{
		{
			name: "reviewed local failure",
			failure: domainevaluation.AttemptFailure{
				Stage: "output_validation", Code: "provider_output_schema_invalid",
			},
			wantStage: "output_validation",
			wantCode:  "provider_output_schema_invalid",
		},
		{
			name: "reviewed provider failure",
			failure: domainevaluation.AttemptFailure{
				Stage: "semantic_evaluation", Code: "provider_timeout",
			},
			wantStage: "semantic_evaluation",
			wantCode:  "provider_timeout",
		},
		{
			name: "dynamic values collapse",
			failure: domainevaluation.AttemptFailure{
				Stage: "remote-stage-42", Code: "remote_dynamic_code_42",
			},
			wantStage: promptEvaluationFailureLabelOther,
			wantCode:  promptEvaluationFailureLabelOther,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			counter := promptEvaluationAttemptFailuresTotal.WithLabelValues(testCase.wantStage, testCase.wantCode)
			before := testutil.ToFloat64(counter)
			observePromptEvaluationAttemptFailure(&testCase.failure)
			if delta := testutil.ToFloat64(counter) - before; delta != 1 {
				t.Fatalf("attempt failure metric delta = %v", delta)
			}
		})
	}
}

func TestPromptEvaluationAttemptFailureMetricsIgnoreSuccessfulAttempts(t *testing.T) {
	counter := promptEvaluationAttemptFailuresTotal.WithLabelValues(
		promptEvaluationFailureLabelOther,
		promptEvaluationFailureLabelOther,
	)
	before := testutil.ToFloat64(counter)
	observePromptEvaluationAttemptFailure(nil)
	if delta := testutil.ToFloat64(counter) - before; delta != 0 {
		t.Fatalf("nil failure metric delta = %v", delta)
	}
}
