package calculationadapter

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

// CalcResultFromOutcome translates canonical Execution directly to calculation.Result.
func CalcResultFromOutcome(execution *domainoutcome.Execution) *calculation.Result {
	if execution == nil {
		return nil
	}
	result := &calculation.Result{
		PrimaryLabel: execution.Summary.PrimaryLabel,
		Primary:      scoreValueFromOutcome(execution.Primary),
		Level:        levelFromOutcome(execution.Level),
		Dimensions:   make([]calculation.DimensionResult, 0, len(execution.Dimensions)),
	}
	for _, dimension := range execution.Dimensions {
		result.Dimensions = append(result.Dimensions, dimensionResultFromOutcome(dimension))
	}
	return result
}

// MergeCalcResultIntoOutcome merges calculation facts directly into Execution.
func MergeCalcResultIntoOutcome(execution *domainoutcome.Execution, result *calculation.Result) *domainoutcome.Execution {
	if execution == nil || result == nil {
		return execution
	}
	if result.Primary != nil {
		execution.Primary = scoreValueToOutcome(result.Primary)
	}
	if result.Level != nil {
		execution.Level = levelToOutcome(result.Level)
	}
	if result.PrimaryLabel != "" {
		execution.Summary.PrimaryLabel = result.PrimaryLabel
		if (execution.Summary.Level == nil || *execution.Summary.Level == "") && result.Level != nil && result.Level.Code != "" {
			level := result.Level.Code
			execution.Summary.Level = &level
		}
	}
	execution.Dimensions = mergeDimensionResults(execution.Dimensions, result.Dimensions)
	return execution
}

func mergeDimensionResults(existing []domainoutcome.DimensionResult, calculated []calculation.DimensionResult) []domainoutcome.DimensionResult {
	if len(calculated) == 0 {
		return existing
	}
	byCode := make(map[string]int, len(existing))
	for index := range existing {
		byCode[existing[index].Code] = index
	}
	for _, dimension := range calculated {
		if position, ok := byCode[dimension.Code]; ok {
			existing[position] = mergeDimensionResult(existing[position], dimension)
			continue
		}
		existing = append(existing, dimensionResultToOutcome(dimension))
		byCode[dimension.Code] = len(existing) - 1
	}
	return existing
}

func mergeDimensionResult(existing domainoutcome.DimensionResult, calculated calculation.DimensionResult) domainoutcome.DimensionResult {
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

func dimensionResultFromOutcome(dimension domainoutcome.DimensionResult) calculation.DimensionResult {
	result := calculation.DimensionResult{
		Code: dimension.Code, Name: dimension.Name, Kind: CalculationKindFromOutcome(dimension.Kind),
		Role: dimension.Role, ParentCode: dimension.ParentCode,
		HierarchyLevel: dimension.HierarchyLevel, SortOrder: dimension.SortOrder,
		Score: scoreValueFromOutcome(dimension.Score), Level: levelFromOutcome(dimension.Level),
	}
	if dimension.NormReference != nil {
		result.NormReference = &calculation.NormReference{
			ScoreKind: calculation.ScoreKind(dimension.NormReference.ScoreKind), Benchmark: dimension.NormReference.Benchmark,
			TableVersion: dimension.NormReference.TableVersion, FormVariant: dimension.NormReference.FormVariant,
			MinAgeMonths: dimension.NormReference.MinAgeMonths, MaxAgeMonths: dimension.NormReference.MaxAgeMonths,
			Gender: dimension.NormReference.Gender,
		}
	}
	for _, score := range dimension.DerivedScores {
		result.DerivedScores = append(result.DerivedScores, *scoreValueFromOutcome(&score))
	}
	return result
}

func dimensionResultToOutcome(dimension calculation.DimensionResult) domainoutcome.DimensionResult {
	result := domainoutcome.DimensionResult{
		Code: dimension.Code, Name: dimension.Name, Kind: OutcomeKindFromCalculation(dimension.Kind),
		Role: dimension.Role, ParentCode: dimension.ParentCode,
		HierarchyLevel: dimension.HierarchyLevel, SortOrder: dimension.SortOrder,
		Score: scoreValueToOutcome(dimension.Score), Level: levelToOutcome(dimension.Level),
	}
	if dimension.NormReference != nil {
		result.NormReference = &domainoutcome.NormReference{
			ScoreKind: domainoutcome.ScoreKind(dimension.NormReference.ScoreKind), Benchmark: dimension.NormReference.Benchmark,
			TableVersion: dimension.NormReference.TableVersion, FormVariant: dimension.NormReference.FormVariant,
			MinAgeMonths: dimension.NormReference.MinAgeMonths, MaxAgeMonths: dimension.NormReference.MaxAgeMonths,
			Gender: dimension.NormReference.Gender,
		}
	}
	for _, score := range dimension.DerivedScores {
		result.DerivedScores = append(result.DerivedScores, *scoreValueToOutcome(&score))
	}
	return result
}
