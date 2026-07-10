package definition

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelpreview"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// TypologyDefinitionHandler owns typology DefinitionV2 validation, payload
// projection and draft report preview.
type TypologyDefinitionHandler struct {
	QuestionnaireQuery questionnaireapp.QuestionnaireQueryService
	ReportPreviewer    modelpreview.ReportPreviewer
}

func (TypologyDefinitionHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == domain.KindTypology
}

func (TypologyDefinitionHandler) PrepareForSave(_ context.Context, _ *domain.AssessmentModel, input SaveInput) (SaveResult, []domain.DomainValidationIssue, error) {
	if issues := ValidateDefinitionV2(input.DefinitionV2); len(issues) > 0 {
		return SaveResult{}, issues, nil
	}
	format := input.PayloadFormat
	if format == "" {
		format = domain.PayloadFormatPersonalityTypologyV1
	}
	result := SaveResult{Payload: domain.DefinitionPayload{Format: format, Data: append([]byte(nil), input.Payload...)}, DefinitionV2: input.DefinitionV2}
	if input.Algorithm != "" {
		result.Algorithm = domain.Algorithm(input.Algorithm)
	}
	if input.SubKind != "" {
		result.SubKind = domain.SubKind(input.SubKind)
	}
	return result, nil, nil
}

func (h TypologyDefinitionHandler) ValidateForPublish(ctx context.Context, model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil {
		return []domain.DomainValidationIssue{{Field: "model", Message: "模型不能为空", Code: "model.required", Level: domain.ValidationLevelError}}
	}
	issues := model.ValidateForPublish().Issues
	issues = append(issues, ValidateDefinitionV2ForPublish(ctx, model.DefinitionV2, nil)...)
	if len(issues) > 0 {
		return issues
	}
	runtime, err := modeltypology.RuntimeSpecFromDefinition(model.DefinitionV2)
	if err != nil || runtime == nil {
		if err == nil {
			err = fmt.Errorf("typology runtime specification is empty")
		}
		return append(issues, domain.DomainValidationIssue{Field: "definition_v2", Code: "definition_v2.runtime.invalid", Message: err.Error(), Level: domain.ValidationLevelError})
	}
	questionnaire, questionnaireIssues := h.questionnaireSnapshotForPublish(ctx, model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion)
	if len(questionnaireIssues) > 0 {
		return append(issues, questionnaireIssues...)
	}
	return append(issues, modeltypology.ValidateRuntimeSpecForPublishWithContext(runtime, questionnaire, modeltypology.RuntimeSpecValidationContext{})...)
}

func (TypologyDefinitionHandler) BuildSnapshotPayload(_ context.Context, model *domain.AssessmentModel) (SnapshotBuildResult, error) {
	if model == nil || model.DefinitionV2 == nil {
		return SnapshotBuildResult{}, fmt.Errorf("typology definition_v2 is required")
	}
	if model.SubKind != domain.SubKindTypology {
		return SnapshotBuildResult{}, fmt.Errorf("typology model sub_kind %s is not typology", model.SubKind)
	}
	payload, err := modeltypology.PayloadFromDefinition(modeltypology.DefinitionEnvelope{
		Code: model.Code, Version: modelRevisionVersion(model), Title: model.Title,
		QuestionnaireCode: model.Binding.QuestionnaireCode, QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Status: string(domain.ModelStatusPublished), Algorithm: model.Algorithm,
	}, model.DefinitionV2)
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return SnapshotBuildResult{}, fmt.Errorf("marshal typology payload: %w", err)
	}
	decisionKind, err := model.DecisionKindForDefinition()
	if err != nil {
		return SnapshotBuildResult{}, err
	}
	return SnapshotBuildResult{Kind: domain.KindTypology, SubKind: domain.SubKindTypology, Algorithm: model.Algorithm, PayloadFormat: domain.PayloadFormatPersonalityTypologyV1, DecisionKind: decisionKind, Payload: encoded}, nil
}

func (h TypologyDefinitionHandler) PreviewReport(ctx context.Context, model *domain.AssessmentModel, raw json.RawMessage) (*PreviewResult, error) {
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
	if issues := h.ValidateForPublish(ctx, model); len(issues) > 0 {
		return nil, NewValidationError(issues)
	}
	questionnaire, err := h.previewQuestionnaire(ctx, model)
	if err != nil {
		return nil, err
	}
	if issues := validateTypologyPreviewAnswers(input.Answers, questionnaire); len(issues) > 0 {
		return nil, NewValidationError(issues)
	}
	built, err := h.BuildSnapshotPayload(ctx, model)
	if err != nil {
		return nil, err
	}
	var payload modeltypology.Payload
	if err := json.Unmarshal(built.Payload, &payload); err != nil {
		return nil, previewInvalid("模型定义 payload 格式无效")
	}
	if h.ReportPreviewer == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "报告预览服务未配置")
	}
	result, err := h.ReportPreviewer.PreviewReport(ctx, modelpreview.Request{
		SubKind: model.SubKind, Algorithm: model.Algorithm, Code: model.Code, Version: modelRevisionVersion(model), Title: model.Title,
		QuestionnaireCode: model.Binding.QuestionnaireCode, QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Input: typologyPreviewExecutionInput(model, questionnaire, &payload, input.Answers),
	})
	if err != nil {
		return nil, err
	}
	return previewResultFromReport(result), nil
}

