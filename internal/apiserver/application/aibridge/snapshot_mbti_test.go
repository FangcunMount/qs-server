package aibridge

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	source "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportsource"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func mbtiSnapshotContent(zero bool) report.Content {
	c := report.Content{
		Model:       report.ModelIdentity{Kind: "typology", Algorithm: "personality_typology", Code: "MBTI_OEJTS", Version: "v64-report-202608-v1", Title: "16人格测评（基础版）"},
		Conclusion:  "本次测评类型为 ISFJ。",
		ModelExtra:  &report.ModelExtra{Kind: "personality_type", TypeCode: "ISFJ", TypeName: "守卫者", MatchPercent: 40.625, OneLiner: "本次测评结果", ImageURL: "must-not-cross-boundary", Rarity: &report.ModelRarity{Percent: 3}},
		Suggestions: []report.Suggestion{{Category: report.SuggestionCategoryGeneral, Content: "结合具体情境理解本次结果。"}},
	}
	codes, left, right := []string{"EI", "SN", "TF", "JP"}, []string{"I", "S", "F", "J"}, []string{"E", "N", "T", "P"}
	raw, strength := []float64{15, 23, 20, 12}, []float64{56.25, 6.25, 25, 75}
	for i, code := range codes {
		if zero {
			raw[i], strength[i] = 24, 0
			c.ModelExtra.MatchPercent = 0
		}
		c.Dimensions = append(c.Dimensions, report.NewNeutralDimensionInterpret(report.NewDimensionCode(code), report.DimensionKindPole, code, raw[i], nil, nil, "标准报告原说明", "").WithPoleFacts(&report.PoleFacts{SchemaVersion: report.PoleFactsSchema, LeftPole: left[i], RightPole: right[i], Preference: left[i], Strength: strength[i], MinScore: 8, MaxScore: 40, Threshold: 24, CompositionOrder: i + 1}))
	}
	return c
}

func mbtiSnapshotSource(t *testing.T, c report.Content) *source.Current {
	t.Helper()
	r, err := report.RestoreInterpretReport(report.InterpretReportInput{
		ID: meta.FromUint64(99), GenerationID: meta.FromUint64(100), OutcomeID: meta.FromUint64(101), InterpretationRunID: meta.FromUint64(102),
		Association: report.Association{OrgID: 1, AssessmentID: meta.FromUint64(42), TesteeID: 7}, ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersion("2026-08-v1"),
		ContentSchemaVersion: "report-content/v1", BuilderIdentity: "typology", GeneratedAt: time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC), Content: c,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &source.Current{Report: r, Outcome: evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: r.OutcomeID(), OrgID: 1, AssessmentID: meta.FromUint64(42), TesteeID: 7, Model: evaluationfact.ModelIdentity{Kind: "typology", Algorithm: "personality_typology", Code: "MBTI_OEJTS", Version: c.Model.Version, Title: c.Model.Title}, Runtime: evaluationfact.RuntimeIdentity{DecisionKind: "pole_composition"}})}
}

// CI exports bytes from the real Go projector for the Python decoder/assembler.
// This is synthetic test data, never a production report export.
func TestMBTISnapshotContractVector(t *testing.T) {
	vectors := make(map[string]json.RawMessage)
	for _, zero := range []bool{false, true} {
		current := mbtiSnapshotSource(t, mbtiSnapshotContent(zero))
		raw, err := reportSnapshot(current)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got["schema_version"] != "qs-report-snapshot/v2" || len(got) != 8 {
			t.Fatalf("wrong projection: %s", raw)
		}
		extra := got["model_extra"].(map[string]any)
		if len(extra) != 6 || extra["type_code"] != "ISFJ" {
			t.Fatalf("extra fields leaked: %v", extra)
		}
		axes := got["dimensions"].([]any)
		for i, d := range current.Report.Content().Dimensions {
			axis := axes[i].(map[string]any)
			p := axis["pole_facts"].(map[string]any)
			if axis["raw_score"] != d.RawScore() || p["strength"] != d.PoleFacts().Strength || len(axis) != 7 {
				t.Fatalf("changed facts: %v", axis)
			}
		}
		key := "mbti"
		if zero {
			key = "mbti_zero"
			if extra["match_percent"] != float64(0) {
				t.Fatal("lost zero match")
			}
		}
		vectors[key] = raw
	}
	raw, err := reportSnapshot(snapshotSource(t, true))
	if err != nil {
		t.Fatal(err)
	}
	// Fixed v1 byte baseline: MBTI fields must never alter the scale projection.
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != "260a8c05fbb5fc2d624131e97576a226a225bf3a6984f28b88e7c7345894af6a" {
		t.Fatalf("scale snapshot bytes changed: %s", got)
	}
	vectors["scale"] = raw
	if path := os.Getenv("QS_AI_SNAPSHOT_VECTOR_OUT"); path != "" {
		encoded, err := json.Marshal(vectors)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLegacyMBTISnapshotIsNotApplicableAndCannotBeStaged(t *testing.T) {
	for _, future := range []bool{false, true} {
		t.Run(fmt.Sprint(future), func(t *testing.T) {
			c := mbtiSnapshotContent(false)
			for i := range c.Dimensions {
				c.Dimensions[i] = c.Dimensions[i].WithPoleFacts(nil)
			}
			if future {
				c.Model.Version = "future-version"
			}
			current := mbtiSnapshotSource(t, c)
			if _, err := reportSnapshot(current); !errors.Is(err, source.ErrNotApplicable) {
				t.Fatalf("fallback: %v", err)
			}
			store := &stagingStore{}
			p := &Participant{Access: accessStub{}, Sources: &sourceStub{current: current}, Bridge: &Service{Store: store}}
			status, err := p.Source(context.Background(), Actor{"1", "parent"}, 7, 42)
			if err != nil || status.Status != "not_applicable" {
				t.Fatalf("source=%v err=%v", status, err)
			}
			err = p.Request(context.Background(), Actor{"1", "parent"}, 7, 42, 99, "bcf4a3c3-288b-47e0-8d51-762ce4f215ea")
			if !errors.Is(err, source.ErrNotApplicable) || len(store.requests) != 0 {
				t.Fatalf("staged unsupported report: %v", err)
			}
		})
	}
}

func TestMBTISnapshotRejectsFrozenRangeOrSuggestionMismatch(t *testing.T) {
	for _, scenario := range []string{"range", "suggestion", "outcome"} {
		t.Run(scenario, func(t *testing.T) {
			c := mbtiSnapshotContent(false)
			if scenario == "range" {
				p := c.Dimensions[0].PoleFacts()
				p.MinScore = 0
				c.Dimensions[0] = c.Dimensions[0].WithPoleFacts(p)
			}
			if scenario == "suggestion" {
				code := report.FactorCode("unknown")
				c.Suggestions[0].FactorCode = &code
			}
			current := mbtiSnapshotSource(t, c)
			if scenario == "outcome" {
				current.Outcome = evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: current.Report.OutcomeID(), OrgID: 1, AssessmentID: meta.FromUint64(42), TesteeID: 7, Model: evaluationfact.ModelIdentity{Code: "other"}})
			}
			if _, err := reportSnapshot(current); !errors.Is(err, source.ErrInconsistent) {
				t.Fatalf("accepted inconsistent facts: %v", err)
			}
		})
	}
}
