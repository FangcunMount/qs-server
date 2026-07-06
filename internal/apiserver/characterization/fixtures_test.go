package characterization_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	evaluationinputdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func scaleInputSnapshot() *evaluationinput.InputSnapshot {
	return &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:    evaluationinput.EvaluationModelKindScale,
			Code:    "S-001",
			Version: "1.0.0",
			Title:   "Scale",
		},
		MedicalScale: &scalesnapshot.ScaleSnapshot{
			Code:                 "S-001",
			Title:                "Scale",
			ScaleVersion:         "1.0.0",
			QuestionnaireCode:    "Q-001",
			QuestionnaireVersion: "1.0.0",
			Status:               "published",
			Factors: []scalesnapshot.FactorSnapshot{
				{
					Code:            "total",
					Title:           "总分",
					IsTotalScore:    true,
					QuestionCodes:   []string{"q1", "q2"},
					ScoringStrategy: "sum",
					InterpretRules: []scalesnapshot.InterpretRuleSnapshot{
						{Min: 0, Max: 10, RiskLevel: "low", Conclusion: "low", Suggestion: "keep"},
					},
				},
				{
					Code:            "sleep",
					Title:           "睡眠",
					QuestionCodes:   []string{"q2"},
					ScoringStrategy: "sum",
					MaxScore:        ptrFloat64(3),
					InterpretRules: []scalesnapshot.InterpretRuleSnapshot{
						{Min: 0, Max: 1, RiskLevel: "low", Conclusion: "睡眠尚可", Suggestion: "保持作息"},
						{Min: 2, Max: 3, RiskLevel: "medium", Conclusion: "睡眠问题明显", Suggestion: "建立睡前放松流程"},
					},
				},
			},
		},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			QuestionnaireCode:    "Q-001",
			QuestionnaireVersion: "1.0.0",
			Answers: []evaluationinput.AnswerSnapshot{
				{QuestionCode: "q1", Score: 3},
				{QuestionCode: "q2", Score: 2},
			},
		},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-001", Version: "1.0.0"},
	}
}

func submittedScaleAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	scaleRef := assessment.NewMedicalScaleRef(meta.FromUint64(9001), meta.NewCode("S-001"), "Scale")
	modelRef := assessment.NewEvaluationModelRefByCode(
		assessment.EvaluationModelKindScale,
		meta.NewCode("S-001"),
		"1.0.0",
		"Scale",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7001)),
		assessment.WithMedicalScale(scaleRef),
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

func draftScaleAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	scaleRef := assessment.NewMedicalScaleRef(meta.FromUint64(9001), meta.NewCode("S-001"), "Scale")
	modelRef := assessment.NewEvaluationModelRefByCode(
		assessment.EvaluationModelKindScale,
		meta.NewCode("S-001"),
		"1.0.0",
		"Scale",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7001)),
		assessment.WithMedicalScale(scaleRef),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	return a
}

