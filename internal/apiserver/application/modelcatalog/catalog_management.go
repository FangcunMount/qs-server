package modelcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// AssessmentCatalogManagementService owns catalogue metadata and binding
// commands. Definition editing and publication remain separate use cases.
type AssessmentCatalogManagementService struct {
	ModelRepo         modelcatalogport.ModelRepository
	Published         modelcatalogport.PublishedModelRepository
	Authorizer        Authorizer
	BindingPolicies   QuestionnaireBindingPolicies
	Effects           LifecycleEffectsRegistry
	Now               func() time.Time
	GenerateScaleCode func() (string, error)
}

func (s AssessmentCatalogManagementService) Create(ctx context.Context, actor ActorContext, input CreateModelDTO) (*ModelSummary, error) {
	kind, ok := APIKindToDomainKind(input.Kind)
	if !ok {
		return nil, errors.WithCode(code.ErrInvalidArgument, "model kind is invalid")
	}
	if err := s.authorize(ctx, actor, Resource{Kind: kind}); err != nil {
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
		if err := initializeScaleDefinition(model, s.now()); err != nil {
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
	return modelSummaryFromAssessmentModel(model), nil
}

func (s AssessmentCatalogManagementService) UpdateBasicInfo(ctx context.Context, actor ActorContext, input UpdateBasicInfoDTO) (*ModelSummary, error) {
	model, err := s.loadAndAuthorize(ctx, actor, input.Code)
	if err != nil {
		return nil, err
	}
	if err := model.UpdateBasicInfo(input.Title, input.Description, domain.SubKind(input.SubKind), domain.Algorithm(input.Algorithm), domain.ProductChannel(input.ProductChannel), input.Category, input.Tags, s.now()); err != nil {
		return nil, err
	}
	if model.Kind == domain.KindScale {
		if err := model.UpdateAudienceMetadata(input.Stages, input.ApplicableAges, input.Reporters, s.now()); err != nil {
			return nil, err
		}
		if err := refreshScaleDraftProjection(model); err != nil {
			return nil, err
		}
	}
	if err := s.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return modelSummaryFromAssessmentModel(model), nil
}

func (s AssessmentCatalogManagementService) BindQuestionnaire(ctx context.Context, actor ActorContext, input BindQuestionnaireDTO) (*QuestionnaireBindingResult, error) {
	model, err := s.loadAndAuthorize(ctx, actor, input.Code)
	if err != nil {
		return nil, err
	}
	binding, err := s.BindingPolicies.Validate(ctx, model, domain.QuestionnaireBinding{QuestionnaireCode: input.QuestionnaireCode, QuestionnaireVersion: input.QuestionnaireVersion})
	if err != nil {
		return nil, err
	}
	if err := model.BindQuestionnaire(binding, s.now()); err != nil {
		return nil, err
	}
	if model.Kind == domain.KindScale {
		if err := refreshScaleDraftProjection(model); err != nil {
			return nil, err
		}
	}
	if err := s.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return &QuestionnaireBindingResult{QuestionnaireCode: binding.QuestionnaireCode, QuestionnaireVersion: binding.QuestionnaireVersion}, nil
}

func (s AssessmentCatalogManagementService) Archive(ctx context.Context, actor ActorContext, modelCode string) (*ModelSummary, error) {
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
	s.Effects.AfterTransition(ctx, model, LifecycleActionArchived)
	return modelSummaryFromAssessmentModel(model), nil
}

func (s AssessmentCatalogManagementService) Delete(ctx context.Context, actor ActorContext, modelCode string) error {
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
func (s AssessmentCatalogManagementService) SynchronizeQuestionnaireVersion(ctx context.Context, actor ActorContext, questionnaireCode, questionnaireVersion string) error {
	if !isTrustedServiceActor(actor) {
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
		if err := s.Authorizer.Authorize(ctx, actor, ActionManageCatalog, Resource{Code: model.Code, Kind: model.Kind}); err != nil {
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
			if err := refreshScaleDraftProjection(model); err != nil {
				return err
			}
		}
		if err := s.ModelRepo.Update(ctx, model); err != nil {
			return err
		}
	}
	return nil
}

func (s AssessmentCatalogManagementService) loadAndAuthorize(ctx context.Context, actor ActorContext, modelCode string) (*domain.AssessmentModel, error) {
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
	if err := s.authorize(ctx, actor, Resource{Code: model.Code, Kind: model.Kind}); err != nil {
		return nil, err
	}
	return model, nil
}

func (s AssessmentCatalogManagementService) authorize(ctx context.Context, actor ActorContext, resource Resource) error {
	if s.Authorizer == nil {
		return errors.WithCode(code.ErrInternalServerError, "catalogue authorizer is not configured")
	}
	return s.Authorizer.Authorize(ctx, actor, ActionManageCatalog, resource)
}

func (s AssessmentCatalogManagementService) createCode(kind domain.Kind, requested string) (string, error) {
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

func (s AssessmentCatalogManagementService) now() time.Time {
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
	case domain.KindBehavioralRating:
		return domain.AlgorithmBrief2
	case domain.KindCognitive:
		return domain.AlgorithmSPM
	default:
		return ""
	}
}

func modelSummaryFromAssessmentModel(model *domain.AssessmentModel) *ModelSummary {
	if model == nil {
		return nil
	}
	result := &ModelSummary{
		Code:                 model.Code,
		Kind:                 DomainKindToAPIKind(model.Kind),
		SubKind:              string(model.SubKind),
		Algorithm:            string(model.Algorithm),
		Title:                model.Title,
		Description:          model.Description,
		Status:               string(model.Status),
		Category:             model.Category,
		Stages:               append([]string(nil), model.Stages...),
		ApplicableAges:       append([]string(nil), model.ApplicableAges...),
		Reporters:            append([]string(nil), model.Reporters...),
		Tags:                 append([]string(nil), model.Tags...),
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		CreatedAt:            model.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            model.UpdatedAt.Format(time.RFC3339),
	}
	populateModelSummaryIdentity(result, model.Kind, model.SubKind, model.Algorithm, model.ProductChannel)
	return result
}

func initializeScaleDefinition(model *domain.AssessmentModel, now time.Time) error {
	if model == nil || model.Kind != domain.KindScale {
		return nil
	}
	model.DefinitionV2 = &domain.Definition{}
	return refreshScaleDraftProjectionAt(model, now)
}

func refreshScaleDraftProjection(model *domain.AssessmentModel) error {
	return refreshScaleDraftProjectionAt(model, time.Now().UTC())
}

func refreshScaleDraftProjectionAt(model *domain.AssessmentModel, now time.Time) error {
	if model == nil || model.Kind != domain.KindScale || model.DefinitionV2 == nil {
		return nil
	}
	snapshot := scalepayload.ScaleSnapshotFromDefinition(scalepayload.ExecutionEnvelope{
		Code:                 model.Code,
		ScaleVersion:         "1.0.0",
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status:               string(model.Status),
	}, model.DefinitionV2)
	if snapshot == nil {
		return fmt.Errorf("scale definition projection is empty")
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	model.Definition = domain.DefinitionPayload{Format: domain.PayloadFormatAssessmentScaleV1, Data: payload}
	model.UpdatedAt = now
	return nil
}
