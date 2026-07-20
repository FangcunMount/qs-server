package evaluationinput_test

import (
	"encoding/json"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func TestMarshalReportInputUsesLegacyShapeWithoutAssets(t *testing.T) {
	t.Parallel()
	payload := evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{Code: "PHQ9"}}
	raw, err := evaluationinput.MarshalReportInput(payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	var peek struct {
		SchemaVersion *uint `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &peek); err != nil {
		t.Fatal(err)
	}
	if peek.SchemaVersion != nil {
		t.Fatalf("legacy report input must not set schema_version: %s", raw)
	}
}

func TestMarshalReportInputV2FreezesInterpretationAssets(t *testing.T) {
	t.Parallel()
	payload := evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{Code: "PHQ9"}}
	assets := &interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{
		OutcomeCode: "low", Summary: "偏低",
	}}}
	raw, err := evaluationinput.MarshalReportInput(payload, assets)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := evaluationinput.SnapshotFromReportInput(raw, evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Code: "PHQ9"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.InterpretationAssets == nil {
		t.Fatal("expected frozen interpretation assets")
	}
	got, ok := snapshot.InterpretationAssets.FindOutcome("low")
	if !ok || got.Summary != "偏低" {
		t.Fatalf("frozen assets = %#v ok=%v", got, ok)
	}
}
