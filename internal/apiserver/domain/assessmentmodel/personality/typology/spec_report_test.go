package typology

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
)

func TestReportSpecResolvedAdapterKey(t *testing.T) {
	mbtiMapping := OutcomeMappingSpec{DetailKind: OutcomeDetailPersonalityType}
	sbtiMapping := OutcomeMappingSpec{
		DetailKind:       OutcomeDetailPersonalityType,
		DetailAdapterKey: DetailAdapterSBTI,
	}

	t.Run("explicit adapter key", func(t *testing.T) {
		spec := ReportSpec{Kind: ReportKindPersonalityType, AdapterKey: ReportAdapterSBTI}
		if got := spec.ResolvedAdapterKey(mbtiMapping, assessmentmodel.DecisionKindPoleComposition); got != ReportAdapterSBTI {
			t.Fatalf("ResolvedAdapterKey() = %s, want sbti", got)
		}
	})

	t.Run("trait profile kind", func(t *testing.T) {
		spec := ReportSpec{Kind: ReportKindTraitProfile}
		if got := spec.ResolvedAdapterKey(mbtiMapping, assessmentmodel.DecisionKindTraitProfile); got != ReportAdapterTraitProfile {
			t.Fatalf("ResolvedAdapterKey() = %s, want trait_profile", got)
		}
	})

	t.Run("personality type from outcome mapping", func(t *testing.T) {
		spec := ReportSpec{Kind: ReportKindPersonalityType}
		if got := spec.ResolvedAdapterKey(sbtiMapping, assessmentmodel.DecisionKindNearestPattern); got != ReportAdapterSBTI {
			t.Fatalf("ResolvedAdapterKey() = %s, want sbti", got)
		}
		if got := spec.ResolvedAdapterKey(mbtiMapping, assessmentmodel.DecisionKindPoleComposition); got != ReportAdapterPersonalityType {
			t.Fatalf("ResolvedAdapterKey() = %s, want personality_type", got)
		}
	})
}

func TestOutcomeMappingResolvedDetailAdapterKeyUsesGenericDefaults(t *testing.T) {
	if got := (OutcomeMappingSpec{DetailKind: OutcomeDetailPersonalityType}).ResolvedDetailAdapterKey(assessmentmodel.DecisionKindNearestPattern); got != DetailAdapterPersonalityType {
		t.Fatalf("ResolvedDetailAdapterKey() = %s, want personality_type", got)
	}
	if got := (OutcomeMappingSpec{DetailKind: OutcomeDetailTraitProfile}).ResolvedDetailAdapterKey(assessmentmodel.DecisionKindTraitProfile); got != DetailAdapterTraitProfile {
		t.Fatalf("ResolvedDetailAdapterKey() = %s, want trait_profile", got)
	}
	if got := (OutcomeMappingSpec{DetailKind: OutcomeDetailPersonalityType, DetailAdapterKey: DetailAdapterSBTI}).ResolvedDetailAdapterKey(assessmentmodel.DecisionKindNearestPattern); got != DetailAdapterSBTI {
		t.Fatalf("ResolvedDetailAdapterKey() = %s, want sbti", got)
	}
}

func TestReportSpecTemplateWithoutAdapterHasNoResolvedAdapter(t *testing.T) {
	spec := ReportSpec{Kind: ReportKindTemplate, TemplateID: "custom"}
	if got := spec.ResolvedAdapterKey(OutcomeMappingSpec{DetailKind: OutcomeDetailPersonalityType}, assessmentmodel.DecisionKindPoleComposition); got != "" {
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
	if spec.Report.AdapterKey != ReportAdapterSBTI {
		t.Fatalf("Report.AdapterKey = %s, want sbti", spec.Report.AdapterKey)
	}
}
