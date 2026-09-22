package interpretation

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	inputadapter "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
)

// Replay the anonymized baseline through the public input, builder and storage
// boundaries. Identifiers are synthetic; no live subject or raw answers are used.
func mbtiBaselineArtifact(t *testing.T) *report.InterpretReport {
	t.Helper()
	codes := []string{"EI", "SN", "TF", "JP"}
	left, right := []string{"I", "S", "F", "J"}, []string{"E", "N", "T", "P"}
	strengths, scores := []float64{56.25, 6.25, 25, 75}, []float64{15, 23, 20, 12}
	poles := &evaluationinput.MBTIPoleCatalog{SchemaVersion: evaluationinput.MBTIPoleCatalogSchema}
	execution := evaluationfact.Execution{
		Primary: &evaluationfact.ScoreValue{Kind: evaluationfact.ScoreKindMatchPercent, Value: 40.625},
	}
	for i, code := range codes {
		poles.Axes = append(poles.Axes, evaluationinput.MBTIPoleAxis{Code: code, Name: "冻结轴 " + code, LeftPole: left[i], RightPole: right[i], MinScore: 8, MaxScore: 40, Threshold: 24})
		execution.Dimensions = append(execution.Dimensions, evaluationfact.DimensionResult{Code: code, Score: &evaluationfact.ScoreValue{Kind: evaluationfact.ScoreKindRawTotal, Value: scores[i]}, Preference: left[i], Strength: &strengths[i]})
	}
	raw, err := json.Marshal(execution)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	payload["Detail"] = map[string]any{"Payload": map[string]any{"type_code": "ISFJ", "match_percent": 40.625}}
	raw, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		ModelRef:     evaluationinput.ModelRef{Kind: "typology", Algorithm: "personality_typology", Code: "MBTI_OEJTS", Version: "v64-report-202608-v1"},
		DecisionKind: modelcatalog.DecisionKindPoleComposition, MBTIPoles: poles,
		TypologyRouting: &evaluationinput.TypologyRoutingFreeze{DecisionKind: "pole_composition", ReportKind: "personality_type", AdapterKey: "personality_type", TemplateID: "mbti", TemplateVersion: "2026-08-v1"},
		Assets: &interpretationassets.Assets{
			Outcomes:   []interpretationassets.OutcomePresentation{{OutcomeCode: "ISFJ", Title: "守卫者", Summary: "冻结摘要"}},
			ReportSpec: interpretationassets.ReportSpec{Sections: []interpretationassets.ReportSection{{Code: "personality", Kind: "personality_type", AdapterKey: "personality_type", TemplateID: "mbti", TemplateVersion: "2026-08-v1"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(1), OrgID: 1, AssessmentID: meta.FromUint64(2), TesteeID: 3, RunID: "test:1",
		Model:         evaluationfact.ModelIdentity{Kind: modelcatalog.KindTypology, Algorithm: modelcatalog.AlgorithmPersonalityTypology, Code: "MBTI_OEJTS", Version: "v64-report-202608-v1"},
		Runtime:       evaluationfact.RuntimeIdentity{DecisionKind: modelcatalog.DecisionKindPoleComposition},
		SchemaVersion: 2, Payload: raw, ReportInput: frozen, EvaluatedAt: time.Unix(100, 0),
	})
	in, err := inputadapter.FromOutcomeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := rendering.NewTypologyBuilder().Build(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := report.NewInterpretReport(report.InterpretReportInput{
		ID: meta.FromUint64(4), GenerationID: meta.FromUint64(5), OutcomeID: record.ID(), InterpretationRunID: meta.FromUint64(6),
		Association: in.Association, ReportType: policy.ReportTypeStandard, TemplateVersion: in.Report.TemplateVersion,
		BuilderIdentity: report.BuilderIdentityTypology, ContentSchemaVersion: report.ContentSchemaVersionV1, Content: draft.Content(), GeneratedAt: time.Unix(101, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func TestMBTIFactsSurviveFrozenReplayAndBSON(t *testing.T) {
	artifact := mbtiBaselineArtifact(t)
	mapper := NewLifecycleMapper()
	raw, err := bson.Marshal(mapper.ReportToPO(artifact))
	if err != nil {
		t.Fatal(err)
	}
	var po InterpretReportPO
	if err := bson.Unmarshal(raw, &po); err != nil {
		t.Fatal(err)
	}
	restored, err := mapper.ReportToDomain(&po)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(artifact.Content(), restored.Content()) {
		t.Fatal("content changed during BSON round trip")
	}
	for i, want := range []float64{56.25, 6.25, 25, 75} {
		d := restored.Content().Dimensions[i]
		p := d.PoleFacts()
		if p == nil || p.Strength != want || p.CompositionOrder != i+1 || p.MinScore != 8 || p.MaxScore != 40 || d.Name() != "冻结轴 "+d.Code().String() {
			t.Fatalf("lost exact facts: %+v", p)
		}
		p.Strength = 99
		if restored.Content().Dimensions[i].PoleFacts().Strength != want {
			t.Fatal("mutable facts escaped report")
		}
	}
	if restored.Content().ModelExtra.TypeCode != "ISFJ" || restored.Content().PrimaryScore.Value != 40.625 {
		t.Fatal("changed authoritative summary")
	}
}

func TestMBTIPersistedFactsRejectDamageAndRetainLegacy(t *testing.T) {
	mapper := NewLifecycleMapper()
	artifact := mbtiBaselineArtifact(t)
	tests := map[string]func(*InterpretReportPO){
		"missing strength": func(p *InterpretReportPO) { p.Dimensions[0].PoleFacts.Strength = nil },
		"missing minimum":  func(p *InterpretReportPO) { p.Dimensions[0].PoleFacts.MinScore = nil },
		"partial":          func(p *InterpretReportPO) { p.Dimensions[0].PoleFacts = nil },
		"unknown version":  func(p *InterpretReportPO) { p.Dimensions[0].PoleFacts.SchemaVersion = "future" },
		"wrong order":      func(p *InterpretReportPO) { *p.Dimensions[1].PoleFacts.CompositionOrder = 1 },
		"wrong type":       func(p *InterpretReportPO) { p.Dimensions[0].PoleFacts.Preference = "E" },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			po := mapper.ReportToPO(artifact)
			change(po)
			if _, err := mapper.ReportToDomain(po); err == nil {
				t.Fatal("damaged facts accepted")
			}
		})
	}
	po := mapper.ReportToPO(artifact)
	for i := range po.Dimensions {
		po.Dimensions[i].PoleFacts = nil
	}
	restored, err := mapper.ReportToDomain(po)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range restored.Content().Dimensions {
		if d.PoleFacts() != nil {
			t.Fatal("legacy facts invented")
		}
	}
	raw, err := bson.Marshal(po)
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, item := range doc["dimensions"].(bson.A) {
		if _, exists := item.(bson.M)["pole_facts"]; exists {
			t.Fatal("legacy BSON gained extension")
		}
	}
}

func TestMBTIPersistedZeroStrengthIsPresent(t *testing.T) {
	mapper := NewLifecycleMapper()
	po := mapper.ReportToPO(mbtiBaselineArtifact(t))
	*po.Dimensions[0].PoleFacts.Strength = 0
	po.Dimensions[0].RawScore = 24
	po.Dimensions[0].Score.Value = 24
	raw, err := bson.Marshal(po)
	if err != nil {
		t.Fatal(err)
	}
	var decoded InterpretReportPO
	if err := bson.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	got, err := mapper.ReportToDomain(&decoded)
	if err != nil {
		t.Fatal(err)
	}
	p := got.Content().Dimensions[0].PoleFacts()
	if p == nil || p.Strength != 0 || p.Preference != "I" {
		t.Fatal("zero preference strength was lost")
	}
}
