package patterns

import modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"

// MBTIResultDetailFromPersonalityType 投影通用人格类型明细 为 旧 MBTI 结构。
func MBTIResultDetailFromPersonalityType(detail PersonalityTypeDetail) MBTIResultDetail {
	dimensions := make([]MBTIDimensionResult, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, MBTIDimensionResult{
			Code:       dim.Code,
			Name:       dim.Name,
			LeftPole:   dim.LeftPole,
			RightPole:  dim.RightPole,
			RawScore:   dim.RawScore,
			Preference: dim.Preference,
			Strength:   dim.Strength,
		})
	}
	matchPercent := detail.MatchPercent
	if matchPercent == 0 && detail.Similarity > 0 {
		matchPercent = detail.Similarity * 100
	}
	return MBTIResultDetail{
		TypeCode:     detail.TypeCode,
		TypeName:     detail.TypeName,
		OneLiner:     detail.OneLiner,
		MatchPercent: matchPercent,
		ImageURL:     detail.ImageURL,
		Dimensions:   dimensions,
		Profile: modeltypology.MBTILegacyTypeProfile{
			TypeCode:    detail.TypeCode,
			TypeName:    detail.TypeName,
			OneLiner:    detail.OneLiner,
			Summary:     firstNonEmpty(detail.Summary, detail.Commentary),
			Strengths:   append([]string(nil), detail.Strengths...),
			Weaknesses:  append([]string(nil), detail.Weaknesses...),
			Suggestions: append([]string(nil), detail.Suggestions...),
			ImageURL:    detail.ImageURL,
		},
		Source: modeltypology.MBTILegacySource{
			Attribution:   detail.Source.Attribution,
			License:       detail.Source.License,
			NonCommercial: detail.Source.NonCommercial,
		},
	}
}

// SBTIResultDetailFromPersonalityType 投影通用人格类型明细 为 旧 SBTI 结构。
func SBTIResultDetailFromPersonalityType(detail PersonalityTypeDetail) SBTIResultDetail {
	dimensions := make([]SBTIDimensionResult, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, SBTIDimensionResult{
			Code:     dim.Code,
			Name:     dim.Name,
			Model:    dim.Model,
			RawScore: dim.RawScore,
			Level:    dim.Level,
		})
	}
	similarity := detail.Similarity
	if similarity == 0 && detail.MatchPercent > 0 {
		similarity = detail.MatchPercent / 100
	}
	return SBTIResultDetail{
		TypeCode:       detail.TypeCode,
		TypeName:       detail.TypeName,
		OneLiner:       detail.OneLiner,
		Pattern:        detail.Pattern,
		Similarity:     similarity,
		ImageURL:       detail.ImageURL,
		Rarity:         convertRarityFromGeneric(detail.Rarity),
		Dimensions:     dimensions,
		Outcome:        convertOutcomeFromGeneric(detail.Outcome),
		Source:         convertSBTISourceFromGeneric(detail.Source),
		SpecialTrigger: detail.SpecialTrigger,
	}
}

// BigFiveResultDetailFromTraitProfile 投影通用特质画像 为 旧 BigFive 结构。
func BigFiveResultDetailFromTraitProfile(detail TraitProfileDetail) BigFiveResultDetail {
	traits := make([]BigFiveTraitResult, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		traits = append(traits, BigFiveTraitResult(trait))
	}
	return BigFiveResultDetail{Traits: traits, Source: detail.Source}
}

// PersonalityTypeDetailFromMBTI 转换旧版 MBTI 明细 为 通用人格类型结构。
func PersonalityTypeDetailFromMBTI(detail MBTIResultDetail) PersonalityTypeDetail {
	dimensions := make([]PersonalityDimensionResult, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, PersonalityDimensionResult{
			Code:       dim.Code,
			Name:       dim.Name,
			LeftPole:   dim.LeftPole,
			RightPole:  dim.RightPole,
			RawScore:   dim.RawScore,
			Preference: dim.Preference,
			Strength:   dim.Strength,
		})
	}
	return PersonalityTypeDetail{
		TypeCode:     detail.TypeCode,
		TypeName:     detail.TypeName,
		OneLiner:     detail.OneLiner,
		Summary:      detail.Profile.Summary,
		MatchPercent: detail.MatchPercent,
		Similarity:   detail.MatchPercent / 100,
		ImageURL:     detail.ImageURL,
		Dimensions:   dimensions,
		Strengths:    append([]string(nil), detail.Profile.Strengths...),
		Weaknesses:   append([]string(nil), detail.Profile.Weaknesses...),
		Suggestions:  append([]string(nil), detail.Profile.Suggestions...),
		Outcome: modeltypology.Outcome{
			Code:     detail.TypeCode,
			Name:     detail.TypeName,
			OneLiner: detail.OneLiner,
			Summary:  detail.Profile.Summary,
		},
		Source: modeltypology.Source{
			Attribution:   detail.Source.Attribution,
			License:       detail.Source.License,
			NonCommercial: detail.Source.NonCommercial,
		},
	}
}

