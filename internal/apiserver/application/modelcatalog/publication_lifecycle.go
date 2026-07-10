package modelcatalog

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// AssessmentPublicationService owns actor-authorized publish and unpublish
// commands. Definition identity is resolved only by Registry.
type AssessmentPublicationService struct {
	ModelRepo  modelcatalogport.ModelRepository
	Published  modelcatalogport.PublishedModelRepository
	Authorizer Authorizer
	Registry   appdefinition.Registry
	Now        func() time.Time
	After      func(context.Context, *domain.AssessmentModel, string)
}

func (s AssessmentPublicationService) Publish(ctx context.Context, actor ActorContext, modelCode string) (*domain.AssessmentModel, error) {
	model, err := s.loadAndAuthorize(ctx, actor, modelCode)
	if err != nil {
		return nil, err
	}
	publisher := publication.Publisher{Registry: s.Registry, ModelRepo: s.ModelRepo, Repo: s.Published, Now: s.Now}
	if _, err := publisher.Publish(ctx, model, publication.PublishOptions{ReplaceKind: model.Kind}); err != nil {
		return nil, err
	}
	s.after(ctx, model, "publish")
	return model, nil
}

func (s AssessmentPublicationService) Unpublish(ctx context.Context, actor ActorContext, modelCode string) (*domain.AssessmentModel, error) {
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
	s.after(ctx, model, "unpublish")
	return model, nil
}

func (s AssessmentPublicationService) loadAndAuthorize(ctx context.Context, actor ActorContext, modelCode string) (*domain.AssessmentModel, error) {
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
	if err := s.Authorizer.Authorize(ctx, actor, ActionPublishCatalog, Resource{Code: model.Code, Kind: model.Kind}); err != nil {
		return nil, err
	}
	return model, nil
}

func (s AssessmentPublicationService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s AssessmentPublicationService) after(ctx context.Context, model *domain.AssessmentModel, action string) {
	if s.After != nil && model != nil {
		s.After(ctx, model, action)
	}
}
