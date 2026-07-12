package report

import "github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"

const (
	ScoreKindRawTotal     = "raw_total"
	ScoreKindMatchPercent = "match_percent"
)

// ScoreValue 是规范主 score on report。
type ScoreValue struct {
	Kind  string
	Value float64
	Label string
	Max   *float64
}

// ResultLevel 是规范结果等级 on report。
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

// IsHighSeverity 报告是否 severity 应该 trigger high-risk workflows。
func IsHighSeverity(severity string) bool {
	return eventoutcome.IsHighSeverity(severity)
}
