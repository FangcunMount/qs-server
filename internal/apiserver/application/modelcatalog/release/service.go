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
	appevolution "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/evolution"
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
	Transactions       apptransaction.Runner
	Models             modelcatalogport.ModelRepository
	Published          modelcatalogport.PublishedSnapshotRepository
	Authorizer         modelcatalog.Authorizer
	Registry           appdefinition.Registry
	Bindings           appbinding.Policies
	Evolution          appevolution.Policy
	Questionnaires     questionnaire.QuestionnaireLifecycleService
	QuestionnaireQuery questionnaire.QuestionnaireQueryService
	Effects            lifecycle.EffectsRegistry
	Now                func() time.Time
}

var _ modelcatalog.AssessmentReleaseService = Service{}

func (s Service) PublishRelease(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*modelcatalog.AssessmentRelease, error) {
	start := time.Now()
	var result *modelcatalog.AssessmentRelease
	var transitionedModel *domain.AssessmentModel
	alreadyPublished := false
	err := s.withTransaction(ctx, func(txCtx context.Context) error {
		model, err := s.loadAndAuthorize(txCtx, actor, modelCode, modelcatalog.ActionPublishCatalog)
		if err != nil {
			return err
		}
		if model.IsPublished() {
			if err := s.ensurePublishedPair(txCtx, model); err != nil {
				return err
			}
			alreadyPublished = true
			result = releaseFrom(model, "published", "")
			return nil
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
				return modelcatalog.MapDraftWriteError(err)
			}
		}
		if err := s.Evolution.GuardPublishIdentity(txCtx, model); err != nil {
			return err
		}
		if err := s.Bindings.BeforePublish(txCtx, model); err != nil {
			return err
		}
		publisher := publication.Publisher{Registry: s.Registry, ModelRepo: s.Models, Repo: s.Published, Now: s.Now}
		if _, err := publisher.Publish(txCtx, model, publication.PublishOptions{ReplaceKind: model.Kind}); err != nil {
			return err
		}
		transitionedModel = model
		result = releaseFrom(model, questionnaireResult.Status, "")
		return nil
	})
	if err != nil {
		logger.L(ctx).Errorw("测评发布事务失败", "release_action", "publish", "model_code", modelCode, "transaction_result", "rolled_back", "duration_ms", time.Since(start).Milliseconds(), "error", err.Error())
		return nil, err
	}
	if alreadyPublished {
		logger.L(ctx).Infow("测评发布幂等命中", "release_action", "publish", "model_code", result.ModelCode, "questionnaire_code", result.QuestionnaireCode, "questionnaire_version", result.QuestionnaireVersion, "previous_status", "published", "transaction_result", "committed", "duration_ms", time.Since(start).Milliseconds())
		return result, nil
	}
	// Effects run after the transaction has committed, avoiding a QR/cache
	// consumer observing a release that later rolls back.
	s.invalidateQuestionnaireCache(ctx, result.QuestionnaireCode)
	s.Effects.AfterTransition(ctx, transitionedModel, lifecycle.ActionPublished)
	logger.L(ctx).Infow("测评发布成功", "release_action", "publish", "model_code", result.ModelCode, "questionnaire_code", result.QuestionnaireCode, "questionnaire_version", result.QuestionnaireVersion, "transaction_result", "committed", "duration_ms", time.Since(start).Milliseconds())
	return result, nil
}

// UnpublishRelease atomically takes the current questionnaire/model pair
// offline. Retained snapshots remain exact-version readable for assessments
// created before the transition.
func (s Service) UnpublishRelease(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*modelcatalog.AssessmentRelease, error) {
	start := time.Now()
	var result *modelcatalog.AssessmentRelease
	var transitionedModel *domain.AssessmentModel
	err := s.withTransaction(ctx, func(txCtx context.Context) error {
		model, err := s.loadAndAuthorize(txCtx, actor, modelCode, modelcatalog.ActionPublishCatalog)
		if err != nil {
			return err
		}
		if model.Binding.QuestionnaireCode == "" {
			return errors.WithCode(code.ErrInvalidArgument, "assessment release requires a questionnaire binding")
		}
		questionnaireResult, err := s.Questionnaires.UnpublishForRelease(txCtx, model.Binding.QuestionnaireCode)
		if err != nil {
			return err
		}
		if err := s.Published.DeletePublished(txCtx, model.Kind, model.Code); err != nil {
			return err
		}
		if model.IsPublished() {
			if err := model.MarkUnpublished(s.now()); err != nil {
				return err
			}
			if err := s.Models.Update(txCtx, model); err != nil {
				return modelcatalog.MapDraftWriteError(err)
			}
		}
		transitionedModel = model
		result = releaseFrom(model, questionnaireResult.Status, "draft")
		return nil
	})
	if err != nil {
		logger.L(ctx).Errorw("测评下架事务失败", "release_action", "unpublish", "model_code", modelCode, "transaction_result", "rolled_back", "duration_ms", time.Since(start).Milliseconds(), "error", err.Error())
		return nil, err
	}
	s.invalidateQuestionnaireCache(ctx, result.QuestionnaireCode)
	s.Effects.AfterTransition(ctx, transitionedModel, lifecycle.ActionUnpublished)
	logger.L(ctx).Infow("测评下架成功", "release_action", "unpublish", "model_code", result.ModelCode, "questionnaire_code", result.QuestionnaireCode, "questionnaire_version", result.QuestionnaireVersion, "transaction_result", "committed", "duration_ms", time.Since(start).Milliseconds())
	return result, nil
}

