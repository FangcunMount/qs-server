package evaluation

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	rulesetscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale"
)

type ScaleFactorReportScore struct {
	FactorCode   string
	FactorName   string
	RawScore     float64
	RiskLevel    domainReport.RiskLevel
	Conclusion   string
	Suggestion   string
	IsTotalScore bool
}

type ScaleReportInput struct {
	AssessmentID domainReport.ID
	Scale        *rulesetscale.ScaleSnapshot
	TotalScore   float64
	RiskLevel    domainReport.RiskLevel
	Conclusion   string
	Suggestion   string
	FactorScores []ScaleFactorReportScore
}

func BuildScaleReport(composer domainReport.ReportBuilder, input ScaleReportInput) (*domainReport.InterpretReport, error) {
	if composer == nil {
		return nil, domainReport.ErrInvalidArgument
	}
	return composer.Build(scaleGenerateReportInput(input))
}

func scaleGenerateReportInput(input ScaleReportInput) domainReport.GenerateReportInput {
	reportInput := domainReport.GenerateReportInput{
		AssessmentID: input.AssessmentID,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Conclusion:   input.Conclusion,
		Suggestion:   input.Suggestion,
	}
	if input.Scale != nil {
		reportInput.ScaleName = input.Scale.Title
		reportInput.ScaleCode = input.Scale.Code
	}
	reportInput.FactorScores = scaleFactorScoreInputs(input.FactorScores, input.Scale)
	return reportInput
}

func scaleFactorScoreInputs(
	factorScores []ScaleFactorReportScore,
	scaleSnapshot *rulesetscale.ScaleSnapshot,
) []domainReport.FactorScoreInput {
	factorMeta := make(map[string]rulesetscale.FactorSnapshot)
	if scaleSnapshot != nil {
		for _, f := range scaleSnapshot.Factors {
			factorMeta[f.Code] = f
		}
	}
	inputs := make([]domainReport.FactorScoreInput, 0, len(factorScores))
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
		inputs = append(inputs, domainReport.FactorScoreInput{
			FactorCode:   domainReport.FactorCode(fs.FactorCode),
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
