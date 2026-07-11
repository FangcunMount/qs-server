package ruleset

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	seedfixtures "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/seedfixtures"
)

func TestCatalogBindingResolverResolveAssessmentBindingSBTI(t *testing.T) {
	catalog, err := NewDefaultStaticCatalog()
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
}

func TestCatalogBindingResolverResolveAssessmentBindingMBTI(t *testing.T) {
	catalog, err := NewDefaultStaticCatalog()
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

func TestCatalogBindingResolverNilCatalog(t *testing.T) {
	var resolver *CatalogBindingResolver
	binding, ok, err := resolver.ResolveAssessmentBinding(context.Background(), "Q", "1")
	if err != nil || ok || !binding.Ref.IsEmpty() {
		t.Fatalf("got binding=%#v ok=%v err=%v", binding, ok, err)
	}
}
