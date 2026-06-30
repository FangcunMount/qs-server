package personality

import (
	"context"
	"encoding/json"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	personalitydomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
	snapshot, err := personalitydomain.BuildPublishedSnapshot(model)
	if err != nil {
		return nil, err
	}
	var typologyPayload modeltypology.Payload
	if err := json.Unmarshal(snapshot.Payload, &typologyPayload); err != nil {
		return nil, invalidArgument("模型定义 payload 格式无效")
	}
	executionInput := previewExecutionInput(model, questionnaire, &typologyPayload, input.Answers)
	submitted, err := previewSubmittedAssessment(model, snapshot)
	if err != nil {
		return nil, err
	}
	executor, err := typologyevaluation.NewConfiguredTypologyExecutor()
	if err != nil {
		return nil, err
	}
	outcome, err := executor.Execute(ctx, evaluationexecute.ExecutionInput{
		Assessment: submitted,
		Input:      executionInput,
	})
	if err != nil {
		return nil, err
	}
	reportBuilder, err := typologyevaluation.NewConfiguredReportBuilder()
	if err != nil {
		return nil, err
	}
	report, err := reportBuilder.Build(ctx, evaluationresult.Outcome{
		Assessment: submitted,
		Input:      executionInput,
		Execution:  outcome,
	})
	if err != nil {
		return nil, err
	}
	return previewReportResult(outcome, report), nil
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

func previewSubmittedAssessment(model *domain.AssessmentModel, snapshot *domain.PublishedModelSnapshot) (*assessment.Assessment, error) {
	version := fmt.Sprintf("v%d", model.Version)
	if snapshot != nil && snapshot.Model.Version != "" {
		version = snapshot.Model.Version
	}
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		model.SubKind,
		model.Algorithm,
		meta.ID(0),
		meta.NewCode(model.Code),
		version,
		model.Title,
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(1),
		assessment.NewQuestionnaireRefByCode(meta.NewCode(model.Binding.QuestionnaireCode), model.Binding.QuestionnaireVersion),
		assessment.NewAnswerSheetRef(meta.FromUint64(1)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(1)),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		return nil, err
	}
	if err := a.Submit(); err != nil {
		return nil, err
	}
	a.ClearEvents()
	return a, nil
}

func previewReportResult(outcome *assessment.AssessmentOutcome, report *domainreport.InterpretReport) *PreviewReportResult {
	result := &PreviewReportResult{
		Outcome:        previewOutcomeFromAssessmentOutcome(outcome),
		ScoreDetail:    previewScoresFromOutcome(outcome),
		ReportSections: previewSectionsFromReport(report),
		RawReport:      report,
	}
	if len(result.ScoreDetail) == 0 {
		result.ScoreDetail = nil
	}
	return result
}

func previewOutcomeFromAssessmentOutcome(outcome *assessment.AssessmentOutcome) PreviewOutcome {
	if outcome == nil {
		return PreviewOutcome{}
	}
	if outcome.Profile != nil {
		return PreviewOutcome{Code: outcome.Profile.Code, Title: outcome.Profile.Name}
	}
	if outcome.Level != nil {
		return PreviewOutcome{Code: outcome.Level.Code, Title: outcome.Level.Label}
	}
	return PreviewOutcome{}
}

func previewScoresFromOutcome(outcome *assessment.AssessmentOutcome) map[string]float64 {
	scores := map[string]float64{}
	if outcome == nil {
		return scores
	}
	if outcome.Primary != nil {
		scores["primary"] = outcome.Primary.Value
	}
	for _, dim := range outcome.Dimensions {
		if dim.Score != nil && dim.Code != "" {
			scores[dim.Code] = dim.Score.Value
		}
	}
	return scores
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
