package evaluationinput

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

// Only model rules are reproduced here; no answers or live identifiers are used.
func mbtiFreezeFixture() (*InputSnapshot, ModelRef) {
	ref := ModelRef{Kind: EvaluationModelKindTypology, Algorithm: "personality_typology", Code: "MBTI_OEJTS", Version: "v64-report-202608-v1"}
	def := &definition.Definition{Measure: definition.MeasureSpec{FactorGraph: factor.FactorGraph{Roots: []string{"EI", "SN", "TF", "JP"}}}, ReportMap: definition.ReportMap{Sections: []definition.ReportSection{{Kind: "personality_type", AdapterKey: "personality_type", TemplateID: "mbti", TemplateVersion: "2026-08-v1"}}}}
	td := conclusion.TypeDecision{Kind: modelcatalog.DecisionKindPoleComposition}
	names := []string{"外向 / 内向", "实感 / 直觉", "思考 / 情感", "判断 / 感知"}
	left := []string{"I", "S", "F", "J"}
	right := []string{"E", "N", "T", "P"}
	for i, code := range def.Measure.FactorGraph.Roots {
		def.Measure.Factors = append(def.Measure.Factors, factor.Factor{Code: code, Title: names[i]})
		scoring := factor.Scoring{FactorCode: code, Strategy: factor.ScoringStrategySum}
		for n := 0; n < 8; n++ {
			scoring.Sources = append(scoring.Sources, factor.ScoringSource{Kind: factor.ScoringSourceQuestion, Code: code, ScoringMode: factor.QuestionScoringModeQuestionScore, Sign: 1, Weight: 1})
		}
		def.Measure.Scoring = append(def.Measure.Scoring, scoring)
		td.Poles = append(td.Poles, conclusion.TypePole{FactorCode: code, LeftPole: left[i], RightPole: right[i], Threshold: 24})
	}
	def.Conclusions = []conclusion.Conclusion{conclusion.TypeConclusion{Decision: td}}
	return &InputSnapshot{Model: &ModelSnapshot{Kind: ref.Kind, Algorithm: ref.Algorithm, Code: ref.Code, Version: ref.Version, DecisionKind: "pole_composition"}, DefinitionV2: def, InterpretationAssets: &interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "ISFJ", Title: "守卫者"}}}}, ref
}

func TestMBTIPolesFreezeExactModelAndReplayWithoutRuntimePayload(t *testing.T) {
	in, ref := mbtiFreezeFixture()
	opts := BuildFreezeOptionsFromSnapshot(in, ref, modelcatalog.DecisionKindPoleComposition)
	raw, err := MarshalReportInput(opts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "DefinitionV2") || strings.Contains(string(raw), "Contributions") || strings.Contains(string(raw), "AnswerSheet") {
		t.Fatal("executable/answer payload retained")
	}
	restored, err := SnapshotFromReportInput(raw, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.MBTIPoles, opts.MBTIPoles) {
		t.Fatal("pole catalog did not round trip")
	}
	for _, a := range restored.MBTIPoles.Axes {
		if a.MinScore != 8 || a.MaxScore != 40 || a.Threshold != 24 {
			t.Fatalf("wrong frozen boundaries: %+v", a)
		}
	}
	// Changes to a currently loaded definition cannot change the stored projection.
	in.DefinitionV2.Measure.Factors[0].Title = "new head"
	again, err := SnapshotFromReportInput(raw, ref)
	if err != nil || again.MBTIPoles.Axes[0].Name != "外向 / 内向" {
		t.Fatal("frozen name changed", err)
	}
}

func TestMBTIPolesRejectCorruptOrCrossModelEnvelope(t *testing.T) {
	in, ref := mbtiFreezeFixture()
	opts := BuildFreezeOptionsFromSnapshot(in, ref, modelcatalog.DecisionKindPoleComposition)
	raw, err := MarshalReportInput(opts)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(map[string]any){
		"missing minimum": func(v map[string]any) {
			delete(v["mbti_poles"].(map[string]any)["axes"].([]any)[0].(map[string]any), "min_score")
		},
		"null minimum": func(v map[string]any) {
			v["mbti_poles"].(map[string]any)["axes"].([]any)[0].(map[string]any)["min_score"] = nil
		},
		"version":      func(v map[string]any) { v["mbti_poles"].(map[string]any)["schema_version"] = "unknown" },
		"missing axis": func(v map[string]any) { p := v["mbti_poles"].(map[string]any); p["axes"] = p["axes"].([]any)[:3] },
		"wrong poles": func(v map[string]any) {
			v["mbti_poles"].(map[string]any)["axes"].([]any)[0].(map[string]any)["left_pole"] = "E"
		},
		"cross model": func(v map[string]any) { v["model_ref"].(map[string]any)["Code"] = "OTHER" },
		"threshold": func(v map[string]any) {
			v["mbti_poles"].(map[string]any)["axes"].([]any)[0].(map[string]any)["threshold"] = 41
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			var v map[string]any
			if err := json.Unmarshal(raw, &v); err != nil {
				t.Fatal(err)
			}
			change(v)
			bad, _ := json.Marshal(v)
			if _, err := SnapshotFromReportInput(bad, ModelRef{}); err == nil {
				t.Fatal("accepted invalid facts")
			}
		})
	}
}

func TestMBTIPolesLegacyAbsentIsNotBackfilledButNewWritesRequireIt(t *testing.T) {
	in, ref := mbtiFreezeFixture()
	opts := BuildFreezeOptionsFromSnapshot(in, ref, modelcatalog.DecisionKindPoleComposition)
	raw, err := MarshalReportInput(opts)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	delete(envelope, "mbti_poles")
	legacy, _ := json.Marshal(envelope)
	restored, err := SnapshotFromReportInput(legacy, ref)
	if err != nil || restored.MBTIPoles != nil {
		t.Fatal("legacy synthesized", err)
	}
	opts.MBTIPoles = nil
	if _, err := MarshalReportInput(opts); err == nil {
		t.Fatal("new write lost mandatory poles")
	}
	in.DefinitionV2 = nil
	opts = BuildFreezeOptionsFromSnapshot(in, ref, modelcatalog.DecisionKindPoleComposition)
	if _, err := MarshalReportInput(opts); err == nil {
		t.Fatal("missing definition accepted")
	}
}

func TestMBTIPolesRejectMismatchedFrozenSource(t *testing.T) {
	in, ref := mbtiFreezeFixture()
	raw, err := MarshalReportInput(BuildFreezeOptionsFromSnapshot(in, ref, modelcatalog.DecisionKindPoleComposition))
	if err != nil {
		t.Fatal(err)
	}
	other := ref
	other.Algorithm = "other"
	if _, err := SnapshotFromReportInput(raw, other); err == nil {
		t.Fatal("accepted a different outcome algorithm")
	}
	in.Model.Version = "current-head"
	if _, err := MarshalReportInput(BuildFreezeOptionsFromSnapshot(in, ref, modelcatalog.DecisionKindPoleComposition)); err == nil {
		t.Fatal("accepted a different source model version")
	}
}
