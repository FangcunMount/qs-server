package configured

import (
	"fmt"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	calcclassification "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func assemblePersonalityTypeDetail(input DetailInput) (any, error) {
	outcomeCode := firstNonEmpty(input.Selected.Code, input.Candidate.Code)
	outcome, ok := input.Payload.FindOutcome(outcomeCode)
	if !ok {
		return nil, fmt.Errorf("personality type outcome %s is not configured", outcomeCode)
	}
	score := input.Selected.Similarity
	if score == 0 {
		score = input.Candidate.MatchScore
	}
	matchPercent, similarity := calcclassification.DualScaleFromScore(score)
	dimensions, err := buildPersonalityDimensions(input)
	if err != nil {
		return nil, err
	}
	return outcometypology.PersonalityTypeDetail{
		TypeCode:       outcome.Code,
		TypeName:       outcome.Name,
		OneLiner:       outcome.OneLiner,
		Summary:        outcome.Summary,
		Pattern:        outcome.Pattern,
		MatchPercent:   matchPercent,
		Similarity:     similarity,
		ImageURL:       firstNonEmpty(outcome.ImageURL, outcome.Image),
		Rarity:         outcome.Rarity,
		Dimensions:     dimensions,
		Strengths:      append([]string(nil), outcome.Strengths...),
		Weaknesses:     append([]string(nil), outcome.Weaknesses...),
		Suggestions:    append([]string(nil), outcome.Suggestions...),
		Outcome:        outcome,
		Source:         input.Payload.Source,
		SpecialTrigger: input.Selected.Trigger,
		IsSpecial:      outcome.IsSpecial,
		Commentary:     outcome.Commentary,
	}, nil
}

func assembleTraitProfileDetail(input DetailInput) (any, error) {
	traits := make([]outcometypology.TraitProfileFactorResult, 0, len(input.Spec.FactorGraph.DecisionFactorOrder()))
	for _, dimCode := range input.Spec.FactorGraph.DecisionFactorOrder() {
		meta, ok := dimensionMetaForFactor(input.Spec.FactorGraph, dimCode)
		if !ok {
			return nil, fmt.Errorf("trait metadata for factor %s is not defined", dimCode)
		}
		raw, ok := input.Candidate.TraitScores[calcclassification.FactorID(dimCode)]
		if !ok {
			score, scoreOK := input.Vector.Scores[calcclassification.FactorID(dimCode)]
			if !scoreOK {
				return nil, fmt.Errorf("missing trait score for %s", dimCode)
			}
			raw = score.Raw
		}
		traits = append(traits, outcometypology.TraitProfileFactorResult{
			Code:     meta.Code,
			Name:     meta.Name,
			RawScore: raw,
		})
	}
	return outcometypology.TraitProfileDetail{
		Traits: traits,
		Source: input.Payload.Source,
	}, nil
}

// AssemblePersonalityTypeDetail 暴露机制中性人格类型明细组装，供 legacy adapter 使用。
func AssemblePersonalityTypeDetail(input DetailInput) (outcometypology.PersonalityTypeDetail, error) {
	generic, err := assemblePersonalityTypeDetail(input)
	if err != nil {
		return outcometypology.PersonalityTypeDetail{}, err
	}
	return generic.(outcometypology.PersonalityTypeDetail), nil
}

// AssembleTraitProfileDetail 暴露机制中性特质画像明细组装，供 legacy adapter 使用。
func AssembleTraitProfileDetail(input DetailInput) (outcometypology.TraitProfileDetail, error) {
	generic, err := assembleTraitProfileDetail(input)
	if err != nil {
		return outcometypology.TraitProfileDetail{}, err
	}
	return generic.(outcometypology.TraitProfileDetail), nil
}

