package typology

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestExecutorImplementsEvaluatorContract(t *testing.T) {
	var _ evaluationexecute.Evaluator = (*Executor)(nil)
}

func TestExecutorKeys(t *testing.T) {
	if got := NewMBTIExecutor().Key(); got != evaluation.EvaluatorKeyMBTI {
		t.Fatalf("mbti key = %s, want %s", got, evaluation.EvaluatorKeyMBTI)
	}
	if got := NewSBTIExecutor().Key(); got != evaluation.EvaluatorKeySBTI {
		t.Fatalf("sbti key = %s, want %s", got, evaluation.EvaluatorKeySBTI)
	}
}

func TestExecutorAlgorithmGuard(t *testing.T) {
	executor := NewMBTIExecutor()
	_, err := executor.Execute(context.TODO(), evaluationexecute.ExecutionInput{})
	if err == nil {
		t.Fatal("Execute error = nil, want configuration error")
	}
	_ = assessmentmodel.AlgorithmMBTI
}

func TestExecutorFillsPrimaryAndLevel(t *testing.T) {
	executor := NewMBTIExecutor()
	outcome, err := executor.Execute(context.TODO(), evaluationexecute.ExecutionInput{
		Assessment: submittedMBTIAssessment(t),
		Input:      mbtiExecutorInputSnapshot(),
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if outcome == nil || outcome.Primary == nil {
		t.Fatal("outcome primary is required")
	}
	if outcome.Primary.Kind != assessment.OutcomeScoreKindMatchPercent {
		t.Fatalf("primary kind = %s, want %s", outcome.Primary.Kind, assessment.OutcomeScoreKindMatchPercent)
	}
	if outcome.Level == nil || outcome.Level.Code != "INTJ" {
		t.Fatalf("level = %#v, want INTJ type code", outcome.Level)
	}
	if outcome.Profile == nil || outcome.Profile.Code != "INTJ" || outcome.Profile.Kind != assessment.ProfileKindPersonalityType {
		t.Fatalf("profile = %#v, want INTJ personality_type", outcome.Profile)
	}
}

func mbtiExecutorInputSnapshot() *port.InputSnapshot {
	model := mbtiINTJFixtureModel()
	return &port.InputSnapshot{
		Model:        port.NewMBTIModelSnapshot(model),
		ModelPayload: port.MBTIModelPayload{Model: model},
		AnswerSheet: &port.AnswerSheetSnapshot{
			QuestionnaireCode:    "MBTI_TEST",
			QuestionnaireVersion: "1.0.0",
			Answers: []port.AnswerSnapshot{
				{QuestionCode: "Q_EI", Score: 1},
				{QuestionCode: "Q_SN", Score: 5},
				{QuestionCode: "Q_TF", Score: 1},
				{QuestionCode: "Q_JP", Score: 1},
			},
		},
		Questionnaire: &port.QuestionnaireSnapshot{Code: "MBTI_TEST", Version: "1.0.0"},
	}
}

func mbtiINTJFixtureModel() *modeltypology.MBTILegacyModel {
	return &modeltypology.MBTILegacyModel{
		Code:                 "MBTI_TEST",
		Version:              "1.0.0",
		Title:                "MBTI 测试",
		QuestionnaireCode:    "MBTI_TEST",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		DimensionOrder:       []string{"EI", "SN", "TF", "JP"},
		Dimensions: map[string]modeltypology.MBTILegacyDimension{
			"EI": {Code: "EI", Name: "外向-内向", LeftPole: "I", RightPole: "E", Constant: 24, Threshold: 24},
			"SN": {Code: "SN", Name: "感觉-直觉", LeftPole: "S", RightPole: "N", Constant: 24, Threshold: 24},
			"TF": {Code: "TF", Name: "思考-情感", LeftPole: "T", RightPole: "F", Constant: 24, Threshold: 24},
			"JP": {Code: "JP", Name: "判断-知觉", LeftPole: "J", RightPole: "P", Constant: 24, Threshold: 24},
		},
		QuestionMappings: []modeltypology.MBTILegacyQuestionMapping{
			{QuestionCode: "Q_EI", Dimension: "EI", Sign: -1},
			{QuestionCode: "Q_SN", Dimension: "SN", Sign: 1},
			{QuestionCode: "Q_TF", Dimension: "TF", Sign: -1},
			{QuestionCode: "Q_JP", Dimension: "JP", Sign: -1},
		},
		TypeProfiles: []modeltypology.MBTILegacyTypeProfile{
			{
				TypeCode:    "INTJ",
				TypeName:    "建筑师",
				OneLiner:    "独立战略家",
				Summary:     "善于长远规划",
				Strengths:   []string{"系统思考"},
				Weaknesses:  []string{"表达克制"},
				Suggestions: []string{"保留沟通空间"},
			},
		},
		Source: modeltypology.MBTILegacySource{
			Attribution:   "OEJTS",
			License:       "CC BY-NC-SA 4.0",
			NonCommercial: true,
		},
	}
}

func submittedMBTIAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		assessmentmodel.SubKindTypology,
		assessmentmodel.AlgorithmMBTI,
		meta.ID(0),
		meta.NewCode("MBTI_TEST"),
		"1.0.0",
		"MBTI 测试",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8002),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("MBTI_TEST"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6002)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7002)),
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
