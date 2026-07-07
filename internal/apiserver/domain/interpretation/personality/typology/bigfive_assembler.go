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
	}, TraitProfileReportTemplate{
		Kind:             "bigfive",
		DefaultModelName: "Big Five 五大人格特质测评",
		DefaultModelCode: "BIGFIVE_V1",
		TypeName:         "五大人格特质",
		ConclusionTitle:  "五大人格特质画像",
		OneLiner:         "基于各维度原始分展示人格特质分布",
	})
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
