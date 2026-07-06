package personality_test

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/personality"
	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestPersonalityPreviewPublishExecuteConsistency(t *testing.T) {
	payload, err := os.ReadFile("../../../testdata/personality/frontend_payload_mbti.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	questionnaire := frontendMBTIQuestionnaire()
	modelRepo := &memoryModelRepo{models: map[string]*domain.AssessmentModel{}}
	publishedRepo := &memoryPublishedRepo{snapshots: map[string]*domain.PublishedModelSnapshot{}}
	svc := personality.NewService(personality.Dependencies{
		ModelRepo:          modelRepo,
		PublishedRepo:      publishedRepo,
		QuestionnaireQuery: questionnaireQueryStub{questionnaire: questionnaire},
	})

	created, err := svc.Create(context.Background(), personality.CreateInput{
		Code:                 "personality_e2e_mbti",
		Title:                "E2E MBTI",
		Algorithm:            "mbti",
		SubKind:              personality.SubKindTypology,
		QuestionnaireCode:    questionnaire.Code,
		QuestionnaireVersion: questionnaire.Version,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Status != personality.StatusDraft {
		t.Fatalf("status = %s, want draft", created.Status)
	}

	if _, err := svc.UpdateDefinition(context.Background(), created.Code, personality.DefinitionInput{
		PayloadFormat: domain.PayloadFormatPersonalityTypologyV1,
		Payload:       payload,
	}); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}

	validation, err := svc.Validate(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if validation == nil || !validation.Passed {
		t.Fatalf("Validate() passed = false, issues = %#v", validation)
	}

	previewAnswers := []personality.PreviewAnswer{
		{QuestionCode: "Q_EI", Score: floatPtr(1)},
		{QuestionCode: "Q_SN", Score: floatPtr(5)},
		{QuestionCode: "Q_TF", Score: floatPtr(1)},
		{QuestionCode: "Q_JP", Score: floatPtr(1)},
	}
	previewPayload, err := json.Marshal(personality.PreviewReportInput{Answers: previewAnswers})
	if err != nil {
		t.Fatalf("Marshal preview payload: %v", err)
	}
	previewResult, err := svc.PreviewReport(context.Background(), created.Code, previewPayload)
	if err != nil {
		t.Fatalf("PreviewReport: %v", err)
	}
	if previewResult.Outcome.Code != "INTJ" {
		t.Fatalf("preview outcome code = %s, want INTJ", previewResult.Outcome.Code)
	}
	if len(previewResult.ScoreDetail) == 0 {
		t.Fatal("preview score_detail is empty")
	}
	if len(previewResult.ReportSections) == 0 {
		t.Fatal("preview report_sections is empty")
	}
	if previewResult.RawReport == nil {
		t.Fatal("preview raw_report is nil")
	}

	publishedSummary, err := svc.Publish(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if publishedSummary.Status != personality.StatusPublished {
		t.Fatalf("published status = %s, want published", publishedSummary.Status)
	}

	snapshot, ok := publishedRepo.snapshots[created.Code]
	if !ok || snapshot == nil {
		t.Fatal("published snapshot was not saved")
	}
	storedModel, err := modelRepo.FindByCode(context.Background(), created.Code)
	if err != nil {
		t.Fatalf("FindByCode: %v", err)
	}
	wantVersion := "v" + strconv.FormatInt(storedModel.Version, 10)
	if snapshot.Model.Version != wantVersion {
		t.Fatalf("snapshot version = %s, want %s", snapshot.Model.Version, wantVersion)
	}

	formalOutcome, formalReport, err := executePublishedPersonalityAssessment(
		context.Background(),
		storedModel,
		snapshot,
		questionnaire,
		previewAnswers,
	)
	if err != nil {
		t.Fatalf("execute published assessment: %v", err)
	}
	if formalReport == nil {
		t.Fatal("formal report is nil")
	}

	assertPreviewMatchesExecution(t, previewResult, formalOutcome)
}

func executePublishedPersonalityAssessment(
	ctx context.Context,
	model *domain.AssessmentModel,
	snapshot *domain.PublishedModelSnapshot,
	questionnaire *questionnaireapp.QuestionnaireResult,
	answers []personality.PreviewAnswer,
) (*assessment.AssessmentOutcome, *domainreport.InterpretReport, error) {
	var typologyPayload modeltypology.Payload
	if err := json.Unmarshal(snapshot.Payload, &typologyPayload); err != nil {
		return nil, nil, err
	}

	executionInput := publishedExecutionInput(model, questionnaire, &typologyPayload, answers)
	submitted, err := publishedSubmittedAssessment(model, snapshot)
	if err != nil {
		return nil, nil, err
	}

	executor, err := typologyevaluation.NewConfiguredTypologyExecutor()
	if err != nil {
		return nil, nil, err
	}
	outcome, err := executor.Execute(ctx, evaluationexecute.ExecutionInput{
		Assessment: submitted,
		Input:      executionInput,
	})
	if err != nil {
		return nil, nil, err
	}

	reportBuilder, err := typologyevaluation.NewConfiguredReportBuilder()
	if err != nil {
		return nil, nil, err
	}
	report, err := reportBuilder.Build(ctx, evaluationresult.Outcome{
		Assessment: submitted,
		Input:      executionInput,
		Execution:  outcome,
	})
	if err != nil {
		return nil, nil, err
	}
	return outcome, report, nil
}

func publishedExecutionInput(
	model *domain.AssessmentModel,
	questionnaire *questionnaireapp.QuestionnaireResult,
	payload *modeltypology.Payload,
	answers []personality.PreviewAnswer,
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
	questions := make([]evaluationinput.QuestionSnapshot, 0, len(questionnaire.Questions))
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
		questions = append(questions, item)
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
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{
			Code:      questionnaire.Code,
			Version:   questionnaire.Version,
			Title:     questionnaire.Title,
			Questions: questions,
		},
	}
}

func publishedSubmittedAssessment(
	model *domain.AssessmentModel,
	snapshot *domain.PublishedModelSnapshot,
) (*assessment.Assessment, error) {
	version := "v" + strconv.FormatInt(model.Version, 10)
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

func assertPreviewMatchesExecution(
	t *testing.T,
	preview *personality.PreviewReportResult,
	outcome *assessment.AssessmentOutcome,
) {
	t.Helper()
	if preview == nil || outcome == nil {
		t.Fatal("preview or execution outcome is nil")
	}

	formalCode := ""
	formalTitle := ""
	if outcome.Profile != nil {
		formalCode = outcome.Profile.Code
		formalTitle = outcome.Profile.Name
	} else if outcome.Level != nil {
		formalCode = outcome.Level.Code
		formalTitle = outcome.Level.Label
	}
	if preview.Outcome.Code != formalCode {
		t.Fatalf("outcome code mismatch: preview=%s formal=%s", preview.Outcome.Code, formalCode)
	}
	if preview.Outcome.Title != formalTitle {
		t.Fatalf("outcome title mismatch: preview=%s formal=%s", preview.Outcome.Title, formalTitle)
	}

	formalScores := executionScoreDetail(outcome)
	for key, want := range preview.ScoreDetail {
		got, ok := formalScores[key]
		if !ok {
			t.Fatalf("formal score_detail missing key %q", key)
		}
		if got != want {
			t.Fatalf("score_detail[%s] = %v, want %v", key, got, want)
		}
	}
	for key := range formalScores {
		if _, ok := preview.ScoreDetail[key]; !ok {
			t.Fatalf("preview score_detail missing key %q", key)
		}
	}
}

func executionScoreDetail(outcome *assessment.AssessmentOutcome) map[string]float64 {
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
