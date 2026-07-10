package factor

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestDefinitionFromFactorDTOsBuildsCanonicalScaleDefinition(t *testing.T) {
	t.Parallel()
	def, err := definitionFromFactorDTOs([]shared.FactorDTO{{
		Code: "total", Title: "Total", IsTotalScore: true,
		QuestionCodes: []string{"q1"}, ScoringStrategy: "sum",
		InterpretRules: []shared.InterpretRuleDTO{{MinScore: 0, MaxScore: 10, RiskLevel: "low", Conclusion: "Low"}},
	}})
	if err != nil {
		t.Fatalf("definitionFromFactorDTOs: %v", err)
	}
	if len(def.Measure.Factors) != 1 || def.Measure.Factors[0].Role != factor.FactorRoleTotal {
		t.Fatalf("factors = %#v", def.Measure.Factors)
	}
	if len(def.Measure.Scoring) != 1 || len(def.Measure.Scoring[0].Sources) != 1 || def.Measure.Scoring[0].Sources[0].Code != "q1" {
		t.Fatalf("scoring = %#v", def.Measure.Scoring)
	}
	risk, ok := def.Conclusions[0].(conclusion.RiskConclusion)
	if !ok || len(risk.Rules) != 1 || risk.Rules[0].Summary != "Low" {
		t.Fatalf("conclusions = %#v", def.Conclusions)
	}
}
