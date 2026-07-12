package characterization_test

import (
	"context"
	"testing"

	interpretationinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/input"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

type draftReportBuilder interface {
	Build(context.Context, interpinput.InterpretationInput) (*report.Draft, error)
}

type characterizationReport struct{ content report.Content }

func (r *characterizationReport) Conclusion() string             { return r.content.Conclusion }
func (r *characterizationReport) ModelExtra() *report.ModelExtra { return r.content.ModelExtra }
func (r *characterizationReport) Dimensions() []report.DimensionInterpret {
	return r.content.Dimensions
}
func (r *characterizationReport) Suggestions() []report.Suggestion { return r.content.Suggestions }
func (r *characterizationReport) Model() report.ModelIdentity      { return r.content.Model }
func (r *characterizationReport) PrimaryScore() *report.ScoreValue { return r.content.PrimaryScore }
func (r *characterizationReport) Level() *report.ResultLevel       { return r.content.Level }
func (r *characterizationReport) ModelCode() string                { return r.content.Model.Code }
func (r *characterizationReport) ModelName() string                { return r.content.Model.Title }
func (r *characterizationReport) TotalScore() float64 {
	if r.content.PrimaryScore == nil {
		return 0
	}
	return r.content.PrimaryScore.Value
}
func (r *characterizationReport) RiskLevel() report.RiskLevel {
	if r.content.Level == nil {
		return report.RiskLevelNone
	}
	return report.RiskLevel(r.content.Level.Code)
}

func buildPreviewReport(t *testing.T, builder draftReportBuilder, outcome interpretationinput.PreviewOutcome) *characterizationReport {
	t.Helper()
	input, err := interpretationinput.FromPreviewOutcome(outcome)
	if err != nil {
		t.Fatalf("adapt interpretation input: %v", err)
	}
	draft, err := builder.Build(context.Background(), input)
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}
	return &characterizationReport{content: draft.Content()}
}
