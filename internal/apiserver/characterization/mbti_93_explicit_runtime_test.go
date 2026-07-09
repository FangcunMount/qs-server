package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// MBTI_93_V1 uses explicit runtime + model code; it must not require a new Algorithm constant or module.
func TestV2MBTI93ExplicitRuntimeRunsWithoutNewAlgorithmOrModule(t *testing.T) {
	t.Parallel()

	for _, algorithm := range []modelcatalog.Algorithm{"", modelcatalog.AlgorithmPersonalityTypology} {
		algorithm := algorithm
		t.Run(string(algorithmOrConfigured(algorithm)), func(t *testing.T) {
			t.Parallel()

			executor, err := typologyeval.NewConfiguredTypologyExecutor()
			if err != nil {
				t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
			}
			reportBuilder, err := typologyeval.NewConfiguredReportBuilder()
			if err != nil {
				t.Fatalf("NewConfiguredReportBuilder: %v", err)
			}

			payload := mbti93ExplicitRuntimePayload(algorithm)
			snapshot := mbti93InputSnapshot(payload)
			assessmentEntity := submittedMBTI93Assessment(t)

			outcome, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
				Assessment: assessmentEntity,
				Input:      snapshot,
			})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			detail := requirePersonalityTypeDetail(t, outcome.Detail.Payload)
			if detail.TypeCode != "INTJ" || detail.MatchPercent != 40 {
				t.Fatalf("detail = %#v, want INTJ@40", detail)
			}

			report, err := reportBuilder.Build(context.Background(), evaloutcome.Outcome{
				Assessment: assessmentEntity,
				Input:      snapshot,
				Execution:  outcome,
			})
			if err != nil {
				t.Fatalf("Build report: %v", err)
			}
			if report.Conclusion() == "" {
				t.Fatal("expected non-empty report conclusion")
			}
			if extra := report.ModelExtra(); extra == nil || extra.TypeCode != "INTJ" {
				t.Fatalf("ModelExtra = %#v, want INTJ", extra)
			}
		})
	}
}

func algorithmOrConfigured(algorithm modelcatalog.Algorithm) string {
	if algorithm == "" {
		return "empty_algorithm"
	}
	return string(algorithm)
}

func mbti93ExplicitRuntimePayload(algorithm modelcatalog.Algorithm) *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:                 "MBTI_93_V1",
		Version:              "1.0.0",
		Title:                "MBTI 93题版",
		QuestionnaireCode:    "MBTI_93_V1",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Algorithm:            algorithm,
		DimensionOrder:       []string{"EI", "SN", "TF", "JP"},
		Dimensions: map[string]modeltypology.Dimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
			"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
			"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
			"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
		},
		QuestionMappings: []modeltypology.QuestionMapping{
			{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
			{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
			{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
			{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
		},
		Outcomes: []modeltypology.Outcome{
			{Code: "INTJ", Name: "建筑师", OneLiner: "独立战略家"},
		},
		Runtime: &modeltypology.RuntimeSpec{
			FactorGraph: modeltypology.FactorGraphSpec{
				DimensionOrder: []string{"EI", "SN", "TF", "JP"},
				Dimensions: map[string]modeltypology.Dimension{
					"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
					"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
					"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
					"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
				},
				QuestionMappings: []modeltypology.QuestionMapping{
					{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
					{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
					{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
					{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
				},
			},
			Decision: modeltypology.PersonalityDecisionSpec{
				Kind: modelcatalog.DecisionKindPoleComposition,
			},
			OutcomeMapping: modeltypology.OutcomeMappingSpec{
				DetailKind:       modeltypology.OutcomeDetailPersonalityType,
				DetailAdapterKey: modeltypology.DetailAdapterPersonalityType,
			},
			Report: modeltypology.ReportSpec{
				Kind:          modeltypology.ReportKindPersonalityType,
				AdapterKey:    modeltypology.ReportAdapterPersonalityType,
				CategoryLabel: "MBTI",
			},
		},
	}
}

func mbti93InputSnapshot(payload *modeltypology.Payload) *evaluationinput.InputSnapshot {
	return &evaluationinput.InputSnapshot{
		Model:        evaluationinput.NewTypologyModelSnapshot(payload),
		ModelPayload: evaluationinput.TypologyModelPayload{Payload: payload},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			QuestionnaireCode:    payload.QuestionnaireCode,
			QuestionnaireVersion: payload.QuestionnaireVersion,
			Answers: []evaluationinput.AnswerSnapshot{
				{QuestionCode: "Q_EI", Score: 1},
				{QuestionCode: "Q_SN", Score: 5},
				{QuestionCode: "Q_TF", Score: 1},
				{QuestionCode: "Q_JP", Score: 1},
			},
		},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{
			Code:    payload.QuestionnaireCode,
			Version: payload.QuestionnaireVersion,
		},
	}
}

func submittedMBTI93Assessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmPersonalityTypology,
		meta.ID(0),
		meta.NewCode("MBTI_93_V1"),
		"1.0.0",
		"MBTI 93题版",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8011),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("MBTI_93_V1"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6011)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7011)),
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
