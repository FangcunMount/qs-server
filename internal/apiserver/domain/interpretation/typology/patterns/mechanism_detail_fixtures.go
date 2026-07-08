package patterns

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
