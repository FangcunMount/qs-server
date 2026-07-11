package writer

import (
	domaininterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

// legacyReportFromDraft is the sole compatibility conversion used by the old
// writer. Builders return Draft and have no report lifecycle responsibility.
func legacyReportFromDraft(input interpinput.InterpretationInput, draft *report.Draft) *report.InterpretReport {
	if draft == nil {
		return nil
	}
	content := draft.Content()
	total := 0.0
	if content.PrimaryScore != nil {
		total = content.PrimaryScore.Value
	}
	risk := report.RiskLevelNone
	if content.Level != nil && domaininterpretation.IsRiskLevelCode(content.Level.Code) {
		risk = report.RiskLevel(content.Level.Code)
	}
	legacy := report.NewInterpretReport(
		report.ID(input.Association.AssessmentID), content.Model.Title, content.Model.Code, total, risk,
		content.Conclusion, content.Dimensions, content.Suggestions, content.ModelExtra,
	)
	return report.AttachOutcomeSummary(legacy, content.Model, content.PrimaryScore, content.Level)
}
