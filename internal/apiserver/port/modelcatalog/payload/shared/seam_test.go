package shared_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	taskperfsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
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
	got, err := taskperfsnapshot.ParseDefinitionPayload("COG-001", "1.0.0", "认知", "published", raw)
	if err != nil {
		t.Fatalf("ParseDefinitionPayload: %v", err)
	}
	if len(got.Factors) != 1 || got.Factors[0].ResolvedRole() != factor.FactorRoleTotal {
		t.Fatalf("factors = %#v", got.Factors)
	}
}
