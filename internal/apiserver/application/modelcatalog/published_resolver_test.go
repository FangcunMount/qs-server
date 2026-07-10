package modelcatalog

import (
	"context"
	"testing"

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

type resolverAuthorizerStub struct{}

func (resolverAuthorizerStub) Authorize(context.Context, ActorContext, Action, Resource) error {
	return nil
}

func TestPublishedResolverRequiresDefinitionV2(t *testing.T) {
	t.Parallel()
	resolver := Resolver{Reader: resolverReaderStub{model: &modelcatalogport.PublishedModel{Code: "S"}}, Authorizer: resolverAuthorizerStub{}}
	_, err := resolver.ResolveByRef(context.Background(), ActorContext{Principal: securityplane.Principal{Kind: securityplane.PrincipalKindService}}, modelcatalogport.Ref{Kind: domain.KindScale, Code: "S", Version: "v1"})
	if err == nil {
		t.Fatal("ResolveByRef() error = nil, want missing definition_v2")
	}
}
