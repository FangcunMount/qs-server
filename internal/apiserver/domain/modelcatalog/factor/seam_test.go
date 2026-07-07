package factor_test

import (
	"testing"

	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
	cognitivesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/cognitive/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

func TestBehavioralRatingParseUsesSharedFactorShape(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [{"code": "total", "title": "总分", "question_codes": ["q1"], "scoring_strategy": "sum", "is_total_score": true}],
		"interpret_rules": [{"dimension_code": "total", "ranges": [{"min_score": 0, "max_score": 10, "conclusion": "low", "level": "low"}]}]
	}`)
	got, err := behavioralsnapshot.ParseDefinitionPayload("BA-001", "1.0.0", "行为能力", "published", raw)
	if err != nil {
		t.Fatalf("ParseDefinitionPayload: %v", err)
	}
	if len(got.Factors) != 1 || got.Factors[0].ResolvedRole() != factor.FactorRoleTotal {
		t.Fatalf("factors = %#v", got.Factors)
	}
	scale := got.ToScaleSnapshot()
	if scale == nil || scale.Factors[0].InterpretRules[0].RiskLevel != "low" {
		t.Fatalf("scale = %#v", scale)
	}
}

func TestCognitiveParseUsesSharedFactorShape(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [{"code": "total", "title": "总分", "question_codes": ["q1"], "scoring_strategy": "sum", "is_total_score": true}],
		"interpret_rules": [{"dimension_code": "total", "ranges": [{"min_score": 0, "max_score": 10, "conclusion": "low", "level": "low"}]}]
	}`)
	got, err := cognitivesnapshot.ParseDefinitionPayload("COG-001", "1.0.0", "认知", "published", raw)
	if err != nil {
		t.Fatalf("ParseDefinitionPayload: %v", err)
	}
	if len(got.Factors) != 1 || got.Factors[0].ResolvedRole() != factor.FactorRoleTotal {
		t.Fatalf("factors = %#v", got.Factors)
	}
}

func TestScaleCanonicalRoundTripPreservesExecutionShape(t *testing.T) {
	t.Parallel()

	original := scalesnapshot.FactorSnapshot{
		Code: "f1", ScoringStrategy: "sum", IsTotalScore: true,
		ScoringParams: scalesnapshot.ScoringParamsSnapshot{CntOptionContents: []string{"a", "b"}},
		InterpretRules: []scalesnapshot.InterpretRuleSnapshot{{
			Min: 0, Max: 5, RiskLevel: "low", Conclusion: "ok",
		}},
	}
	got := scalesnapshot.FactorSnapshotFromCanonical(original.Canonical())
	if got.Code != original.Code || got.ScoringStrategy != original.ScoringStrategy {
		t.Fatalf("round trip = %#v", got)
	}
	if len(got.ScoringParams.CntOptionContents) != 2 || got.InterpretRules[0].RiskLevel != "low" {
		t.Fatalf("execution shape = %#v", got)
	}
}

func TestBuildFromCanonicalFactorsProjectsInterpretRules(t *testing.T) {
	t.Parallel()

	scale := scalesnapshot.BuildFromModelFactors(
		"BA-001", "1.0.0", "demo", "", "", "published",
		[]factor.FactorSnapshot{{
			Code: "total", ScoringStrategy: "sum",
			InterpretRules: []factor.ScoreRangeRule{{MinScore: 0, MaxScore: 10, Level: "low"}},
		}},
	)
	if scale == nil || len(scale.Factors) != 1 {
		t.Fatalf("scale = %#v", scale)
	}
	if scale.Factors[0].InterpretRules[0].RiskLevel != "low" {
		t.Fatalf("interpret = %#v", scale.Factors[0].InterpretRules)
	}
}
