// Package management owns actor-authorized catalog metadata and binding use cases.
package management

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	appbinding "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/binding"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/lifecycle"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Service owns catalogue metadata and binding commands. Definition editing and
// publication remain separate use cases.
type Service struct {
	ModelRepo         modelcatalogport.ModelRepository
	Published         modelcatalogport.PublishedSnapshotRepository
	Authorizer        modelcatalog.Authorizer
	BindingPolicies   appbinding.Policies
	Effects           lifecycle.EffectsRegistry
	Now               func() time.Time
	GenerateScaleCode func() (string, error)
}

func (s Service) Create(ctx context.Context, actor modelcatalog.ActorContext, input modelcatalog.CreateModelDTO) (*modelcatalog.ModelSummary, error) {
	kind, ok := modelcatalog.APIKindToDomainKind(input.Kind)
	if !ok {
		return nil, errors.WithCode(code.ErrInvalidArgument, "model kind is invalid")
	}
	if err := domain.ValidateNewProductChannel(kind, domain.ProductChannel(input.ProductChannel)); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, actor, modelcatalog.Resource{Kind: kind}); err != nil {
		return nil, err
	}
	codeValue, err := s.createCode(kind, input.Code)
	if err != nil {
		return nil, err
	}
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:           codeValue,
		Kind:           kind,
		SubKind:        createSubKind(kind, input.SubKind),
		Algorithm:      createAlgorithm(kind, input.Algorithm),
		ProductChannel: domain.ProductChannel(input.ProductChannel),
		Title:          input.Title,
		Description:    input.Description,
		Category:       input.Category,
		Tags:           input.Tags,
		Now:            s.now(),
	})
	if err != nil {
		return nil, err
	}
	if kind == domain.KindScale {
		if err := model.UpdateAudienceMetadata(input.Stages, input.ApplicableAges, input.Reporters, s.now()); err != nil {
			return nil, err
		}
		if err := appdefinition.InitializeScaleDefinition(model, s.now()); err != nil {
			return nil, err
		}
	}
	if input.QuestionnaireCode != "" || input.QuestionnaireVersion != "" {
		binding, err := s.BindingPolicies.Validate(ctx, model, domain.QuestionnaireBinding{QuestionnaireCode: input.QuestionnaireCode, QuestionnaireVersion: input.QuestionnaireVersion})
		if err != nil {
			return nil, err
		}
		if err := model.BindQuestionnaire(binding, s.now()); err != nil {
			return nil, err
		}
	}
	if s.ModelRepo == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "catalogue model repository is not configured")
	}
	if err := s.ModelRepo.Create(ctx, model); err != nil {
		return nil, err
	}
	return modelcatalog.ModelSummaryFromAssessmentModel(model), nil
}

// RestoreDraftFromPublished repairs a legacy state where an active immutable
// snapshot outlived its mutable assessment_models head. The restored model is
// intentionally a draft; the old snapshot remains active until normal release
// publication replaces it.
func (s Service) RestoreDraftFromPublished(ctx context.Context, actor modelcatalog.ActorContext, codeValue string) (*modelcatalog.ModelSummary, error) {
	if codeValue == "" {
		return nil, errors.WithCode(code.ErrInvalidArgument, "model code is required")
	}
	if s.ModelRepo == nil || s.Published == nil || s.Authorizer == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "catalogue stores are not configured")
	}

	if model, err := s.ModelRepo.FindByCode(ctx, codeValue); err == nil {
		if err := s.authorize(ctx, actor, modelcatalog.Resource{Code: model.Code, Kind: model.Kind}); err != nil {
			return nil, err
		}
		logger.L(ctx).Infow("测评草稿恢复幂等命中", "action", "restore_assessment_model_draft", "model_code", model.Code, "result", "draft_exists")
		return modelcatalog.ModelSummaryFromAssessmentModel(model), nil
	} else if !domain.IsNotFound(err) {
		return nil, err
	}

	snapshot, err := s.findPublishedSnapshot(ctx, codeValue)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, actor, modelcatalog.Resource{Code: snapshot.Code, Kind: snapshot.Kind}); err != nil {
		return nil, err
	}

	model := draftFromPublishedSnapshot(snapshot, s.now())
	if err := s.ModelRepo.Create(ctx, model); err != nil {
		if existing, findErr := s.ModelRepo.FindByCode(ctx, codeValue); findErr == nil {
			logger.L(ctx).Infow("测评草稿恢复并发幂等命中", "action", "restore_assessment_model_draft", "model_code", existing.Code, "result", "draft_created_by_peer")
			return modelcatalog.ModelSummaryFromAssessmentModel(existing), nil
		}
		return nil, err
	}
	logger.L(ctx).Infow("已从发布快照恢复测评草稿", "action", "restore_assessment_model_draft", "model_code", model.Code, "model_kind", model.Kind, "snapshot_version", snapshot.Version, "result", "created")
	return modelcatalog.ModelSummaryFromAssessmentModel(model), nil
}