func (h TypologyDefinitionHandler) questionnaireSnapshotForPublish(ctx context.Context, codeValue, version string) (modeltypology.QuestionnaireSnapshot, []domain.DomainValidationIssue) {
	if codeValue == "" || version == "" {
		return modeltypology.QuestionnaireSnapshot{}, nil
	}
	if h.QuestionnaireQuery == nil {
		return modeltypology.QuestionnaireSnapshot{}, []domain.DomainValidationIssue{{Field: "binding.questionnaire", Message: "问卷查询服务未配置", Code: "binding.questionnaire_query.unavailable", Level: domain.ValidationLevelError}}
	}
	questionnaire, err := h.QuestionnaireQuery.GetPublishedByCodeVersion(ctx, codeValue, version)
	if err != nil || questionnaire == nil {
		return modeltypology.QuestionnaireSnapshot{}, []domain.DomainValidationIssue{{Field: "binding.questionnaire", Message: "绑定问卷不存在或未发布", Code: "binding.questionnaire.not_found", Level: domain.ValidationLevelError}}
	}
	if len(questionnaire.Questions) == 0 {
		return modeltypology.QuestionnaireSnapshot{}, []domain.DomainValidationIssue{{Field: "binding.questionnaire", Message: "绑定问卷题目不能为空", Code: "binding.questionnaire.questions.required", Level: domain.ValidationLevelError}}
	}
	return questionnaireSnapshotFromResult(questionnaire), nil
}

func (h TypologyDefinitionHandler) previewQuestionnaire(ctx context.Context, model *domain.AssessmentModel) (*questionnaireapp.QuestionnaireResult, error) {
	if model.Binding.QuestionnaireCode == "" || model.Binding.QuestionnaireVersion == "" {
		return nil, previewInvalid("模型未绑定问卷版本")
	}
	if h.QuestionnaireQuery == nil {
		return nil, errors.WithCode(code.ErrInternalServerError, "问卷查询服务未配置")
	}
	questionnaire, err := h.QuestionnaireQuery.GetPublishedByCodeVersion(ctx, model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion)
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
	options := make(map[string]map[string]struct{}, len(questionnaire.Questions))
	for _, question := range questionnaire.Questions {
		values := make(map[string]struct{}, len(question.Options))
		for _, option := range question.Options {
			values[option.Value] = struct{}{}
		}
		options[question.Code] = values
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
		values, exists := options[codeValue]
		if !exists {
			issues = append(issues, domain.DomainValidationIssue{Field: field + ".question_code", Message: fmt.Sprintf("question_code %q 不存在于绑定问卷", codeValue), Code: "question_code.not_found", Level: domain.ValidationLevelError})
			continue
		}
		if answer.Score == nil && answer.Value == nil {
			issues = append(issues, domain.DomainValidationIssue{Field: field, Message: "value 或 score 至少提供一个", Code: "answer.value_or_score.required", Level: domain.ValidationLevelError})
			continue
		}
		if value, ok := answer.Value.(string); ok && strings.TrimSpace(value) != "" {
			if _, valid := values[value]; !valid {
				issues = append(issues, domain.DomainValidationIssue{Field: field + ".value", Message: fmt.Sprintf("value %q 不是题目 %q 的有效选项", value, codeValue), Code: "answer.value.invalid_option", Level: domain.ValidationLevelError})
			}
		}
	}
	return issues
}

func typologyPreviewExecutionInput(model *domain.AssessmentModel, questionnaire *questionnaireapp.QuestionnaireResult, payload *modeltypology.Payload, answers []typologyPreviewAnswer) *evaluationinput.InputSnapshot {
	answerSnapshots := make([]evaluationinput.AnswerSnapshot, 0, len(answers))
	for _, answer := range answers {
		score := 0.0
		if answer.Score != nil {
			score = *answer.Score
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

func questionnaireSnapshotFromResult(questionnaire *questionnaireapp.QuestionnaireResult) modeltypology.QuestionnaireSnapshot {
	if questionnaire == nil {
		return modeltypology.QuestionnaireSnapshot{}
	}
	snapshot := modeltypology.QuestionnaireSnapshot{Code: questionnaire.Code, Version: questionnaire.Version, Questions: make([]modeltypology.QuestionSnapshot, 0, len(questionnaire.Questions))}
	for _, question := range questionnaire.Questions {
		item := modeltypology.QuestionSnapshot{Code: question.Code, OptionCodes: make([]string, 0, len(question.Options))}
		for _, option := range question.Options {
			item.OptionCodes = append(item.OptionCodes, option.Value)
		}
		snapshot.Questions = append(snapshot.Questions, item)
	}
	return snapshot
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

func previewSectionsFromReport(reportValue *domainreport.InterpretReport) []PreviewSection {
	if reportValue == nil {
		return nil
	}
	sections := make([]PreviewSection, 0)
	if value := reportValue.Conclusion(); value != "" {
		sections = append(sections, PreviewSection{Title: "结论", Content: value, Kind: "conclusion"})
	}
	if extra := reportValue.ModelExtra(); extra != nil && extra.Commentary != "" {
		sections = append(sections, PreviewSection{Title: "解读", Content: extra.Commentary, Kind: "commentary"})
	}
	for _, dimension := range reportValue.Dimensions() {
		if value := dimension.Description(); value != "" {
			sections = append(sections, PreviewSection{Title: dimension.Name(), Content: value, Kind: "dimension"})
		}
	}
	for _, suggestion := range reportValue.Suggestions() {
		if suggestion.Content != "" {
			sections = append(sections, PreviewSection{Title: string(suggestion.Category), Content: suggestion.Content, Kind: "suggestion"})
		}
	}
	return sections
}

func modelRevisionVersion(model *domain.AssessmentModel) string {
	return fmt.Sprintf("v%d", model.Revision())
}

func previewInvalid(format string, args ...any) error {
	return errors.WithCode(code.ErrInvalidArgument, format, args...)
}
