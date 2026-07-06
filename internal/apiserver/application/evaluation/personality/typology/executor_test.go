package typology

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestExecutorImplementsEvaluatorContract(t *testing.T) {
	var _ evaluationexecute.Evaluator = (*Executor)(nil)
}

func TestExecutorKeys(t *testing.T) {
	configured, err := NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
	if got := configured.Key(); got != evaluation.EvaluatorKeyPersonalityTypology {
		t.Fatalf("configured key = %s, want %s", got, evaluation.EvaluatorKeyPersonalityTypology)
	}

	mbtiExecutor, err := NewTypologyExecutor(modelcatalog.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("NewTypologyExecutor(mbti): %v", err)
	}
	if got := mbtiExecutor.Key(); got != evaluation.EvaluatorKeyMBTI {
		t.Fatalf("mbti key = %s, want %s", got, evaluation.EvaluatorKeyMBTI)
	}
	sbtiExecutor, err := NewTypologyExecutor(modelcatalog.AlgorithmSBTI)
	if err != nil {
		t.Fatalf("NewTypologyExecutor(sbti): %v", err)
	}
	if got := sbtiExecutor.Key(); got != evaluation.EvaluatorKeySBTI {
		t.Fatalf("sbti key = %s, want %s", got, evaluation.EvaluatorKeySBTI)
	}
}

