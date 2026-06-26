package report

// EventModelIdentity is the wire projection of ModelIdentity on domain events.
type EventModelIdentity struct {
	Kind      string `json:"kind"`
	SubKind   string `json:"sub_kind,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`
	Code      string `json:"code"`
	Version   string `json:"version,omitempty"`
	Title     string `json:"title,omitempty"`
}

// EventScoreValue is the wire projection of ScoreValue on domain events.
type EventScoreValue struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// EventResultLevel is the wire projection of ResultLevel on domain events.
type EventResultLevel struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}

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
