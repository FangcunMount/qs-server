package configured

import (
	"fmt"

	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/trait"
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
	matchPercent := score
	similarity := score
	if score > 0 && score <= 1 {
		matchPercent = score * 100
	} else if score > 1 {
		similarity = score / 100
	}
	dimensions, err := buildPersonalityDimensions(input)
	if err != nil {
		return nil, err
	}
	return evaluationtypology.PersonalityTypeDetail{
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
	traits := make([]evaluationtypology.TraitProfileFactorResult, 0, len(input.Spec.FactorGraph.DecisionFactorOrder()))
	for _, dimCode := range input.Spec.FactorGraph.DecisionFactorOrder() {
		meta, ok := dimensionMetaForFactor(input.Spec.FactorGraph, dimCode)
		if !ok {
			return nil, fmt.Errorf("trait metadata for factor %s is not defined", dimCode)
		}
		raw, ok := input.Candidate.TraitScores[trait.FactorID(dimCode)]
		if !ok {
			score, scoreOK := input.Vector.Scores[trait.FactorID(dimCode)]
			if !scoreOK {
				return nil, fmt.Errorf("missing trait score for %s", dimCode)
			}
			raw = score.Raw
		}
		traits = append(traits, evaluationtypology.TraitProfileFactorResult{
			Code:     meta.Code,
			Name:     meta.Name,
			RawScore: raw,
		})
	}
	return evaluationtypology.TraitProfileDetail{
		Traits: traits,
		Source: input.Payload.Source,
	}, nil
}

func buildPersonalityDimensions(input DetailInput) ([]evaluationtypology.PersonalityDimensionResult, error) {
	if input.Decision.Kind == trait.DecisionKindPoleComposition || len(input.Decision.Poles) > 0 {
		return buildPolePersonalityDimensions(input)
	}
	if input.Decision.Kind == trait.DecisionKindNearestPattern || len(input.Decision.PatternOrder) > 0 ||
		input.Spec.Decision.Kind == modelcatalog.DecisionKindNearestPattern {
		return buildPatternPersonalityDimensions(input)
	}
	return nil, nil
}

func buildPolePersonalityDimensions(input DetailInput) ([]evaluationtypology.PersonalityDimensionResult, error) {
	if len(input.Vector.Scores) == 0 {
		return nil, nil
	}
	dimensions := make([]evaluationtypology.PersonalityDimensionResult, 0, len(input.Decision.Poles))
	for _, pole := range input.Decision.Poles {
		score, ok := input.Vector.Scores[pole.FactorID]
		if !ok {
			return nil, fmt.Errorf("missing factor score for %s", pole.FactorID)
		}
		meta, ok := dimensionMetaForFactor(input.Spec.FactorGraph, string(pole.FactorID))
		if !ok {
			return nil, fmt.Errorf("pole metadata for factor %s is not defined", pole.FactorID)
		}
		preference, strength := trait.ResolvePole(pole, score.Raw)
		dimensions = append(dimensions, evaluationtypology.PersonalityDimensionResult{
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

func buildPatternPersonalityDimensions(input DetailInput) ([]evaluationtypology.PersonalityDimensionResult, error) {
	if len(input.Vector.Scores) == 0 {
		return nil, nil
	}
	order := input.Decision.PatternOrder
	if len(order) == 0 {
		for _, factorID := range input.Spec.FactorGraph.DecisionFactorOrder() {
			order = append(order, trait.FactorID(factorID))
		}
	}
	dimensions := make([]evaluationtypology.PersonalityDimensionResult, 0, len(order))
	for _, factorID := range order {
		meta, ok := dimensionMetaForFactor(input.Spec.FactorGraph, string(factorID))
		if !ok {
			return nil, fmt.Errorf("pattern metadata for factor %s is not defined", factorID)
		}
		score, ok := input.Vector.Scores[factorID]
		if !ok {
			return nil, fmt.Errorf("missing factor score for %s", factorID)
		}
		dimensions = append(dimensions, evaluationtypology.PersonalityDimensionResult{
			Code:     meta.Code,
			Name:     meta.Name,
			Model:    meta.Model,
			RawScore: score.Raw,
			Level:    trait.LevelForScore(score.Raw, input.Decision.LevelRule),
		})
	}
	return dimensions, nil
}
