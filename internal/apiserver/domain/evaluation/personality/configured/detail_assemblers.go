package configured

import (
	"fmt"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/profile"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

func assembleMBTIDetail(input DetailInput) (any, error) {
	outcomeCode := firstNonEmpty(input.Selected.Code, input.Candidate.Code)
	outcome, ok := input.Payload.FindOutcome(outcomeCode)
	if !ok {
		return nil, fmt.Errorf("mbti type profile not found for %s", outcomeCode)
	}
	dimensions := make([]evaluationtypology.MBTIDimensionResult, 0, len(input.Decision.Poles))
	for _, pole := range input.Decision.Poles {
		score, ok := input.Vector.Scores[pole.FactorID]
		if !ok {
			return nil, fmt.Errorf("missing factor score for %s", pole.FactorID)
		}
		meta, ok := dimensionMetaForFactor(input.Spec.FactorGraph, string(pole.FactorID))
		if !ok {
			return nil, fmt.Errorf("pole metadata for factor %s is not defined", pole.FactorID)
		}
		preference, strength := profile.ResolvePole(pole, score.Raw)
		dimensions = append(dimensions, evaluationtypology.MBTIDimensionResult{
			Code:       meta.Code,
			Name:       meta.Name,
			LeftPole:   meta.LeftPole,
			RightPole:  meta.RightPole,
			RawScore:   score.Raw,
			Preference: preference,
			Strength:   strength,
		})
	}
	similarity := input.Selected.Similarity
	if similarity == 0 {
		similarity = input.Candidate.MatchScore
	}
	return evaluationtypology.MBTIResultDetail{
		TypeCode:     outcomeCode,
		TypeName:     outcome.Name,
		OneLiner:     outcome.OneLiner,
		MatchPercent: similarity,
		ImageURL:     outcome.ImageURL,
		Dimensions:   dimensions,
		Profile: modeltypology.MBTILegacyTypeProfile{
			TypeCode:    outcome.Code,
			TypeName:    outcome.Name,
			OneLiner:    outcome.OneLiner,
			Summary:     outcome.Summary,
			Strengths:   append([]string(nil), outcome.Strengths...),
			Weaknesses:  append([]string(nil), outcome.Weaknesses...),
			Suggestions: append([]string(nil), outcome.Suggestions...),
			ImageURL:    outcome.ImageURL,
		},
		Source: modeltypology.MBTILegacySource{
			Attribution:   input.Payload.Source.Attribution,
			License:       input.Payload.Source.License,
			NonCommercial: input.Payload.Source.NonCommercial,
		},
	}, nil
}

func assembleSBTIDetail(input DetailInput) (any, error) {
	outcomeCode := firstNonEmpty(input.Selected.Code, input.Candidate.Code)
	outcome, ok := input.Payload.FindOutcome(outcomeCode)
	if !ok {
		return nil, fmt.Errorf("sbti outcome %s is not configured", outcomeCode)
	}
	similarity := input.Selected.Similarity
	if similarity == 0 {
		similarity = input.Candidate.MatchScore
	}
	var dimensions []evaluationtypology.SBTIDimensionResult
	if levels := buildSBTIDimensions(input); levels != nil {
		dimensions = make([]evaluationtypology.SBTIDimensionResult, 0, len(levels))
		for _, dim := range levels {
			dimensions = append(dimensions, evaluationtypology.SBTIDimensionResult{
				Code:     dim.Code,
				Name:     dim.Name,
				Model:    dim.Model,
				RawScore: dim.RawScore,
				Level:    dim.Level,
			})
		}
	}
	return evaluationtypology.SBTIResultDetail{
		TypeCode:       outcome.Code,
		TypeName:       outcome.Name,
		OneLiner:       outcome.OneLiner,
		Pattern:        outcome.Pattern,
		Similarity:     similarity,
		ImageURL:       outcome.Image,
		Rarity:         convertRarity(outcome.Rarity),
		Dimensions:     dimensions,
		Outcome:        convertOutcome(outcome),
		Source:         convertSource(input.Payload.Source),
		SpecialTrigger: input.Selected.Trigger,
	}, nil
}

func assembleBigFiveDetail(input DetailInput) (any, error) {
	traits := make([]evaluationtypology.BigFiveTraitResult, 0, len(input.Spec.FactorGraph.DecisionFactorOrder()))
	for _, dimCode := range input.Spec.FactorGraph.DecisionFactorOrder() {
		meta, ok := dimensionMetaForFactor(input.Spec.FactorGraph, dimCode)
		if !ok {
			return nil, fmt.Errorf("trait metadata for factor %s is not defined", dimCode)
		}
		raw, ok := input.Candidate.TraitScores[profile.FactorID(dimCode)]
		if !ok {
			score, scoreOK := input.Vector.Scores[profile.FactorID(dimCode)]
			if !scoreOK {
				return nil, fmt.Errorf("missing trait score for %s", dimCode)
			}
			raw = score.Raw
		}
		traits = append(traits, evaluationtypology.BigFiveTraitResult{
			Code:     meta.Code,
			Name:     meta.Name,
			RawScore: raw,
		})
	}
	return evaluationtypology.BigFiveResultDetail{
		Traits: traits,
		Source: input.Payload.Source,
	}, nil
}

func convertRarity(rarity modeltypology.Rarity) modeltypology.SBTILegacyRarity {
	return modeltypology.SBTILegacyRarity(rarity)
}

func convertOutcome(outcome modeltypology.Outcome) modeltypology.SBTILegacyOutcome {
	return modeltypology.SBTILegacyOutcome{
		Code:       outcome.Code,
		Name:       outcome.Name,
		OneLiner:   outcome.OneLiner,
		Pattern:    outcome.Pattern,
		Image:      outcome.Image,
		Rarity:     convertRarity(outcome.Rarity),
		IsSpecial:  outcome.IsSpecial,
		Trigger:    outcome.Trigger,
		Commentary: outcome.Commentary,
	}
}

func convertSource(source modeltypology.Source) modeltypology.SBTILegacySource {
	return modeltypology.SBTILegacySource{
		WikiRepo:      source.WikiRepo,
		SourceSite:    source.SourceSite,
		License:       source.License,
		Attribution:   source.Attribution,
		ImageBaseURL:  source.ImageBaseURL,
		NonCommercial: source.NonCommercial,
	}
}
