package behavioral_rating_test

import (
	"encoding/json"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralratingdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating"
)

func TestBuildPublishedSnapshotDefaultsBrief2PrimaryDimension(t *testing.T) {
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

	snapshot, err := behavioralratingdomain.BuildPublishedSnapshot(model)
	if err != nil {
		t.Fatalf("BuildPublishedSnapshot: %v", err)
	}
	var body struct {
		Dimensions []struct {
			Code string `json:"code"`
		} `json:"dimensions"`
		Brief2 struct {
			FormVariant          string `json:"form_variant"`
			PrimaryDimensionCode string `json:"primary_dimension_code"`
		} `json:"brief2"`
	}
	if err := json.Unmarshal(snapshot.Payload, &body); err != nil {
		t.Fatalf("decode published payload: %v", err)
	}
	if len(body.Dimensions) != 1 || body.Dimensions[0].Code != "inhibit" {
		t.Fatalf("dimensions = %#v, want original dimensions preserved", body.Dimensions)
	}
	if body.Brief2.FormVariant != "parent" {
		t.Fatalf("brief2 form_variant = %q, want parent", body.Brief2.FormVariant)
	}
	if body.Brief2.PrimaryDimensionCode != "gec" {
		t.Fatalf("primary_dimension_code = %q, want gec", body.Brief2.PrimaryDimensionCode)
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
