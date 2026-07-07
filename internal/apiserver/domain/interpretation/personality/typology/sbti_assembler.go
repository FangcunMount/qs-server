package typology

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type SBTIReportInput struct {
	AssessmentID domainreport.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    domainreport.RiskLevel
	Detail       SBTIReportDetail
}

// BuildSBTIReport 组装 SBTI typology 解读报告。
func BuildSBTIReport(input SBTIReportInput) (*domainreport.InterpretReport, error) {
	return BuildPersonalityTypeReport(PersonalityTypeReportInput{
		AssessmentID: input.AssessmentID,
		ModelCode:    input.ModelCode,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Detail:       sbtiMechanismDetail(input.Detail),
	}, SBTIPersonalityTypeTemplate())
}

func sbtiMechanismDetail(detail SBTIReportDetail) PersonalityTypeReportDetail {
	dimensions := make([]PersonalityTypeDimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, PersonalityTypeDimensionReport{
			Code: dim.Code, Name: dim.Name, Model: dim.Model, RawScore: dim.RawScore, Level: dim.Level,
		})
	}
	return PersonalityTypeReportDetail{
		TypeCode: detail.TypeCode, TypeName: detail.TypeName, OneLiner: detail.OneLiner,
		MatchPercent: detail.Similarity * 100, ImageURL: detail.ImageURL,
		IsSpecial: detail.Outcome.IsSpecial, SpecialTrigger: detail.SpecialTrigger,
		Commentary: detail.Outcome.Commentary, Dimensions: dimensions,
		Rarity: PersonalityTypeRarityReport{
			Percent: detail.Rarity.Percent, Label: detail.Rarity.Label, OneInX: detail.Rarity.OneInX,
		},
		Profile:           PersonalityTypeProfileReport{Summary: detail.Outcome.Commentary},
		SourceAttribution: detail.Source.Attribution, SourceLicense: detail.Source.License,
		SourceNonCommercial: detail.Source.NonCommercial,
	}
}
