package runtime

import (
	"context"
	"testing"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

type resolverReaderStub struct {
	model *modelcatalogport.PublishedModel
}

func (s resolverReaderStub) GetPublishedModelByRef(context.Context, modelcatalogport.Ref) (*modelcatalogport.PublishedModel, error) {
	return s.model, nil
}
func (s resolverReaderStub) FindPublishedModelByQuestionnaire(context.Context, string, string) (*modelcatalogport.PublishedModel, error) {
	return s.model, nil
}

func (s resolverReaderStub) FindPublishedModelByCode(context.Context, domain.Kind, string) (*modelcatalogport.PublishedModel, error) {
	return s.model, nil
}

func (s resolverReaderStub) ListPublishedModels(context.Context, modelcatalogport.ListPublishedFilter) ([]*modelcatalogport.PublishedModel, int64, error) {
	return []*modelcatalogport.PublishedModel{s.model}, 1, nil
}

type resolverAuthorizerStub struct{}

func (resolverAuthorizerStub) Authorize(context.Context, modelcatalog.ActorContext, modelcatalog.Action, modelcatalog.Resource) error {
	return nil
}

func TestPublishedResolverRequiresDefinitionV2(t *testing.T) {
	t.Parallel()
	resolver := Resolver{Reader: resolverReaderStub{model: &modelcatalogport.PublishedModel{Code: "S"}}, Authorizer: resolverAuthorizerStub{}}
	_, err := resolver.ResolveByRef(context.Background(), modelcatalog.ActorContext{Principal: securityplane.Principal{Kind: securityplane.PrincipalKindService}}, modelcatalogport.Ref{Kind: domain.KindScale, Code: "S", Version: "v1"})
	if err == nil {
		t.Fatal("ResolveByRef() error = nil, want missing definition_v2")
	}
}

func TestTrustedRuntimeCatalogRejectsPublishedModelWithoutDefinitionV2(t *testing.T) {
	t.Parallel()
	catalog := NewTrustedRuntimeCatalog(resolverReaderStub{model: &modelcatalogport.PublishedModel{Code: "S"}}, resolverReaderStub{model: &modelcatalogport.PublishedModel{Code: "S"}})
	_, err := catalog.GetPublishedModelByRef(context.Background(), modelcatalogport.Ref{Kind: domain.KindScale, Code: "S", Version: "v1"})
	if err == nil {
		t.Fatal("GetPublishedModelByRef() error = nil, want missing definition_v2")
	}
}
