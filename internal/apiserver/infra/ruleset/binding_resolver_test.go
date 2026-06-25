package ruleset

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/snapshot"
	evaluationinputPort "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestCatalogBindingResolverResolveAssessmentBindingSBTI(t *testing.T) {
	catalog, err := NewDefaultStaticCatalog(nil)
	if err != nil {
		t.Fatalf("NewDefaultStaticCatalog: %v", err)
	}
	resolver := NewAssessmentBindingResolver(catalog)

	binding, ok, err := resolver.ResolveAssessmentBinding(
		context.Background(),
		evaluationinputPort.DefaultSBTIQuestionnaireCode,
		evaluationinputPort.DefaultSBTIModelVersion,
	)
	if err != nil {
		t.Fatalf("ResolveAssessmentBinding: %v", err)
	}
	if !ok {
		t.Fatal("expected binding")
	}
	if binding.Ref.Kind != domain.RuleSetKindSBTI {
		t.Fatalf("kind = %s, want sbti", binding.Ref.Kind)
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
		evaluationinputPort.DefaultMBTIQuestionnaireCode,
		evaluationinputPort.DefaultMBTIModelVersion,
	)
	if err != nil {
		t.Fatalf("ResolveAssessmentBinding: %v", err)
	}
	if !ok {
		t.Fatal("expected binding")
	}
	if binding.Ref.Code != evaluationinputPort.DefaultMBTIModelCode {
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
	if binding.Ref.Kind != domain.RuleSetKindScale {
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
