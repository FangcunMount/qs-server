package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// V1 contract: MBTI_16 and MBTI_32 share AlgorithmMBTI and pole_composition;
// only questionnaire_code and question_mappings differ — no new module registration required.
func TestV1MBTIMultiVersionExecutorPreservesScoringWithoutNewModule(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		payload   *modeltypology.Payload
		answers   []port.AnswerSnapshot
		wantType  string
		wantMatch float64
	}{
		{
			name:      "MBTI_16",
			payload:   mbti16Payload(),
			answers:   mbti16Answers(),
			wantType:  "INTJ",
			wantMatch: 40,
		},
		{
			name:      "MBTI_32",
			payload:   mbti32Payload(),
			answers:   mbti32Answers(),
			wantType:  "INTJ",
			wantMatch: 40,
		},
	}

	executor, err := typologyeval.NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			snapshot := &port.InputSnapshot{
				Model:        port.NewTypologyModelSnapshot(tc.payload),
				ModelPayload: port.TypologyModelPayload{Payload: tc.payload},
				AnswerSheet: &port.AnswerSheetSnapshot{
					QuestionnaireCode:    tc.payload.QuestionnaireCode,
					QuestionnaireVersion: tc.payload.QuestionnaireVersion,
					Answers:              tc.answers,
				},
				Questionnaire: &port.QuestionnaireSnapshot{
					Code:    tc.payload.QuestionnaireCode,
					Version: tc.payload.QuestionnaireVersion,
				},
			}
			result, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
				Assessment: submittedMBTIAssessment(t),
				Input:      snapshot,
			})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			detail := requirePersonalityTypeDetail(t, result.Detail.Payload)
			if detail.TypeCode != tc.wantType {
				t.Fatalf("TypeCode = %s, want %s", detail.TypeCode, tc.wantType)
			}
			if detail.MatchPercent != tc.wantMatch {
				t.Fatalf("MatchPercent = %.2f, want %.0f", detail.MatchPercent, tc.wantMatch)
			}
		})
	}
}

func mbti16Payload() *modeltypology.Payload {
	return modeltypology.FromMBTI(&modeltypology.MBTILegacyModel{
		Code:                 "MBTI_16",
		Version:              "1.0.0",
		Title:                "MBTI 16题版",
		QuestionnaireCode:    "MBTI_16",
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
			{TypeCode: "INTJ", TypeName: "建筑师", OneLiner: "独立战略家"},
		},
	})
}

func mbti32Payload() *modeltypology.Payload {
	return modeltypology.FromMBTI(&modeltypology.MBTILegacyModel{
		Code:                 "MBTI_32",
		Version:              "1.0.0",
		Title:                "MBTI 32题版",
		QuestionnaireCode:    "MBTI_32",
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
			{QuestionCode: "Q_EI_1", Dimension: "EI", Sign: -1},
			{QuestionCode: "Q_EI_2", Dimension: "EI", Sign: -1},
			{QuestionCode: "Q_SN_1", Dimension: "SN", Sign: 1},
			{QuestionCode: "Q_SN_2", Dimension: "SN", Sign: 1},
			{QuestionCode: "Q_TF_1", Dimension: "TF", Sign: -1},
			{QuestionCode: "Q_TF_2", Dimension: "TF", Sign: -1},
			{QuestionCode: "Q_JP_1", Dimension: "JP", Sign: -1},
			{QuestionCode: "Q_JP_2", Dimension: "JP", Sign: -1},
		},
		TypeProfiles: []modeltypology.MBTILegacyTypeProfile{
			{TypeCode: "INTJ", TypeName: "建筑师", OneLiner: "独立战略家"},
		},
	})
}

func mbti16Answers() []port.AnswerSnapshot {
	return []port.AnswerSnapshot{
		{QuestionCode: "Q_EI", Score: 1},
		{QuestionCode: "Q_SN", Score: 5},
		{QuestionCode: "Q_TF", Score: 1},
		{QuestionCode: "Q_JP", Score: 1},
	}
}

func mbti32Answers() []port.AnswerSnapshot {
	return []port.AnswerSnapshot{
		{QuestionCode: "Q_EI_1", Score: 1},
		{QuestionCode: "Q_EI_2", Score: 1},
		{QuestionCode: "Q_SN_1", Score: 5},
		{QuestionCode: "Q_SN_2", Score: 5},
		{QuestionCode: "Q_TF_1", Score: 1},
		{QuestionCode: "Q_TF_2", Score: 1},
		{QuestionCode: "Q_JP_1", Score: 1},
		{QuestionCode: "Q_JP_2", Score: 1},
	}
}
