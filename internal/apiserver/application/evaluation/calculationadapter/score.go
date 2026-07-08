package calculationadapter

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

func scoreValueFromOutcome(score *assessment.OutcomeScoreValue) *calculation.ScoreValue {
	if score == nil {
		return nil
	}
	return &calculation.ScoreValue{
		Kind:  calculation.ScoreKind(score.Kind),
		Value: score.Value,
		Label: score.Label,
		Max:   score.Max,
	}
}

func scoreValueToOutcome(score *calculation.ScoreValue) *assessment.OutcomeScoreValue {
	if score == nil {
		return nil
	}
	return &assessment.OutcomeScoreValue{
		Kind:  assessment.OutcomeScoreKind(score.Kind),
		Value: score.Value,
		Label: score.Label,
		Max:   score.Max,
	}
}

func levelFromOutcome(level *assessment.OutcomeResultLevel) *calculation.ResultLevel {
	if level == nil {
		return nil
	}
	return &calculation.ResultLevel{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}

func levelToOutcome(level *calculation.ResultLevel) *assessment.OutcomeResultLevel {
	if level == nil {
		return nil
	}
	return &assessment.OutcomeResultLevel{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}
