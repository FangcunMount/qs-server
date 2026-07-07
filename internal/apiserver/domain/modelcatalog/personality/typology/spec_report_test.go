package typology

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestReportSpecResolvedAdapterKey(t *testing.T) {
	mbtiMapping := OutcomeMappingSpec{DetailKind: OutcomeDetailPersonalityType}
	sbtiMapping := OutcomeMappingSpec{
		DetailKind:       OutcomeDetailPersonalityType,
		DetailAdapterKey: DetailAdapterSBTI,
	}

	t.Run("explicit adapter key", func(t *testing.T) {
		spec := ReportSpec{Kind: ReportKindPersonalityType, AdapterKey: ReportAdapterSBTI}
		if got := spec.ResolvedAdapterKey(mbtiMapping, modelcatalog.DecisionKindPoleComposition); got != ReportAdapterSBTI {
			t.Fatalf("ResolvedAdapterKey() = %s, want sbti", got)
		}
	})

	t.Run("trait profile kind", func(t *testing.T) {
		spec := ReportSpec{Kind: ReportKindTraitProfile}
		if got := spec.ResolvedAdapterKey(mbtiMapping, modelcatalog.DecisionKindTraitProfile); got != ReportAdapterTraitProfile {
			t.Fatalf("ResolvedAdapterKey() = %s, want trait_profile", got)
		}
	})

	t.Run("personality type kind without explicit adapter", func(t *testing.T) {
		spec := ReportSpec{Kind: ReportKindPersonalityType}
		if got := spec.ResolvedAdapterKey(sbtiMapping, modelcatalog.DecisionKindNearestPattern); got != ReportAdapterPersonalityType {
			t.Fatalf("ResolvedAdapterKey() = %s, want personality_type", got)
		}
		if got := spec.ResolvedAdapterKey(mbtiMapping, modelcatalog.DecisionKindPoleComposition); got != ReportAdapterPersonalityType {
			t.Fatalf("ResolvedAdapterKey() = %s, want personality_type", got)
		}
	})
}

func TestOutcomeMappingResolvedDetailAdapterKeyUsesGenericDefaults(t *testing.T) {
	if got := (OutcomeMappingSpec{DetailKind: OutcomeDetailPersonalityType}).ResolvedDetailAdapterKey(modelcatalog.DecisionKindNearestPattern); got != DetailAdapterPersonalityType {
		t.Fatalf("ResolvedDetailAdapterKey() = %s, want personality_type", got)
	}
	if got := (OutcomeMappingSpec{DetailKind: OutcomeDetailTraitProfile}).ResolvedDetailAdapterKey(modelcatalog.DecisionKindTraitProfile); got != DetailAdapterTraitProfile {
		t.Fatalf("ResolvedDetailAdapterKey() = %s, want trait_profile", got)
	}
	if got := (OutcomeMappingSpec{DetailKind: OutcomeDetailPersonalityType, DetailAdapterKey: DetailAdapterSBTI}).ResolvedDetailAdapterKey(modelcatalog.DecisionKindNearestPattern); got != DetailAdapterSBTI {
		t.Fatalf("ResolvedDetailAdapterKey() = %s, want sbti", got)
	}
}

func TestReportSpecTemplateWithoutAdapterHasNoResolvedAdapter(t *testing.T) {
	spec := ReportSpec{Kind: ReportKindTemplate, TemplateID: "custom"}
	if got := spec.ResolvedAdapterKey(OutcomeMappingSpec{DetailKind: OutcomeDetailPersonalityType}, modelcatalog.DecisionKindPoleComposition); got != "" {
		t.Fatalf("ResolvedAdapterKey() = %s, want empty", got)
	}
}

func TestToRuntimeSpecSetsReportAdapterKey(t *testing.T) {
	payload := FromSBTI(&SBTILegacyModel{
		Code:           "SBTI_FUN",
		Version:        "1.0.0",
		DimensionOrder: []string{"D1"},
		Dimensions: map[string]SBTILegacyDimension{
			"D1": {Code: "D1", Name: "D1"},
		},
	})
	spec, err := payload.ToRuntimeSpec()
	if err != nil {
		t.Fatalf("ToRuntimeSpec: %v", err)
	}
	if spec.Report.AdapterKey != ReportAdapterPersonalityType {
		t.Fatalf("Report.AdapterKey = %s, want personality_type", spec.Report.AdapterKey)
	}
	if spec.Report.CategoryLabel != "SBTI" {
		t.Fatalf("Report.CategoryLabel = %s, want SBTI", spec.Report.CategoryLabel)
	}
}
