package ruleset

import (
	"context"
	"testing"

	domscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/authoring/scale"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
	rulesetscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset/codec"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestScaleRuleSetSnapshotRoundTrip(t *testing.T) {
	model := &rulesetscale.ScaleSnapshot{
		Code:                 "SCL-001",
		ScaleVersion:         "1.0.0",
		Title:                "Demo Scale",
		QuestionnaireCode:    "QNR-001",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
		Factors: []rulesetscale.FactorSnapshot{
			{Code: "total", Title: "Total", IsTotalScore: true},
		},
	}
	snapshot, err := ScaleRuleSetSnapshot(model)
	if err != nil {
		t.Fatalf("ScaleRuleSetSnapshot: %v", err)
	}
	if snapshot.Definition.Kind != domain.RuleSetKindScale {
		t.Fatalf("kind = %s", snapshot.Definition.Kind)
	}
	if snapshot.DecisionKind != domain.DecisionKindScoreRangeInterpretation {
		t.Fatalf("decision = %s", snapshot.DecisionKind)
	}
	got, err := codec.DecodeScale(snapshot)
	if err != nil {
		t.Fatalf("DecodeScale: %v", err)
	}
	if got.Code != model.Code {
		t.Fatalf("code = %s", got.Code)
	}
}

func TestScaleRuleSetPublisherUpsertsPublishedScale(t *testing.T) {
	writer := &stubRuleWriter{}
	publisher := NewScaleRuleSetPublisher(writer)
	scale, err := domscale.NewMedicalScale(
		meta.NewCode("SCL-001"),
		"Demo",
		domscale.WithQuestionnaire(meta.NewCode("QNR-001"), "1.0.0"),
		domscale.WithScaleVersion("1.0.0"),
		domscale.WithStatus(domscale.StatusPublished),
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
	if writer.last.Definition.Kind != domain.RuleSetKindScale {
		t.Fatalf("kind = %s", writer.last.Definition.Kind)
	}
}

type stubRuleWriter struct {
	last *domain.RuleSetSnapshot
}

func (s *stubRuleWriter) UpsertPublished(_ context.Context, snapshot *domain.RuleSetSnapshot) error {
	s.last = snapshot
	return nil
}
