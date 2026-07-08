package ruleset

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	seedfixtures "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/seedfixtures"
)

func TestCatalogBindingResolverResolveAssessmentBindingSBTI(t *testing.T) {
	catalog, err := NewDefaultStaticCatalog(nil)
	if err != nil {
		t.Fatalf("NewDefaultStaticCatalog: %v", err)
	}
	resolver := NewAssessmentBindingResolver(catalog)

	binding, ok, err := resolver.ResolveAssessmentBinding(
		context.Background(),
		seedfixtures.SBTIQuestionnaireCode,
		seedfixtures.SBTIModelVersion,
	)
	if err != nil {
		t.Fatalf("ResolveAssessmentBinding: %v", err)
	}
	if !ok {
		t.Fatal("expected binding")
	}
	if binding.Ref.Kind != domain.KindTypology ||
		binding.Ref.SubKind != domain.SubKindTypology ||
		binding.Ref.Algorithm != domain.AlgorithmSBTI {
		t.Fatalf("ref = %#v, want personality/typology/sbti", binding.Ref)
	}
	if binding.MedicalScaleID != nil {
		t.Fatalf("MedicalScaleID = %#v, want nil", binding.MedicalScaleID)
	}
}

func TestCatalogBindingResolverResolveAssessmentBindingMBTI(t *testing.T) {
	catalog, err := NewDefaultStaticCatalog(nil)
	if err != nil {
		t.Fatalf("NewDefaultStaticCatalog: %v", err)
	}
	resolver := NewAssessmentBindingResolver(catalog)

	binding, ok, err := resolver.ResolveAssessmentBinding(
		context.Background(),
		seedfixtures.MBTIQuestionnaireCode,
		seedfixtures.MBTIModelVersion,
	)
	if err != nil {
		t.Fatalf("ResolveAssessmentBinding: %v", err)
	}
	if !ok {
		t.Fatal("expected binding")
	}
	if binding.Ref.Code != seedfixtures.MBTIModelCode {
		t.Fatalf("code = %s", binding.Ref.Code)
	}
}

func TestCatalogBindingResolverResolveAssessmentBindingScale(t *testing.T) {
	scaleModel := &scalesnapshot.ScaleSnapshot{
		ID:                   42,
		Code:                 "SCL-001",
		ScaleVersion:         "1.0.0",
		Title:                "Demo Scale",
		QuestionnaireCode:    "QNR-SCALE",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
	}
	catalog := NewStaticCompositeCatalog(nil, stubScaleBindingSource{model: scaleModel})
	resolver := NewAssessmentBindingResolver(catalog)

	binding, ok, err := resolver.ResolveAssessmentBinding(context.Background(), "QNR-SCALE", "1.0.0")
	if err != nil {
		t.Fatalf("ResolveAssessmentBinding: %v", err)
	}
	if !ok {
		t.Fatal("expected binding")
	}
	if binding.Ref.Kind != domain.KindScale {
		t.Fatalf("kind = %s, want scale", binding.Ref.Kind)
	}
	if binding.MedicalScaleID == nil || *binding.MedicalScaleID != 42 {
		t.Fatalf("MedicalScaleID = %#v, want 42", binding.MedicalScaleID)
	}
	if binding.MedicalScaleCode == nil || *binding.MedicalScaleCode != "SCL-001" {
		t.Fatalf("MedicalScaleCode = %#v", binding.MedicalScaleCode)
	}
	if binding.MedicalScaleName == nil || *binding.MedicalScaleName != "Demo Scale" {
		t.Fatalf("MedicalScaleName = %#v", binding.MedicalScaleName)
	}
	if binding.ScaleVersion == nil || *binding.ScaleVersion != "1.0.0" {
		t.Fatalf("ScaleVersion = %#v", binding.ScaleVersion)
	}
}

func TestCatalogBindingResolverNilCatalog(t *testing.T) {
	var resolver *CatalogBindingResolver
	binding, ok, err := resolver.ResolveAssessmentBinding(context.Background(), "Q", "1")
	if err != nil || ok || !binding.Ref.IsEmpty() {
		t.Fatalf("got binding=%#v ok=%v err=%v", binding, ok, err)
	}
}
