package taskperformance

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type Service interface {
	List(ctx context.Context, input ListInput) (*ModelListResult, error)
	Create(ctx context.Context, input CreateInput) (*ModelSummary, error)
	Get(ctx context.Context, modelCode string) (*ModelSummary, error)
	UpdateBasicInfo(ctx context.Context, input UpdateBasicInfoInput) (*ModelSummary, error)
	Delete(ctx context.Context, modelCode string) error
	BindQuestionnaire(ctx context.Context, input BindQuestionnaireInput) (*QuestionnaireBindingResult, error)
	GetDefinition(ctx context.Context, modelCode string) (*DefinitionResult, error)
	UpdateDefinition(ctx context.Context, modelCode string, input DefinitionInput) (*DefinitionResult, error)
	Publish(ctx context.Context, modelCode string) (*ModelSummary, error)
	Unpublish(ctx context.Context, modelCode string) (*ModelSummary, error)
	Archive(ctx context.Context, modelCode string) (*ModelSummary, error)
}

type Dependencies struct {
	ModelRepo          port.ModelRepository
	PublishedRepo      port.PublishedModelRepository
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
}

type service struct {
	deps Dependencies
}

func NewService(deps Dependencies) Service {
	return &service{deps: deps}
}

func (s *service) List(ctx context.Context, input ListInput) (*ModelListResult, error) {
	if s.deps.ModelRepo == nil {
		return &ModelListResult{Page: input.Page, PageSize: input.PageSize}, nil
	}
	models, total, err := s.deps.ModelRepo.List(ctx, port.ListFilter{
		Kind:     domain.KindCognitive,
		Status:   domain.ModelStatus(input.Status),
		Keyword:  input.Keyword,
		Page:     input.Page,
		PageSize: input.PageSize,
	})
	if err != nil {
		return nil, err
	}
	result := &ModelListResult{
		Page:     input.Page,
		PageSize: input.PageSize,
		Total:    total,
		Items:    make([]ModelSummary, 0, len(models)),
	}
	for _, model := range models {
		result.Items = append(result.Items, *summaryFromModel(model))
	}
	return result, nil
}

func (s *service) Create(ctx context.Context, input CreateInput) (*ModelSummary, error) {
	if s.deps.ModelRepo == nil {
		return nil, unavailable("认知模型仓储未配置")
	}
	now := time.Now().UTC()
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:           input.Code,
		Kind:           domain.KindCognitive,
		Algorithm:      domain.AlgorithmSPM,
		ProductChannel: domain.ProductChannel(input.ProductChannel),
		Title:          input.Title,
		Description:    input.Description,
		Category:       input.Category,
		Tags:           input.Tags,
		Now:            now,
	})
	if err != nil {
		return nil, mapDomainError(err)
	}
	if input.QuestionnaireCode != "" || input.QuestionnaireVersion != "" {
		if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
			QuestionnaireCode:    input.QuestionnaireCode,
			QuestionnaireVersion: input.QuestionnaireVersion,
		}, now); err != nil {
			return nil, mapDomainError(err)
		}
	}
	if err := s.deps.ModelRepo.Create(ctx, model); err != nil {
		return nil, err
	}
	return summaryFromModel(model), nil
}

func (s *service) Get(ctx context.Context, modelCode string) (*ModelSummary, error) {
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return summaryFromModel(model), nil
}

func (s *service) UpdateBasicInfo(ctx context.Context, input UpdateBasicInfoInput) (*ModelSummary, error) {
	model, err := s.loadModel(ctx, input.Code)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := model.UpdateBasicInfo(
		input.Title,
		input.Description,
		"",
		"",
		domain.ProductChannel(input.ProductChannel),
		input.Category,
		input.Tags,
		now,
	); err != nil {
		return nil, mapDomainError(err)
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return summaryFromModel(model), nil
}

func (s *service) Delete(ctx context.Context, modelCode string) error {
	if s.deps.ModelRepo == nil {
		return unavailable("认知模型仓储未配置")
	}
	return s.deps.ModelRepo.Delete(ctx, modelCode)
}

func (s *service) BindQuestionnaire(ctx context.Context, input BindQuestionnaireInput) (*QuestionnaireBindingResult, error) {
	model, err := s.loadModel(ctx, input.Code)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
	}, now); err != nil {
		return nil, mapDomainError(err)
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return &QuestionnaireBindingResult{
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
	}, nil
}

