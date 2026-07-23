package descriptor

import (
	"testing"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestOutcomeCompletenessPolicyValidatesDecisionFacts(t *testing.T) {
	tests := []struct {
		name      string
		decision  modelcatalog.DecisionKind
		execution *domainoutcome.Execution
		ok        bool
	}{
		{
			name: "score range requires primary level", decision: modelcatalog.DecisionKindScoreRange,
			execution: &domainoutcome.Execution{},
		},
		{
			name: "score range requires scored dimension level", decision: modelcatalog.DecisionKindScoreRange,
			execution: &domainoutcome.Execution{
				Level: &domainoutcome.ResultLevel{Code: "low"},
				Dimensions: []domainoutcome.DimensionResult{{
					Code: "factor", Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal},
				}},
			},
		},
		{
			name: "score range complete", decision: modelcatalog.DecisionKindScoreRange,
			execution: &domainoutcome.Execution{
				Level: &domainoutcome.ResultLevel{Code: "low"},
				Dimensions: []domainoutcome.DimensionResult{{
					Code: "factor", Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal},
					Level: &domainoutcome.ResultLevel{Code: "low"},
				}},
			}, ok: true,
		},
		{
			name: "ability does not require every dimension level", decision: modelcatalog.DecisionKindAbilityLevel,
			execution: &domainoutcome.Execution{
				Level: &domainoutcome.ResultLevel{Code: "average"},
				Dimensions: []domainoutcome.DimensionResult{{
					Code: "memory", Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal},
				}},
			}, ok: true,
		},
		{
			name: "classification requires identity", decision: modelcatalog.DecisionKindTraitProfile,
			execution: &domainoutcome.Execution{},
		},
		{
			name: "classification primary label is identity", decision: modelcatalog.DecisionKindTraitProfile,
			execution: &domainoutcome.Execution{Summary: domainoutcome.Summary{PrimaryLabel: "type_1"}}, ok: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := DefaultOutcomeCompletenessPolicy(tc.decision).ValidateExecution(tc.execution)
			if tc.ok && err != nil {
				t.Fatalf("ValidateExecution() error = %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("ValidateExecution() error = nil")
			}
		})
	}
}