func (s Service) UpdateBasicInfo(ctx context.Context, actor modelcatalog.ActorContext, input modelcatalog.UpdateBasicInfoDTO) (*modelcatalog.ModelSummary, error) {
	model, err := s.loadAndAuthorize(ctx, actor, input.Code)
	if err != nil {
		return nil, err
	}
	if err := model.ForkDraftFromPublished(s.now()); err != nil {
		return nil, err
	}
	if model.Kind == domain.KindScale {
		if err := model.UpdateScaleBasicInfo(input.Title, input.Description, domain.SubKind(input.SubKind), domain.Algorithm(input.Algorithm), domain.ProductChannel(input.ProductChannel), input.Category, input.Tags, input.Stages, input.ApplicableAges, input.Reporters, s.now()); err != nil {
			return nil, err
		}
		if err := appdefinition.RefreshScaleDraftProjection(model); err != nil {
			return nil, err
		}
	} else if err := model.UpdateBasicInfo(input.Title, input.Description, domain.SubKind(input.SubKind), domain.Algorithm(input.Algorithm), domain.ProductChannel(input.ProductChannel), input.Category, input.Tags, s.now()); err != nil {
		return nil, err
	}
	if err := s.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return modelcatalog.ModelSummaryFromAssessmentModel(model), nil
}

func (s Service) BindQuestionnaire(ctx context.Context, actor modelcatalog.ActorContext, input modelcatalog.BindQuestionnaireDTO) (*modelcatalog.QuestionnaireBindingResult, error) {
	model, err := s.loadAndAuthorize(ctx, actor, input.Code)
	if err != nil {
		return nil, err
	}
	binding, err := s.BindingPolicies.Validate(ctx, model, domain.QuestionnaireBinding{QuestionnaireCode: input.QuestionnaireCode, QuestionnaireVersion: input.QuestionnaireVersion})
	if err != nil {
		return nil, err
	}
	if err := model.ForkDraftFromPublished(s.now()); err != nil {
		return nil, err
	}
	if err := model.BindQuestionnaire(binding, s.now()); err != nil {
		return nil, err
	}
	if model.Kind == domain.KindScale {
		if err := appdefinition.RefreshScaleDraftProjection(model); err != nil {
			return nil, err
		}
	}
	if err := s.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return &modelcatalog.QuestionnaireBindingResult{QuestionnaireCode: binding.QuestionnaireCode, QuestionnaireVersion: binding.QuestionnaireVersion}, nil
}

