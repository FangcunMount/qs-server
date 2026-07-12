package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
)

// EventModelIdentity 是线缆投影 of ModelIdentity on 领域事件。
type EventModelIdentity = eventoutcome.ModelIdentity

// EventScoreValue 是线缆投影 of ScoreValue on 领域事件。
type EventScoreValue = eventoutcome.ScoreValue

// EventResultLevel 是线缆投影 of ResultLevel on 领域事件。
type EventResultLevel = eventoutcome.ResultLevel

func EventModelIdentityFrom(model report.ModelIdentity) EventModelIdentity {
	return EventModelIdentity(model)
}

func EventScoreValueFrom(score *report.ScoreValue) *EventScoreValue {
	if score == nil {
		return nil
	}
	return &EventScoreValue{
		Kind:  score.Kind,
		Value: score.Value,
		Label: score.Label,
		Max:   score.Max,
	}
}

func EventResultLevelFrom(level *report.ResultLevel) *EventResultLevel {
	if level == nil {
		return nil
	}
	return &EventResultLevel{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}
