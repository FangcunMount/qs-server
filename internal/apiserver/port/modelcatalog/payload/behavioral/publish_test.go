package behavioral_test

import (
	"encoding/json"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/publishedmodel"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestBuildPublishedModelUsesDefinitionV2Decision(t *testing.T) {
	t.Parallel()

	model := &domain.AssessmentModel{
		Kind:       domain.KindBehavioralRating,
		Algorithm:  domain.AlgorithmBrief2,
		Code:       "brief2-v2",
		Version:    1,
		Title:      "Brief-2 V2",
		Definition: domain.DefinitionPayload{Data: []byte(`{"dimensions":[]}`)},
		DefinitionV2: &domain.Definition{Conclusions: []domain.Conclusion{
			domain.NormConclusion{FactorCode: "bri", Primary: true},
		}},
	}

	snapshot, err := publishedmodel.BuildAssessmentSnapshot(model)
	if err != nil {
		t.Fatalf("BuildAssessmentSnapshot: %v", err)
	}
	if snapshot.DecisionKind != domain.DecisionKindNormLookup {
		t.Fatalf("decision kind = %q, want norm_lookup", snapshot.DecisionKind)
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
		DefinitionV2: &domain.Definition{Conclusions: []domain.Conclusion{
			domain.NormConclusion{FactorCode: "bri", Primary: true},
		}},
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
