package report

const (
	ScoreKindRawTotal      = "raw_total"
	ScoreKindMatchPercent  = "match_percent"
	ScoreKindTScore        = "t_score"
	ScoreKindPercentile    = "percentile"
	ScoreKindStandardScore = "standard_score"
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

// NormReference identifies the norm table and selected cohort used for a
// dimension's derived score.
type NormReference struct {
	ScoreKind    string
	Benchmark    float64
	TableVersion string
	FormVariant  string
	MinAgeMonths int
	MaxAgeMonths int
	Gender       string
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
