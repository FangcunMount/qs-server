package typology

import (
	"fmt"
	"strings"

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
	maxScore := 6.0
	return BuildPersonalityTypeReport(PersonalityTypeReportInput{
		AssessmentID: input.AssessmentID,
		ModelCode:    input.ModelCode,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Detail:       sbtiMechanismDetail(input.Detail),
	}, PersonalityTypeReportTemplate{
		Kind:              "sbti",
		DefaultModelName:  "SBTI 趣味人格测评",
		DefaultModelCode:  "SBTI_FUN",
		DimensionMaxScore: &maxScore,
		DimensionDescription: func(name, _ string, rawScore, _ float64, level, model string) string {
			description := strings.TrimSpace(fmt.Sprintf("%s：%s 档，原始分 %.0f/6", name, level, rawScore))
			if model != "" {
				description = model + " / " + description
			}
			return description
		},
		ConclusionSuffix: func(detail PersonalityTypeReportDetail) string {
			if detail.MatchPercent > 0 {
				return fmt.Sprintf("（匹配度 %.0f%%）", detail.MatchPercent)
			}
			return ""
		},
	})
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
