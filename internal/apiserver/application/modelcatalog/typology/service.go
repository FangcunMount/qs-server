package typology

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publication"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelpreview"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// CacheSignalNotifier 缓存失效信令发布端口（best-effort，非领域事件）。
type CacheSignalNotifier interface {
	NotifyTypologyModelCacheChanged(ctx context.Context, code, action string)
}

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
	ModelRepo           port.ModelRepository
	PublishedRepo       port.PublishedModelRepository
	QuestionnaireQuery  questionnaireapp.QuestionnaireQueryService
	CacheSignalNotifier CacheSignalNotifier
	ReportPreviewer     modelpreview.ReportPreviewer
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
		Kind:      domain.KindTypology,
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
		Code:           input.Code,
		Kind:           domain.KindTypology,
		SubKind:        subKind,
		Algorithm:      algorithm,
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
	save, issues, err := s.definitionHandler().PrepareForSave(ctx, model, appdefinition.SaveInput{
		PayloadFormat: input.PayloadFormat,
		Payload:       input.Payload,
		Algorithm:     input.Algorithm,
		SubKind:       input.SubKind,
	})
	if len(issues) > 0 {
		return nil, validationFailed(domainIssuesToValidation(issues))
	}
	if err != nil {
		return nil, invalidArgument("%s", err.Error())
	}
	now := time.Now().UTC()
	if err := model.UpdateDefinition(save.Payload, now); err != nil {
		return nil, mapDomainError(err)
	}
	if save.Algorithm != "" {
		model.Algorithm = save.Algorithm
	}
	if save.SubKind != "" {
		model.SubKind = save.SubKind
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
	issues := domainIssuesToValidation(s.definitionHandler().ValidateForPublish(ctx, model))
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
	if _, err := s.publisher().Publish(ctx, model, publication.PublishOptions{
		ReplaceKind:    domain.KindTypology,
		AfterPublished: s.notifyCacheChanged,
	}); err != nil {
		var validationErr *appdefinition.ValidationError
		if stderrors.As(err, &validationErr) {
			return nil, validationFailed(domainIssuesToValidation(validationErr.Issues))
		}
		if stderrors.Is(err, domain.ErrInvalidArgument) || stderrors.Is(err, domain.ErrInvalidState) {
			return nil, mapDomainError(err)
		}
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
		if err := s.deps.PublishedRepo.DeletePublished(ctx, domain.KindTypology, modelCode); err != nil {
			return nil, err
		}
	}
	if err := model.MarkUnpublished(now); err != nil {
		return nil, mapDomainError(err)
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	s.notifyCacheChanged(ctx, modelCode, "unpublish")
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
		if err := s.deps.PublishedRepo.DeletePublished(ctx, domain.KindTypology, modelCode); err != nil {
			return nil, err
		}
	}
	if err := model.MarkArchived(now); err != nil {
		return nil, mapDomainError(err)
	}
	if err := s.deps.ModelRepo.Update(ctx, model); err != nil {
		return nil, err
	}
	if wasPublished {
		s.notifyCacheChanged(ctx, modelCode, "archive")
	}
	return summaryFromModel(model), nil
}

func (s *service) notifyCacheChanged(ctx context.Context, code, action string) {
	if s.deps.CacheSignalNotifier == nil || code == "" {
		return
	}
	s.deps.CacheSignalNotifier.NotifyTypologyModelCacheChanged(ctx, code, action)
}

func (s *service) validateModelForPublish(ctx context.Context, model *domain.AssessmentModel) []ValidationIssue {
	return domainIssuesToValidation(s.definitionHandler().ValidateForPublish(ctx, model))
}

func (s *service) definitionHandler() DefinitionHandler {
	return DefinitionHandler{QuestionnaireQuery: s.deps.QuestionnaireQuery}
}

func (s *service) publisher() publication.Publisher {
	handler := s.definitionHandler()
	return publication.Publisher{
		Registry:  appdefinition.NewRegistry(handler),
		ModelRepo: s.deps.ModelRepo,
		Repo:      s.deps.PublishedRepo,
	}
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
	if len(issues) == 0 {
		return nil
	}
	return &validationFailedError{issues: issues}
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
