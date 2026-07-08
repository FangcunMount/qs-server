package norming_test

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
)

func TestRequirePrimaryDimensionCodeForPublish(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"dimensions":[{"code":"inhibit"}],"brief2":{"form_variant":"parent"}}`)
	if _, err := norming.RequirePrimaryDimensionCodeForPublish(payload); err == nil {
		t.Fatal("expected error when primary_dimension_code missing")
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

	snapshot, err := publishing.BuildPublishedSnapshot(model)
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
