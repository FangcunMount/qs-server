package score

import domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"

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
) []domainreport.FactorScoreInput {
	factorMeta := make(map[string]FactorReportModel)
	if model != nil {
		for _, f := range model.Factors {
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
