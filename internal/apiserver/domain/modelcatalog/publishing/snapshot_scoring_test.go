package publishing_test

import (
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
)

func TestBuildScoringPublishedSnapshotFromAssessmentModel(t *testing.T) {
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Kind:      binding.KindScale,
		Code:      "PHQ9",
		Title:     "PHQ-9",
		Algorithm: binding.AlgorithmScaleDefault,
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	if err := model.UpdateDefinition(publishing.DefinitionPayload{Data: []byte(`{"code":"PHQ9","status":"published"}`)}, time.Now()); err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	snapshot, err := publishing.BuildPublishedSnapshot(model)
	if err != nil {
		t.Fatalf("BuildPublishedSnapshot: %v", err)
	}
	if snapshot.PayloadFormat != publishing.PayloadFormatAssessmentScaleV1 {
		t.Fatalf("payload format = %s", snapshot.PayloadFormat)
	}
	if snapshot.Model.Kind != binding.KindScale || snapshot.Decision.Kind != binding.DecisionKindScoreRange {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestBuildScoringPublishedSnapshotFromScale(t *testing.T) {
	scale := &scalesnapshot.ScaleSnapshot{
		Code:                 "SCL-001",
		ScaleVersion:         "1.0.0",
		Title:                "Demo Scale",
		QuestionnaireCode:    "QNR-001",
		QuestionnaireVersion: "1.0.0",
		Status:               "published",
	}
	snapshot, err := publishing.BuildScoringPublishedSnapshotFromScale(scale)
	if err != nil {
		t.Fatalf("BuildScoringPublishedSnapshotFromScale: %v", err)
	}
	if snapshot.PayloadFormat != publishing.PayloadFormatAssessmentScaleV1 {
		t.Fatalf("payload format = %s", snapshot.PayloadFormat)
	}
	if snapshot.Model.Code != scale.Code || snapshot.Binding.QuestionnaireCode != scale.QuestionnaireCode {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestBuildPublishedSnapshotCoversRuntimeExecutableKinds(t *testing.T) {
	t.Parallel()

	covered := map[binding.Kind]struct{}{
		binding.KindScale:            {},
		binding.KindTypology:         {},
		binding.KindBehavioralRating: {},
		binding.KindCognitive:        {},
	}
	for _, kind := range binding.RuntimeExecutableKinds() {
		if _, ok := covered[kind]; !ok {
			t.Fatalf("BuildPublishedSnapshot missing branch for runtime executable kind %q", kind)
		}
	}
}