func (s Service) Archive(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*modelcatalog.ModelSummary, error) {
	model, err := s.loadAndAuthorize(ctx, actor, modelCode)
	if err != nil {
		return nil, err
	}
	wasPublished := model.IsPublished()
	if wasPublished {
		if err := s.Published.DeletePublished(ctx, model.Kind, model.Code); err != nil {
			return nil, err
		}
	}
	if err := model.MarkArchived(s.now()); err != nil {
		return nil, err
	}
	if err := s.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	s.Effects.AfterTransition(ctx, model, lifecycle.ActionArchived)
	return modelcatalog.ModelSummaryFromAssessmentModel(model), nil
}

func (s Service) Delete(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) error {
	model, err := s.loadAndAuthorize(ctx, actor, modelCode)
	if err != nil {
		return err
	}
	if !model.IsArchived() {
		return errors.WithCode(code.ErrInvalidArgument, "only archived assessment models can be deleted")
	}
	if s.Published == nil || s.ModelRepo == nil {
		return errors.WithCode(code.ErrInternalServerError, "catalogue stores are not configured")
	}
	if _, err := s.Published.FindPublishedByModelCode(ctx, model.Kind, model.Code); err == nil {
		return errors.WithCode(code.ErrInvalidArgument, "published assessment model must be removed before deletion")
	} else if !domain.IsNotFound(err) {
		return err
	}
	return s.ModelRepo.Delete(ctx, modelCode)
}

// SynchronizeQuestionnaireVersion is the internal actor use case called after
// questionnaire publication. It never mutates a published or archived model.
func (s Service) SynchronizeQuestionnaireVersion(ctx context.Context, actor modelcatalog.ActorContext, questionnaireCode, questionnaireVersion string) error {
	if !modelcatalog.IsTrustedServiceActor(actor) {
		return errors.WithCode(code.ErrPermissionDenied, "trusted service actor is required")
	}
	if questionnaireCode == "" || questionnaireVersion == "" {
		return errors.WithCode(code.ErrInvalidArgument, "questionnaire code and version are required")
	}
	if s.ModelRepo == nil || s.Authorizer == nil {
		return errors.WithCode(code.ErrInternalServerError, "catalogue management service is not configured")
	}
	for _, kind := range []domain.Kind{domain.KindScale, domain.KindTypology, domain.KindBehavioralRating, domain.KindCognitive} {
		model, err := s.ModelRepo.FindByQuestionnaireCode(ctx, kind, questionnaireCode)
		if err != nil {
			if domain.IsNotFound(err) {
				continue
			}
			return err
		}
		if model == nil || !model.IsDraft() || model.Binding.QuestionnaireVersion == questionnaireVersion {
			continue
		}
		if err := s.Authorizer.Authorize(ctx, actor, modelcatalog.ActionManageCatalog, modelcatalog.Resource{Code: model.Code, Kind: model.Kind}); err != nil {
			return err
		}
		binding, err := s.BindingPolicies.Validate(ctx, model, domain.QuestionnaireBinding{QuestionnaireCode: questionnaireCode, QuestionnaireVersion: questionnaireVersion})
		if err != nil {
			return err
		}
		if err := model.BindQuestionnaire(binding, s.now()); err != nil {
			return err
		}
		if model.Kind == domain.KindScale {
			if err := appdefinition.RefreshScaleDraftProjection(model); err != nil {
				return err
			}
		}
		if err := s.ModelRepo.Update(ctx, model); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) loadAndAuthorize(ctx context.Context, actor modelcatalog.ActorContext, modelCode string) (*domain.AssessmentModel, error) {
	if modelCode == "" {
		return nil, errors.WithCode(code.ErrInvalidArgument, "model code is required")
	}
	if s.ModelRepo == nil || s.Authorizer == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "catalogue management service is not configured")
	}
	model, err := s.ModelRepo.FindByCode(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, actor, modelcatalog.Resource{Code: model.Code, Kind: model.Kind}); err != nil {
		return nil, err
	}
	return model, nil
}

