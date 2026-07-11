package reporting

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

// DraftFromLegacyReport is a short-lived bridge for domain mechanism helpers
// which still assemble the old report representation. The public Builder
// contract returns Draft; lifecycle conversion remains at the legacy writer.
func DraftFromLegacyReport(input interpinput.InterpretationInput, legacy *report.InterpretReport) *report.Draft {
	if legacy == nil {
		return nil
	}
	model := input.Model
	if model.IsEmpty() {
		model = report.ModelIdentity{Code: legacy.ModelCode(), Title: legacy.ModelName()}
	}
	return report.NewDraft(report.Content{
		Model:        model,
		PrimaryScore: input.Result.Primary,
		Level:        input.Result.Level,
		Conclusion:   legacy.Conclusion(),
		Dimensions:   legacy.Dimensions(),
		Suggestions:  legacy.Suggestions(),
		ModelExtra:   legacy.ModelExtra(),
	})
}

// LegacyReportFromDraft is only for explicit in-process compatibility callers
// such as preview. Production persistence uses writer's private adapter until
// the Artifact committer becomes the only completion path.
func LegacyReportFromDraft(input interpinput.InterpretationInput, draft *report.Draft) *report.InterpretReport {
	if draft == nil {
		return nil
	}
	content := draft.Content()
	total := 0.0
	if content.PrimaryScore != nil {
		total = content.PrimaryScore.Value
	}
	risk := report.RiskLevelNone
	if content.Level != nil && domainReport.IsRiskLevelCode(content.Level.Code) {
		risk = report.RiskLevel(content.Level.Code)
	}
	legacy := report.NewInterpretReport(report.ID(input.Association.AssessmentID), content.Model.Title, content.Model.Code, total, risk, content.Conclusion, content.Dimensions, content.Suggestions, content.ModelExtra)
	return report.AttachOutcomeSummary(legacy, content.Model, content.PrimaryScore, content.Level)
}
