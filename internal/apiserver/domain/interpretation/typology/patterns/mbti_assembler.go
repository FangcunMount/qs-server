package patterns

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type MBTIReportInput struct {
	AssessmentID domainreport.ID
	ModelCode    string
	TotalScore   float64
	RiskLevel    domainreport.RiskLevel
	Detail       MBTIReportDetail
}

// BuildMBTIReport 组装 MBTI typology 解读报告。
func BuildMBTIReport(input MBTIReportInput) (*domainreport.InterpretReport, error) {
	return BuildPersonalityTypeReport(PersonalityTypeReportInput{
		AssessmentID: input.AssessmentID,
		ModelCode:    input.ModelCode,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Detail:       mbtiMechanismDetail(input.Detail),
	}, MBTIPersonalityTypeTemplate())
}

func mbtiMechanismDetail(detail MBTIReportDetail) PersonalityTypeReportDetail {
	dimensions := make([]PersonalityTypeDimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, PersonalityTypeDimensionReport{
			Code: dim.Code, Name: dim.Name, LeftPole: dim.LeftPole, RightPole: dim.RightPole,
			RawScore: dim.RawScore, Preference: dim.Preference, Strength: dim.Strength,
		})
	}
	return PersonalityTypeReportDetail{
		TypeCode: detail.TypeCode, TypeName: detail.TypeName, OneLiner: detail.OneLiner,
		MatchPercent: detail.MatchPercent, ImageURL: detail.ImageURL, Dimensions: dimensions,
		Profile: PersonalityTypeProfileReport{
			Summary: detail.Profile.Summary, Strengths: detail.Profile.Strengths,
			Weaknesses: detail.Profile.Weaknesses, Suggestions: detail.Profile.Suggestions,
		},
		SourceAttribution: detail.Source.Attribution, SourceLicense: detail.Source.License,
		SourceNonCommercial: detail.Source.NonCommercial,
	}
}
