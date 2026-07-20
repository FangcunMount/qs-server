package evaluationinput

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

func TestNewBehavioralRatingModelSnapshotPreservesExplicitAlgorithm(t *testing.T) {
	snapshot := &behavioralsnapshot.Snapshot{
		Code:    "bJFKi3",
		Version: "v11",
		Norming: &behavioralsnapshot.NormingProfile{},
	}

	model := NewBehavioralRatingModelSnapshot(snapshot, modelcatalog.AlgorithmSPMSensory)
	if model == nil {
		t.Fatal("model snapshot is nil")
	}
	if model.Algorithm != string(modelcatalog.AlgorithmSPMSensory) {
		t.Fatalf("algorithm = %s, want %s", model.Algorithm, modelcatalog.AlgorithmSPMSensory)
	}
}

func TestNewBehavioralRatingModelSnapshotKeepsLegacyNormingFallback(t *testing.T) {
	snapshot := &behavioralsnapshot.Snapshot{Norming: &behavioralsnapshot.NormingProfile{}}

	model := NewBehavioralRatingModelSnapshot(snapshot, "")
	if model == nil {
		t.Fatal("model snapshot is nil")
	}
	if model.Algorithm != string(modelcatalog.AlgorithmBrief2) {
		t.Fatalf("algorithm = %s, want %s", model.Algorithm, modelcatalog.AlgorithmBrief2)
	}
}

func TestNewBehavioralRatingModelSnapshotEmptyAlgorithmUsesBrief2(t *testing.T) {
	t.Parallel()
	model := NewBehavioralRatingModelSnapshot(&behavioralsnapshot.Snapshot{Code: "LEGACY"}, "")
	if model == nil {
		t.Fatal("model snapshot is nil")
	}
	if model.Algorithm != string(modelcatalog.AlgorithmBrief2) {
		t.Fatalf("algorithm = %s, want %s", model.Algorithm, modelcatalog.AlgorithmBrief2)
	}
	if !modelcatalog.IsCanonicalPublishAlgorithm(modelcatalog.KindBehavioralRating, modelcatalog.Algorithm(model.Algorithm)) {
		t.Fatal("empty-algorithm fill should be canonical brief2")
	}
}
