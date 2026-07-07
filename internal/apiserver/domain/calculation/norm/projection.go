package norm

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

// Projection 应用常模/T 分 tables 基于 原始 维度分。
type Projection struct {
	Tables               *NormTables
	Subject              Subject
	PrimaryDimensionCode string
}

// Apply 补充计算结果 使用 常模推导的分数和等级。
func (p Projection) Apply(result *calculation.Result) *calculation.Result {
	if result == nil {
		return result
	}
	if p.Tables != nil {
		dimensions := make([]calculation.DimensionResult, 0, len(result.Dimensions))
		for _, dim := range result.Dimensions {
			enriched := dim
			if dim.Score == nil {
				dimensions = append(dimensions, enriched)
				continue
			}
			normScore, ok := LookupNormScore(p.Tables, dim.Code, dim.Score.Value, p.Subject)
			if ok {
				enriched.DerivedScores = append(enriched.DerivedScores,
					calculation.ScoreValue{Kind: calculation.ScoreKindTScore, Value: normScore.TScore},
					calculation.ScoreValue{Kind: calculation.ScoreKindPercentile, Value: normScore.Percentile},
				)
				if level, conclusion, suggestion, interpreted := InterpretTScore(p.Tables, dim.Code, normScore.TScore); interpreted {
					enriched.Level = &calculation.ResultLevel{Code: level, Label: conclusion}
					if conclusion != "" {
						enriched.Description = conclusion
					}
					if suggestion != "" {
						enriched.Suggestion = suggestion
					}
				}
			}
			dimensions = append(dimensions, enriched)
		}
		result.Dimensions = dimensions
	}
	if primary := primaryDimension(result.Dimensions, p.PrimaryDimensionCode); primary != nil && primary.Level != nil {
		result.Level = primary.Level
		if primary.Description != "" {
			result.PrimaryLabel = primary.Description
		}
	}
	return result
}

func primaryDimension(dimensions []calculation.DimensionResult, configuredCode string) *calculation.DimensionResult {
	if configuredCode != "" {
		for i := range dimensions {
			if dimensions[i].Code == configuredCode {
				return &dimensions[i]
			}
		}
	}
	// Deprecated: 发布时未配置 primary_dimension_code 时的旧版兜底。
	for i := range dimensions {
		if dimensions[i].Code == "total" || dimensions[i].Code == "gec" {
			return &dimensions[i]
		}
	}
	if len(dimensions) == 1 {
		return &dimensions[0]
	}
	return nil
}
