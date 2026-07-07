package behavioralrating

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	brief2norm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
)

// brief2NormProjection applies Brief-2 norm/T-score tables on top of raw scale scores.
// It lives in the application layer because it depends on Brief-2 norm-table domain assets
// (a modelcatalog algorithm extension) and is not a generic calculation primitive.
type brief2NormProjection struct {
	tables               *brief2norm.NormTables
	subject              brief2norm.Subject
	primaryDimensionCode string
}

func (p brief2NormProjection) apply(result *calculation.Result) *calculation.Result {
	if result == nil {
		return result
	}
	if p.tables != nil {
		dimensions := make([]calculation.DimensionResult, 0, len(result.Dimensions))
		for _, dim := range result.Dimensions {
			enriched := dim
			if dim.Score == nil {
				dimensions = append(dimensions, enriched)
				continue
			}
			normScore, ok := brief2norm.LookupNormScore(p.tables, dim.Code, dim.Score.Value, p.subject)
			if ok {
				enriched.DerivedScores = append(enriched.DerivedScores,
					calculation.ScoreValue{Kind: calculation.ScoreKindTScore, Value: normScore.TScore},
					calculation.ScoreValue{Kind: calculation.ScoreKindPercentile, Value: normScore.Percentile},
				)
				if level, conclusion, suggestion, interpreted := brief2norm.InterpretTScore(p.tables, dim.Code, normScore.TScore); interpreted {
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
	if primary := primaryDimension(result.Dimensions, p.primaryDimensionCode); primary != nil && primary.Level != nil {
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
	// Deprecated: legacy fallback when primary_dimension_code is not configured on publish.
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
