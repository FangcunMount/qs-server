package outcome

import (
	"encoding/json"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

// MarshalRecordV2 serializes only durable scoring/classification facts. The
// legacy Summary/Tags projection is intentionally not part of schema v2.
func MarshalRecordV2(source *domainoutcome.Execution) ([]byte, error) {
	execution := ExecutionForRecordV2(source)
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

// ExecutionForRecordV2 returns the pure-fact representation written by new
// Outcome records. The caller's in-memory Execution is never mutated.
func ExecutionForRecordV2(source *domainoutcome.Execution) *domainoutcome.Execution {
	if source == nil {
		return nil
	}
	result := *source
	result.Summary.Tags = nil
	if source.Primary != nil {
		primary := *source.Primary
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
	result.Dimensions = append([]domainoutcome.DimensionResult(nil), source.Dimensions...)
	result.Validity = append([]domainoutcome.ValidityResult(nil), source.Validity...)

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
			Code: dimension.Code, Name: dimension.Name, Kind: domainoutcome.DimensionKindPole,
			Score:      &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: raw, Label: dimension.Preference},
			Preference: dimension.Preference, Strength: &strength, LeftPole: dimension.LeftPole,
			RightPole: dimension.RightPole, Model: dimension.Model,
		}
		if dimension.Level != "" {
			item.Level = &domainoutcome.ResultLevel{Code: dimension.Level, Label: dimension.Level}
		}
		result = append(result, item)
	}
	return result
}

func traitDimensions(source []outcometypology.TraitProfileFactorResult) []domainoutcome.DimensionResult {
	result := make([]domainoutcome.DimensionResult, 0, len(source))
	for _, trait := range source {
		result = append(result, domainoutcome.DimensionResult{
			Code: trait.Code, Name: trait.Name, Kind: domainoutcome.DimensionKindTrait,
			Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: trait.RawScore},
		})
	}
	return result
}
