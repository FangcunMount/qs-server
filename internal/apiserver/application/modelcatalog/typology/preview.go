package typology

import (
	"context"
	"encoding/json"
	"fmt"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelpreview"
)

func (s *service) PreviewReport(ctx context.Context, modelCode string, payload json.RawMessage) (*PreviewReportResult, error) {
	model, err := s.loadModel(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	input, err := decodePreviewReportInput(payload)
	if err != nil {
		return nil, err
	}
	if len(input.Answers) == 0 {
		return nil, invalidArgument("预览答卷 answers 不能为空")
	}
	issues := s.validateModelForPublish(ctx, model)
	if len(issues) > 0 {
		return nil, validationFailed(issues)
	}
	questionnaire, err := s.previewQuestionnaire(ctx, model)
	if err != nil {
		return nil, err
	}
	if questionnaire == nil {
		return nil, invalidArgument("模型绑定问卷不存在")
	}
	if issues := validatePreviewAnswers(input.Answers, questionnaire); len(issues) > 0 {
		return nil, validationFailed(issues)
	}
	snapshot, err := publishing.BuildPublishedSnapshot(model)
	if err != nil {
		return nil, err
	}
	var typologyPayload modeltypology.Payload
	if err := json.Unmarshal(snapshot.Payload, &typologyPayload); err != nil {
		return nil, invalidArgument("模型定义 payload 格式无效")
	}
	if s.deps.ReportPreviewer == nil {
		return nil, unavailable("报告预览服务未配置")
	}
	executionInput := previewExecutionInput(model, questionnaire, &typologyPayload, input.Answers)
	result, err := s.deps.ReportPreviewer.PreviewReport(ctx, modelpreview.Request{
		SubKind:              model.SubKind,
		Algorithm:            model.Algorithm,
		Code:                 model.Code,
		Version:              previewModelVersion(model, snapshot),
		Title:                model.Title,
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		Input:                executionInput,
	})
	if err != nil {
		return nil, err
	}
	return previewReportResult(result), nil
}

func previewModelVersion(model *domain.AssessmentModel, snapshot *domain.PublishedModelSnapshot) string {
	if snapshot != nil && snapshot.Model.Version != "" {
		return snapshot.Model.Version
	}
	return fmt.Sprintf("v%d", model.Version)
}

func (s *service) previewQuestionnaire(ctx context.Context, model *domain.AssessmentModel) (*questionnaireapp.QuestionnaireResult, error) {
	if model == nil || model.Binding.QuestionnaireCode == "" || model.Binding.QuestionnaireVersion == "" {
		return nil, invalidArgument("模型未绑定问卷版本")
	}
	if s.deps.QuestionnaireQuery == nil {
		return nil, unavailable("问卷查询服务未配置")
	}
	questionnaire, err := s.deps.QuestionnaireQuery.GetPublishedByCodeVersion(
		ctx,
		model.Binding.QuestionnaireCode,
		model.Binding.QuestionnaireVersion,
	)
	if err != nil {
		return nil, invalidArgument("绑定问卷不存在或未发布：%s", err.Error())
	}
	if questionnaire == nil || len(questionnaire.Questions) == 0 {
		return nil, invalidArgument("绑定问卷题目不能为空")
	}
	return questionnaire, nil
}

func decodePreviewReportInput(payload json.RawMessage) (PreviewReportInput, error) {
	var input PreviewReportInput
	if len(payload) == 0 {
		return input, invalidArgument("预览答卷 payload 不能为空")
	}
	if err := json.Unmarshal(payload, &input); err == nil && len(input.Answers) > 0 {
		return input, nil
	}
	var answers []PreviewAnswer
	if err := json.Unmarshal(payload, &answers); err != nil {
		return input, invalidArgument("预览答卷 payload 格式无效")
	}
	input.Answers = answers
	return input, nil
}

func previewExecutionInput(
	model *domain.AssessmentModel,
	questionnaire *questionnaireapp.QuestionnaireResult,
	payload *modeltypology.Payload,
	answers []PreviewAnswer,
) *evaluationinput.InputSnapshot {
	answerSnapshots := make([]evaluationinput.AnswerSnapshot, 0, len(answers))
	for _, answer := range answers {
		score := 0.0
		if answer.Score != nil {
			score = *answer.Score
		}
		answerSnapshots = append(answerSnapshots, evaluationinput.AnswerSnapshot{
			QuestionCode: answer.QuestionCode,
			Score:        score,
			Value:        answer.Value,
		})
	}
	return &evaluationinput.InputSnapshot{
		Model:        evaluationinput.NewTypologyModelSnapshot(payload),
		ModelPayload: evaluationinput.TypologyModelPayload{Payload: payload},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			QuestionnaireCode:    model.Binding.QuestionnaireCode,
			QuestionnaireVersion: model.Binding.QuestionnaireVersion,
			QuestionnaireTitle:   questionnaire.Title,
			Answers:              answerSnapshots,
		},
		Questionnaire: questionnaireSnapshotForExecution(questionnaire),
	}
}

