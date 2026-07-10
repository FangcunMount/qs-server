package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func TestScaleCompatibilityProjectorUsesDefinitionV2InsteadOfPayload(t *testing.T) {
	t.Parallel()
	model := &modelcatalogport.PublishedModel{
		Kind: domain.KindScale, Code: "S-1", Version: "v2", Title: "Scale", Status: "published", Payload: []byte(`not-json`),
		DefinitionV2: scalepayload.DefinitionFromScaleSnapshot(&scalepayload.ScaleSnapshot{Factors: []scalepayload.FactorSnapshot{{Code: "total", Title: "Total", IsTotalScore: true}}}),
	}
	modelcatalogport.SetLegacyScaleBinding(model, modelcatalogport.LegacyScaleBinding{MedicalScaleID: 8, ScaleVersion: "1.0.0"})
	result, err := (ScaleCompatibilityProjector{}).ProjectPublished(model)
	if err != nil {
		t.Fatalf("ProjectPublished() error = %v", err)
	}
	if result.ScaleVersion != "1.0.0" || len(result.Factors) != 1 || result.Factors[0].Code != "total" {
		t.Fatalf("ProjectPublished() = %#v", result)
	}
}
