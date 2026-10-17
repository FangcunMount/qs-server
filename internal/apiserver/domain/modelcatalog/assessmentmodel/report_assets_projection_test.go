package assessmentmodel

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

func TestNewAndDefinitionEditAssignCurrentLegacyAssetVersion(t *testing.T) {
	model, err := New(NewInput{Code: "M", Title: "model", Kind: binding.KindScale})
	if err != nil {
		t.Fatal(err)
	}
	if model.ReportAssetsProjectionVersion != interpretationassets.ProjectionLegacyV1 {
		t.Fatal("new draft marker absent")
	}
	model.ReportAssetsProjectionVersion = 0 // Historical head read.
	if err := model.UpdateBasicInfo("metadata", "", model.Algorithm, "", nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if model.ReportAssetsProjectionVersion != 0 {
		t.Fatal("metadata edit inferred an asset version")
	}
	if err := model.UpdateDefinition(&definition.Definition{}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if model.ReportAssetsProjectionVersion != interpretationassets.ProjectionLegacyV1 {
		t.Fatal("new Definition revision not stamped")
	}
}
