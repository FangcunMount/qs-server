package eventoutcome

// RiskLevel is the legacy scale risk-level code carried on outcome events.
type RiskLevel string

const (
	RiskLevelNone   RiskLevel = "none"
	RiskLevelLow    RiskLevel = "low"
	RiskLevelMedium RiskLevel = "medium"
	RiskLevelHigh   RiskLevel = "high"
	RiskLevelSevere RiskLevel = "severe"
)

// IsHighSeverity reports whether severity should trigger high-risk workflows.
func IsHighSeverity(severity string) bool {
	return severity == "high"
}

// IsRiskLevelCode reports whether code is a legacy scale risk-level value.
func IsRiskLevelCode(code string) bool {
	switch RiskLevel(code) {
	case RiskLevelNone, RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelSevere:
		return true
	default:
		return false
	}
}

// IsHighRiskCode reports whether code maps to high or severe risk.
func IsHighRiskCode(code string) bool {
	switch RiskLevel(code) {
	case RiskLevelHigh, RiskLevelSevere:
		return true
	default:
		return false
	}
}

// LevelIsHighRisk reports whether an outcome level should trigger high-risk workflows.
func LevelIsHighRisk(level *ResultLevel) bool {
	if level == nil {
		return false
	}
	if IsHighSeverity(level.Severity) {
		return true
	}
	if IsRiskLevelCode(level.Code) {
		return IsHighRiskCode(level.Code)
	}
	return false
}

// AttentionRiskLevel maps an outcome level to the legacy risk_level used by attention sync.
func AttentionRiskLevel(level *ResultLevel) string {
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
