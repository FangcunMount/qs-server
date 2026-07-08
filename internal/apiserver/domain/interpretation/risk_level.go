package interpretation

// IsRiskLevelCode 报告是否编码是旧量表风险等级值。
func IsRiskLevelCode(code string) bool {
	switch RiskLevel(code) {
	case RiskLevelNone, RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelSevere:
		return true
	default:
		return false
	}
}
