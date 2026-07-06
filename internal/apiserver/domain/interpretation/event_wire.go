package interpretation

import "github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"

// EventModelIdentity is the wire projection of ModelIdentity on domain events.
type EventModelIdentity = eventoutcome.ModelIdentity

// EventScoreValue is the wire projection of ScoreValue on domain events.
type EventScoreValue = eventoutcome.ScoreValue

// EventResultLevel is the wire projection of ResultLevel on domain events.
type EventResultLevel = eventoutcome.ResultLevel

func EventModelIdentityFrom(model ModelIdentity) EventModelIdentity {
	return EventModelIdentity(model)
}

func EventScoreValueFrom(score *ScoreValue) *EventScoreValue {
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

func EventResultLevelFrom(level *ResultLevel) *EventResultLevel {
	if level == nil {
		return nil
	}
	return &EventResultLevel{
		Code:     level.Code,
		Label:    level.Label,
		Severity: level.Severity,
	}
}
