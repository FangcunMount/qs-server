package typology

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type BigFiveReportInput struct {
	AssessmentID domainreport.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    domainreport.RiskLevel
	Detail       BigFiveReportDetail
}

// BuildBigFiveReport 组装 Big Five trait-profile 解读报告。
func BuildBigFiveReport(input BigFiveReportInput) (*domainreport.InterpretReport, error) {
	return BuildTraitProfileReport(TraitProfileReportInput{
		AssessmentID: input.AssessmentID,
		ModelCode:    input.ModelCode,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Detail:       bigFiveMechanismDetail(input.Detail),
	}, BigFiveTraitProfileTemplate())
}

func bigFiveMechanismDetail(detail BigFiveReportDetail) TraitProfileReportDetail {
	traits := make([]TraitProfileFactorReport, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		traits = append(traits, TraitProfileFactorReport(trait))
	}
	return TraitProfileReportDetail{
		Traits: traits,
		Source: TraitProfileSourceReport{
			Attribution: detail.Source.Attribution, License: detail.Source.License, NonCommercial: detail.Source.NonCommercial,
		},
	}
}
