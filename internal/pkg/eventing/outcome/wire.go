package eventoutcome

// ModelIdentity is the wire projection of model identity on outcome-enriched events.
type ModelIdentity struct {
	Kind      string `json:"kind"`
	Algorithm string `json:"algorithm,omitempty"`
	Code      string `json:"code"`
	Version   string `json:"version,omitempty"`
	Title     string `json:"title,omitempty"`
}

// ScoreValue is the wire projection of primary score on outcome-enriched events.
type ScoreValue struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// ResultLevel is the wire projection of outcome level on outcome-enriched events.
type ResultLevel struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}
