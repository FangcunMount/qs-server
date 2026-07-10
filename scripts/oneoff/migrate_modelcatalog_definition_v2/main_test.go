package main

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestMaterializeNormalizesLegacyPersonalityKind(t *testing.T) {
	payload := []byte(`{
		"code":"MBTI_LEGACY","version":"1.0.0","status":"published","algorithm":"mbti",
		"dimension_order":["EI"],
		"dimensions":{"EI":{"code":"EI","name":"Extraversion","left_pole":"I","right_pole":"E"}},
		"question_mappings":[{"question_code":"Q1","dimension":"EI","sign":1}],
		"outcomes":[{"code":"ENFP","name":"Campaigner"}],
		"matching_spec":{"kind":"pole_composition"}
	}`)
	got, err := materialize(domain.KindPersonality, domain.AlgorithmMBTI, payload)
	if err != nil {
		t.Fatalf("materialize legacy personality: %v", err)
	}
	if got.Definition == nil || len(got.Definition.Measure.Factors) != 1 || got.Definition.Measure.Factors[0].Code != "EI" {
		t.Fatalf("definition = %#v", got.Definition)
	}
}
