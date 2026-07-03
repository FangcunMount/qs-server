package grpcbridge

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
)

// QuestionnaireCatalogReader 将 infra gRPC 输出转换为 application DTO。
type QuestionnaireCatalogReader struct {
	inner QuestionnaireReader
}

// NewQuestionnaireCatalogReader 构造问卷 catalog ACL 适配器。
func NewQuestionnaireCatalogReader(inner QuestionnaireReader) *QuestionnaireCatalogReader {
	return &QuestionnaireCatalogReader{inner: inner}
}

func (r *QuestionnaireCatalogReader) GetQuestionnaire(ctx context.Context, code, version string) (*questionnaire.QuestionnaireResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*QuestionnaireOutput, error) { return r.inner.GetQuestionnaire(ctx, code, version) },
		toQuestionnaireResponse,
	)
}

func (r *QuestionnaireCatalogReader) ListQuestionnaires(ctx context.Context, page, pageSize int32, status, title string) (*questionnaire.ListQuestionnairesResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*ListQuestionnairesOutput, error) {
			return r.inner.ListQuestionnaires(ctx, page, pageSize, status, title)
		},
		toListQuestionnairesResponse,
	)
}

func toListQuestionnairesResponse(out *ListQuestionnairesOutput) *questionnaire.ListQuestionnairesResponse {
	items := make([]questionnaire.QuestionnaireSummaryResponse, len(out.Questionnaires))
	for i, q := range out.Questionnaires {
		items[i] = questionnaire.QuestionnaireSummaryResponse{
			Code:          q.Code,
			Title:         q.Title,
			Description:   q.Description,
			ImgURL:        q.ImgURL,
			Status:        q.Status,
			Version:       q.Version,
			Type:          q.Type,
			QuestionCount: q.QuestionCount,
			CreatedAt:     q.CreatedAt,
			UpdatedAt:     q.UpdatedAt,
		}
	}
	return &questionnaire.ListQuestionnairesResponse{
		Questionnaires: items,
		Total:          out.Total,
		Page:           out.Page,
		PageSize:       out.PageSize,
	}
}

func toQuestionnaireResponse(q *QuestionnaireOutput) *questionnaire.QuestionnaireResponse {
	if q == nil {
		return nil
	}
	questions := make([]questionnaire.QuestionResponse, len(q.Questions))
	for i, question := range q.Questions {
		questions[i] = toQuestionResponse(&question)
	}
	return &questionnaire.QuestionnaireResponse{
		Code:        q.Code,
		Title:       q.Title,
		Description: q.Description,
		ImgURL:      q.ImgURL,
		Status:      q.Status,
		Version:     q.Version,
		Type:        q.Type,
		Questions:   questions,
		CreatedAt:   q.CreatedAt,
		UpdatedAt:   q.UpdatedAt,
	}
}

func toQuestionResponse(q *QuestionOutput) questionnaire.QuestionResponse {
	options := make([]questionnaire.OptionResponse, len(q.Options))
	for i, opt := range q.Options {
		options[i] = questionnaire.OptionResponse{
			Code:    opt.Code,
			Content: opt.Content,
			Score:   opt.Score,
		}
	}
	validationRules := make([]questionnaire.ValidationRuleResponse, len(q.ValidationRules))
	for i, rule := range q.ValidationRules {
		validationRules[i] = questionnaire.ValidationRuleResponse{
			RuleType:    rule.RuleType,
			TargetValue: rule.TargetValue,
		}
	}
	var calcRule *questionnaire.CalculationRuleResponse
	if q.CalculationRule != nil {
		calcRule = &questionnaire.CalculationRuleResponse{
			FormulaType: q.CalculationRule.FormulaType,
		}
	}
	return questionnaire.QuestionResponse{
		Code:            q.Code,
		Type:            q.Type,
		Title:           q.Title,
		Tips:            q.Tips,
		Placeholder:     q.Placeholder,
		Options:         options,
		ValidationRules: validationRules,
		CalculationRule: calcRule,
	}
}
