package behavioral_rating_test

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralratingdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating"
)

func TestBuildPublishedSnapshotRequiresNormingPrimaryDimension(t *testing.T) {
	t.Parallel()

	model := &domain.AssessmentModel{
		Kind:      domain.KindBehavioralRating,
		Algorithm: domain.AlgorithmBrief2,
		Code:      "brief2-demo",
		Version:   1,
		Title:     "Brief-2 Demo",
		Definition: domain.DefinitionPayload{
			Data: []byte(`{"dimensions":[{"code":"inhibit"}],"brief2":{"form_variant":"parent"}}`),
		},
	}

	_, err := behavioralratingdomain.BuildPublishedSnapshot(model)
	if err == nil {
		t.Fatal("BuildPublishedSnapshot: want error when primary_dimension_code missing")
	}
}

func TestBuildPublishedSnapshotPreservesConfiguredPrimaryDimension(t *testing.T) {
	t.Parallel()

	model := &domain.AssessmentModel{
		Kind:      domain.KindBehavioralRating,
		Algorithm: domain.AlgorithmBrief2,
		Code:      "brief2-demo",
		Version:   1,
		Title:     "Brief-2 Demo",
		Definition: domain.DefinitionPayload{
			Data: []byte(`{"dimensions":[],"brief2":{"primary_dimension_code":"bri"}}`),
		},
	}

	snapshot, err := behavioralratingdomain.BuildPublishedSnapshot(model)
	if err != nil {
		t.Fatalf("BuildPublishedSnapshot: %v", err)
	}
	var body struct {
		Brief2 struct {
			PrimaryDimensionCode string `json:"primary_dimension_code"`
		} `json:"brief2"`
	}
	if err := json.Unmarshal(snapshot.Payload, &body); err != nil {
		t.Fatalf("decode published payload: %v", err)
	}
	if body.Brief2.PrimaryDimensionCode != "bri" {
		t.Fatalf("primary_dimension_code = %q, want bri", body.Brief2.PrimaryDimensionCode)
	}
}