func (s Service) findPublishedSnapshot(ctx context.Context, codeValue string) (*modelcatalogport.PublishedModel, error) {
	for _, kind := range []domain.Kind{domain.KindScale, domain.KindTypology, domain.KindBehavioralRating, domain.KindCognitive} {
		snapshot, err := s.Published.FindPublishedByModelCode(ctx, kind, codeValue)
		if err == nil {
			return snapshot, nil
		}
		if !domain.IsNotFound(err) {
			return nil, err
		}
	}
	// A deactivated snapshot is still a valid source for draft repair. This
	// keeps archive/unpublish semantics while preserving historical recovery.
	for _, kind := range []domain.Kind{domain.KindScale, domain.KindTypology, domain.KindBehavioralRating, domain.KindCognitive} {
		snapshot, err := s.Published.FindLatestPublishedByModelCode(ctx, kind, codeValue)
		if err == nil {
			return snapshot, nil
		}
		if !domain.IsNotFound(err) {
			return nil, err
		}
	}
	return nil, domain.ErrNotFound
}

func draftFromPublishedSnapshot(snapshot *modelcatalogport.PublishedModel, now time.Time) *domain.AssessmentModel {
	return &domain.AssessmentModel{
		Code:           snapshot.Code,
		Kind:           snapshot.Kind,
		SubKind:        snapshot.SubKind,
		Algorithm:      snapshot.Algorithm,
		ProductChannel: snapshot.ProductChannel,
		Title:          snapshot.Title,
		Description:    snapshot.Description,
		Category:       snapshot.Category,
		Stages:         append([]string(nil), snapshot.Stages...),
		ApplicableAges: append([]string(nil), snapshot.ApplicableAges...),
		Reporters:      append([]string(nil), snapshot.Reporters...),
		Tags:           append([]string(nil), snapshot.Tags...),
		Status:         domain.ModelStatusDraft,
		Binding: domain.QuestionnaireBinding{
			QuestionnaireCode:    snapshot.QuestionnaireCode,
			QuestionnaireVersion: snapshot.QuestionnaireVersion,
		},
		Definition: domain.DefinitionPayload{
			Format: snapshot.PayloadFormat,
			Data:   append([]byte(nil), snapshot.Payload...),
		},
		DefinitionV2: snapshot.DefinitionV2,
		Version:      revisionFromSnapshotVersion(snapshot.Version),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func revisionFromSnapshotVersion(version string) int64 {
	revision, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(version), "v"), 10, 64)
	if err != nil || revision < 1 {
		return 1
	}
	return revision
}

func (s Service) authorize(ctx context.Context, actor modelcatalog.ActorContext, resource modelcatalog.Resource) error {
	if s.Authorizer == nil {
		return errors.WithCode(code.ErrInternalServerError, "catalogue authorizer is not configured")
	}
	return s.Authorizer.Authorize(ctx, actor, modelcatalog.ActionManageCatalog, resource)
}

func (s Service) createCode(kind domain.Kind, requested string) (string, error) {
	if requested != "" || kind != domain.KindScale {
		return requested, nil
	}
	if s.GenerateScaleCode != nil {
		return s.GenerateScaleCode()
	}
	generated, err := meta.GenerateCode()
	if err != nil {
		return "", err
	}
	return string(generated), nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func createSubKind(kind domain.Kind, input string) domain.SubKind {
	if input != "" {
		return domain.SubKind(input)
	}
	if kind == domain.KindTypology {
		return domain.SubKindTypology
	}
	return ""
}

func createAlgorithm(kind domain.Kind, input string) domain.Algorithm {
	if input != "" {
		return domain.Algorithm(input)
	}
	switch kind {
	case domain.KindScale:
		return domain.AlgorithmScaleDefault
	case domain.KindTypology:
		return domain.AlgorithmPersonalityTypology
	case domain.KindBehavioralRating:
		return domain.AlgorithmBrief2
	case domain.KindCognitive:
		return domain.AlgorithmSPM
	default:
		return ""
	}
}

var _ modelcatalog.CatalogManagementService = Service{}
