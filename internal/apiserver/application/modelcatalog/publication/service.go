package publication

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appbinding "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/binding"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/lifecycle"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Service owns actor-authorized publish and unpublish
// commands. Definition identity is resolved only by Registry.
type Service struct {
	ModelRepo  modelcatalogport.ModelRepository
	Published  modelcatalogport.PublishedModelRepository
	Authorizer modelcatalog.Authorizer
	Registry   appdefinition.Registry
	Bindings   appbinding.Policies
	Effects    lifecycle.EffectsRegistry
	Now        func() time.Time
}

func (s Service) Publish(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*modelcatalog.ModelSummary, error) {
	model, err := s.loadAndAuthorize(ctx, actor, modelCode)
	if err != nil {
		return nil, err
	}
	if err := s.Bindings.BeforePublish(ctx, model); err != nil {
		return nil, err
	}
	if model.Kind == domain.KindScale {
		if err := appdefinition.RefreshScaleDraftProjection(model); err != nil {
			return nil, err
		}
	}
	publisher := Publisher{Registry: s.Registry, ModelRepo: s.ModelRepo, Repo: s.Published, Now: s.Now}
	if _, err := publisher.Publish(ctx, model, PublishOptions{ReplaceKind: model.Kind}); err != nil {
		return nil, err
	}
	s.Effects.AfterTransition(ctx, model, lifecycle.ActionPublished)
	return modelcatalog.ModelSummaryFromAssessmentModel(model), nil
}

func (s Service) Unpublish(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*modelcatalog.ModelSummary, error) {
	model, err := s.loadAndAuthorize(ctx, actor, modelCode)
	if err != nil {
		return nil, err
	}
	if err := model.MarkUnpublished(s.now()); err != nil {
		return nil, err
	}
	if err := s.Published.DeletePublished(ctx, model.Kind, model.Code); err != nil {
		return nil, err
	}
	if err := s.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	s.Effects.AfterTransition(ctx, model, lifecycle.ActionUnpublished)
	return modelcatalog.ModelSummaryFromAssessmentModel(model), nil
}

func (s Service) loadAndAuthorize(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*domain.AssessmentModel, error) {
	if modelCode == "" {
		return nil, errors.WithCode(code.ErrInvalidArgument, "model code is required")
	}
	if s.ModelRepo == nil || s.Published == nil || s.Authorizer == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "publication lifecycle service is not configured")
	}
	model, err := s.ModelRepo.FindByCode(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	if err := s.Authorizer.Authorize(ctx, actor, modelcatalog.ActionPublishCatalog, modelcatalog.Resource{Code: model.Code, Kind: model.Kind}); err != nil {
		return nil, err
	}
	return model, nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

var _ modelcatalog.PublicationService = Service{}
