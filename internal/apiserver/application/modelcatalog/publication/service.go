package publication

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appbinding "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/binding"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	appevolution "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/evolution"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/lifecycle"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Service owns actor-authorized publish and unpublish
// commands. Definition identity is resolved only by Registry.
type Service struct {
	Transactions apptransaction.Runner
	ModelRepo    modelcatalogport.ModelRepository
	Published    modelcatalogport.PublishedSnapshotRepository
	Authorizer   modelcatalog.Authorizer
	Registry     appdefinition.Registry
	Bindings     appbinding.Policies
	Evolution    appevolution.Policy
	Effects      lifecycle.EffectsRegistry
	Now          func() time.Time
}

func (s Service) Publish(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*modelcatalog.ModelSummary, error) {
	model, err := s.loadAndAuthorize(ctx, actor, modelCode)
	if err != nil {
		return nil, err
	}
	if s.Transactions == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "publication transaction runner is not configured")
	}
	if err := s.Transactions.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.Evolution.GuardPublishIdentity(txCtx, model); err != nil {
			return err
		}
		if err := s.Bindings.BeforePublish(txCtx, model); err != nil {
			return err
		}
		if model.Kind == domain.KindScale {
			if err := appdefinition.RefreshScaleDraftProjection(model); err != nil {
				return err
			}
		}
		publisher := Publisher{Registry: s.Registry, ModelRepo: s.ModelRepo, Repo: s.Published, Now: s.Now}
		_, err := publisher.Publish(txCtx, model, PublishOptions{ReplaceKind: model.Kind})
		return err
	}); err != nil {
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
		return nil, modelcatalog.MapDraftWriteError(err)
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
