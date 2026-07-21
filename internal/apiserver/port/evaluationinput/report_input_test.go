package evaluationinput_test

import (
	"encoding/json"
	"testing"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func reportAssets() *interpretationassets.Assets {
	return &interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "low", Summary: "偏低"}}}
}

func TestMarshalReportInputEmitsOnlyCurrentSchemaWithoutPayload(t *testing.T) {
	opts := evaluationinput.ReportInputFreezeOptions{Assets: reportAssets(), ModelRef: evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Code: "PHQ9", Version: "v1"}, AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring, FactorCatalog: []evaluationinput.FactorCatalogEntry{{Code: "TOTAL", IsTotalScore: true}}}
	raw, err := evaluationinput.MarshalReportInput(opts)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["schema_version"] != float64(evaluationinput.CurrentReportInputSchema) || envelope["payload"] != nil {
		t.Fatalf("envelope = %s", raw)
	}
	if _, err := evaluationinput.SnapshotFromReportInput(raw, opts.ModelRef); err != nil {
		t.Fatal(err)
	}
}

func TestReportInputRejectsNonCurrentSchemas(t *testing.T) {
	model := evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Code: "PHQ9", Version: "v1"}
	for _, raw := range [][]byte{[]byte(`{"scale":{"code":"PHQ9"}}`), []byte(`{"schema_version":2}`), []byte(`{"schema_version":4}`)} {
		if _, err := evaluationinput.SnapshotFromReportInput(raw, model); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestBehavioralReportInputRequiresAndRestoresNorming(t *testing.T) {
	opts := evaluationinput.ReportInputFreezeOptions{Assets: reportAssets(), ModelRef: evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindBehavioralRating, Code: "BRIEF2", Version: "v1"}, AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm, FactorCatalog: []evaluationinput.FactorCatalogEntry{{Code: "gec"}}, Norming: &evaluationinput.NormingFreeze{NormTables: &calcnorm.NormTables{NormTableVersion: "n1"}}}
	raw, err := evaluationinput.MarshalReportInput(opts)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := evaluationinput.SnapshotFromReportInput(raw, opts.ModelRef)
	if err != nil {
		t.Fatal(err)
	}
	behavioral, ok := evaluationinput.BehavioralRatingPayload(snapshot)
	if !ok || behavioral.Snapshot.Norming.NormTablesOrNil() == nil {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}
