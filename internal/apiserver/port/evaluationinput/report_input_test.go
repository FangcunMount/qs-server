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
	opts := evaluationinput.ReportInputFreezeOptions{Assets: reportAssets(), ModelRef: evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Code: "PHQ9", Version: "v1"}, DecisionKind: modelcatalog.DecisionKindScoreRange, FactorCatalog: []evaluationinput.FactorCatalogEntry{{Code: "TOTAL", IsTotalScore: true}}}
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
	opts := evaluationinput.ReportInputFreezeOptions{Assets: reportAssets(), ModelRef: evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindBehavioralRating, Code: "BRIEF2", Version: "v1"}, DecisionKind: modelcatalog.DecisionKindNormLookup, FactorCatalog: []evaluationinput.FactorCatalogEntry{{Code: "gec"}}, Norming: &evaluationinput.NormingFreeze{NormTables: &calcnorm.NormTables{NormTableVersion: "n1"}}}
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

func TestReportInputRestoresFrozenFactorScoreVisibility(t *testing.T) {
	tests := []struct {
		name       string
		sections   []interpretationassets.ReportSection
		wantCodes  []string
		configured bool
	}{
		{name: "not configured"},
		{
			name: "explicit empty hides every factor",
			sections: []interpretationassets.ReportSection{{
				Code: "factor_scores", Kind: "factor_scores", SourceRefs: []string{},
			}},
			configured: true,
		},
		{
			name: "exact visible factors",
			sections: []interpretationassets.ReportSection{{
				Code: "factor_scores", Kind: "factor_scores", SourceRefs: []string{"visible", "total"},
			}},
			wantCodes: []string{"visible", "total"}, configured: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assets := &interpretationassets.Assets{
				Outcomes:   []interpretationassets.OutcomePresentation{{OutcomeCode: "normal", Summary: "正常"}},
				ReportSpec: interpretationassets.ReportSpec{Sections: tt.sections},
			}
			opts := evaluationinput.ReportInputFreezeOptions{
				Assets: assets,
				ModelRef: evaluationinput.ModelRef{
					Kind: evaluationinput.EvaluationModelKindScale, Code: "SCALE", Version: "v1",
				},
				DecisionKind:    modelcatalog.DecisionKindScoreRange,
				FactorCatalog:   []evaluationinput.FactorCatalogEntry{{Code: "visible"}, {Code: "total", IsTotalScore: true}},
			}
			raw, err := evaluationinput.MarshalReportInput(opts)
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := evaluationinput.SnapshotFromReportInput(raw, opts.ModelRef)
			if err != nil {
				t.Fatal(err)
			}

			gotCodes, configured := evaluationinput.FactorScoreVisibleCodesFromSnapshot(snapshot)
			if configured != tt.configured {
				t.Fatalf("configured = %v, want %v; codes = %#v", configured, tt.configured, gotCodes)
			}
			if len(gotCodes) != len(tt.wantCodes) {
				t.Fatalf("codes = %#v, want %#v", gotCodes, tt.wantCodes)
			}
			for i := range tt.wantCodes {
				if gotCodes[i] != tt.wantCodes[i] {
					t.Fatalf("codes = %#v, want %#v", gotCodes, tt.wantCodes)
				}
			}
		})
	}
}

func TestTraitProfileReportInputAllowsFrozenReportSpecAndFactorCatalogWithoutOutcomeRegistry(t *testing.T) {
	assets := &interpretationassets.Assets{ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{
		Code: "trait_profile", Kind: "trait_profile", AdapterKey: "trait_profile", TemplateID: "enneagram",
	}}}}
	opts := evaluationinput.ReportInputFreezeOptions{
		Assets: assets,
		ModelRef: evaluationinput.ModelRef{
			Kind: evaluationinput.EvaluationModelKindTypology,
			Algorithm: "personality_typology", Code: "ENNEAGRAM_45", Version: "v16",
		},
		DecisionKind:    modelcatalog.DecisionKindTraitProfile,
		FactorCatalog:   []evaluationinput.FactorCatalogEntry{{Code: "type_1", Title: "完美型"}},
		TypologyRouting: &evaluationinput.TypologyRoutingFreeze{
			DecisionKind: "trait_profile", ReportKind: "trait_profile", AdapterKey: "trait_profile", TemplateID: "enneagram",
		},
	}

	raw, err := evaluationinput.MarshalReportInput(opts)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := evaluationinput.SnapshotFromReportInput(raw, opts.ModelRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.FactorCatalog) != 1 || snapshot.FactorCatalog[0].Title != "完美型" {
		t.Fatalf("factor catalog = %#v", snapshot.FactorCatalog)
	}
}

func TestClassifiedTypeReportInputStillRequiresFrozenOutcomePresentation(t *testing.T) {
	assets := &interpretationassets.Assets{ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{
		Code: "personality_type", Kind: "personality_type", AdapterKey: "personality_type", TemplateID: "mbti",
	}}}}
	_, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Assets: assets,
		ModelRef: evaluationinput.ModelRef{
			Kind: evaluationinput.EvaluationModelKindTypology,
			Algorithm: "personality_typology", Code: "MBTI", Version: "v1",
		},
		DecisionKind:    modelcatalog.DecisionKindPoleComposition,
		FactorCatalog:   []evaluationinput.FactorCatalogEntry{{Code: "EI", Title: "外向-内向"}},
		TypologyRouting: &evaluationinput.TypologyRoutingFreeze{
			DecisionKind: "pole_composition", ReportKind: "personality_type", AdapterKey: "personality_type", TemplateID: "mbti",
		},
	})
	if err == nil {
		t.Fatal("classified personality type without frozen outcomes/profiles was accepted")
	}
}
