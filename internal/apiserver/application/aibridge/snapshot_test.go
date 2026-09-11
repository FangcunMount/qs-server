package aibridge

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/source"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestSnapshotPreservesPrivateDimensionFactsAndParticipantVisibility(t *testing.T) {
	current := snapshotSource(t, true)
	raw, err := reportSnapshot(current)
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Schema string `json:"schema_version"`
		Source struct {
			ReportID string `json:"report_id"`
		} `json:"source"`
		Dimensions []struct {
			Code        string   `json:"code"`
			Score       float64  `json:"raw_score"`
			Max         *float64 `json:"max_score"`
			Description string   `json:"description"`
			Parent      string   `json:"parent_code"`
			Level       struct {
				Label string `json:"label"`
			} `json:"level"`
			Derived []struct {
				Value float64 `json:"value"`
			} `json:"derived_scores"`
			Norm struct {
				Version string `json:"table_version"`
			} `json:"norm_reference"`
		} `json:"dimensions"`
		Suggestions []struct {
			Index   int    `json:"source_index"`
			Content string `json:"content"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value.Schema != "qs-report-snapshot/v1" || value.Source.ReportID != "99" || len(value.Dimensions) != 1 {
		t.Fatalf("incomplete snapshot: %s", raw)
	}
	d := value.Dimensions[0]
	if d.Code != "visible" || d.Score != 12 || d.Max != nil || d.Description != "标准描述 {{locale}}" || d.Parent != "parent" || d.Level.Label != "稳定" || len(d.Derived) != 1 || d.Derived[0].Value != 50 || d.Norm.Version != "norm-v1" {
		t.Fatalf("lost dimension facts: %+v", d)
	}
	if len(value.Suggestions) != 1 || value.Suggestions[0].Index != 1 || value.Suggestions[0].Content != "通用建议" {
		t.Fatalf("visibility or original reference index lost: %s", raw)
	}
}

func TestSnapshotFailsClosedWithoutOutcomeOrVisibility(t *testing.T) {
	current := snapshotSource(t, false)
	if _, err := reportSnapshot(current); !errors.Is(err, source.ErrInconsistent) {
		t.Fatalf("missing visibility: %v", err)
	}
	current = snapshotSource(t, true)
	current.Outcome = nil
	if _, err := reportSnapshot(current); !errors.Is(err, source.ErrInconsistent) {
		t.Fatalf("missing outcome: %v", err)
	}
	current.Outcome = evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: meta.FromUint64(101), OrgID: 2})
	if _, err := reportSnapshot(current); !errors.Is(err, source.ErrInconsistent) {
		t.Fatalf("mismatched outcome: %v", err)
	}
}

func snapshotSource(t *testing.T, configured bool) *source.Current {
	t.Helper()
	level := &report.ResultLevel{Code: "stable", Label: "稳定", Severity: "none"}
	dimension := report.NewNeutralDimensionInterpret(report.NewDimensionCode("visible"), report.DimensionKindFactor, "公开维度", 12, nil, level, "标准描述 {{locale}}", "观察建议").
		WithHierarchy("child", "parent", 1, 2).
		WithScoreContext([]report.ScoreValue{{Kind: "t_score", Value: 50}}, level, &report.NormReference{TableVersion: "norm-v1"})
	hiddenCode := report.FactorCode("hidden")
	content := report.Content{
		Model:       report.ModelIdentity{Kind: "scale", Code: "case", Algorithm: "sum", Version: "v1", Title: "测试"},
		Dimensions:  []report.DimensionInterpret{dimension, report.NewDimensionInterpret(hiddenCode, "不可见维度", 99, nil, report.RiskLevelNone, "不可见描述", "不可见建议")},
		Suggestions: []report.Suggestion{{Category: report.SuggestionCategoryDimension, Content: "不可见建议", FactorCode: &hiddenCode}, {Category: report.SuggestionCategoryGeneral, Content: "通用建议"}},
	}
	if configured {
		p := report.NewFrozenPresentationProfile([]string{"visible"})
		content.PresentationProfile = &p
	}
	r, err := report.RestoreInterpretReport(report.InterpretReportInput{
		ID: meta.FromUint64(99), GenerationID: meta.FromUint64(100), OutcomeID: meta.FromUint64(101), InterpretationRunID: meta.FromUint64(102),
		Association: report.Association{OrgID: 1, AssessmentID: meta.FromUint64(42), TesteeID: 7}, ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionCurrent,
		ContentSchemaVersion: "standard-v1", BuilderIdentity: "test", GeneratedAt: time.Now(), Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &source.Current{Report: r, Outcome: evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: r.OutcomeID(), OrgID: 1, TesteeID: 7, AssessmentID: meta.FromUint64(42)})}
}
