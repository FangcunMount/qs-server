package ruleset

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
	aminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
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

type stubRuleWriter struct {
	last *port.PublishedModel
}

func (s *stubRuleWriter) UpsertPublishedModel(_ context.Context, snapshot *port.PublishedModel) error {
	s.last = snapshot
	return nil
}
