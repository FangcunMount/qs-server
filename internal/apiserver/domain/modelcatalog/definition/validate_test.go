package definition_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

func TestValidateAcceptsCompleteDefinition(t *testing.T) {
	t.Parallel()

	def := definition.Definition{
		Measure: definition.MeasureSpec{
			Factors: []factor.Factor{
				{Code: "total", Role: factor.FactorRoleTotal},
				{Code: "trait", Role: factor.FactorRoleDimension},
			},
			Scoring: []factor.Scoring{{
				FactorCode: "trait",
				Sources: []factor.ScoringSource{{
					Kind:         factor.ScoringSourceQuestion,
					Code:         "q1",
					Sign:         -1,
					OptionScores: map[string]float64{"A": 1, "B": 5},
				}},
				Strategy:      factor.ScoringStrategySum,
				OptionScoring: factor.OptionScoringCompat,
			}},
		},
		Calibration: definition.Calibration{NormRefs: []norm.Ref{{FactorCode: "total", NormTableVersion: "2026"}}},
		Outcomes:    []conclusion.Outcome{{Code: "type_a", Title: "Type A"}},
		Conclusions: []conclusion.Conclusion{
			conclusion.NormConclusion{
				FactorCode: "total", ScoreBasis: conclusion.ScoreBasisTScore, Primary: true,
				Rules: []conclusion.ScoreRangeOutcome{{MinScore: 40, MaxScore: 60, Level: "average"}},
			},
			conclusion.AbilityConclusion{
				FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw,
				Rules: []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, OutcomeCode: "type_a"}},
			},
			conclusion.TypeConclusion{
				FactorCodes: []string{"trait"},
				Decision:    conclusion.TypeDecision{Kind: binding.DecisionKindTraitProfile},
				Profiles:    []conclusion.TypeOutcomeProfile{{OutcomeCode: "type_a"}},
			},
		},
		ReportMap: definition.ReportMap{Sections: []definition.ReportSection{{
			Code: "personality", Kind: "template", TemplateID: "type_a", AdapterKey: "trait_profile",
		}}},
	}

	if issues := definition.Validate(def); len(issues) != 0 {
		t.Fatalf("Validate() issues = %#v", issues)
	}
}

func TestValidateReportsCrossLayerReferenceErrors(t *testing.T) {
	t.Parallel()

	issues := definition.Validate(definition.Definition{
		Measure: definition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}}},
		Calibration: definition.Calibration{NormRefs: []norm.Ref{
			{FactorCode: "missing", NormTableVersion: "2026"},
			{FactorCode: "missing", NormTableVersion: "2026"},
		}},
		Outcomes: []conclusion.Outcome{{Code: "same"}, {Code: "same"}},
		Conclusions: []conclusion.Conclusion{
			conclusion.NormConclusion{FactorCode: "missing", ScoreBasis: conclusion.ScoreBasis("unknown")},
			conclusion.TypeConclusion{Decision: conclusion.TypeDecision{Kind: binding.DecisionKindNormLookup}, Profiles: []conclusion.TypeOutcomeProfile{{OutcomeCode: "missing"}}},
		},
		ReportMap: definition.ReportMap{Sections: []definition.ReportSection{{Code: "report", TemplateID: "x"}, {Code: "report"}}},
	})

	if len(issues) < 7 {
		t.Fatalf("Validate() issue count = %d, want cross-layer violations: %#v", len(issues), issues)
	}
}
