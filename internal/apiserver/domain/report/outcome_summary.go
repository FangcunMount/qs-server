package report

const (
	ScoreKindRawTotal     = "raw_total"
	ScoreKindMatchPercent = "match_percent"
)

// ScoreValue is the canonical primary score on a report.
type ScoreValue struct {
	Kind  string
	Value float64
	Label string
	Max   *float64
}

// ResultLevel is the canonical outcome level on a report.
type ResultLevel struct {
	Code     string
	Label    string
	Severity string
}

func NewRawTotalScore(value float64, max *float64) *ScoreValue {
	return &ScoreValue{Kind: ScoreKindRawTotal, Value: value, Max: max}
}

func NewMatchPercentScore(value float64, label string) *ScoreValue {
	return &ScoreValue{Kind: ScoreKindMatchPercent, Value: value, Label: label}
}

func LevelFromRisk(risk RiskLevel) *ResultLevel {
	if risk == "" {
		return nil
	}
	return &ResultLevel{
		Code:     string(risk),
		Label:    string(risk),
		Severity: severityFromRisk(risk),
	}
}

func severityFromRisk(risk RiskLevel) string {
	switch risk {
	case RiskLevelSevere, RiskLevelHigh:
		return "high"
	case RiskLevelMedium:
		return "medium"
	case RiskLevelLow:
		return "low"
	default:
		return "none"
	}
}

// IsHighSeverity reports whether severity should trigger high-risk workflows.
func IsHighSeverity(severity string) bool {
	return severity == "high"
}

// AttentionRiskLevel maps a v2 level projection to the legacy risk_level used by attention sync.
func AttentionRiskLevel(level *EventResultLevel) string {
	if level == nil {
		return string(RiskLevelNone)
	}
	if IsRiskLevelCode(level.Code) {
		return level.Code
	}
	switch level.Severity {
	case "high":
		return string(RiskLevelHigh)
	case "medium":
		return string(RiskLevelMedium)
	case "low":
		return string(RiskLevelLow)
	default:
		return string(RiskLevelNone)
	}
}
