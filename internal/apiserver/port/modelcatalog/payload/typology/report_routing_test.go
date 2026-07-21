package typology_test

import (
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestResolveTypologyReportRouting_LegacyAbsent(t *testing.T) {
	t.Parallel()

	routing, err := typology.ResolveTypologyReportRouting(nil)
	if err != nil {
		t.Fatalf("ResolveTypologyReportRouting(nil) err = %v", err)
	}
	if routing.Source != typology.ReportRoutingHistoricalLegacy {
		t.Fatalf("Source = %s, want historical_legacy", routing.Source)
	}
	if routing.TemplateID != "" || routing.AdapterKey != "" {
		t.Fatalf("legacy absent should leave routing empty, got %#v", routing)
	}

	payload := &typology.Payload{
		Algorithm:      binding.AlgorithmMBTI,
		DimensionOrder: []string{"EI"},
		Dimensions:     map[string]typology.Dimension{"EI": {Code: "EI", Name: "EI", LeftPole: "I", RightPole: "E"}},
		MatchingSpec:   typology.MatchingSpec{Kind: binding.DecisionKindPoleComposition},
	}
	routing, err = typology.ResolveTypologyReportRouting(payload)
	if err != nil {
		t.Fatalf("legacy payload err = %v", err)
	}
	if routing.Source != typology.ReportRoutingHistoricalLegacy {
		t.Fatalf("Source = %s, want historical_legacy", routing.Source)
	}
	if routing.TemplateID == "" && routing.AdapterKey == "" {
		t.Fatal("legacy derive should populate TemplateID or AdapterKey")
	}
}

func TestResolveTypologyReportRouting_ExplicitValid(t *testing.T) {
	t.Parallel()

	payload := &typology.Payload{
		DimensionOrder: []string{"EI"},
		Dimensions:     map[string]typology.Dimension{"EI": {Code: "EI", Name: "EI", LeftPole: "I", RightPole: "E"}},
		MatchingSpec:   typology.MatchingSpec{Kind: binding.DecisionKindPoleComposition},
		Runtime: &typology.RuntimeSpec{
			FactorGraph: typology.FactorGraphSpec{
				DimensionOrder: []string{"EI"},
				Dimensions:     map[string]typology.Dimension{"EI": {Code: "EI", Name: "EI", LeftPole: "I", RightPole: "E"}},
			},
			Decision:       typology.PersonalityDecisionSpec{Kind: binding.DecisionKindPoleComposition},
			OutcomeMapping: typology.OutcomeMappingSpec{DetailKind: typology.OutcomeDetailPersonalityType},
			Report: typology.ReportSpec{
				Kind:       typology.ReportKindTemplate,
				AdapterKey: typology.ReportAdapterPersonalityType,
				TemplateID: "mbti",
			},
		},
	}

	routing, err := typology.ResolveTypologyReportRouting(payload)
	if err != nil {
		t.Fatalf("ResolveTypologyReportRouting() err = %v", err)
	}
	if routing.Source != typology.ReportRoutingExplicitRuntime {
		t.Fatalf("Source = %s, want explicit_runtime", routing.Source)
	}
	if routing.TemplateID != "mbti" {
		t.Fatalf("TemplateID = %q, want mbti", routing.TemplateID)
	}
	if routing.AdapterKey != typology.ReportAdapterPersonalityType {
		t.Fatalf("AdapterKey = %s, want personality_type", routing.AdapterKey)
	}
	if routing.DecisionKind != binding.DecisionKindPoleComposition {
		t.Fatalf("DecisionKind = %s, want pole_composition", routing.DecisionKind)
	}
}

func TestResolveTypologyReportRouting_ExplicitMalformed(t *testing.T) {
	t.Parallel()

	payload := &typology.Payload{
		Runtime: &typology.RuntimeSpec{
			// Missing required decision/factor graph → ToRuntimeSpec fails.
			Report: typology.ReportSpec{Kind: typology.ReportKindTemplate},
		},
	}

	routing, err := typology.ResolveTypologyReportRouting(payload)
	if !errors.Is(err, typology.ErrRuntimeSpecInvalid) {
		t.Fatalf("err = %v, want ErrRuntimeSpecInvalid", err)
	}
	if routing.Source != "" {
		t.Fatalf("malformed explicit runtime must not return a routing source, got %s", routing.Source)
	}
}

func TestIsRegisteredReportTemplateID(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"mbti", "sbti", "bigfive"} {
		if !typology.IsRegisteredReportTemplateID(id) {
			t.Fatalf("IsRegisteredReportTemplateID(%q) = false", id)
		}
	}
	if typology.IsRegisteredReportTemplateID("") {
		t.Fatal("empty TemplateID must not be registered")
	}
	if typology.IsRegisteredReportTemplateID("unknown-template") {
		t.Fatal("unknown TemplateID must not be registered")
	}
}
