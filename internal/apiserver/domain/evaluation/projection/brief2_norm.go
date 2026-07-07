package projection

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	brief2norm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
)

// Brief2NormProjection applies Brief-2 norm/T-score tables on top of raw scale scores.
// Algorithm-specific adapter: not a generic projection primitive.
// When a second norm algorithm appears, extract a shared NormProjection interface and move
// algorithm implementations under evaluation/behavioral_rating/brief2 or modelcatalog extensions.
type Brief2NormProjection struct {
	Tables  *brief2norm.NormTables
	Subject brief2norm.Subject
}

func (p Brief2NormProjection) Apply(outcome *assessment.AssessmentOutcome) *assessment.AssessmentOutcome {
	if outcome == nil || p.Tables == nil {
		return outcome
	}
	dimensions := make([]assessment.DimensionResult, 0, len(outcome.Dimensions))
	for _, dim := range outcome.Dimensions {
		enriched := dim
		if dim.Score == nil {
			dimensions = append(dimensions, enriched)
			continue
		}
		normScore, ok := brief2norm.LookupNormScore(p.Tables, dim.Code, dim.Score.Value, p.Subject)
		if ok {
			enriched.DerivedScores = append(enriched.DerivedScores,
				assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindTScore, Value: normScore.TScore},
				assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindPercentile, Value: normScore.Percentile},
			)
			if level, conclusion, suggestion, interpreted := brief2norm.InterpretTScore(p.Tables, dim.Code, normScore.TScore); interpreted {
				enriched.Level = &assessment.OutcomeResultLevel{Code: level, Label: conclusion}
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
	outcome.Dimensions = dimensions
	if primary := brief2PrimaryDimension(outcome.Dimensions); primary != nil && primary.Level != nil {
		outcome.Level = primary.Level
		if outcome.Summary.Level == nil || *outcome.Summary.Level == "" {
			level := primary.Level.Code
			outcome.Summary.Level = &level
		}
		if primary.Description != "" {
			outcome.Summary.PrimaryLabel = primary.Description
		}
	}
	return outcome
}

func brief2PrimaryDimension(dimensions []assessment.DimensionResult) *assessment.DimensionResult {
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
