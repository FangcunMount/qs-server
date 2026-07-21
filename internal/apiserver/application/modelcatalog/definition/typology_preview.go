package definition

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelpreview"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// TypologyPreviewService owns draft report preview for typology models.
// It depends on publish validation and payload projection but does not own them.
type TypologyPreviewService struct {
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
	ReportPreviewer    modelpreview.ReportPreviewer
	ValidateForPublish func(context.Context, *domain.AssessmentModel) []domain.DomainValidationIssue
}

// PreviewReport previews a typology report from draft answers.
func (s TypologyPreviewService) PreviewReport(ctx context.Context, model *domain.AssessmentModel, raw json.RawMessage) (*PreviewResult, error) {
	if model == nil {
		return nil, previewInvalid("模型不能为空")
	}
	input, err := decodeTypologyPreviewInput(raw)
	if err != nil {
		return nil, err
	}
	if len(input.Answers) == 0 {
		return nil, previewInvalid("预览答卷 answers 不能为空")
	}
	if s.ValidateForPublish != nil {
		if issues := s.ValidateForPublish(ctx, model); domain.HasValidationErrors(issues) {
			return nil, NewValidationError(issues)
		}
	}
	questionnaire, err := s.previewQuestionnaire(ctx, model)
	if err != nil {
		return nil, err
	}
	if issues := validateTypologyPreviewAnswers(input.Answers, questionnaire); len(issues) > 0 {
		return nil, NewValidationError(issues)
	}
	payload, err := (RuntimeMaterializer{}).MaterializeTypologyRuntime(model, string(domain.ModelStatusPublished))
	if err != nil {
		return nil, err
	}
	if s.ReportPreviewer == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "报告预览服务未配置")
	}
	result, err := s.ReportPreviewer.PreviewReport(ctx, modelpreview.Request{
		SubKind: model.SubKind, Algorithm: model.Algorithm, Code: model.Code, Version: modelRevisionVersion(model), Title: model.Title,
		QuestionnaireCode: model.Binding.QuestionnaireCode, QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Input: typologyPreviewExecutionInput(model, questionnaire, payload, input.Answers),
	})
	if err != nil {
		return nil, err
	}
	return previewResultFromReport(result), nil
}

func (s TypologyPreviewService) previewQuestionnaire(ctx context.Context, model *domain.AssessmentModel) (*questionnaireapp.QuestionnaireResult, error) {
	if model.Binding.QuestionnaireCode == "" || model.Binding.QuestionnaireVersion == "" {
		return nil, previewInvalid("模型未绑定问卷版本")
	}
	if s.QuestionnaireQuery == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "问卷查询服务未配置")
	}
	questionnaire, err := s.QuestionnaireQuery.GetPublishedByCodeVersion(ctx, model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion)
	if err != nil || questionnaire == nil {
		return nil, previewInvalid("绑定问卷不存在或未发布")
	}
	if len(questionnaire.Questions) == 0 {
		return nil, previewInvalid("绑定问卷题目不能为空")
	}
	return questionnaire, nil
}

type typologyPreviewAnswer struct {
	QuestionCode string   `json:"question_code"`
	Value        any      `json:"value,omitempty"`
	Score        *float64 `json:"score,omitempty"`
}

type typologyPreviewInput struct {
	Answers []typologyPreviewAnswer `json:"answers"`
}

func decodeTypologyPreviewInput(raw json.RawMessage) (typologyPreviewInput, error) {
	if len(raw) == 0 {
		return typologyPreviewInput{}, previewInvalid("预览答卷 payload 不能为空")
	}
	var input typologyPreviewInput
	if err := json.Unmarshal(raw, &input); err == nil && len(input.Answers) > 0 {
		return input, nil
	}
	var answers []typologyPreviewAnswer
	if err := json.Unmarshal(raw, &answers); err != nil {
		return typologyPreviewInput{}, previewInvalid("预览答卷 payload 格式无效")
	}
	return typologyPreviewInput{Answers: answers}, nil
}

func validateTypologyPreviewAnswers(answers []typologyPreviewAnswer, questionnaire *questionnaireapp.QuestionnaireResult) []domain.DomainValidationIssue {
	questions := make(map[string]questionnaireapp.QuestionResult, len(questionnaire.Questions))
	for _, question := range questionnaire.Questions {
		questions[question.Code] = question
	}
	seen := make(map[string]struct{}, len(answers))
	issues := make([]domain.DomainValidationIssue, 0)
	for index, answer := range answers {
		field := fmt.Sprintf("answers[%d]", index)
		codeValue := strings.TrimSpace(answer.QuestionCode)
		if codeValue == "" {
			issues = append(issues, domain.DomainValidationIssue{Field: field + ".question_code", Message: "question_code 不能为空", Code: "question_code.required", Level: domain.ValidationLevelError})
			continue
		}
		if _, duplicate := seen[codeValue]; duplicate {
			issues = append(issues, domain.DomainValidationIssue{Field: field + ".question_code", Message: fmt.Sprintf("question_code %q 重复", codeValue), Code: "question_code.duplicate", Level: domain.ValidationLevelError})
		}
		seen[codeValue] = struct{}{}
		question, exists := questions[codeValue]
		if !exists {
			issues = append(issues, domain.DomainValidationIssue{Field: field + ".question_code", Message: fmt.Sprintf("question_code %q 不存在于绑定问卷", codeValue), Code: "question_code.not_found", Level: domain.ValidationLevelError})
			continue
		}
		if len(question.Options) > 0 {
			value, ok := answer.Value.(string)
			value = strings.TrimSpace(value)
			if !ok || value == "" {
				issues = append(issues, domain.DomainValidationIssue{Field: field + ".value", Message: "有选项题目必须提供 value", Code: "answer.value.required", Level: domain.ValidationLevelError})
				continue
			}
			option, valid := previewOption(question, value)
			if !valid {
				issues = append(issues, domain.DomainValidationIssue{Field: field + ".value", Message: fmt.Sprintf("value %q 不是题目 %q 的有效选项", value, codeValue), Code: "answer.value.invalid_option", Level: domain.ValidationLevelError})
				continue
			}
			if answer.Score != nil && *answer.Score != float64(option.Score) {
				issues = append(issues, domain.DomainValidationIssue{Field: field + ".score", Message: fmt.Sprintf("score 与问卷选项 %q 的分值不一致", value), Code: "answer.score.mismatch", Level: domain.ValidationLevelError})
			}
			continue
		}
		if answer.Score == nil {
			issues = append(issues, domain.DomainValidationIssue{Field: field + ".score", Message: "无选项题目必须提供 score", Code: "answer.score.required", Level: domain.ValidationLevelError})
		} else if math.IsNaN(*answer.Score) || math.IsInf(*answer.Score, 0) {
			issues = append(issues, domain.DomainValidationIssue{Field: field + ".score", Message: "score 必须是有限数字", Code: "answer.score.invalid", Level: domain.ValidationLevelError})
		}
	}
	return issues
}

