package calculationadapter

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// CalcResultFromOutcome translates 测评结果 为 计算结果。
func CalcResultFromOutcome(outcome *assessment.AssessmentOutcome) *calculation.Result {
	if outcome == nil {
		return nil
	}
	result := &calculation.Result{
		PrimaryLabel: outcome.Summary.PrimaryLabel,
		Dimensions:   make([]calculation.DimensionResult, 0, len(outcome.Dimensions)),
	}
	if outcome.Primary != nil {
		result.Primary = scoreValueFromOutcome(outcome.Primary)
	}
	if outcome.Level != nil {
		result.Level = levelFromOutcome(outcome.Level)
	}
	for _, dim := range outcome.Dimensions {
		result.Dimensions = append(result.Dimensions, dimensionResultFromOutcome(dim))
	}
	return result
}

// MergeCalcResultIntoOutcome merges 计算结果 back 为 测评结果。
func MergeCalcResultIntoOutcome(outcome *assessment.AssessmentOutcome, result *calculation.Result) *assessment.AssessmentOutcome {
	if outcome == nil || result == nil {
		return outcome
	}
	if result.Primary != nil {
		outcome.Primary = scoreValueToOutcome(result.Primary)
	}
	if result.Level != nil {
		outcome.Level = levelToOutcome(result.Level)
	}
	if result.PrimaryLabel != "" {
		outcome.Summary.PrimaryLabel = result.PrimaryLabel
		if (outcome.Summary.Level == nil || *outcome.Summary.Level == "") && result.Level != nil {
			level := result.Level.Code
			if level != "" {
				outcome.Summary.Level = &level
			}
		}
	}
	outcome.Dimensions = mergeDimensionResults(outcome.Dimensions, result.Dimensions)
	return outcome
}

func mergeDimensionResults(existing []assessment.DimensionResult, calculated []calculation.DimensionResult) []assessment.DimensionResult {
	if len(calculated) == 0 {
		return existing
	}
	byCode := make(map[string]int, len(existing))
	for i := range existing {
		byCode[existing[i].Code] = i
	}
	for _, dim := range calculated {
		if pos, ok := byCode[dim.Code]; ok {
			existing[pos] = mergeDimensionResult(existing[pos], dim)
			continue
		}
		existing = append(existing, dimensionResultToOutcome(dim))
		byCode[dim.Code] = len(existing) - 1
	}
	return existing
}

func mergeDimensionResult(existing assessment.DimensionResult, calculated calculation.DimensionResult) assessment.DimensionResult {
	merged := dimensionResultToOutcome(calculated)
	if existing.Name != "" {
		merged.Name = existing.Name
	}
	if existing.Kind != "" {
		merged.Kind = existing.Kind
	}
	if existing.Score != nil && merged.Score == nil {
		merged.Score = existing.Score
	}
	return merged
}

func dimensionResultFromOutcome(dim assessment.DimensionResult) calculation.DimensionResult {
	out := calculation.DimensionResult{
		Code:           dim.Code,
		Name:           dim.Name,
		Kind:           CalculationKindFromAssessment(dim.Kind),
		Role:           dim.Role,
		ParentCode:     dim.ParentCode,
		HierarchyLevel: dim.HierarchyLevel,
		SortOrder:      dim.SortOrder,
		Description:    dim.Description,
		Suggestion:     dim.Suggestion,
	}
	if dim.Score != nil {
		out.Score = scoreValueFromOutcome(dim.Score)
	}
	if len(dim.DerivedScores) > 0 {
		out.DerivedScores = make([]calculation.ScoreValue, 0, len(dim.DerivedScores))
		for _, score := range dim.DerivedScores {
			if converted := scoreValueFromOutcome(&score); converted != nil {
				out.DerivedScores = append(out.DerivedScores, *converted)
			}
		}
	}
	if dim.Level != nil {
		out.Level = levelFromOutcome(dim.Level)
	}
	return out
}

func dimensionResultToOutcome(dim calculation.DimensionResult) assessment.DimensionResult {
	out := assessment.DimensionResult{
		Code:           dim.Code,
		Name:           dim.Name,
		Kind:           AssessmentKindFromCalculation(dim.Kind),
		Role:           dim.Role,
		ParentCode:     dim.ParentCode,
		HierarchyLevel: dim.HierarchyLevel,
		SortOrder:      dim.SortOrder,
		Description:    dim.Description,
		Suggestion:     dim.Suggestion,
	}
	if dim.Score != nil {
		out.Score = scoreValueToOutcome(dim.Score)
	}
	if len(dim.DerivedScores) > 0 {
		out.DerivedScores = make([]assessment.OutcomeScoreValue, 0, len(dim.DerivedScores))
		for _, score := range dim.DerivedScores {
			if converted := scoreValueToOutcome(&score); converted != nil {
				out.DerivedScores = append(out.DerivedScores, *converted)
			}
		}
	}
	if dim.Level != nil {
		out.Level = levelToOutcome(dim.Level)
	}
	return out
}
