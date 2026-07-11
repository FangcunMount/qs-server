package plan

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestPublishedScaleCatalogUsesPublishedDefinition(t *testing.T) {
	t.Parallel()

	lister := &publishedScaleListerStub{model: &modelcatalogport.PublishedModel{
		Kind:         domain.KindScale,
		Code:         "SCL-1",
		Title:        "Published Scale",
		DefinitionV2: &modeldefinition.Definition{},
	}}
	catalog := NewPublishedScaleCatalog(lister)

	exists, err := catalog.ExistsByCode(context.Background(), "SCL-1")
	if err != nil {
		t.Fatalf("ExistsByCode: %v", err)
	}
	if !exists {
		t.Fatal("ExistsByCode = false, want published DefinitionV2 scale")
	}
	if got := catalog.ResolveTitle(context.Background(), "SCL-1"); got != "Published Scale" {
		t.Fatalf("ResolveTitle = %q, want Published Scale", got)
	}
	if lister.kind != domain.KindScale || lister.code != "SCL-1" {
		t.Fatalf("published lookup = %s/%s, want scale/SCL-1", lister.kind, lister.code)
	}
}

func TestPublishedScaleCatalogRejectsMissingDefinition(t *testing.T) {
	t.Parallel()

	catalog := NewPublishedScaleCatalog(&publishedScaleListerStub{model: &modelcatalogport.PublishedModel{
		Kind:  domain.KindScale,
		Code:  "SCL-1",
		Title: "Incomplete Scale",
	}})

	exists, err := catalog.ExistsByCode(context.Background(), "SCL-1")
	if err != nil {
		t.Fatalf("ExistsByCode: %v", err)
	}
	if exists {
		t.Fatal("ExistsByCode = true for published scale without DefinitionV2")
	}
	if got := catalog.ResolveTitle(context.Background(), "SCL-1"); got != "SCL-1" {
		t.Fatalf("ResolveTitle = %q, want code fallback", got)
	}
}

type publishedScaleListerStub struct {
	model *modelcatalogport.PublishedModel
	err   error
	kind  domain.Kind
	code  string
}

func (s *publishedScaleListerStub) FindPublishedModelByCode(_ context.Context, kind domain.Kind, code string) (*modelcatalogport.PublishedModel, error) {
	s.kind = kind
	s.code = code
	return s.model, s.err
}

func (s *publishedScaleListerStub) ListPublishedModels(context.Context, modelcatalogport.ListPublishedFilter) ([]*modelcatalogport.PublishedModel, int64, error) {
	return nil, 0, nil
}
