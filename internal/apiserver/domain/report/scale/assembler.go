package scale

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func BuildReport(composer domainreport.ReportBuilder, input ReportInput) (*domainreport.InterpretReport, error) {
	if composer == nil {
		return nil, domainreport.ErrInvalidArgument
	}
	return composer.Build(generateReportInput(input))
}

func generateReportInput(input ReportInput) domainreport.GenerateReportInput {
	reportInput := domainreport.GenerateReportInput{
		AssessmentID: input.AssessmentID,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Conclusion:   input.Conclusion,
		Suggestion:   input.Suggestion,
	}
	if input.Scale != nil {
		reportInput.ModelName = input.Scale.Title
		reportInput.ModelCode = input.Scale.Code
	}
	reportInput.FactorScores = factorScoreInputs(input.FactorScores, input.Scale)
	return reportInput
}

func factorScoreInputs(
	factorScores []FactorReportScore,
	scaleModel *ReportModel,
) []domainreport.FactorScoreInput {
	factorMeta := make(map[string]FactorReportModel)
	if scaleModel != nil {
		for _, f := range scaleModel.Factors {
			factorMeta[f.Code] = f
		}
	}
	inputs := make([]domainreport.FactorScoreInput, 0, len(factorScores))
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
		inputs = append(inputs, domainreport.FactorScoreInput{
			FactorCode:   domainreport.FactorCode(fs.FactorCode),
			FactorName:   factorName,
			RawScore:     fs.RawScore,
			MaxScore:     maxScore,
			RiskLevel:    fs.RiskLevel,
			Description:  fs.Conclusion,
			Suggestion:   fs.Suggestion,
			IsTotalScore: fs.IsTotalScore,
		})
	}
	return inputs
}