func (s *service) GetDefinition(ctx context.Context, modelCode string) (*DefinitionResult, error) {
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return definitionResultFromModel(model), nil
}

func (s *service) UpdateDefinition(ctx context.Context, modelCode string, input DefinitionInput) (*DefinitionResult, error) {
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := model.UpdateDefinition(domain.DefinitionPayload{Data: append([]byte(nil), input.Payload...)}, now); err != nil {
		return nil, mapDomainError(err)
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return definitionResultFromModel(model), nil
}

func definitionResultFromModel(model *domain.AssessmentModel) *DefinitionResult {
	if model == nil {
		return nil
	}
	return &DefinitionResult{
		Kind:           KindCognitive,
		Algorithm:      string(model.Algorithm),
		ProductChannel: string(domain.ResolveProductChannel(model.Kind, model.ProductChannel)),
		PayloadFormat:  draftPayloadFormat(model),
		Payload:        append([]byte(nil), model.Definition.Data...),
	}
}

func draftPayloadFormat(model *domain.AssessmentModel) string {
	algorithm := model.Algorithm
	if algorithm == "" {
		algorithm = domain.AlgorithmSPM
	}
	return domain.DraftPayloadFormatForModel(domain.KindCognitive, algorithm)
}

func (s *service) Publish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	if s.deps.PublishedRepo == nil {
		return nil, unavailable("认知模型发布仓储未配置")
	}
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	if model.Definition.IsEmpty() {
		return nil, invalidArgument("认知模型定义不能为空")
	}
	if err := publishValidationError(model); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := model.MarkPublished(now); err != nil {
		return nil, mapDomainError(err)
	}
	snapshot, err := publishing.BuildPublishedSnapshot(model)
	if err != nil {
		return nil, invalidArgument("%s", err.Error())
	}
	if err := s.deps.PublishedRepo.DeletePublished(ctx, domain.KindCognitive, modelCode); err != nil {
		return nil, err
	}
	if err := s.deps.PublishedRepo.Save(ctx, snapshot); err != nil {
		return nil, err
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		_ = s.deps.PublishedRepo.DeletePublished(ctx, domain.KindCognitive, modelCode)
		return nil, err
	}
	return summaryFromModel(model), nil
}

func (s *service) Unpublish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if s.deps.PublishedRepo != nil {
		if err := s.deps.PublishedRepo.DeletePublished(ctx, domain.KindCognitive, modelCode); err != nil {
			return nil, err
		}
	}
	if err := model.MarkUnpublished(now); err != nil {
		return nil, mapDomainError(err)
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return summaryFromModel(model), nil
}

func (s *service) Archive(ctx context.Context, modelCode string) (*ModelSummary, error) {
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if s.deps.PublishedRepo != nil {
		_ = s.deps.PublishedRepo.DeletePublished(ctx, domain.KindCognitive, modelCode)
	}
	if err := model.MarkArchived(now); err != nil {
		return nil, mapDomainError(err)
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return summaryFromModel(model), nil
}

func (s *service) loadModel(ctx context.Context, modelCode string) (*domain.AssessmentModel, error) {
	if modelCode == "" {
		return nil, invalidArgument("模型编码不能为空")
	}
	if s.deps.ModelRepo == nil {
		return nil, unavailable("认知模型仓储未配置")
	}
	model, err := s.deps.ModelRepo.FindByCode(ctx, modelCode)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil, errors.WithCode(code.ErrMedicalScaleNotFound, "测评模型不存在")
		}
		return nil, err
	}
	if model.Kind != domain.KindCognitive {
		return nil, errors.WithCode(code.ErrMedicalScaleNotFound, "测评模型不存在")
	}
	return model, nil
}

func unavailable(msg string) error {
	return errors.WithCode(code.ErrInternalServerError, "%s", msg)
}

func invalidArgument(format string, args ...any) error {
	return errors.WithCode(code.ErrInvalidArgument, format, args...)
}

func mapDomainError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case stderrors.Is(err, domain.ErrInvalidArgument):
		return invalidArgument("%s", err.Error())
	case stderrors.Is(err, domain.ErrInvalidState):
		return invalidArgument("%s", err.Error())
	default:
		return err
	}
}
