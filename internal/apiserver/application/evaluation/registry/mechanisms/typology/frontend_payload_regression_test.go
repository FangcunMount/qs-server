package typology

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	interpretationinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/input"
	typologyreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/typology"
	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	domainmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	evaluationfactcodec "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestFrontendPayloadMBTIEndToEndContract(t *testing.T) {
	t.Parallel()
	runFrontendPayloadContract(t, frontendPayloadCase{
		file:          "../../../../../testdata/personality/frontend_payload_mbti.json",
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
		file:          "../../../../../testdata/personality/frontend_payload_sbti.json",
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
	materialized, err := modeltypology.ImportLegacyDefinition(payloadData, draftPayload.Algorithm)
	if err != nil {
		t.Fatalf("ImportLegacyDefinition: %v", err)
	}

	model, err := domainmodel.NewAssessmentModel(domainmodel.NewAssessmentModelInput{
		Code:      tc.code,
		Kind:      domainmodel.KindTypology,
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
	if err := model.UpdateDefinitionWithV2(domainmodel.DefinitionPayload{
		Format: domainmodel.PayloadFormatPersonalityTypologyV1,
		Data:   payloadData,
	}, materialized.Definition, model.CreatedAt); err != nil {
		t.Fatalf("UpdateDefinitionWithV2: %v", err)
	}
	if err := model.MarkPublished(model.CreatedAt); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}

	snapshot, err := (appdefinition.TypologyDefinitionHandler{}).BuildSnapshotPayload(context.Background(), model)
	if err != nil {
		t.Fatalf("Build published model: %v", err)
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
	assessed := frontendSubmittedAssessment(t, tc, publishedPayload.Algorithm)
	outcome, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: assessed,
		Input:      input,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if outcome.Profile == nil || outcome.Profile.Code != tc.wantProfileCode {
		t.Fatalf("profile = %#v, want code %s", outcome.Profile, tc.wantProfileCode)
	}

	reportBuilder, err := typologyreporting.NewConfiguredReportBuilder()
	if err != nil {
		t.Fatalf("NewConfiguredReportBuilder: %v", err)
	}
	ref := outcome.ModelRef
	factModel := evaluationfact.ModelIdentity{Kind: ref.Kind(), SubKind: ref.SubKind(), Algorithm: ref.Algorithm(), Code: ref.Code().String(), Version: ref.Version(), Title: ref.Title()}
	decoded, err := evaluationfactcodec.DecodeTransientExecution(outcome, factModel, evaluationfact.RuntimeIdentity{})
	if err != nil {
		t.Fatalf("decode preview execution: %v", err)
	}
	interpretationInput, err := interpretationinput.FromPreviewOutcome(interpretationinput.PreviewOutcome{
		Association: domainreport.Association{OrgID: assessed.OrgID(), AssessmentID: assessed.ID(), TesteeID: assessed.TesteeID().Uint64()},
		Input:       input, Execution: decoded,
	})
	if err != nil {
		t.Fatalf("adapt interpretation input: %v", err)
	}
	draft, err := reportBuilder.Build(context.Background(), interpretationInput)
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}
	if draft == nil {
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
