package ruleset

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestBuildScalePublishedSnapshotRoundTrip(t *testing.T) {
	model := &scalesnapshot.ScaleSnapshot{
		Code:                 "SCL-001",
		ScaleVersion:         "1.0.0",
		Title:                "Demo Scale",
		QuestionnaireCode:    "QNR-001",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Factors: []scalesnapshot.FactorSnapshot{
			{Code: "total", Title: "Total", IsTotalScore: true},
		},
	}
	snapshot, err := aminfra.BuildScalePublishedSnapshot(model)
	if err != nil {
		t.Fatalf("BuildScalePublishedSnapshot: %v", err)
	}
	if snapshot.Kind != domain.KindScale {
		t.Fatalf("kind = %s", snapshot.Kind)
	}
	if snapshot.PayloadFormat != domain.PayloadFormatAssessmentScaleV1 {
		t.Fatalf("payload format = %s", snapshot.PayloadFormat)
	}
	if snapshot.DecisionKind != domain.DecisionKindScoreRange {
		t.Fatalf("decision = %s", snapshot.DecisionKind)
	}
}

func TestScalePublisherUpsertsPublishedScale(t *testing.T) {
	writer := &stubRuleWriter{}
	publisher := NewScalePublisher(writer)
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode("SCL-001"),
		"Demo",
		scaledefinition.WithQuestionnaire(meta.NewCode("QNR-001"), "1.0.0"),
		scaledefinition.WithScaleVersion("1.0.0"),
		scaledefinition.WithStatus(scaledefinition.StatusPublished),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale: %v", err)
	}
	if err := publisher.PublishPublishedScale(t.Context(), scale); err != nil {
		t.Fatalf("PublishPublishedScale: %v", err)
	}
	if writer.last == nil {
		t.Fatal("expected upsert")
	}
	if writer.last.Kind != domain.KindScale {
		t.Fatalf("kind = %s", writer.last.Kind)
	}
}

func TestScalePublisherMatchesAssessmentModelPublishedSnapshot(t *testing.T) {
	writer := &stubRuleWriter{}
	publisher := NewScalePublisher(writer)
	maxScore := 10.0
	factor, err := scaledefinition.NewFactor(
		scaledefinition.NewFactorCode("total"),
		"Total",
		scaledefinition.WithIsTotalScore(true),
		scaledefinition.WithScoringStrategy(scaledefinition.ScoringStrategySum),
		scaledefinition.WithMaxScore(&maxScore),
		scaledefinition.WithInterpretRules([]scaledefinition.InterpretationRule{
			scaledefinition.NewInterpretationRule(
				scaledefinition.NewScoreRange(0, 10),
				scaledefinition.RiskLevelNone,
				"none",
				"keep",
			),
		}),
	)
	if err != nil {
		t.Fatalf("NewFactor: %v", err)
	}
	scale, err := scaledefinition.NewMedicalScale(
		meta.NewCode("SCL-002"),
		"Demo",
		scaledefinition.WithQuestionnaire(meta.NewCode("QNR-002"), "1.0.0"),
		scaledefinition.WithScaleVersion("1.0.0"),
		scaledefinition.WithStatus(scaledefinition.StatusPublished),
		scaledefinition.WithFactors([]*scaledefinition.Factor{factor}),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale: %v", err)
	}
	model, err := legacyadapter.AssessmentModelFromMedicalScale(scale, time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("AssessmentModelFromMedicalScale: %v", err)
	}
	want, err := publishedmodel.BuildAssessmentSnapshot(model)
	if err != nil {
		t.Fatalf("BuildAssessmentSnapshot: %v", err)
	}

	if err := publisher.PublishPublishedScale(t.Context(), scale); err != nil {
		t.Fatalf("PublishPublishedScale: %v", err)
	}
	if writer.last.DefinitionV2 == nil {
		t.Fatal("DefinitionV2 is nil")
	}
	gotWithoutDefinitionV2 := *writer.last
	gotWithoutDefinitionV2.DefinitionV2 = nil
	wantWithoutDefinitionV2 := *want
	wantWithoutDefinitionV2.DefinitionV2 = nil
	if !reflect.DeepEqual(&gotWithoutDefinitionV2, &wantWithoutDefinitionV2) {
		t.Fatalf("published snapshot mismatch\n got: %#v\nwant: %#v", &gotWithoutDefinitionV2, &wantWithoutDefinitionV2)
	}
}

type stubRuleWriter struct {
	last *port.PublishedModel
}

func (s *stubRuleWriter) UpsertPublishedModel(_ context.Context, snapshot *port.PublishedModel) error {
	s.last = snapshot
	return nil
}
