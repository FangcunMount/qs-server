package cognitive_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	cognitive "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

func TestDefinitionPayloadRoundTripPreservesSPMSemanticConfiguration(t *testing.T) {
	t.Parallel()
	input := []byte(`{
  "dimensions":[
    {"code":"set_a","title":"Set A","question_codes":["q1"],"scoring_strategy":"sum","is_show":true},
    {"code":"total","title":"Total","role":"total","question_codes":["q1"],"scoring_strategy":"sum","is_show":true}
  ],
  "spm":{
    "item_set_codes":["set_a"],
    "norm_table_version":"spm-cn-2024",
    "ability_conclusions":[{"factor_code":"total","score_basis":"raw_score","ranges":[{"min_score":0,"max_score":60,"level":"average","summary":"Average"}]}]
  }
}`)
	definition, err := cognitive.DefinitionFromLegacyPayload(input)
	if err != nil {
		t.Fatalf("DefinitionFromLegacyPayload: %v", err)
	}
	payload, err := cognitive.PayloadFromDefinition(definition)
	if err != nil {
		t.Fatalf("PayloadFromDefinition: %v", err)
	}
	roundTrip, err := cognitive.DefinitionFromLegacyPayload(payload)
	if err != nil {
		t.Fatalf("round-trip DefinitionFromLegacyPayload: %v", err)
	}
	if len(roundTrip.Calibration.NormRefs) != 2 || roundTrip.Calibration.NormRefs[0].NormTableVersion != "spm-cn-2024" || roundTrip.Calibration.NormRefs[1].NormTableVersion != "spm-cn-2024" {
		t.Fatalf("norm refs = %#v", roundTrip.Calibration.NormRefs)
	}
	var taskSetRole factor.FactorRole
	for _, item := range roundTrip.Measure.Factors {
		if item.Code == "set_a" {
			taskSetRole = item.Role
		}
	}
	if taskSetRole != factor.FactorRoleTaskSet {
		t.Fatalf("set_a role = %q, want task_set", taskSetRole)
	}
	var ability conclusion.AbilityConclusion
	found := false
	for _, item := range roundTrip.Conclusions {
		candidate, ok := item.(conclusion.AbilityConclusion)
		if ok && candidate.FactorCode == "total" {
			ability, found = candidate, true
			break
		}
	}
	if !found || ability.ScoreBasis != conclusion.ScoreBasisRaw || len(ability.Rules) != 1 || ability.Rules[0].Summary != "Average" {
		t.Fatalf("ability conclusion = %#v", ability)
	}
}