func (s Service) ArchiveRelease(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*modelcatalog.AssessmentRelease, error) {
	start := time.Now()
	var result *modelcatalog.AssessmentRelease
	var transitionedModel *domain.AssessmentModel
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
			// A draft head can still have an older active release. Archiving the
			// release family must therefore archive by snapshot state rather than
			// by the mutable head status.
			if err := s.Published.DeletePublished(txCtx, model.Kind, model.Code); err != nil {
				return err
			}
			if err := model.MarkArchived(s.now()); err != nil {
				return err
			}
			if err := s.Models.Update(txCtx, model); err != nil {
				return modelcatalog.MapDraftWriteError(err)
			}
		}
		transitionedModel = model
		result = releaseFrom(model, questionnaireResult.Status, "archived")
		return nil
	})
	if err != nil {
		logger.L(ctx).Errorw("测评归档事务失败", "release_action", "archive", "model_code", modelCode, "transaction_result", "rolled_back", "duration_ms", time.Since(start).Milliseconds(), "error", err.Error())
		return nil, err
	}
	s.invalidateQuestionnaireCache(ctx, result.QuestionnaireCode)
	s.Effects.AfterTransition(ctx, transitionedModel, lifecycle.ActionArchived)
	logger.L(ctx).Infow("测评归档成功", "release_action", "archive", "model_code", result.ModelCode, "questionnaire_code", result.QuestionnaireCode, "questionnaire_version", result.QuestionnaireVersion, "transaction_result", "committed", "duration_ms", time.Since(start).Milliseconds())
	return result, nil
}

func (s Service) ensurePublishedPair(ctx context.Context, model *domain.AssessmentModel) error {
	if model == nil {
		return errors.WithCode(code.ErrInvalidArgument, "assessment model is required")
	}
	if model.Binding.QuestionnaireCode == "" || model.Binding.QuestionnaireVersion == "" {
		return errors.WithCode(code.ErrConflict, "release.pair.incomplete: published model is missing questionnaire binding")
	}
	active, err := s.Published.FindPublishedByModelCode(ctx, model.Kind, model.Code)
	if err != nil {
		if domain.IsNotFound(err) {
			return errors.WithCode(code.ErrConflict, "release.pair.incomplete: active assessment snapshot is missing")
		}
		return err
	}
	if active == nil {
		return errors.WithCode(code.ErrConflict, "release.pair.incomplete: active assessment snapshot is missing")
	}
	if status := domain.NormalizeReleaseStatus(active.ReleaseStatus, active.Status == "published"); status != "" && !status.IsActive() {
		return errors.WithCode(code.ErrConflict, "release.pair.incomplete: active assessment snapshot is missing")
	}
	if active.QuestionnaireCode != model.Binding.QuestionnaireCode || active.QuestionnaireVersion != model.Binding.QuestionnaireVersion {
		return errors.WithCode(code.ErrConflict, "release.pair.incomplete: active snapshot questionnaire binding does not match model head")
	}
	if s.QuestionnaireQuery == nil {
		return errors.WithCode(code.ErrInternalServerError, "assessment release questionnaire query is not configured")
	}
	publishedQ, err := s.QuestionnaireQuery.GetPublishedByCodeVersion(ctx, model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion)
	if err != nil {
		return errors.WithCode(code.ErrConflict, "release.questionnaire.not_active: %v", err)
	}
	if publishedQ == nil || publishedQ.Status != "published" {
		return errors.WithCode(code.ErrConflict, "release.questionnaire.not_active: questionnaire side is not published")
	}
	return nil
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

func (s Service) invalidateQuestionnaireCache(ctx context.Context, code string) {
	if invalidator, ok := s.Questionnaires.(questionnaire.QuestionnaireReleaseCacheInvalidator); ok {
		invalidator.InvalidateReleaseCache(ctx, code)
	}
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
