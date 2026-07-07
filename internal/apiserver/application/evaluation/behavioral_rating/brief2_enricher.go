package behavioralrating

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	brief2norm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// EnrichBrief2Outcome applies Brief-2 norm/T-score projection on top of raw scale scoring.
func EnrichBrief2Outcome(
	outcome *assessment.AssessmentOutcome,
	snapshot *behavioralsnapshot.Snapshot,
	subject brief2norm.Subject,
) *assessment.AssessmentOutcome {
	if outcome == nil || snapshot == nil || snapshot.Brief2 == nil {
		return outcome
	}
	tables := snapshot.Brief2.NormTablesOrNil()
	if tables == nil {
		return outcome
	}
	dimensions := make([]assessment.DimensionResult, 0, len(outcome.Dimensions))
	for _, dim := range outcome.Dimensions {
		enriched := dim
		if dim.Score == nil {
			dimensions = append(dimensions, enriched)
			continue
		}
		normScore, ok := brief2norm.LookupNormScore(tables, dim.Code, dim.Score.Value, subject)
		if ok {
			enriched.DerivedScores = append(enriched.DerivedScores,
				assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindTScore, Value: normScore.TScore},
				assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKindPercentile, Value: normScore.Percentile},
			)
			if level, conclusion, suggestion, interpreted := brief2norm.InterpretTScore(tables, dim.Code, normScore.TScore); interpreted {
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
	if primary := primaryDimension(outcome.Dimensions); primary != nil && primary.Level != nil {
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

func NormSubjectFromInput(input *evaluationinput.InputSnapshot) brief2norm.Subject {
	if input == nil || input.NormSubject == nil {
		return brief2norm.Subject{}
	}
	return brief2norm.Subject{
		AgeMonths: input.NormSubject.AgeMonths,
		Gender:    input.NormSubject.Gender,
	}
}

func primaryDimension(dimensions []assessment.DimensionResult) *assessment.DimensionResult {
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