func typologyPreviewExecutionInput(model *domain.AssessmentModel, questionnaire *questionnaireapp.QuestionnaireResult, payload *modeltypology.Payload, answers []typologyPreviewAnswer) *evaluationinput.InputSnapshot {
	questionsByCode := make(map[string]questionnaireapp.QuestionResult, len(questionnaire.Questions))
	for _, question := range questionnaire.Questions {
		questionsByCode[question.Code] = question
	}
	answerSnapshots := make([]evaluationinput.AnswerSnapshot, 0, len(answers))
	for _, answer := range answers {
		score := 0.0
		if answer.Score != nil {
			score = *answer.Score
		} else if value, ok := answer.Value.(string); ok {
			if option, found := previewOption(questionsByCode[answer.QuestionCode], value); found {
				score = float64(option.Score)
			}
		}
		answerSnapshots = append(answerSnapshots, evaluationinput.AnswerSnapshot{QuestionCode: answer.QuestionCode, Score: score, Value: answer.Value})
	}
	questions := make([]evaluationinput.QuestionSnapshot, 0, len(questionnaire.Questions))
	for _, question := range questionnaire.Questions {
		item := evaluationinput.QuestionSnapshot{Code: question.Code, Type: question.Type, Options: make([]evaluationinput.OptionSnapshot, 0, len(question.Options))}
		for _, option := range question.Options {
			item.Options = append(item.Options, evaluationinput.OptionSnapshot{Code: option.Value, Content: option.Label, Score: float64(option.Score)})
		}
		questions = append(questions, item)
	}
	return &evaluationinput.InputSnapshot{
		Model: evaluationinput.NewTypologyModelSnapshot(payload), ModelPayload: evaluationinput.TypologyModelPayload{Payload: payload},
		AnswerSheet:   &evaluationinput.AnswerSheetSnapshot{QuestionnaireCode: model.Binding.QuestionnaireCode, QuestionnaireVersion: model.Binding.QuestionnaireVersion, QuestionnaireTitle: questionnaire.Title, Answers: answerSnapshots},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: questionnaire.Code, Version: questionnaire.Version, Title: questionnaire.Title, Questions: questions},
	}
}

func previewOption(question questionnaireapp.QuestionResult, value string) (questionnaireapp.OptionResult, bool) {
	for _, option := range question.Options {
		if option.Value == value {
			return option, true
		}
	}
	return questionnaireapp.OptionResult{}, false
}

func previewResultFromReport(result *modelpreview.Result) *PreviewResult {
	if result == nil {
		return &PreviewResult{}
	}
	out := &PreviewResult{OutcomeCode: result.OutcomeCode, OutcomeTitle: result.OutcomeTitle, ScoreDetail: result.Scores, RawReport: result.Report}
	if len(out.ScoreDetail) == 0 {
		out.ScoreDetail = nil
	}
	out.ReportSections = previewSectionsFromReport(result.Report)
	return out
}

func previewSectionsFromReport(reportValue *domainreport.Draft) []PreviewSection {
	if reportValue == nil {
		return nil
	}
	content := reportValue.Content()
	sections := make([]PreviewSection, 0)
	if value := content.Conclusion; value != "" {
		sections = append(sections, PreviewSection{Title: "结论", Content: value, Kind: "conclusion"})
	}
	if extra := content.ModelExtra; extra != nil && extra.Commentary != "" {
		sections = append(sections, PreviewSection{Title: "解读", Content: extra.Commentary, Kind: "commentary"})
	}
	for _, dimension := range content.Dimensions {
		if value := dimension.Description(); value != "" {
			sections = append(sections, PreviewSection{Title: dimension.Name(), Content: value, Kind: "dimension"})
		}
	}
	for _, suggestion := range content.Suggestions {
		if suggestion.Content != "" {
			sections = append(sections, PreviewSection{Title: string(suggestion.Category), Content: suggestion.Content, Kind: "suggestion"})
		}
	}
	return sections
}

func previewInvalid(format string, args ...any) error {
	return errors.WithCode(code.ErrInvalidArgument, format, args...)
}
