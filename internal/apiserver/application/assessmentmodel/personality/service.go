package personality

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	personalitydomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type Service interface {
	List(ctx context.Context, input ListInput) (*ModelListResult, error)
	Create(ctx context.Context, input CreateInput) (*ModelSummary, error)
	Get(ctx context.Context, modelCode string) (*ModelSummary, error)
	UpdateBasicInfo(ctx context.Context, input UpdateBasicInfoInput) (*ModelSummary, error)
	Delete(ctx context.Context, modelCode string) error
	BindQuestionnaire(ctx context.Context, input BindQuestionnaireInput) (*QuestionnaireBindingResult, error)
	GetQuestionnaire(ctx context.Context, modelCode string) (*QuestionnaireBindingResult, error)
	GetDefinition(ctx context.Context, modelCode string) (*DefinitionResult, error)
	UpdateDefinition(ctx context.Context, modelCode string, input DefinitionInput) (*DefinitionResult, error)
	Validate(ctx context.Context, modelCode string) (*ValidationResult, error)
	PreviewReport(ctx context.Context, modelCode string, payload json.RawMessage) (*PreviewReportResult, error)
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
		Kind:      domain.KindPersonality,
		SubKind:   domain.SubKind(input.SubKind),
		Status:    domain.ModelStatus(input.Status),
		Keyword:   input.Keyword,
		Category:  input.Category,
		Algorithm: domain.Algorithm(input.Algorithm),
		Page:      input.Page,
		PageSize:  input.PageSize,
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
		return nil, unavailable("人格模型仓储未配置")
	}
	subKind, algorithm, err := normalizeCreateInput(input)
	if err != nil {
		return nil, invalidArgument("人格模型 algorithm 不能为空")
	}
	now := time.Now().UTC()
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:        input.Code,
		Kind:        domain.KindPersonality,
		SubKind:     subKind,
		Algorithm:   algorithm,
		Title:       input.Title,
		Description: input.Description,
		Category:    input.Category,
		Tags:        input.Tags,
		Now:         now,
	})
	if err != nil {
		return nil, mapDomainError(err)
	}
	if input.QuestionnaireCode != "" || input.QuestionnaireVersion != "" {
		binding, err := s.validateQuestionnaireBinding(ctx, input.QuestionnaireCode, input.QuestionnaireVersion)
		if err != nil {
			return nil, err
		}
		if err := model.BindQuestionnaire(binding, now); err != nil {
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
		domain.SubKind(input.SubKind),
		domain.Algorithm(input.Algorithm),
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
		return unavailable("人格模型仓储未配置")
	}
	return s.deps.ModelRepo.Delete(ctx, modelCode)
}

