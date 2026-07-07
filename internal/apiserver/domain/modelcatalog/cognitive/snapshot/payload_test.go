package snapshot_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/cognitive/snapshot"
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
	got, err := snapshot.ParseDefinitionPayload("BA-001", "1.0.0", "认知测评", "published", raw)
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

func TestParseSPMPayloadPreservesProfile(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"dimensions": [{"code": "total", "title": "总分", "question_codes": ["q1"], "scoring_strategy": "sum"}],
		"interpret_rules": [{"dimension_code": "total", "ranges": [{"min_score": 0, "max_score": 10, "conclusion": "ok"}]}],
		"spm": {
			"time_limit_seconds": 900,
			"item_set_codes": ["A", "B", "C", "D", "E"],
			"norm_table_version": "2024"
		}
	}`)
	got, err := snapshot.ParsePublishedPayload(
		"assessmentmodel.cognitive.spm.v1",
		"COG-001", "v1", "SPM", "published", raw,
	)
	if err != nil {
		t.Fatalf("ParsePublishedPayload: %v", err)
	}
	if got.SPM == nil || got.SPM.TimeLimitSeconds != 900 || len(got.SPM.ItemSetCodes) != 5 {
		t.Fatalf("spm profile = %#v", got.SPM)
	}
}
