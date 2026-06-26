package assessment

// EventModelIdentity is the wire projection of model identity on assessment events.
type EventModelIdentity struct {
	Kind      string `json:"kind"`
	SubKind   string `json:"sub_kind,omitempty"`
	Algorithm string `json:"algorithm,omitempty"`
	Code      string `json:"code"`
	Version   string `json:"version,omitempty"`
	Title     string `json:"title,omitempty"`
}

// EventScoreValue is the wire projection of primary score on assessment events.
type EventScoreValue struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// EventResultLevel is the wire projection of outcome level on assessment events.
type EventResultLevel struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}

func isHighEventSeverity(severity string) bool {
	switch severity {
	case "high", "critical":
		return true
	default:
		return false
	}
}

func isRiskLevelEventCode(code string) bool {
	switch RiskLevel(code) {
	case RiskLevelHigh, RiskLevelSevere:
		return true
	default:
		return false
	}
}
