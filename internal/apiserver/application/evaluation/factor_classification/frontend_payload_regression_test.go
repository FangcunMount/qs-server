package factor_classification

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	personalitydomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestFrontendPayloadMBTIEndToEndContract(t *testing.T) {
	t.Parallel()
	runFrontendPayloadContract(t, frontendPayloadCase{
		file:          "../../../testdata/personality/frontend_payload_mbti.json",
		code:          "FRONTEND_MBTI",
		title:         "Frontend MBTI",
		questionnaire: "Q_FRONTEND_MBTI",
		answers: []port.AnswerSnapshot{
			{QuestionCode: "Q_EI", Score: 1},
			{QuestionCode: "Q_SN", Score: 5},
			{QuestionCode: "Q_TF", Score: 1},
			{QuestionCode: "Q_JP", Score: 1},
		},
		questions: []modeltypology.QuestionSnapshot{
			{Code: "Q_EI"},
			{Code: "Q_SN"},
			{Code: "Q_TF"},
			{Code: "Q_JP"},
		},
		wantProfileCode: "INTJ",
	})
}

func TestFrontendPayloadSBTIEndToEndContract(t *testing.T) {
	t.Parallel()
	runFrontendPayloadContract(t, frontendPayloadCase{
		file:          "../../../testdata/personality/frontend_payload_sbti.json",
		code:          "FRONTEND_SBTI",
		title:         "Frontend SBTI",
		questionnaire: "Q_FRONTEND_SBTI",
		answers: []port.AnswerSnapshot{
			{QuestionCode: "Q1", Value: "C"},
			{QuestionCode: "Q2", Value: "C"},
			{QuestionCode: "Q3", Value: "C"},
			{QuestionCode: "Q4", Value: "C"},
		},
		questions: []modeltypology.QuestionSnapshot{
			{Code: "Q1", OptionCodes: []string{"A", "B", "C"}},
			{Code: "Q2", OptionCodes: []string{"A", "B", "C"}},
			{Code: "Q3", OptionCodes: []string{"A", "B", "C"}},
			{Code: "Q4", OptionCodes: []string{"A", "B", "C"}},
		},
		wantProfileCode: "HIGH",
	})
}

type frontendPayloadCase struct {
	file            string
	code            string
	title           string
	questionnaire   string
	answers         []port.AnswerSnapshot
	questions       []modeltypology.QuestionSnapshot
	wantProfileCode string
}

func runFrontendPayloadContract(t *testing.T, tc frontendPayloadCase) {
	t.Helper()
	payloadData, err := os.ReadFile(tc.file)
	if err != nil {
		t.Fatalf("read payload fixture: %v", err)
	}
	var draftPayload modeltypology.Payload
	if err := json.Unmarshal(payloadData, &draftPayload); err != nil {
		t.Fatalf("decode payload fixture: %v", err)
	}
	if draftPayload.Algorithm == "" {
		t.Fatal("fixture algorithm is required")
	}

	model, err := domainmodel.NewAssessmentModel(domainmodel.NewAssessmentModelInput{
		Code:      tc.code,
		Kind:      domainmodel.KindPersonality,
		SubKind:   domainmodel.SubKindTypology,
		Algorithm: draftPayload.Algorithm,
		Title:     tc.title,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if err := model.BindQuestionnaire(domainmodel.QuestionnaireBinding{
		QuestionnaireCode:    tc.questionnaire,
		QuestionnaireVersion: "1.0.0",
	}, model.CreatedAt); err != nil {
		t.Fatalf("BindQuestionnaire: %v", err)
	}
	if err := model.UpdateDefinition(domainmodel.DefinitionPayload{
		Format: domainmodel.PayloadFormatPersonalityTypologyV1,
		Data:   payloadData,
	}, model.CreatedAt); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	if err := model.MarkPublished(model.CreatedAt); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	snapshot, err := personalitydomain.BuildPublishedSnapshot(model)
	if err != nil {
		t.Fatalf("BuildPublishedSnapshot: %v", err)
	}
	var publishedPayload modeltypology.Payload
	if err := json.Unmarshal(snapshot.Payload, &publishedPayload); err != nil {
		t.Fatalf("decode published payload: %v", err)
	}
	runtime, err := publishedPayload.ToRuntimeSpec()
	if err != nil {
		t.Fatalf("ToRuntimeSpec: %v", err)
	}
	issues := modeltypology.ValidateRuntimeSpecForPublishWithContext(
		runtime,
		modeltypology.QuestionnaireSnapshot{Code: tc.questionnaire, Version: "1.0.0", Questions: tc.questions},
		modeltypology.RuntimeSpecValidationContext{Algorithm: publishedPayload.Algorithm, Outcomes: publishedPayload.Outcomes},
	)
	if len(issues) > 0 {
		t.Fatalf("ValidateRuntimeSpecForPublishWithContext issues = %#v", issues)
	}

	input := &port.InputSnapshot{
		Model:        port.NewTypologyModelSnapshot(&publishedPayload),
		ModelPayload: port.TypologyModelPayload{Payload: &publishedPayload},
		AnswerSheet: &port.AnswerSheetSnapshot{
			QuestionnaireCode:    tc.questionnaire,
			QuestionnaireVersion: "1.0.0",
			Answers:              tc.answers,
		},
		Questionnaire: &port.QuestionnaireSnapshot{Code: tc.questionnaire, Version: "1.0.0"},
	}
	executor, err := NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
	outcome, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: frontendSubmittedAssessment(t, tc, publishedPayload.Algorithm),
		Input:      input,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.Profile == nil || outcome.Profile.Code != tc.wantProfileCode {
		t.Fatalf("profile = %#v, want code %s", outcome.Profile, tc.wantProfileCode)
	}

	reportBuilder, err := NewConfiguredReportBuilder()
	if err != nil {
		t.Fatalf("NewConfiguredReportBuilder: %v", err)
	}
	report, err := reportBuilder.Build(context.Background(), evaloutcome.Outcome{
		Assessment: frontendSubmittedAssessment(t, tc, publishedPayload.Algorithm),
		Input:      input,
		Execution:  outcome,
	})
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}
	if report == nil {
		t.Fatal("report is nil")
	}
}

func frontendSubmittedAssessment(t *testing.T, tc frontendPayloadCase, algorithm domainmodel.Algorithm) *assessment.Assessment {
	t.Helper()
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		domainmodel.SubKindTypology,
		algorithm,
		meta.ID(0),
		meta.NewCode(tc.code),
		"v4",
		tc.title,
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(9001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode(tc.questionnaire), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(9002)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(9003)),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	a.ClearEvents()
	return a
}
