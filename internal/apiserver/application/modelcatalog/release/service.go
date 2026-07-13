// Package release owns the paired questionnaire/model publication boundary.
package release

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appbinding "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/binding"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/lifecycle"
	publication "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	questionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// Service coordinates a complete release in one Mongo session transaction.
// It is deliberately the only caller allowed to publish a questionnaire for a
// model, so the questionnaire version cannot be supplied by a client.
type Service struct {
	Transactions   apptransaction.Runner
	Models         modelcatalogport.ModelRepository
	Published      modelcatalogport.PublishedModelRepository
	Authorizer     modelcatalog.Authorizer
	Registry       appdefinition.Registry
	Bindings       appbinding.Policies
	Questionnaires questionnaire.QuestionnaireLifecycleService
	Effects        lifecycle.EffectsRegistry
	Now            func() time.Time
}

var _ modelcatalog.AssessmentReleaseService = Service{}

func (s Service) PublishRelease(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*modelcatalog.AssessmentRelease, error) {
	start := time.Now()
	var result *modelcatalog.AssessmentRelease
	err := s.withTransaction(ctx, func(txCtx context.Context) error {
		model, err := s.loadAndAuthorize(txCtx, actor, modelCode, modelcatalog.ActionPublishCatalog)
		if err != nil {
			return err
		}
		if model.IsPublished() {
			return errors.WithCode(code.ErrInvalidArgument, "assessment release is already published")
		}
		if model.Binding.QuestionnaireCode == "" {
			return errors.WithCode(code.ErrInvalidArgument, "assessment release requires a questionnaire binding")
		}
		questionnaireResult, err := s.Questionnaires.PublishForRelease(txCtx, model.Binding.QuestionnaireCode)
		if err != nil {
			return err
		}
		binding, err := s.Bindings.Validate(txCtx, model, domain.QuestionnaireBinding{
			QuestionnaireCode:    questionnaireResult.Code,
			QuestionnaireVersion: questionnaireResult.Version,
		})
		if err != nil {
			return err
		}
		if binding != model.Binding {
			if err := model.BindQuestionnaire(binding, s.now()); err != nil {
				return err
			}
			if model.Kind == domain.KindScale {
				if err := appdefinition.RefreshScaleDraftProjection(model); err != nil {
					return err
				}
			}
			if err := s.Models.Update(txCtx, model); err != nil {
				return err
			}
		}
		publisher := publication.Publisher{Registry: s.Registry, ModelRepo: s.Models, Repo: s.Published, Now: s.Now}
		if _, err := publisher.Publish(txCtx, model, publication.PublishOptions{ReplaceKind: model.Kind}); err != nil {
			return err
		}
		result = releaseFrom(model, questionnaireResult.Status, "")
		return nil
	})
	if err != nil {
		logger.L(ctx).Errorw("测评发布事务失败", "release_action", "publish", "model_code", modelCode, "transaction_result", "rolled_back", "duration_ms", time.Since(start).Milliseconds(), "error", err.Error())
		return nil, err
	}
	// Effects run after the transaction has committed, avoiding a QR/cache
	// consumer observing a release that later rolls back.
	model, err := s.Models.FindByCode(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	s.Effects.AfterTransition(ctx, model, lifecycle.ActionPublished)
	logger.L(ctx).Infow("测评发布成功", "release_action", "publish", "model_code", result.ModelCode, "questionnaire_code", result.QuestionnaireCode, "questionnaire_version", result.QuestionnaireVersion, "transaction_result", "committed", "duration_ms", time.Since(start).Milliseconds())
	return result, nil
}

func (s Service) ArchiveRelease(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*modelcatalog.AssessmentRelease, error) {
	start := time.Now()
	var result *modelcatalog.AssessmentRelease
	err := s.withTransaction(ctx, func(txCtx context.Context) error {
		model, err := s.loadAndAuthorize(txCtx, actor, modelCode, modelcatalog.ActionManageCatalog)
		if err != nil {
			return err
		}
		if model.Binding.QuestionnaireCode == "" {
			return errors.WithCode(code.ErrInvalidArgument, "assessment release requires a questionnaire binding")
		}
		questionnaireResult, err := s.Questionnaires.ArchiveForRelease(txCtx, model.Binding.QuestionnaireCode)
		if err != nil {
			return err
		}
		if !model.IsArchived() {
			if model.IsPublished() {
				if err := s.Published.DeletePublished(txCtx, model.Kind, model.Code); err != nil {
					return err
				}
			}
			if err := model.MarkArchived(s.now()); err != nil {
				return err
			}
			if err := s.Models.Update(txCtx, model); err != nil {
				return err
			}
		}
		result = releaseFrom(model, questionnaireResult.Status, "archived")
		return nil
	})
	if err != nil {
		logger.L(ctx).Errorw("测评归档事务失败", "release_action", "archive", "model_code", modelCode, "transaction_result", "rolled_back", "duration_ms", time.Since(start).Milliseconds(), "error", err.Error())
		return nil, err
	}
	model, err := s.Models.FindByCode(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	s.Effects.AfterTransition(ctx, model, lifecycle.ActionArchived)
	logger.L(ctx).Infow("测评归档成功", "release_action", "archive", "model_code", result.ModelCode, "questionnaire_code", result.QuestionnaireCode, "questionnaire_version", result.QuestionnaireVersion, "transaction_result", "committed", "duration_ms", time.Since(start).Milliseconds())
	return result, nil
}

func (s Service) withTransaction(ctx context.Context, fn func(context.Context) error) error {
	if s.Transactions == nil {
		return errors.WithCode(code.ErrInternalServerError, "assessment release transaction runner is not configured")
	}
	return s.Transactions.WithinTransaction(ctx, fn)
}

func (s Service) loadAndAuthorize(ctx context.Context, actor modelcatalog.ActorContext, modelCode string, action modelcatalog.Action) (*domain.AssessmentModel, error) {
	if modelCode == "" {
		return nil, errors.WithCode(code.ErrInvalidArgument, "model code is required")
	}
	if s.Models == nil || s.Published == nil || s.Questionnaires == nil || s.Authorizer == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "assessment release service is not configured")
	}
	model, err := s.Models.FindByCode(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	if err := s.Authorizer.Authorize(ctx, actor, action, modelcatalog.Resource{Code: model.Code, Kind: model.Kind}); err != nil {
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

func releaseFrom(model *domain.AssessmentModel, questionnaireStatus, forcedStatus string) *modelcatalog.AssessmentRelease {
	status := string(model.Status)
	if forcedStatus != "" {
		status = forcedStatus
	}
	result := &modelcatalog.AssessmentRelease{
		ModelCode: model.Code, ModelStatus: status,
		QuestionnaireCode: model.Binding.QuestionnaireCode, QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		QuestionnaireStatus: questionnaireStatus,
	}
	if model.PublishedAt != nil {
		result.PublishedAt = model.PublishedAt.UTC().Format(time.RFC3339)
	}
	if model.ArchivedAt != nil {
		result.ArchivedAt = model.ArchivedAt.UTC().Format(time.RFC3339)
	}
	return result
}
