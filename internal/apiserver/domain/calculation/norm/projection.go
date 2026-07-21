package norm

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

// Projection 应用常模/T 分 tables 基于 原始 维度分。
type Projection struct {
	Tables               *NormTables
	Subject              Subject
	PrimaryDimensionCode string
	RequiredFactorCodes  []string
}

// Apply 补充计算结果 使用 常模推导的分数和等级。
func (p Projection) Apply(result *calculation.Result) (*calculation.Result, error) {
	if result == nil {
		return result, nil
	}
	required := make(map[string]struct{}, len(p.RequiredFactorCodes))
	for _, factorCode := range p.RequiredFactorCodes {
		if factorCode != "" {
			required[factorCode] = struct{}{}
		}
	}
	if p.Tables == nil {
		if factorCode := firstRequiredFactor(p.RequiredFactorCodes); factorCode != "" {
			return nil, resolutionError(ErrorKindInvalid, factorCode, nil, fmt.Errorf("required norm tables are missing"))
		}
	} else {
		if err := ValidateTables(p.Tables); err != nil {
			return nil, resolutionError(ErrorKindInvalid, firstRequiredFactor(p.RequiredFactorCodes), nil, err)
		}
		resolvedRequired := make(map[string]struct{}, len(required))
		dimensions := make([]calculation.DimensionResult, 0, len(result.Dimensions))
		for _, dim := range result.Dimensions {
			enriched := dim
			_, isRequired := required[dim.Code]
			_, hasNormTable := factorTable(p.Tables, dim.Code)
			if !isRequired && !hasNormTable {
				dimensions = append(dimensions, enriched)
				continue
			}
			if dim.Score == nil {
				if isRequired {
					return nil, resolutionError(ErrorKindInvalid, dim.Code, nil, fmt.Errorf("required factor raw score is missing"))
				}
				dimensions = append(dimensions, enriched)
				continue
			}
			resolution, err := ResolveNormScore(p.Tables, dim.Code, dim.Score.Value, p.Subject)
			if err != nil {
				if isRequired {
					return nil, err
				}
			} else {
				normScore := resolution.Score
				enriched.DerivedScores = append(enriched.DerivedScores,
					calculation.ScoreValue{Kind: calculation.ScoreKindTScore, Value: normScore.TScore},
					calculation.ScoreValue{Kind: calculation.ScoreKindPercentile, Value: normScore.Percentile},
				)
				if normScore.StandardScore != nil {
					enriched.DerivedScores = append(enriched.DerivedScores, calculation.ScoreValue{Kind: calculation.ScoreKindStandardScore, Value: *normScore.StandardScore})
				}
				enriched.NormReference = &calculation.NormReference{
					ScoreKind: calculation.ScoreKindTScore, Benchmark: 50,
					TableVersion: p.Tables.NormTableVersion, FormVariant: p.Tables.FormVariant,
					MinAgeMonths: normScore.Reference.MinAgeMonths, MaxAgeMonths: normScore.Reference.MaxAgeMonths,
					Gender: normScore.Reference.Gender,
				}
				if level, _, _, interpreted := InterpretTScore(p.Tables, dim.Code, normScore.TScore); interpreted {
					// Decision path keeps OutcomeCode only; presentation is restored at Interpretation (MC-R016).
					enriched.Level = &calculation.ResultLevel{Code: level}
				}
				if isRequired {
					resolvedRequired[dim.Code] = struct{}{}
				}
			}
			dimensions = append(dimensions, enriched)
		}
		for _, factorCode := range p.RequiredFactorCodes {
			if factorCode == "" {
				continue
			}
			if _, ok := resolvedRequired[factorCode]; !ok {
				return nil, resolutionError(ErrorKindInvalid, factorCode, nil, fmt.Errorf("required norm factor is absent from calculation result"))
			}
		}
		result.Dimensions = dimensions
	}
	if primary := primaryDimension(result.Dimensions, p.PrimaryDimensionCode); primary != nil && primary.Level != nil {
		result.Level = primary.Level
	}
	return result, nil
}

func firstRequiredFactor(factorCodes []string) string {
	for _, factorCode := range factorCodes {
		if factorCode != "" {
			return factorCode
		}
	}
	return ""
}

func primaryDimension(dimensions []calculation.DimensionResult, configuredCode string) *calculation.DimensionResult {
	if configuredCode == "" {
		return nil
	}
	for i := range dimensions {
		if dimensions[i].Code == configuredCode {
			return &dimensions[i]
		}
	}
	return nil
}
