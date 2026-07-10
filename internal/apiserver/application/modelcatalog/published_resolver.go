package modelcatalog

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Resolver is the published-only application service for trusted runtime
// consumers. It never reaches ModelRepository or payload decoders.
type Resolver struct {
	Reader     modelcatalogport.PublishedModelReader
	Authorizer Authorizer
}

func (s Resolver) ResolveByRef(ctx context.Context, actor ActorContext, ref modelcatalogport.Ref) (*modelcatalogport.PublishedModel, error) {
	if ref.Code == "" || ref.Version == "" {
		return nil, errors.WithCode(code.ErrInvalidArgument, "published model code and version are required")
	}
	if s.Reader == nil || s.Authorizer == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "published model resolver is not configured")
	}
	if err := s.Authorizer.Authorize(ctx, actor, ActionResolvePublished, Resource{Code: ref.Code, Kind: ref.Kind}); err != nil {
		return nil, err
	}
	model, err := s.Reader.GetPublishedModelByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return requireRuntimeDefinition(model)
}

func (s Resolver) ResolveByQuestionnaire(ctx context.Context, actor ActorContext, questionnaireCode, questionnaireVersion string) (*modelcatalogport.PublishedModel, error) {
	if questionnaireCode == "" {
		return nil, errors.WithCode(code.ErrInvalidArgument, "questionnaire code is required")
	}
	if s.Reader == nil || s.Authorizer == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "published model resolver is not configured")
	}
	if err := s.Authorizer.Authorize(ctx, actor, ActionResolvePublished, Resource{}); err != nil {
		return nil, err
	}
	model, err := s.Reader.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return nil, err
	}
	return requireRuntimeDefinition(model)
}

func requireRuntimeDefinition(model *modelcatalogport.PublishedModel) (*modelcatalogport.PublishedModel, error) {
	if model == nil {
		return nil, domain.ErrNotFound
	}
	if model.DefinitionV2 == nil {
		return nil, errors.WithCode(code.ErrInvalidArgument, "published model definition_v2 is required for runtime: %s", model.Code)
	}
	return model, nil
}

// TrustedRuntimeResolver adapts a verified service actor to the application
// resolver so infrastructure consumers do not construct authorization context.
type TrustedRuntimeResolver struct {
	Resolver PublishedModelResolver
	Actor    ActorContext
}

func (r TrustedRuntimeResolver) GetPublishedModelByRef(ctx context.Context, ref modelcatalogport.Ref) (*modelcatalogport.PublishedModel, error) {
	if r.Resolver == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "trusted published model resolver is not configured")
	}
	return r.Resolver.ResolveByRef(ctx, r.Actor, ref)
}

func (r TrustedRuntimeResolver) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*modelcatalogport.PublishedModel, error) {
	if r.Resolver == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "trusted published model resolver is not configured")
	}
	return r.Resolver.ResolveByQuestionnaire(ctx, r.Actor, questionnaireCode, questionnaireVersion)
}

var _ PublishedModelResolver = Resolver{}
var _ modelcatalogport.PublishedModelReader = TrustedRuntimeResolver{}