// PersonalityTypeDetailFromSBTI 转换旧版 SBTI 明细 为 通用人格类型结构。
func PersonalityTypeDetailFromSBTI(detail SBTIResultDetail) PersonalityTypeDetail {
	dimensions := make([]PersonalityDimensionResult, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, PersonalityDimensionResult{
			Code:     dim.Code,
			Name:     dim.Name,
			Model:    dim.Model,
			RawScore: dim.RawScore,
			Level:    dim.Level,
		})
	}
	return PersonalityTypeDetail{
		TypeCode:       detail.TypeCode,
		TypeName:       detail.TypeName,
		OneLiner:       detail.OneLiner,
		Pattern:        detail.Pattern,
		MatchPercent:   detail.Similarity * 100,
		Similarity:     detail.Similarity,
		ImageURL:       detail.ImageURL,
		Rarity:         modeltypology.Rarity(detail.Rarity),
		Dimensions:     dimensions,
		Outcome:        convertOutcomeToGeneric(detail.Outcome),
		Source:         convertSourceToGeneric(detail.Source),
		SpecialTrigger: detail.SpecialTrigger,
		IsSpecial:      detail.Outcome.IsSpecial,
		Commentary:     detail.Outcome.Commentary,
	}
}

// TraitProfileDetailFromBigFive 转换旧版 BigFive 明细 为 通用特质画像结构。
func TraitProfileDetailFromBigFive(detail BigFiveResultDetail) TraitProfileDetail {
	traits := make([]TraitProfileFactorResult, 0, len(detail.Traits))
	for _, trait := range detail.Traits {
		traits = append(traits, TraitProfileFactorResult(trait))
	}
	return TraitProfileDetail{Traits: traits, Source: detail.Source}
}

func convertRarityFromGeneric(rarity modeltypology.Rarity) modeltypology.SBTILegacyRarity {
	return modeltypology.SBTILegacyRarity(rarity)
}

func convertOutcomeFromGeneric(outcome modeltypology.Outcome) modeltypology.SBTILegacyOutcome {
	return modeltypology.SBTILegacyOutcome{
		Code:       outcome.Code,
		Name:       outcome.Name,
		OneLiner:   outcome.OneLiner,
		Pattern:    outcome.Pattern,
		Image:      outcome.Image,
		Rarity:     convertRarityFromGeneric(outcome.Rarity),
		IsSpecial:  outcome.IsSpecial,
		Trigger:    outcome.Trigger,
		Commentary: outcome.Commentary,
	}
}

func convertSBTISourceFromGeneric(source modeltypology.Source) modeltypology.SBTILegacySource {
	return modeltypology.SBTILegacySource{
		WikiRepo:      source.WikiRepo,
		SourceSite:    source.SourceSite,
		License:       source.License,
		Attribution:   source.Attribution,
		ImageBaseURL:  source.ImageBaseURL,
		NonCommercial: source.NonCommercial,
	}
}

func convertOutcomeToGeneric(outcome modeltypology.SBTILegacyOutcome) modeltypology.Outcome {
	return modeltypology.Outcome{
		Code:       outcome.Code,
		Name:       outcome.Name,
		OneLiner:   outcome.OneLiner,
		Pattern:    outcome.Pattern,
		Image:      outcome.Image,
		Rarity:     modeltypology.Rarity(outcome.Rarity),
		IsSpecial:  outcome.IsSpecial,
		Trigger:    outcome.Trigger,
		Commentary: outcome.Commentary,
	}
}

func convertSourceToGeneric(source modeltypology.SBTILegacySource) modeltypology.Source {
	return modeltypology.Source{
		WikiRepo:      source.WikiRepo,
		SourceSite:    source.SourceSite,
		License:       source.License,
		Attribution:   source.Attribution,
		ImageBaseURL:  source.ImageBaseURL,
		NonCommercial: source.NonCommercial,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