func mbtiINTJModel() *modeltypology.MBTILegacyModel {
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

func mbtiINTJAnswerSheet() *evaluationinputdomain.AnswerSheet {
	return &evaluationinputdomain.AnswerSheet{
		Answers: []evaluationinputdomain.Answer{
			{QuestionCode: "Q_EI", Score: 1},
			{QuestionCode: "Q_SN", Score: 5},
			{QuestionCode: "Q_TF", Score: 1},
			{QuestionCode: "Q_JP", Score: 1},
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

func sbtiCharacterizationModel() *modeltypology.SBTILegacyModel {
	return &modeltypology.SBTILegacyModel{
		Code:                        "SBTI_FUN",
		Version:                     "1.0.0",
		Title:                       "SBTI",
		QuestionnaireCode:           "SBTI_FUN",
		QuestionnaireVersion:        "1.0.0",
		FallbackSimilarityThreshold: 0.6,
		DimensionOrder:              []string{"D1", "D2"},
		Dimensions: map[string]modeltypology.SBTILegacyDimension{
			"D1": {Code: "D1", Name: "行动力", Model: "Alpha"},
			"D2": {Code: "D2", Name: "社交力", Model: "Beta"},
		},
		QuestionMappings: []modeltypology.SBTILegacyQuestionMapping{
			{QuestionCode: "Q1", Dimension: "D1", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q2", Dimension: "D1", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q3", Dimension: "D2", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
			{QuestionCode: "Q4", Dimension: "D2", OptionScores: map[string]float64{"A": 1, "B": 2, "C": 3}},
		},
		NormalOutcomes: []modeltypology.SBTILegacyOutcome{
			{
				Code:       "HIGH",
				Name:       "高能者",
				Pattern:    "HH",
				OneLiner:   "all high",
				Commentary: "你是典型高能者",
				Rarity:     modeltypology.SBTILegacyRarity{Percent: 5.0, Label: "常见", OneInX: 20},
			},
		},
		Source: modeltypology.SBTILegacySource{
			Attribution:   "SBTI Wiki",
			License:       "CC BY-NC-SA 4.0",
			NonCommercial: true,
		},
	}
}

func sbtiHighAnswerSheet() *evaluationinputdomain.AnswerSheet {
	return &evaluationinputdomain.AnswerSheet{
		Answers: []evaluationinputdomain.Answer{
			{QuestionCode: "Q1", Value: "C"},
			{QuestionCode: "Q2", Value: "C"},
			{QuestionCode: "Q3", Value: "C"},
			{QuestionCode: "Q4", Value: "C"},
		},
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
		"SBTI",
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

func ptrFloat64(v float64) *float64 { return &v }

func behavioralRatingInputSnapshot() *evaluationinput.InputSnapshot {
	snapshot := &behavioralsnapshot.Snapshot{
		Code:                 "BR-001",
		Version:              "1.0.0",
		Title:                "行为评分",
		QuestionnaireCode:    "Q-001",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Factors: []behavioralsnapshot.FactorSnapshot{
			{
				Code:            "total",
				Title:           "总分",
				IsTotalScore:    true,
				QuestionCodes:   []string{"q1", "q2"},
				ScoringStrategy: "sum",
				InterpretRules: []behavioralsnapshot.InterpretRuleSnapshot{
					{MinScore: 0, MaxScore: 10, Conclusion: "low", Level: "low", Suggestion: "keep"},
				},
			},
		},
	}
	return &evaluationinput.InputSnapshot{
		Model:        evaluationinput.NewBehavioralRatingModelSnapshot(snapshot),
		ModelPayload: evaluationinput.BehavioralRatingModelPayload{Snapshot: snapshot},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			QuestionnaireCode:    "Q-001",
			QuestionnaireVersion: "1.0.0",
			Answers: []evaluationinput.AnswerSnapshot{
				{QuestionCode: "q1", Score: 3},
				{QuestionCode: "q2", Score: 2},
			},
		},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "Q-001", Version: "1.0.0"},
	}
}

func draftBehavioralRatingAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	modelRef := assessment.NewEvaluationModelRefByCode(
		modelcatalog.KindBehavioralRating,
		meta.NewCode("BR-001"),
		"1.0.0",
		"行为评分",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8005),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6005)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7005)),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	return a
}

func mbtiInputSnapshot() *evaluationinput.InputSnapshot {
	model := mbtiINTJModel()
	payload := modeltypology.FromMBTI(model)
	return &evaluationinput.InputSnapshot{
		Model:        evaluationinput.NewTypologyModelSnapshot(payload),
		ModelPayload: evaluationinput.TypologyModelPayload{Payload: payload},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			QuestionnaireCode:    "MBTI_TEST",
			QuestionnaireVersion: "1.0.0",
			Answers: []evaluationinput.AnswerSnapshot{
				{QuestionCode: "Q_EI", Score: 1},
				{QuestionCode: "Q_SN", Score: 5},
				{QuestionCode: "Q_TF", Score: 1},
				{QuestionCode: "Q_JP", Score: 1},
			},
		},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "MBTI_TEST", Version: "1.0.0"},
	}
}

func sbtiInputSnapshot() *evaluationinput.InputSnapshot {
	model := sbtiCharacterizationModel()
	payload := modeltypology.FromSBTI(model)
	return &evaluationinput.InputSnapshot{
		Model:        evaluationinput.NewTypologyModelSnapshot(payload),
		ModelPayload: evaluationinput.TypologyModelPayload{Payload: payload},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			QuestionnaireCode:    "SBTI_FUN",
			QuestionnaireVersion: "1.0.0",
			Answers: []evaluationinput.AnswerSnapshot{
				{QuestionCode: "Q1", Value: "C"},
				{QuestionCode: "Q2", Value: "C"},
				{QuestionCode: "Q3", Value: "C"},
				{QuestionCode: "Q4", Value: "C"},
			},
		},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "SBTI_FUN", Version: "1.0.0"},
	}
}

