package evaluationinput_test

import (
	"encoding/json"
	"testing"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestMarshalReportInputUsesLegacyShapeWithoutAssets(t *testing.T) {
	t.Parallel()
	payload := evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{Code: "PHQ9"}}
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{Payload: payload})
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
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Payload: payload, Assets: assets,
	})
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

func TestMarshalReportInputV3OmitsPayloadForScale(t *testing.T) {
	t.Parallel()
	max := 27.0
	assets := &interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{
		OutcomeCode: "low", Summary: "偏低",
	}}}
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Payload: evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{Code: "PHQ9"}},
		Assets:  assets,
		ModelRef: evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Code: "PHQ9", Version: "v1"},
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		FactorCatalog:   []evaluationinput.FactorCatalogEntry{{Code: "TOTAL", Title: "总分", MaxScore: &max, IsTotalScore: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var peek struct {
		SchemaVersion *uint           `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &peek); err != nil {
		t.Fatal(err)
	}
	if peek.SchemaVersion == nil || *peek.SchemaVersion != evaluationinput.ReportInputSchemaV3 {
		t.Fatalf("schema = %v, want v3", peek.SchemaVersion)
	}
	if len(peek.Payload) != 0 {
		t.Fatalf("v3 must omit payload, got %s", peek.Payload)
	}
	snapshot, err := evaluationinput.SnapshotFromReportInput(raw, evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Code: "PHQ9"})
	if err != nil {
		t.Fatal(err)
	}
	scale, ok := evaluationinput.ScalePayload(snapshot)
	if !ok || scale == nil || len(scale.Factors) != 1 || scale.Factors[0].Title != "总分" {
		t.Fatalf("catalog scale = %#v ok=%v", scale, ok)
	}
}

func TestMarshalReportInputV3OmitsPayloadForTypology(t *testing.T) {
	t.Parallel()
	assets := &interpretationassets.Assets{Profiles: []interpretationassets.TypeProfilePresentation{{
		OutcomeCode: "INTJ", Commentary: "建筑师", Strengths: []string{"规划"},
	}}}
	src := typology.Source{Attribution: "test", License: "CC"}
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Payload: evaluationinput.TypologyModelPayload{Payload: &typology.Payload{Code: "MBTI", Outcomes: []typology.Outcome{{Code: "INTJ"}}}},
		Assets:  assets,
		ModelRef: evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindTypology, Code: "MBTI", Version: "v1", Algorithm: "mbti"},
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
		TypologySource:  &src,
	})
	if err != nil {
		t.Fatal(err)
	}
	var peek struct {
		SchemaVersion *uint           `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &peek); err != nil {
		t.Fatal(err)
	}
	if peek.SchemaVersion == nil || *peek.SchemaVersion != evaluationinput.ReportInputSchemaV3 {
		t.Fatalf("schema = %v, want v3", peek.SchemaVersion)
	}
	if len(peek.Payload) != 0 {
		t.Fatalf("v3 must omit payload, got %s", peek.Payload)
	}
	snapshot, err := evaluationinput.SnapshotFromReportInput(raw, evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindTypology, Code: "MBTI", Algorithm: "mbti"})
	if err != nil {
		t.Fatal(err)
	}
	tp, ok := evaluationinput.TypologyPayload(snapshot)
	if !ok {
		t.Fatalf("typology payload missing")
	}
	outcome, found := tp.FindOutcome("INTJ")
	if !found || outcome.Commentary != "建筑师" || tp.Source.Attribution != "test" {
		t.Fatalf("typology payload = %#v outcome=%#v", tp, outcome)
	}
}

func TestMarshalReportInputV3OmitsPayloadForBehavioralNorm(t *testing.T) {
	t.Parallel()
	max := 63.0
	tables := &calcnorm.NormTables{TScoreRules: []calcnorm.TScoreInterpretRule{{
		FactorCode: "gec", Ranges: []calcnorm.TScoreRange{{MinT: 60, MaxT: 100, Level: "elevated", Conclusion: "偏高"}},
	}}}
	assets := &interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "elevated", Summary: "偏高"}}}
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Payload: evaluationinput.BehavioralRatingModelPayload{Snapshot: &behavioralsnapshot.Snapshot{Code: "BRIEF2"}},
		Assets:  assets,
		ModelRef: evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindBehavioralRating, Code: "BRIEF2", Version: "v1"},
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm,
		FactorCatalog:   []evaluationinput.FactorCatalogEntry{{Code: "gec", Title: "GEC", MaxScore: &max}},
		Norming:         &evaluationinput.NormingFreeze{NormTables: tables},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := evaluationinput.SnapshotFromReportInput(raw, evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindBehavioralRating, Code: "BRIEF2"})
	if err != nil {
		t.Fatal(err)
	}
	br, ok := evaluationinput.BehavioralRatingPayload(snapshot)
	if !ok || br.Snapshot.Norming.NormTablesOrNil() == nil || len(br.Snapshot.Factors) != 1 {
		t.Fatalf("behavioral payload = %#v ok=%v", br.Snapshot, ok)
	}
}
