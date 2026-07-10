package cognitive_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

func TestParseDefinitionPayloadProjectsToScaleSnapshot(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [
			{
				"code": "total",
				"title": "总分",
				"question_codes": ["q1", "q2"],
				"scoring_strategy": "sum",
				"is_total_score": true
			}
		],
		"interpret_rules": [
			{
				"dimension_code": "total",
				"ranges": [
					{"min_score": 0, "max_score": 10, "conclusion": "low", "level": "low"}
				]
			}
		]
	}`)
	got, err := cognitive.ParseDefinitionPayload("BA-001", "1.0.0", "认知测评", "published", raw)
	if err != nil {
		t.Fatalf("ParseDefinitionPayload: %v", err)
	}
	scale := got.ToScaleSnapshot()
	if scale == nil || len(scale.Factors) != 1 {
		t.Fatalf("scale factors = %#v", scale)
	}
	if scale.Factors[0].Code != "total" || scale.Factors[0].ScoringStrategy != "sum" {
		t.Fatalf("factor = %#v", scale.Factors[0])
	}
	if len(scale.Factors[0].InterpretRules) != 1 || scale.Factors[0].InterpretRules[0].RiskLevel != "low" {
		t.Fatalf("interpret rules = %#v", scale.Factors[0].InterpretRules)
	}
}

func TestParseSPMPayloadAppliesTaskPerformanceMetadata(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [
			{"code": "A", "title": "A", "question_codes": ["q1"], "scoring_strategy": "sum"},
			{"code": "total", "title": "总分", "question_codes": ["q1"], "scoring_strategy": "sum", "is_total_score": true}
		],
		"interpret_rules": [{"dimension_code": "total", "ranges": [{"min_score": 0, "max_score": 10, "conclusion": "ok"}]}],
		"spm": {
			"time_limit_seconds": 900,
			"item_set_codes": ["A", "B", "C", "D", "E"],
			"norm_table_version": "2024"
		}
	}`)
	got, err := cognitive.ParsePublishedPayload(
		"assessmentmodel.cognitive.spm.v1",
		"COG-001", "v1", "SPM", "published", raw,
	)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	if len(got.Factors) != 2 {
		t.Fatalf("len(Factors) = %d, want 2", len(got.Factors))
	}
	if got.Factors[0].Code != "A" || got.Factors[0].Role != factor.FactorRoleTaskSet {
		t.Fatalf("task-set factor = %#v", got.Factors[0])
	}
	if got.Factors[0].Norm == nil || got.Factors[0].Norm.NormTableVersion != "2024" {
		t.Fatalf("task-set norm = %#v", got.Factors[0].Norm)
	}
	if got.Factors[1].Norm == nil || got.Factors[1].Norm.NormTableVersion != "2024" {
		t.Fatalf("total norm = %#v", got.Factors[1].Norm)
	}
}
