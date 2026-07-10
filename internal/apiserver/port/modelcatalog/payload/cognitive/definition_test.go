package cognitive_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

func TestDefinitionFromPayloadProjectsSPMMetadata(t *testing.T) {
	t.Parallel()

	got, err := cognitive.DefinitionFromPayload([]byte(`{
		"dimensions": [
			{"code": "A", "title": "A", "question_codes": ["q1"], "scoring_strategy": "sum"},
			{"code": "total", "title": "总分", "question_codes": ["q1"], "scoring_strategy": "sum", "is_total_score": true}
		],
		"spm": {
			"item_set_codes": ["A"],
			"norm_table_version": "2024"
		}
	}`))
	if err != nil {
		t.Fatalf("DefinitionFromPayload: %v", err)
	}
	if got.Measure.Factors[0].ResolvedRole() != factor.FactorRoleTaskSet {
		t.Fatalf("task-set role = %s", got.Measure.Factors[0].ResolvedRole())
	}
	if len(got.Calibration.NormRefs) != 2 {
		t.Fatalf("norm refs = %#v", got.Calibration.NormRefs)
	}
}
