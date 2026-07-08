package outcome

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestPublishedSnapshotFromInputPreservesExplicitProductChannel(t *testing.T) {
	t.Parallel()

	snapshot, ok := PublishedSnapshotFromInput(&evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:           evaluationinput.EvaluationModelKindBehavioralRating,
			Algorithm:      string(modelcatalog.AlgorithmBehavioralRatingDefault),
			ProductChannel: string(modelcatalog.ProductChannel("screening")),
			Code:           "BR-001",
			Version:        "1.0.0",
			Title:          "筛查行为评分",
		},
	})
	if !ok {
		t.Fatal("PublishedSnapshotFromInput returned false")
	}
	if snapshot.Model.ProductChannel != modelcatalog.ProductChannel("screening") {
		t.Fatalf("product channel = %s, want screening", snapshot.Model.ProductChannel)
	}
}

func TestPublishedSnapshotFromInputDefaultsMissingProductChannel(t *testing.T) {
	t.Parallel()

	snapshot, ok := PublishedSnapshotFromInput(&evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind:      evaluationinput.EvaluationModelKindScale,
			Algorithm: string(modelcatalog.AlgorithmScaleDefault),
			Code:      "PHQ9",
			Version:   "1.0.0",
			Title:     "PHQ-9",
		},
	})
	if !ok {
		t.Fatal("PublishedSnapshotFromInput returned false")
	}
	if snapshot.Model.ProductChannel != modelcatalog.ProductChannelMedicalScale {
		t.Fatalf("product channel = %s, want medical_scale", snapshot.Model.ProductChannel)
	}
}
