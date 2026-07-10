package behavioral_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	behavioral "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

func TestDefinitionPayloadRoundTripPreservesBrief2SemanticConfiguration(t *testing.T) {
	t.Parallel()
	input := []byte(`{
  "dimensions":[
    {"code":"inhibit","title":"Inhibit","question_codes":["q1"],"scoring_strategy":"sum","is_show":true},
    {"code":"bri","title":"BRI","role":"index","children_policy":{"strategy":"sum","children":["inhibit"]},"is_show":true}
  ],
  "brief2":{
    "norm_table_version":"brief2-cn-2024",
    "primary_dimension_code":"bri",
    "norms":[{"factor_code":"bri"}],
    "composite_indexes":[{"code":"bri","strategy":"sum","children":["inhibit"]}],
    "t_score_rules":[{"factor_code":"bri","ranges":[{"min_t":50,"max_t":70,"level":"elevated","conclusion":"Elevated","suggestion":"Review"}]}]
  }
}`)
	definition, err := behavioral.DefinitionFromLegacyPayload(input)
	if err != nil {
		t.Fatalf("DefinitionFromLegacyPayload: %v", err)
	}
	payload, err := behavioral.PayloadFromDefinition(definition)
	if err != nil {
		t.Fatalf("PayloadFromDefinition: %v", err)
	}
	roundTrip, err := behavioral.DefinitionFromLegacyPayload(payload)
	if err != nil {
		t.Fatalf("round-trip DefinitionFromLegacyPayload: %v", err)
	}
	if got := roundTrip.Measure.FactorGraph.ParentCode("inhibit"); got != "bri" {
		t.Fatalf("inhibit parent = %q, want bri", got)
	}
	if len(roundTrip.Calibration.NormRefs) != 1 || roundTrip.Calibration.NormRefs[0].NormTableVersion != "brief2-cn-2024" {
		t.Fatalf("norm refs = %#v", roundTrip.Calibration.NormRefs)
	}
	var norm conclusion.NormConclusion
	found := false
	for _, item := range roundTrip.Conclusions {
		candidate, ok := item.(conclusion.NormConclusion)
		if ok && candidate.FactorCode == "bri" {
			norm, found = candidate, true
			break
		}
	}
	if !found || !norm.Primary || len(norm.Rules) != 1 || norm.Rules[0].Summary != "Elevated" {
		t.Fatalf("norm conclusion = %#v", norm)
	}
}
