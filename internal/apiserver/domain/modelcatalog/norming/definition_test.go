package norming_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming"
)

func TestDefinitionFromPayloadProjectsBrief2Metadata(t *testing.T) {
	t.Parallel()

	definition, err := norming.DefinitionFromPayload([]byte(`{
		"dimensions": [
			{"code": "inhibit", "title": "Inhibit", "question_codes": ["q1"], "scoring_strategy": "sum"},
			{"code": "self_monitor", "title": "Self Monitor", "question_codes": ["q2"], "scoring_strategy": "sum"},
			{"code": "bri", "title": "BRI"},
			{"code": "gec", "title": "GEC"},
			{"code": "inconsistency", "title": "Inconsistency"}
		],
		"brief2": {
			"norm_table_version": "2024",
			"index_codes": ["bri", "gec"],
			"validity_codes": ["inconsistency"],
			"composite_indexes": [
				{"code": "bri", "strategy": "sum", "children": ["inhibit", "self_monitor"]},
				{"code": "gec", "strategy": "sum", "children": ["bri"]}
			],
			"norms": [{"factor_code": "gec"}]
		}
	}`))
	if err != nil {
		t.Fatalf("DefinitionFromPayload: %v", err)
	}

	roles := map[string]factor.FactorRole{}
	for _, item := range definition.Measure.Factors {
		roles[item.Code] = item.ResolvedRole()
	}
	if roles["bri"] != factor.FactorRoleIndex || roles["gec"] != factor.FactorRoleIndex {
		t.Fatalf("index roles = %#v", roles)
	}
	if roles["inconsistency"] != factor.FactorRoleValidity {
		t.Fatalf("validity role = %s", roles["inconsistency"])
	}
	if definition.Measure.FactorGraph.ParentCode("inhibit") != "bri" {
		t.Fatalf("inhibit parent = %q", definition.Measure.FactorGraph.ParentCode("inhibit"))
	}
	if len(definition.Measure.Scoring) != 4 {
		t.Fatalf("scoring = %#v", definition.Measure.Scoring)
	}
	if len(definition.Calibration.NormRefs) != 1 || definition.Calibration.NormRefs[0].FactorCode != "gec" {
		t.Fatalf("norm refs = %#v", definition.Calibration.NormRefs)
	}
}
