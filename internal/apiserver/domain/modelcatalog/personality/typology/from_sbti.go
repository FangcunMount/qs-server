package typology

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// FromSBTI 转换旧版 SBTI 载荷 到 unified 类型学 form。
func FromSBTI(model *SBTILegacyModel) *Payload {
	if model == nil {
		return nil
	}
	dimensions := make(map[string]Dimension, len(model.Dimensions))
	for code, dim := range model.Dimensions {
		dimensions[code] = Dimension{
			Code:  dim.Code,
			Name:  dim.Name,
			Model: dim.Model,
		}
	}
	mappings := make([]QuestionMapping, 0, len(model.QuestionMappings))
	for _, mapping := range model.QuestionMappings {
		optionScores := make(map[string]float64, len(mapping.OptionScores))
		for key, value := range mapping.OptionScores {
			optionScores[key] = value
		}
		mappings = append(mappings, QuestionMapping{
			QuestionCode: mapping.QuestionCode,
			Dimension:    mapping.Dimension,
			OptionScores: optionScores,
		})
	}
	outcomes := make([]Outcome, 0, len(model.NormalOutcomes)+len(model.SpecialOutcomes))
	for _, outcome := range model.NormalOutcomes {
		outcomes = append(outcomes, outcomeFromSBTILegacy(outcome))
	}
	for _, outcome := range model.SpecialOutcomes {
		outcomes = append(outcomes, outcomeFromSBTILegacy(outcome))
	}
	triggers := make([]SpecialTrigger, 0, len(model.SpecialOutcomes))
	for _, outcome := range model.SpecialOutcomes {
		if outcome.Trigger == "" {
			continue
		}
		trigger := SpecialTrigger{
			Code:        outcome.Code,
			Name:        outcome.Name,
			Trigger:     outcome.Trigger,
			OutcomeCode: outcome.Code,
		}
		if len(model.DrinkTrigger.QuestionCodes) > 0 && strings.HasPrefix(outcome.Trigger, "hidden:") {
			trigger.QuestionCodes = append([]string(nil), model.DrinkTrigger.QuestionCodes...)
			trigger.OptionValues = append([]string(nil), model.DrinkTrigger.OptionValues...)
		}
		triggers = append(triggers, trigger)
	}
	return &Payload{
		Code:                 model.Code,
		Version:              model.Version,
		Title:                model.Title,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Status:               model.Status,
		Source: Source{
			WikiRepo:      model.Source.WikiRepo,
			SourceSite:    model.Source.SourceSite,
			License:       model.Source.License,
			Attribution:   model.Source.Attribution,
			ImageBaseURL:  model.Source.ImageBaseURL,
			NonCommercial: model.Source.NonCommercial,
		},
		Algorithm:        modelcatalog.AlgorithmSBTI,
		DimensionOrder:   append([]string(nil), model.DimensionOrder...),
		Dimensions:       dimensions,
		QuestionMappings: mappings,
		Outcomes:         outcomes,
		MatchingSpec: MatchingSpec{
			Kind:                        modelcatalog.DecisionKindNearestPattern,
			FallbackSimilarityThreshold: model.FallbackSimilarityThreshold,
		},
		SpecialTriggers: triggers,
	}
}

func outcomeFromSBTILegacy(outcome SBTILegacyOutcome) Outcome {
	return Outcome{
		Code:     outcome.Code,
		Name:     outcome.Name,
		OneLiner: outcome.OneLiner,
		Pattern:  outcome.Pattern,
		Image:    outcome.Image,
		Rarity: Rarity{
			Percent: outcome.Rarity.Percent,
			Label:   outcome.Rarity.Label,
			OneInX:  outcome.Rarity.OneInX,
		},
		IsSpecial:  outcome.IsSpecial,
		Trigger:    outcome.Trigger,
		Commentary: outcome.Commentary,
	}
}

// ToSBTI 转换类型学载荷 back 到 旧版 SBTI form。
func ToSBTI(payload *Payload) (*SBTILegacyModel, error) {
	if payload == nil {
		return nil, fmt.Errorf("typology payload is nil")
	}
	if payload.Algorithm != modelcatalog.AlgorithmSBTI {
		return nil, fmt.Errorf("typology algorithm %s is not sbti", payload.Algorithm)
	}
	dimensions := make(map[string]SBTILegacyDimension, len(payload.Dimensions))
	for code, dim := range payload.Dimensions {
		dimensions[code] = SBTILegacyDimension{
			Code:  dim.Code,
			Name:  dim.Name,
			Model: dim.Model,
		}
	}
	mappings := make([]SBTILegacyQuestionMapping, 0, len(payload.QuestionMappings))
	for _, mapping := range payload.QuestionMappings {
		optionScores := make(map[string]float64, len(mapping.OptionScores))
		for key, value := range mapping.OptionScores {
			optionScores[key] = value
		}
		mappings = append(mappings, SBTILegacyQuestionMapping{
			QuestionCode: mapping.QuestionCode,
			Dimension:    mapping.Dimension,
			OptionScores: optionScores,
		})
	}
	normalOutcomes := make([]SBTILegacyOutcome, 0, len(payload.Outcomes))
	specialOutcomes := make([]SBTILegacyOutcome, 0)
	for _, outcome := range payload.Outcomes {
		converted := SBTILegacyOutcome{
			Code:     outcome.Code,
			Name:     outcome.Name,
			OneLiner: outcome.OneLiner,
			Pattern:  outcome.Pattern,
			Image:    outcome.Image,
			Rarity: SBTILegacyRarity{
				Percent: outcome.Rarity.Percent,
				Label:   outcome.Rarity.Label,
				OneInX:  outcome.Rarity.OneInX,
			},
			IsSpecial:  outcome.IsSpecial,
			Trigger:    outcome.Trigger,
			Commentary: outcome.Commentary,
		}
		if outcome.IsSpecial {
			specialOutcomes = append(specialOutcomes, converted)
		} else {
			normalOutcomes = append(normalOutcomes, converted)
		}
	}
	drinkTrigger := SBTILegacyDrinkTrigger{}
	for _, trigger := range payload.SpecialTriggers {
		if len(trigger.QuestionCodes) == 0 {
			continue
		}
		drinkTrigger.QuestionCodes = append([]string(nil), trigger.QuestionCodes...)
		drinkTrigger.OptionValues = append([]string(nil), trigger.OptionValues...)
	}
	return &SBTILegacyModel{
		Code:                 payload.Code,
		Version:              payload.Version,
		Title:                payload.Title,
		QuestionnaireCode:    payload.QuestionnaireCode,
		QuestionnaireVersion: payload.QuestionnaireVersion,
		Status:               payload.Status,
		Source: SBTILegacySource{
			WikiRepo:      payload.Source.WikiRepo,
			SourceSite:    payload.Source.SourceSite,
			License:       payload.Source.License,
			Attribution:   payload.Source.Attribution,
			ImageBaseURL:  payload.Source.ImageBaseURL,
			NonCommercial: payload.Source.NonCommercial,
		},
		DimensionOrder:              append([]string(nil), payload.DimensionOrder...),
		Dimensions:                  dimensions,
		QuestionMappings:            mappings,
		NormalOutcomes:              normalOutcomes,
		SpecialOutcomes:             specialOutcomes,
		FallbackSimilarityThreshold: payload.MatchingSpec.FallbackSimilarityThreshold,
		DrinkTrigger:                drinkTrigger,
	}, nil
}