func TestExecutorFillsPrimaryAndLevel(t *testing.T) {
	executor, err := NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
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

func TestExecutorAlgorithmGuard(t *testing.T) {
	executor, err := NewTypologyExecutor(modelcatalog.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("NewTypologyExecutor: %v", err)
	}
	_, err = executor.Execute(context.TODO(), evaluationexecute.ExecutionInput{})
	if err == nil {
		t.Fatal("Execute error = nil, want configuration error")
	}
}

func TestNewTypologyExecutorRejectsUnsupportedAlgorithm(t *testing.T) {
	_, err := NewTypologyExecutor(modelcatalog.Algorithm("typology_unknown"))
	if err == nil {
		t.Fatal("NewTypologyExecutor error = nil, want unsupported algorithm")
	}
}

func TestSBTIExecutorFillsPrimaryAndLevel(t *testing.T) {
	executor, err := NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
	outcome, err := executor.Execute(context.TODO(), evaluationexecute.ExecutionInput{
		Assessment: submittedSBTIAssessment(t),
		Input:      sbtiExecutorInputSnapshot(),
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if outcome == nil || outcome.Primary == nil {
		t.Fatal("outcome primary is required")
	}
	if outcome.Level == nil || outcome.Level.Code != "HIGH" {
		t.Fatalf("level = %#v, want HIGH type code", outcome.Level)
	}
	if outcome.Profile == nil || outcome.Profile.Kind != assessment.ProfileKindPersonalityType {
		t.Fatalf("profile = %#v, want personality_type", outcome.Profile)
	}
}

func TestBigFiveExecutorFillsTraitProfile(t *testing.T) {
	executor, err := NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
	outcome, err := executor.Execute(context.TODO(), evaluationexecute.ExecutionInput{
		Assessment: submittedBigFiveAssessment(t),
		Input:      bigFiveExecutorInputSnapshot(),
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	detail, ok := outcome.Detail.Payload.(evaluationtypology.BigFiveResultDetail)
	if !ok {
		t.Fatalf("payload type = %T, want BigFiveResultDetail", outcome.Detail.Payload)
	}
	if len(detail.Traits) != 2 || detail.Traits[0].RawScore != 6 {
		t.Fatalf("traits = %#v, want openness raw 6", detail.Traits)
	}
	if outcome.Summary.PrimaryLabel != "O" {
		t.Fatalf("PrimaryLabel = %q, want O", outcome.Summary.PrimaryLabel)
	}
	if outcome.Profile == nil || outcome.Profile.Kind != assessment.ProfileKindPersonalityTrait {
		t.Fatalf("profile = %#v, want personality_trait", outcome.Profile)
	}
}

func bigFiveExecutorInputSnapshot() *port.InputSnapshot {
	payload := &modeltypology.Payload{
		Code:                 "BIGFIVE_V1",
		Version:              "1.0.0",
		Title:                "Big Five",
		QuestionnaireCode:    "BIGFIVE_V1",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Algorithm:            modelcatalog.AlgorithmBigFive,
		DimensionOrder:       []string{"O", "C"},
		Dimensions: map[string]modeltypology.Dimension{
			"O": {Code: "O", Name: "Openness"},
			"C": {Code: "C", Name: "Conscientiousness"},
		},
		QuestionMappings: []modeltypology.QuestionMapping{
			{QuestionCode: "O1", Dimension: "O", Sign: 1},
			{QuestionCode: "O2", Dimension: "O", Sign: 1},
			{QuestionCode: "C1", Dimension: "C", Sign: 1},
			{QuestionCode: "C2", Dimension: "C", Sign: 1},
		},
		MatchingSpec: modeltypology.MatchingSpec{
			Kind: modelcatalog.DecisionKindTraitProfile,
		},
	}
	return &port.InputSnapshot{
		Model:        port.NewTypologyModelSnapshot(payload),
		ModelPayload: port.TypologyModelPayload{Payload: payload},
		AnswerSheet: &port.AnswerSheetSnapshot{
			QuestionnaireCode:    "BIGFIVE_V1",
			QuestionnaireVersion: "1.0.0",
			Answers: []port.AnswerSnapshot{
				{QuestionCode: "O1", Score: 4},
				{QuestionCode: "O2", Score: 2},
				{QuestionCode: "C1", Score: 5},
				{QuestionCode: "C2", Score: 3},
			},
		},
		Questionnaire: &port.QuestionnaireSnapshot{Code: "BIGFIVE_V1", Version: "1.0.0"},
	}
}

func submittedBigFiveAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmBigFive,
		meta.ID(0),
		meta.NewCode("BIGFIVE_V1"),
		"1.0.0",
		"Big Five",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8004),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("BIGFIVE_V1"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6004)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7004)),
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

func sbtiFixtureModel() *modeltypology.SBTILegacyModel {
	return &modeltypology.SBTILegacyModel{
		Code:                        "SBTI_FUN",
		Version:                     "1.0.0",
		Title:                       "SBTI 测试",
		QuestionnaireCode:           "SBTI_FUN",
		QuestionnaireVersion:        "1.0.0",
		Status:                      "published",
		FallbackSimilarityThreshold: 0.6,
		DimensionOrder:              []string{"D1", "D2"},
		Dimensions: map[string]modeltypology.SBTILegacyDimension{
			"D1": {Code: "D1", Name: "D1", Model: "M1"},
			"D2": {Code: "D2", Name: "D2", Model: "M2"},
		},
		QuestionMappings: []modeltypology.SBTILegacyQuestionMapping{
			{QuestionCode: "Q1", Dimension: "D1", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q2", Dimension: "D1", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q3", Dimension: "D2", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q4", Dimension: "D2", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
		},
		NormalOutcomes: []modeltypology.SBTILegacyOutcome{
			{Code: "HIGH", Name: "高能者", Pattern: "HH", OneLiner: "活力满满"},
		},
	}
}

func sbtiExecutorInputSnapshot() *port.InputSnapshot {
	model := sbtiFixtureModel()
	payload := modeltypology.FromSBTI(model)
	return &port.InputSnapshot{
		Model:        port.NewTypologyModelSnapshot(payload),
		ModelPayload: port.TypologyModelPayload{Payload: payload},
		AnswerSheet: &port.AnswerSheetSnapshot{
			QuestionnaireCode:    "SBTI_FUN",
			QuestionnaireVersion: "1.0.0",
			Answers: []port.AnswerSnapshot{
				{QuestionCode: "Q1", Value: "C"},
				{QuestionCode: "Q2", Value: "C"},
				{QuestionCode: "Q3", Value: "C"},
				{QuestionCode: "Q4", Value: "C"},
			},
		},
		Questionnaire: &port.QuestionnaireSnapshot{Code: "SBTI_FUN", Version: "1.0.0"},
	}
}

func submittedSBTIAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmSBTI,
		meta.ID(0),
		meta.NewCode("SBTI_FUN"),
		"1.0.0",
		"SBTI 测试",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8003),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("SBTI_FUN"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6003)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7003)),
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

func mbtiExecutorInputSnapshot() *port.InputSnapshot {
	model := mbtiINTJFixtureModel()
	payload := modeltypology.FromMBTI(model)
	return &port.InputSnapshot{
		Model:        port.NewTypologyModelSnapshot(payload),
		ModelPayload: port.TypologyModelPayload{Payload: payload},
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
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmMBTI,
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
