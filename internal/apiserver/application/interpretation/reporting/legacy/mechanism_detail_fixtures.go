package legacy

import reportpatterns "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"

func MBTIMechanismDetail(detail MBTIReportDetail) reportpatterns.PersonalityTypeReportDetail {
	dimensions := make([]reportpatterns.PersonalityTypeDimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, reportpatterns.PersonalityTypeDimensionReport{
			Code: dim.Code, Name: dim.Name, LeftPole: dim.LeftPole, RightPole: dim.RightPole,
			RawScore: dim.RawScore, Preference: dim.Preference, Strength: dim.Strength,
		})
	}
	return reportpatterns.PersonalityTypeReportDetail{
		TypeCode: detail.TypeCode, TypeName: detail.TypeName, OneLiner: detail.OneLiner,
		MatchPercent: detail.MatchPercent, ImageURL: detail.ImageURL, Dimensions: dimensions,
		Profile: reportpatterns.PersonalityTypeProfileReport{
			Summary: detail.Profile.Summary, Strengths: detail.Profile.Strengths,
			Weaknesses: detail.Profile.Weaknesses, Suggestions: detail.Profile.Suggestions,
		},
		SourceAttribution: detail.Source.Attribution, SourceLicense: detail.Source.License,
		SourceNonCommercial: detail.Source.NonCommercial,
	}
}

func SBTIMechanismDetail(detail SBTIReportDetail) reportpatterns.PersonalityTypeReportDetail {
	dimensions := make([]reportpatterns.PersonalityTypeDimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, reportpatterns.PersonalityTypeDimensionReport{
			Code: dim.Code, Name: dim.Name, Model: dim.Model, RawScore: dim.RawScore, Level: dim.Level,
		})
	}
	return reportpatterns.PersonalityTypeReportDetail{
		TypeCode: detail.TypeCode, TypeName: detail.TypeName, OneLiner: detail.OneLiner,
		MatchPercent: detail.Similarity * 100, ImageURL: detail.ImageURL,
		IsSpecial: detail.Outcome.IsSpecial, SpecialTrigger: detail.SpecialTrigger,
		Commentary: detail.Outcome.Commentary, Dimensions: dimensions,
		Rarity: reportpatterns.PersonalityTypeRarityReport{
			Percent: detail.Rarity.Percent, Label: detail.Rarity.Label, OneInX: detail.Rarity.OneInX,
		},
		Profile:           reportpatterns.PersonalityTypeProfileReport{Summary: detail.Outcome.Commentary},
		SourceAttribution: detail.Source.Attribution, SourceLicense: detail.Source.License,
		SourceNonCommercial: detail.Source.NonCommercial,
	}
}

func BigFiveMechanismDetail(detail BigFiveReportDetail) reportpatterns.TraitProfileReportDetail {
	traits := make([]reportpatterns.TraitProfileFactorReport, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		traits = append(traits, reportpatterns.TraitProfileFactorReport(trait))
	}
	return reportpatterns.TraitProfileReportDetail{
		Traits: traits,
		Source: reportpatterns.TraitProfileSourceReport{
			Attribution: detail.Source.Attribution, License: detail.Source.License, NonCommercial: detail.Source.NonCommercial,
		},
	}
}