func buildPersonalityDimensions(input DetailInput) ([]outcometypology.PersonalityDimensionResult, error) {
	if input.Decision.Kind == calcclassification.DecisionKindPoleComposition || len(input.Decision.Poles) > 0 {
		return buildPolePersonalityDimensions(input)
	}
	if input.Decision.Kind == calcclassification.DecisionKindNearestPattern || len(input.Decision.PatternOrder) > 0 ||
		input.Spec.Decision.Kind == modelcatalog.DecisionKindNearestPattern {
		return buildPatternPersonalityDimensions(input)
	}
	if input.Decision.Kind == calcclassification.DecisionKindDominantFactor ||
		input.Spec.Decision.Kind == modelcatalog.DecisionKindDominantFactor {
		return buildDominantPersonalityDimensions(input)
	}
	return nil, nil
}

func buildDominantPersonalityDimensions(input DetailInput) ([]outcometypology.PersonalityDimensionResult, error) {
	dimensions := make([]outcometypology.PersonalityDimensionResult, 0, len(input.Candidate.RankedFactors))
	for index, ranked := range input.Candidate.RankedFactors {
		factorID := calcclassification.FactorID(ranked.Code)
		score, ok := input.Vector.Scores[factorID]
		if !ok {
			return nil, fmt.Errorf("missing factor score for %s", factorID)
		}
		meta, ok := dimensionMetaForFactor(input.Spec.FactorGraph, string(factorID))
		if !ok {
			return nil, fmt.Errorf("dominant factor metadata for %s is not defined", factorID)
		}
		dimensions = append(dimensions, outcometypology.PersonalityDimensionResult{Code: meta.Code, Name: meta.Name, RawScore: score.Raw, Rank: index + 1})
	}
	return dimensions, nil
}

func buildPolePersonalityDimensions(input DetailInput) ([]outcometypology.PersonalityDimensionResult, error) {
	if len(input.Vector.Scores) == 0 {
		return nil, nil
	}
	dimensions := make([]outcometypology.PersonalityDimensionResult, 0, len(input.Decision.Poles))
	for _, pole := range input.Decision.Poles {
		score, ok := input.Vector.Scores[pole.FactorID]
		if !ok {
			return nil, fmt.Errorf("missing factor score for %s", pole.FactorID)
		}
		meta, ok := dimensionMetaForFactor(input.Spec.FactorGraph, string(pole.FactorID))
		if !ok {
			return nil, fmt.Errorf("pole metadata for factor %s is not defined", pole.FactorID)
		}
		preference, strength := calcclassification.ResolvePole(pole, score.Raw)
		dimensions = append(dimensions, outcometypology.PersonalityDimensionResult{
			Code:       meta.Code,
			Name:       meta.Name,
			Model:      meta.Model,
			LeftPole:   meta.LeftPole,
			RightPole:  meta.RightPole,
			RawScore:   score.Raw,
			Preference: preference,
			Strength:   strength,
		})
	}
	return dimensions, nil
}

func buildPatternPersonalityDimensions(input DetailInput) ([]outcometypology.PersonalityDimensionResult, error) {
	if len(input.Vector.Scores) == 0 {
		return nil, nil
	}
	order := input.Decision.PatternOrder
	if len(order) == 0 {
		for _, factorID := range input.Spec.FactorGraph.DecisionFactorOrder() {
			order = append(order, calcclassification.FactorID(factorID))
		}
	}
	dimensions := make([]outcometypology.PersonalityDimensionResult, 0, len(order))
	for _, factorID := range order {
		meta, ok := dimensionMetaForFactor(input.Spec.FactorGraph, string(factorID))
		if !ok {
			return nil, fmt.Errorf("pattern metadata for factor %s is not defined", factorID)
		}
		score, ok := input.Vector.Scores[factorID]
		if !ok {
			return nil, fmt.Errorf("missing factor score for %s", factorID)
		}
		dimensions = append(dimensions, outcometypology.PersonalityDimensionResult{
			Code:     meta.Code,
			Name:     meta.Name,
			Model:    meta.Model,
			RawScore: score.Raw,
			Level:    calcclassification.LevelForScore(score.Raw, input.Decision.LevelRule),
		})
	}
	return dimensions, nil
}