func questionnaireSnapshotForExecution(questionnaire *questionnaireapp.QuestionnaireResult) *evaluationinput.QuestionnaireSnapshot {
	if questionnaire == nil {
		return nil
	}
	snapshot := &evaluationinput.QuestionnaireSnapshot{
		Code:      questionnaire.Code,
		Version:   questionnaire.Version,
		Title:     questionnaire.Title,
		Questions: make([]evaluationinput.QuestionSnapshot, 0, len(questionnaire.Questions)),
	}
	for _, question := range questionnaire.Questions {
		item := evaluationinput.QuestionSnapshot{
			Code:    question.Code,
			Type:    question.Type,
			Options: make([]evaluationinput.OptionSnapshot, 0, len(question.Options)),
		}
		for _, option := range question.Options {
			item.Options = append(item.Options, evaluationinput.OptionSnapshot{
				Code:    option.Value,
				Content: option.Label,
				Score:   float64(option.Score),
			})
		}
		snapshot.Questions = append(snapshot.Questions, item)
	}
	return snapshot
}

func previewReportResult(result *modelpreview.Result) *PreviewReportResult {
	if result == nil {
		return &PreviewReportResult{}
	}
	out := &PreviewReportResult{
		Outcome:        PreviewOutcome{Code: result.OutcomeCode, Title: result.OutcomeTitle},
		ScoreDetail:    result.Scores,
		ReportSections: previewSectionsFromReport(result.Report),
		RawReport:      result.Report,
	}
	if len(out.ScoreDetail) == 0 {
		out.ScoreDetail = nil
	}
	return out
}

func previewSectionsFromReport(report *domainreport.InterpretReport) []PreviewReportSection {
	if report == nil {
		return nil
	}
	sections := make([]PreviewReportSection, 0)
	if conclusion := report.Conclusion(); conclusion != "" {
		sections = append(sections, PreviewReportSection{
			Title:   "结论",
			Content: conclusion,
			Kind:    "conclusion",
		})
	}
	if extra := report.ModelExtra(); extra != nil && extra.Commentary != "" {
		sections = append(sections, PreviewReportSection{
			Title:   "解读",
			Content: extra.Commentary,
			Kind:    "commentary",
		})
	}
	for _, dim := range report.Dimensions() {
		if content := dim.Description(); content != "" {
			sections = append(sections, PreviewReportSection{
				Title:   dim.Name(),
				Content: content,
				Kind:    "dimension",
			})
		}
	}
	for _, suggestion := range report.Suggestions() {
		if suggestion.Content == "" {
			continue
		}
		sections = append(sections, PreviewReportSection{
			Title:   string(suggestion.Category),
			Content: suggestion.Content,
			Kind:    "suggestion",
		})
	}
	return sections
}
