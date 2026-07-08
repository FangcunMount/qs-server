package scoring

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

// FactorScoringReportInput is factor-scoring mechanism report assembly input.
type FactorScoringReportInput struct {
	AssessmentID report.ID
	Scale        *ReportModel
	TotalScore   float64
	RiskLevel    report.RiskLevel
	Conclusion   string
	Suggestion   string
	FactorScores []FactorReportScore
}

// ScaleReportInput is a deprecated alias for FactorScoringReportInput.
type ScaleReportInput = FactorScoringReportInput

// BuildFactorScoringReport assembles factor-scoring mechanism reports.
// When factor conclusion/suggestion is empty, interpretation rules generate them;
// overall conclusion/suggestion falls back to the total-score factor when unset.
func BuildFactorScoringReport(composer report.ReportBuilder, input FactorScoringReportInput) (*report.InterpretReport, error) {
	factorScores := make([]FactorReportScore, 0, len(input.FactorScores))
	for _, fs := range input.FactorScores {
		if fs.Conclusion == "" && fs.Suggestion == "" {
			fs.Conclusion, fs.Suggestion = interpretScaleFactor(input.Scale, fs)
		}
		factorScores = append(factorScores, fs)
	}

	conclusion, suggestion := input.Conclusion, input.Suggestion
	if conclusion == "" && suggestion == "" {
		for _, fs := range factorScores {
			if fs.IsTotalScore {
				conclusion, suggestion = fs.Conclusion, fs.Suggestion
				break
			}
		}
	}

	return BuildReport(composer, ReportInput{
		AssessmentID: input.AssessmentID,
		Model:        input.Scale,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Conclusion:   conclusion,
		Suggestion:   suggestion,
		FactorScores: factorScores,
	})
}

// BuildScaleReport is a deprecated alias for BuildFactorScoringReport.
var BuildScaleReport = BuildFactorScoringReport

func BuildReport(composer report.ReportBuilder, input ReportInput) (*report.InterpretReport, error) {
	if composer == nil {
		return nil, report.ErrInvalidArgument
	}
	return composer.Build(generateReportInput(input))
}

func generateReportInput(input ReportInput) report.GenerateReportInput {
	reportInput := report.GenerateReportInput{
		AssessmentID: input.AssessmentID,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Conclusion:   input.Conclusion,
		Suggestion:   input.Suggestion,
	}
	if input.Model != nil {
		reportInput.ModelName = input.Model.Title
		reportInput.ModelCode = input.Model.Code
	}
	reportInput.FactorScores = factorScoreInputs(input.FactorScores, input.Model)
	return reportInput
}

func factorScoreInputs(
	factorScores []FactorReportScore,
	model *ReportModel,
) []report.FactorScoreInput {
	factorMeta := make(map[string]FactorReportModel)
	if model != nil {
		for _, f := range model.Factors {
			factorMeta[f.Code] = f
		}
	}
	inputs := make([]report.FactorScoreInput, 0, len(factorScores))
	for _, fs := range factorScores {
		meta, ok := factorMeta[fs.FactorCode]
		factorName := fs.FactorName
		var maxScore *float64
		if ok {
			if factorName == "" {
				factorName = meta.Title
			}
			maxScore = meta.MaxScore
		}
		if factorName == "" {
			factorName = fs.FactorCode
		}
		inputs = append(inputs, report.FactorScoreInput{
			FactorCode:     report.FactorCode(fs.FactorCode),
			FactorName:     factorName,
			RawScore:       fs.RawScore,
			MaxScore:       maxScore,
			RiskLevel:      fs.RiskLevel,
			Description:    fs.Conclusion,
			Suggestion:     fs.Suggestion,
			IsTotalScore:   fs.IsTotalScore,
			Role:           fs.Role,
			ParentCode:     fs.ParentCode,
			HierarchyLevel: fs.HierarchyLevel,
			SortOrder:      fs.SortOrder,
		})
	}
	return inputs
}
