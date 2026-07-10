package behavioral_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

func TestPrepareDefinitionForPublishRequiresBrief2PrimaryDimension(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"dimensions":[{"code":"inhibit"}],"brief2":{"form_variant":"parent"}}`)
	if _, err := behavioral.PrepareDefinitionForPublish(payload); err == nil {
		t.Fatal("expected error when primary_dimension_code missing")
	}
}

func TestPrepareDefinitionForPublishPreservesPayloadBytesAndDecisionKind(t *testing.T) {
	t.Parallel()

	payload := []byte("{\n  \"dimensions\": [],\n  \"brief2\": {\"primary_dimension_code\": \"bri\"}\n}\n")
	got, err := behavioral.PrepareDefinitionForPublish(payload)
	if err != nil {
		t.Fatalf("PrepareDefinitionForPublish: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload changed: got %q, want %q", got, payload)
	}
	if got := behavioral.DecisionKindFromDefinitionPayload(payload); got != binding.DecisionKindNormLookup {
		t.Fatalf("brief2 decision kind = %q", got)
	}
	if got := behavioral.DecisionKindFromDefinitionPayload([]byte(`{"dimensions":[]}`)); got != binding.DecisionKindScoreRange {
		t.Fatalf("default decision kind = %q", got)
	}
}

func TestBuildPublishedModelPreservesConfiguredBrief2PrimaryDimension(t *testing.T) {
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

	snapshot, err := publishedmodel.BuildAssessmentSnapshot(model)
	if err != nil {
		t.Fatalf("Build published model: %v", err)
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
