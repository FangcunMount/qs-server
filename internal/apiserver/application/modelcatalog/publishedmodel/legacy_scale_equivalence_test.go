package publishedmodel_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func TestScalePayloadAssessmentModelSnapshotEquivalence(t *testing.T) {
	t.Parallel()

	scaleSnapshot := newPublishedScaleSnapshotForEquivalence()
	payloadPublished, err := publishedmodel.BuildAssessmentSnapshotFromScale(scaleSnapshot)
	if err != nil {
		t.Fatalf("BuildAssessmentSnapshotFromScale: %v", err)
	}

	model := newPublishedScaleAssessmentModelForEquivalence(t, scaleSnapshot)
	got, err := publishedmodel.BuildAssessmentSnapshot(model)
	if err != nil {
		t.Fatalf("BuildAssessmentSnapshot: %v", err)
	}

	if got.Version != "1.0.0" {
		t.Fatalf("snapshot version = %q, want scale version", got.Version)
	}
	if got.DefinitionV2 == nil {
		t.Fatal("got DefinitionV2 is nil")
	}
	if payloadPublished.DefinitionV2 == nil {
		t.Fatal("payloadPublished DefinitionV2 is nil")
	}
	withoutDefinitionV2 := *got
	withoutDefinitionV2.DefinitionV2 = nil
	if got.Description != "scale definition" || got.Category != "adhd" ||
		!reflect.DeepEqual(got.Stages, []string{"deep_assessment"}) ||
		!reflect.DeepEqual(got.ApplicableAges, []string{"school_child"}) ||
		!reflect.DeepEqual(got.Reporters, []string{"parent"}) ||
		!reflect.DeepEqual(got.Tags, []string{"screening"}) {
		t.Fatalf("scale metadata = %#v", got)
	}
	withoutDefinitionV2.Description = ""
	withoutDefinitionV2.Category = ""
	withoutDefinitionV2.Stages = nil
	withoutDefinitionV2.ApplicableAges = nil
	withoutDefinitionV2.Reporters = nil
	withoutDefinitionV2.Tags = nil
	payloadWithoutDefinitionV2 := *payloadPublished
	payloadWithoutDefinitionV2.DefinitionV2 = nil
	if !reflect.DeepEqual(&withoutDefinitionV2, &payloadWithoutDefinitionV2) {
		t.Fatalf("assessment snapshot mismatch\n got: %#v\nwant: %#v", &withoutDefinitionV2, &payloadWithoutDefinitionV2)
	}
}

func newPublishedScaleAssessmentModelForEquivalence(t *testing.T, snapshot *scalesnapshot.ScaleSnapshot) *domain.AssessmentModel {
	t.Helper()

	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: snapshot.Code, Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault, Title: snapshot.Title,
		Description: "scale definition", Category: "adhd", Stages: []string{"deep_assessment"}, ApplicableAges: []string{"school_child"}, Reporters: []string{"parent"}, Tags: []string{"screening"}, Now: now,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if err := model.BindQuestionnaire(domain.QuestionnaireBinding{QuestionnaireCode: snapshot.QuestionnaireCode, QuestionnaireVersion: snapshot.QuestionnaireVersion}, now); err != nil {
		t.Fatalf("BindQuestionnaire: %v", err)
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Marshal scale payload: %v", err)
	}
	if err := model.UpdateDefinitionWithV2(domain.DefinitionPayload{Format: domain.PayloadFormatAssessmentScaleV1, Data: payload}, scalesnapshot.DefinitionFromScaleSnapshot(snapshot), now); err != nil {
		t.Fatalf("UpdateDefinitionWithV2: %v", err)
	}
	if err := model.MarkPublished(now); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}
	return model
}

func newPublishedScaleSnapshotForEquivalence() *scalesnapshot.ScaleSnapshot {
	maxScore := 10.0
	return &scalesnapshot.ScaleSnapshot{
		Code:                 "SCALE_A",
		ScaleVersion:         "1.0.0",
		Title:                "Scale A",
		QuestionnaireCode:    "Q1",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Factors: []scalesnapshot.FactorSnapshot{{
			Code:            "TOTAL",
			Title:           "Total",
			IsTotalScore:    true,
			MaxScore:        &maxScore,
			ScoringStrategy: "sum",
			InterpretRules: []scalesnapshot.InterpretRuleSnapshot{{
				Min: 0, Max: 10, RiskLevel: "low", Conclusion: "low", Suggestion: "watch",
			}},
		}, {
			Code:            "F1",
			Title:           "Factor One",
			QuestionCodes:   []string{"Q1", "Q2"},
			ScoringStrategy: "cnt",
			ScoringParams:   scalesnapshot.ScoringParamsSnapshot{CntOptionContents: []string{"yes", "often"}},
			MaxScore:        &maxScore,
			InterpretRules: []scalesnapshot.InterpretRuleSnapshot{
				{Min: 0, Max: 5, RiskLevel: "low", Conclusion: "low", Suggestion: "watch"},
				{Min: 5, Max: 10, RiskLevel: "high", Conclusion: "high", Suggestion: "act"},
			},
		}, {
			Code:            "F2",
			Title:           "Factor Two",
			QuestionCodes:   []string{"Q3", "Q4"},
			ScoringStrategy: "avg",
			MaxScore:        &maxScore,
			InterpretRules: []scalesnapshot.InterpretRuleSnapshot{
				{Min: 0, Max: 5, RiskLevel: "none", Conclusion: "none", Suggestion: "keep"},
				{Min: 5, Max: 10, RiskLevel: "medium", Conclusion: "medium", Suggestion: "review"},
			},
		}},
	}
}
