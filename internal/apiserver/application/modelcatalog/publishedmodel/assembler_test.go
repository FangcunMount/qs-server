package publishedmodel

import (
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestBuildAssessmentSnapshotContractMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		model         *domain.AssessmentModel
		wantProduct   domain.ProductChannel
		wantKind      domain.Kind
		wantSubKind   domain.SubKind
		wantAlgorithm domain.Algorithm
		wantFormat    string
		wantDecision  domain.DecisionKind
	}{
		{
			name: "medical scale",
			model: newSnapshotTestModel(t, domain.NewAssessmentModelInput{
				Code:           "PHQ9",
				Kind:           domain.KindScale,
				Algorithm:      domain.AlgorithmScaleDefault,
				ProductChannel: domain.ProductChannelMedicalScale,
				Title:          "PHQ-9",
			}, domain.PayloadFormatAssessmentScaleV1, []byte(`{"code":"PHQ9"}`)),
			wantProduct:   domain.ProductChannelMedicalScale,
			wantKind:      domain.KindScale,
			wantAlgorithm: domain.AlgorithmScaleDefault,
			wantFormat:    domain.PayloadFormatAssessmentScaleV1,
			wantDecision:  domain.DecisionKindScoreRange,
		},
		{
			name: "typology",
			model: newSnapshotTestModel(t, domain.NewAssessmentModelInput{
				Code:           "MBTI",
				Kind:           domain.KindTypology,
				SubKind:        domain.SubKindTypology,
				Algorithm:      domain.AlgorithmMBTI,
				ProductChannel: domain.ProductChannelTypology,
				Title:          "MBTI",
			}, domain.PayloadFormatPersonalityTypologyV1, []byte(`{
				"algorithm":"mbti",
				"outcomes":[{"code":"INTJ","name":"建筑师"}],
				"runtime":{
					"factor_graph":{"dimension_order":["EI"],"dimensions":{"EI":{"code":"EI","name":"EI"}},"roots":["EI"]},
					"decision":{"kind":"pole_composition"},
					"outcome_mapping":{"detail_kind":"personality_type","detail_adapter_key":"personality_type"},
					"report":{"kind":"personality_type","adapter_key":"personality_type"}
				}
			}`)),
			wantProduct:   domain.ProductChannelTypology,
			wantKind:      domain.KindTypology,
			wantSubKind:   domain.SubKindTypology,
			wantAlgorithm: domain.AlgorithmMBTI,
			wantFormat:    domain.PayloadFormatPersonalityTypologyV1,
			wantDecision:  domain.DecisionKindPoleComposition,
		},
		{
			name: "behavioral rating",
			model: newSnapshotTestModel(t, domain.NewAssessmentModelInput{
				Code:           "BRIEF2",
				Kind:           domain.KindBehavioralRating,
				Algorithm:      domain.AlgorithmBrief2,
				ProductChannel: domain.ProductChannelBehaviorAbility,
				Title:          "BRIEF-2",
			}, domain.PayloadFormatBehavioralRatingDefaultV1, []byte(`{"dimensions":[],"brief2":{"primary_dimension_code":"bri"}}`)),
			wantProduct:   domain.ProductChannelBehaviorAbility,
			wantKind:      domain.KindBehavioralRating,
			wantAlgorithm: domain.AlgorithmBrief2,
			wantFormat:    domain.PayloadFormatBehavioralRatingDefaultV1,
			wantDecision:  domain.DecisionKindNormLookup,
		},
		{
			name: "cognitive",
			model: newSnapshotTestModel(t, domain.NewAssessmentModelInput{
				Code:           "SPM",
				Kind:           domain.KindCognitive,
				Algorithm:      domain.AlgorithmSPM,
				ProductChannel: domain.ProductChannelBehaviorAbility,
				Title:          "SPM",
			}, domain.PayloadFormatCognitiveDefaultV1, []byte(`{"dimensions":[{"code":"total"}]}`)),
			wantProduct:   domain.ProductChannelBehaviorAbility,
			wantKind:      domain.KindCognitive,
			wantAlgorithm: domain.AlgorithmSPM,
			wantFormat:    domain.PayloadFormatCognitiveDefaultV1,
			wantDecision:  domain.DecisionKindAbilityLevel,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			snapshot, err := BuildAssessmentSnapshot(tc.model)
			if err != nil {
				t.Fatalf("BuildAssessmentSnapshot: %v", err)
			}
			if snapshot.ProductChannel != tc.wantProduct ||
				snapshot.Kind != tc.wantKind ||
				snapshot.SubKind != tc.wantSubKind ||
				snapshot.Algorithm != tc.wantAlgorithm ||
				snapshot.PayloadFormat != tc.wantFormat ||
				snapshot.DecisionKind != tc.wantDecision {
				t.Fatalf("snapshot identity = %#v", snapshot)
			}
			if snapshot.QuestionnaireVersion != "1.0.0" {
				t.Fatalf("questionnaire version = %q", snapshot.QuestionnaireVersion)
			}
			if snapshot.Version == "" || snapshot.Version == snapshot.QuestionnaireVersion {
				t.Fatalf("snapshot version = %q, questionnaire version = %q", snapshot.Version, snapshot.QuestionnaireVersion)
			}
			if len(snapshot.Payload) == 0 {
				t.Fatal("snapshot payload is empty")
			}
		})
	}
}

func newSnapshotTestModel(t *testing.T, input domain.NewAssessmentModelInput, format string, payload []byte) *domain.AssessmentModel {
	t.Helper()

	input.Now = time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(input)
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{
		QuestionnaireCode:    "Q-" + input.Code,
		QuestionnaireVersion: "1.0.0",
	}, input.Now); err != nil {
		t.Fatalf("BindQuestionnaire: %v", err)
	}
	if err := model.UpdateDefinitionWithV2(domain.DefinitionPayload{
		Format: format,
		Data:   payload,
	}, snapshotDefinitionForKind(input.Kind), input.Now); err != nil {
		t.Fatalf("UpdateDefinitionWithV2: %v", err)
	}
	if err := model.MarkPublished(input.Now); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}
	return model
}

func snapshotDefinitionForKind(kind domain.Kind) *domain.Definition {
	switch kind {
	case domain.KindBehavioralRating:
		return &domain.Definition{Conclusions: []domain.Conclusion{
			domain.NormConclusion{FactorCode: "bri", Primary: true},
		}}
	case domain.KindTypology:
		return &domain.Definition{Conclusions: []domain.Conclusion{
			domain.TypeConclusion{Decision: domain.TypeDecision{Kind: domain.DecisionKindPoleComposition}},
		}}
	default:
		return &domain.Definition{}
	}
}
