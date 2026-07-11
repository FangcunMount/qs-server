package calculationadapter

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

func scoreValueFromOutcome(score *domainoutcome.ScoreValue) *calculation.ScoreValue {
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

func scoreValueToOutcome(score *calculation.ScoreValue) *domainoutcome.ScoreValue {
	if score == nil {
		return nil
	}
	return &domainoutcome.ScoreValue{
		Kind:  domainoutcome.ScoreKind(score.Kind),
		Value: score.Value,
		Label: score.Label,
		Max:   score.Max,
	}
}

func levelFromOutcome(level *domainoutcome.ResultLevel) *calculation.ResultLevel {
	if level == nil {
		return nil
	}
	return &calculation.ResultLevel{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}

func levelToOutcome(level *calculation.ResultLevel) *domainoutcome.ResultLevel {
	if level == nil {
		return nil
	}
	return &domainoutcome.ResultLevel{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}
