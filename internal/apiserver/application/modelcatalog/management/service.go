// Package management owns actor-authorized catalog metadata and binding use cases.
package management

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
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Service owns catalogue metadata and binding commands. Definition editing and
// publication remain separate use cases.
type Service struct {
	ModelRepo         modelcatalogport.ModelRepository
	Published         modelcatalogport.PublishedModelRepository
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

func (s Service) UpdateBasicInfo(ctx context.Context, actor modelcatalog.ActorContext, input modelcatalog.UpdateBasicInfoDTO) (*modelcatalog.ModelSummary, error) {
	model, err := s.loadAndAuthorize(ctx, actor, input.Code)
	if err != nil {
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
	case domain.KindBehavioralRating:
		return domain.AlgorithmBrief2
	case domain.KindCognitive:
		return domain.AlgorithmSPM
	default:
		return ""
	}
}

var _ modelcatalog.CatalogManagementService = Service{}
