package outcome

import (
	"encoding/json"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

// MarshalRecordV2 serializes only durable scoring/classification facts. The
// Presentation Summary/Tags are intentionally not part of current schema v2.
func MarshalRecordV2(source *domainoutcome.Execution) ([]byte, error) {
	execution := executionForRecordV2(source)
	if execution == nil {
		return json.Marshal(nil)
	}
	type payload struct {
		ModelRef   domainoutcome.ModelRef
		Detail     domainoutcome.Detail
		Primary    *domainoutcome.ScoreValue
		Level      *domainoutcome.ResultLevel
		Profile    *domainoutcome.ProfileResult
		Dimensions []domainoutcome.DimensionResult
		Validity   []domainoutcome.ValidityResult
	}
	return json.Marshal(payload{
		ModelRef: execution.ModelRef, Detail: execution.Detail, Primary: execution.Primary,
		Level: execution.Level, Profile: execution.Profile, Dimensions: execution.Dimensions,
		Validity: execution.Validity,
	})
}

// executionForRecordV2 returns the pure-fact representation written by new
// Outcome records. The caller's in-memory Execution is never mutated.
func executionForRecordV2(source *domainoutcome.Execution) *domainoutcome.Execution {
	if source == nil {
		return nil
	}
	result := *source
	result.ModelRef.ModelTitle = ""
	result.Summary.Tags = nil
	if source.Primary != nil {
		primary := *source.Primary
		primary.Label = ""
		result.Primary = &primary
	}
	if source.Level != nil {
		level := *source.Level
		level.Label = ""
		result.Level = &level
	}
	if source.Profile != nil {
		profile := *source.Profile
		profile.Name = ""
		profile.Traits = append([]string(nil), source.Profile.Traits...)
		result.Profile = &profile
	}
	result.Dimensions = pureFactDimensions(source.Dimensions)
	result.Validity = pureFactValidity(source.Validity)

	switch detail := source.Detail.Payload.(type) {
	case outcometypology.PersonalityTypeDetail:
		result.Detail.Payload = classificationFact(detail)
		if len(result.Dimensions) == 0 {
			result.Dimensions = personalityDimensions(detail.Dimensions)
		}
	case *outcometypology.PersonalityTypeDetail:
		if detail != nil {
			result.Detail.Payload = classificationFact(*detail)
			if len(result.Dimensions) == 0 {
				result.Dimensions = personalityDimensions(detail.Dimensions)
			}
		}
	case outcometypology.TraitProfileDetail:
		result.Detail.Payload = nil
		if len(result.Dimensions) == 0 {
			result.Dimensions = traitDimensions(detail.Traits)
		}
	case *outcometypology.TraitProfileDetail:
		result.Detail.Payload = nil
		if detail != nil && len(result.Dimensions) == 0 {
			result.Dimensions = traitDimensions(detail.Traits)
		}
	}
	return &result
}

func pureFactDimensions(source []domainoutcome.DimensionResult) []domainoutcome.DimensionResult {
	result := make([]domainoutcome.DimensionResult, 0, len(source))
	for _, dimension := range source {
		item := dimension
		item.Name, item.LeftPole, item.RightPole, item.Model = "", "", "", ""
		if dimension.Score != nil {
			score := *dimension.Score
			score.Label = ""
			item.Score = &score
		}
		item.DerivedScores = append([]domainoutcome.ScoreValue(nil), dimension.DerivedScores...)
		for i := range item.DerivedScores {
			item.DerivedScores[i].Label = ""
		}
		if dimension.Level != nil {
			level := *dimension.Level
			level.Label = ""
			item.Level = &level
		}
		if dimension.NormReference != nil {
			reference := *dimension.NormReference
			item.NormReference = &reference
		}
		result = append(result, item)
	}
	return result
}

func pureFactValidity(source []domainoutcome.ValidityResult) []domainoutcome.ValidityResult {
	result := make([]domainoutcome.ValidityResult, 0, len(source))
	for _, validity := range source {
		validity.Label, validity.Message = "", ""
		result = append(result, validity)
	}
	return result
}

func classificationFact(detail outcometypology.PersonalityTypeDetail) outcometypology.ClassificationFact {
	return outcometypology.ClassificationFact{
		TypeCode: detail.TypeCode, Pattern: detail.Pattern,
		MatchPercent: detail.MatchPercent, Similarity: detail.Similarity,
		SpecialTrigger: detail.SpecialTrigger, IsSpecial: detail.IsSpecial,
	}
}

func personalityDimensions(source []outcometypology.PersonalityDimensionResult) []domainoutcome.DimensionResult {
	result := make([]domainoutcome.DimensionResult, 0, len(source))
	for _, dimension := range source {
		raw, strength := dimension.RawScore, dimension.Strength
		item := domainoutcome.DimensionResult{
			Code:       dimension.Code,
			Kind:       domainoutcome.DimensionKindPole,
			Score:      &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: raw},
			Preference: dimension.Preference,
			Strength:   &strength,
		}
		if dimension.Level != "" {
			item.Level = &domainoutcome.ResultLevel{Code: dimension.Level}
		}
		result = append(result, item)
	}
	return result
}

func traitDimensions(source []outcometypology.TraitProfileFactorResult) []domainoutcome.DimensionResult {
	result := make([]domainoutcome.DimensionResult, 0, len(source))
	for _, trait := range source {
		result = append(result, domainoutcome.DimensionResult{
			Code: trait.Code, Kind: domainoutcome.DimensionKindTrait,
			Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: trait.RawScore},
		})
	}
	return result
}