func bigFiveCharacterizationModel() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:                 "BIGFIVE_V1",
		Version:              "1.0.0",
		Title:                "Big Five",
		QuestionnaireCode:    "BIGFIVE_V1",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Algorithm:            modelcatalog.AlgorithmBigFive,
		DimensionOrder:       []string{"O", "C", "E", "A", "N"},
		Dimensions: map[string]modeltypology.Dimension{
			"O": {Code: "O", Name: "Openness"},
			"C": {Code: "C", Name: "Conscientiousness"},
			"E": {Code: "E", Name: "Extraversion"},
			"A": {Code: "A", Name: "Agreeableness"},
			"N": {Code: "N", Name: "Neuroticism"},
		},
		QuestionMappings: []modeltypology.QuestionMapping{
			{QuestionCode: "O1", Dimension: "O", Sign: 1},
			{QuestionCode: "O2", Dimension: "O", Sign: 1},
			{QuestionCode: "C1", Dimension: "C", Sign: 1},
			{QuestionCode: "C2", Dimension: "C", Sign: 1},
			{QuestionCode: "E1", Dimension: "E", Sign: 1},
			{QuestionCode: "E2", Dimension: "E", Sign: 1},
			{QuestionCode: "A1", Dimension: "A", Sign: 1},
			{QuestionCode: "A2", Dimension: "A", Sign: 1},
			{QuestionCode: "N1", Dimension: "N", Sign: 1},
			{QuestionCode: "N2", Dimension: "N", Sign: 1},
		},
		MatchingSpec: modeltypology.MatchingSpec{
			Kind: modelcatalog.DecisionKindTraitProfile,
		},
		Source: modeltypology.Source{
			Attribution:   "IPIP",
			License:       "CC0",
			NonCommercial: false,
		},
	}
}

func bigFiveAnswerSheet() *evaluationinputdomain.AnswerSheet {
	return &evaluationinputdomain.AnswerSheet{
		Answers: []evaluationinputdomain.Answer{
			{QuestionCode: "O1", Score: 4},
			{QuestionCode: "O2", Score: 2},
			{QuestionCode: "C1", Score: 5},
			{QuestionCode: "C2", Score: 3},
			{QuestionCode: "E1", Score: 3},
			{QuestionCode: "E2", Score: 3},
			{QuestionCode: "A1", Score: 4},
			{QuestionCode: "A2", Score: 4},
			{QuestionCode: "N1", Score: 2},
			{QuestionCode: "N2", Score: 2},
		},
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

func bigFiveInputSnapshot() *evaluationinput.InputSnapshot {
	payload := bigFiveCharacterizationModel()
	return &evaluationinput.InputSnapshot{
		Model:        evaluationinput.NewTypologyModelSnapshot(payload),
		ModelPayload: evaluationinput.TypologyModelPayload{Payload: payload},
		AnswerSheet: &evaluationinput.AnswerSheetSnapshot{
			QuestionnaireCode:    "BIGFIVE_V1",
			QuestionnaireVersion: "1.0.0",
			Answers: []evaluationinput.AnswerSnapshot{
				{QuestionCode: "O1", Score: 4},
				{QuestionCode: "O2", Score: 2},
				{QuestionCode: "C1", Score: 5},
				{QuestionCode: "C2", Score: 3},
				{QuestionCode: "E1", Score: 3},
				{QuestionCode: "E2", Score: 3},
				{QuestionCode: "A1", Score: 4},
				{QuestionCode: "A2", Score: 4},
				{QuestionCode: "N1", Score: 2},
				{QuestionCode: "N2", Score: 2},
			},
		},
		Questionnaire: &evaluationinput.QuestionnaireSnapshot{Code: "BIGFIVE_V1", Version: "1.0.0"},
	}
}

func customExplicitRuntimePayload() *modeltypology.Payload {
	return &modeltypology.Payload{
		Code:                 "CUSTOM_POLE_V1",
		Version:              "1.0.0",
		QuestionnaireCode:    "CUSTOM_POLE_V1",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
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
				DetailKind: modeltypology.OutcomeDetailPersonalityType,
			},
			Report: modeltypology.ReportSpec{
				Kind:          modeltypology.ReportKindPersonalityType,
				CategoryLabel: "Custom Pole Model",
			},
		},
	}
}

func customRuntimeInputSnapshot() *evaluationinput.InputSnapshot {
	payload := customExplicitRuntimePayload()
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

func submittedCustomRuntimeAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmPersonalityTypology,
		meta.ID(0),
		meta.NewCode("CUSTOM_POLE_V1"),
		"1.0.0",
		"Custom Pole Model",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8010),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("CUSTOM_POLE_V1"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6010)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7010)),
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