func (s *service) BindQuestionnaire(ctx context.Context, input BindQuestionnaireInput) (*QuestionnaireBindingResult, error) {
	model, err := s.loadModel(ctx, input.Code)
	if err != nil {
		return nil, err
	}
	binding, err := s.validateQuestionnaireBinding(ctx, input.QuestionnaireCode, input.QuestionnaireVersion)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := model.BindQuestionnaire(binding, now); err != nil {
		return nil, mapDomainError(err)
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return questionnaireBinding(ctx, s.deps.QuestionnaireQuery, model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion)
}

func (s *service) GetQuestionnaire(ctx context.Context, modelCode string) (*QuestionnaireBindingResult, error) {
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return questionnaireBinding(ctx, s.deps.QuestionnaireQuery, model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion)
}

func (s *service) GetDefinition(ctx context.Context, modelCode string) (*DefinitionResult, error) {
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return definitionFromModel(model), nil
}

func (s *service) UpdateDefinition(ctx context.Context, modelCode string, input DefinitionInput) (*DefinitionResult, error) {
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	format := input.PayloadFormat
	if format == "" {
		format = domain.PayloadFormatPersonalityTypologyV1
	}
	if issues := validateDefinitionPayloadForSave(format, input.Payload); len(issues) > 0 {
		return nil, validationFailed(issues)
	}
	now := time.Now().UTC()
	if err := model.UpdateDefinition(domain.DefinitionPayload{
		Format: format,
		Data:   append([]byte(nil), input.Payload...),
	}, now); err != nil {
		return nil, mapDomainError(err)
	}
	if input.Algorithm != "" {
		model.Algorithm = domain.Algorithm(input.Algorithm)
	}
	if input.SubKind != "" {
		model.SubKind = domain.SubKind(input.SubKind)
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return definitionFromModel(model), nil
}

func (s *service) Validate(ctx context.Context, modelCode string) (*ValidationResult, error) {
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	issues := s.validateModelForPublish(ctx, model)
	return NewValidationResult(issues), nil
}

func (s *service) Publish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	if s.deps.PublishedRepo == nil {
		return nil, unavailable("人格模型发布仓储未配置")
	}
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	if issues := s.validateModelForPublish(ctx, model); len(issues) > 0 {
		return nil, validationFailed(issues)
	}
	now := time.Now().UTC()
	if err := model.MarkPublished(now); err != nil {
		return nil, mapDomainError(err)
	}
	snapshot, err := personalitydomain.BuildPublishedSnapshot(model)
	if err != nil {
		return nil, invalidArgument("%s", err.Error())
	}
	if err := s.deps.PublishedRepo.Save(ctx, snapshot); err != nil {
		return nil, err
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		_ = s.deps.PublishedRepo.DeletePublished(ctx, domain.KindPersonality, modelCode)
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
	candidate := *model
	if err := candidate.MarkUnpublished(now); err != nil {
		return nil, mapDomainError(err)
	}
	if s.deps.PublishedRepo != nil {
		if err := s.deps.PublishedRepo.DeletePublished(ctx, domain.KindPersonality, modelCode); err != nil {
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
	wasPublished := model.IsPublished()
	now := time.Now().UTC()
	candidate := *model
	if err := candidate.MarkArchived(now); err != nil {
		return nil, mapDomainError(err)
	}
	if wasPublished && s.deps.PublishedRepo != nil {
		if err := s.deps.PublishedRepo.DeletePublished(ctx, domain.KindPersonality, modelCode); err != nil {
			return nil, err
		}
	}
	if err := model.MarkArchived(now); err != nil {
		return nil, mapDomainError(err)
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	return summaryFromModel(model), nil
}

func (s *service) validateModelForPublish(ctx context.Context, model *domain.AssessmentModel) []ValidationIssue {
	domainIssues := domainIssuesToValidation(model.ValidateForPublish().Issues)
	runtime, validationContext, definitionIssues := validateDefinitionPayloadForPublish(model)
	questionnaire, questionnaireIssues := s.questionnaireSnapshotForPublish(ctx, model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion)
	if len(definitionIssues) > 0 || len(questionnaireIssues) > 0 || runtime == nil {
		return mergeValidationIssues(domainIssues, definitionIssues, questionnaireIssues)
	}
	runtimeIssues := domainIssuesToValidation(modeltypology.ValidateRuntimeSpecForPublishWithContext(runtime, questionnaire, validationContext))
	return mergeValidationIssues(domainIssues, definitionIssues, questionnaireIssues, runtimeIssues)
}

func (s *service) loadModel(ctx context.Context, modelCode string) (*domain.AssessmentModel, error) {
	if modelCode == "" {
		return nil, invalidArgument("模型编码不能为空")
	}
	if s.deps.ModelRepo == nil {
		return nil, unavailable("人格模型仓储未配置")
	}
	model, err := s.deps.ModelRepo.FindByCode(ctx, modelCode)
	if err != nil {
		if domain.IsNotFound(err) {
			return nil, errors.WithCode(code.ErrMedicalScaleNotFound, "测评模型不存在")
		}
		return nil, err
	}
	return model, nil
}

func questionnaireBinding(ctx context.Context, query questionnaireapp.QuestionnaireQueryService, questionnaireCode, questionnaireVersion string) (*QuestionnaireBindingResult, error) {
	result := &QuestionnaireBindingResult{
		QuestionnaireCode:    questionnaireCode,
		QuestionnaireVersion: questionnaireVersion,
	}
	if questionnaireCode == "" || query == nil {
		return result, nil
	}
	var q *questionnaireapp.QuestionnaireResult
	var err error
	if questionnaireVersion != "" {
		q, err = query.GetPublishedByCodeVersion(ctx, questionnaireCode, questionnaireVersion)
	} else {
		q, err = query.GetByCode(ctx, questionnaireCode)
	}
	if err != nil {
		return result, nil
	}
	if q != nil {
		result.Title = q.Title
		result.QuestionCount = len(q.Questions)
	}
	return result, nil
}

func (s *service) validateQuestionnaireBinding(ctx context.Context, questionnaireCode, questionnaireVersion string) (domain.QuestionnaireBinding, error) {
	if questionnaireCode == "" {
		return domain.QuestionnaireBinding{}, invalidArgument("问卷编码不能为空")
	}
	if s.deps.QuestionnaireQuery == nil {
		return domain.QuestionnaireBinding{}, unavailable("问卷查询服务未配置")
	}
	var (
		q   *questionnaireapp.QuestionnaireResult
		err error
	)
	if questionnaireVersion != "" {
		q, err = s.deps.QuestionnaireQuery.GetPublishedByCodeVersion(ctx, questionnaireCode, questionnaireVersion)
	} else {
		q, err = s.deps.QuestionnaireQuery.GetPublishedByCode(ctx, questionnaireCode)
	}
	if err != nil {
		return domain.QuestionnaireBinding{}, invalidArgument("绑定问卷无效：%s", err.Error())
	}
	if q == nil || q.Version == "" {
		return domain.QuestionnaireBinding{}, invalidArgument("绑定问卷无效：问卷不存在或未发布")
	}
	if len(q.Questions) == 0 {
		return domain.QuestionnaireBinding{}, invalidArgument("绑定问卷无效：问卷题目不能为空")
	}
	return domain.QuestionnaireBinding{
		QuestionnaireCode:    q.Code,
		QuestionnaireVersion: q.Version,
	}, nil
}

func (s *service) questionnaireSnapshotForPublish(ctx context.Context, questionnaireCode, questionnaireVersion string) (modeltypology.QuestionnaireSnapshot, []ValidationIssue) {
	if questionnaireCode == "" || questionnaireVersion == "" {
		return modeltypology.QuestionnaireSnapshot{}, nil
	}
	if s.deps.QuestionnaireQuery == nil {
		return modeltypology.QuestionnaireSnapshot{}, []ValidationIssue{{
			Field: "binding.questionnaire", Message: "问卷查询服务未配置",
			Code: "binding.questionnaire_query.unavailable", Level: "error",
		}}
	}
	q, err := s.deps.QuestionnaireQuery.GetPublishedByCodeVersion(ctx, questionnaireCode, questionnaireVersion)
	if err != nil {
		return modeltypology.QuestionnaireSnapshot{}, []ValidationIssue{{
			Field: "binding.questionnaire", Message: "绑定问卷不存在或未发布",
			Code: "binding.questionnaire.not_found", Level: "error",
		}}
	}
	if q == nil {
		return modeltypology.QuestionnaireSnapshot{}, []ValidationIssue{{
			Field: "binding.questionnaire", Message: "绑定问卷不存在或未发布",
			Code: "binding.questionnaire.not_found", Level: "error",
		}}
	}
	if len(q.Questions) == 0 {
		return modeltypology.QuestionnaireSnapshot{}, []ValidationIssue{{
			Field: "binding.questionnaire", Message: "绑定问卷题目不能为空",
			Code: "binding.questionnaire.questions.required", Level: "error",
		}}
	}
	return questionnaireSnapshotFromResult(q), nil
}

func questionnaireSnapshotFromResult(q *questionnaireapp.QuestionnaireResult) modeltypology.QuestionnaireSnapshot {
	if q == nil {
		return modeltypology.QuestionnaireSnapshot{}
	}
	snapshot := modeltypology.QuestionnaireSnapshot{
		Code:      q.Code,
		Version:   q.Version,
		Questions: make([]modeltypology.QuestionSnapshot, 0, len(q.Questions)),
	}
	for _, question := range q.Questions {
		item := modeltypology.QuestionSnapshot{
			Code:        question.Code,
			OptionCodes: make([]string, 0, len(question.Options)),
		}
		for _, option := range question.Options {
			item.OptionCodes = append(item.OptionCodes, option.Value)
		}
		snapshot.Questions = append(snapshot.Questions, item)
	}
	return snapshot
}

func invalidArgument(format string, args ...interface{}) error {
	return errors.WithCode(code.ErrInvalidArgument, format, args...)
}

func unavailable(message string) error {
	return errors.WithCode(code.ErrInternalServerError, "%s", message)
}

func validationFailed(issues []ValidationIssue) error {
	return errors.WithCode(code.ErrInvalidArgument, "模型校验未通过：%s", firstIssueMessage(issues))
}

func firstIssueMessage(issues []ValidationIssue) string {
	if len(issues) == 0 {
		return "unknown validation issue"
	}
	return issues[0].Message
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
