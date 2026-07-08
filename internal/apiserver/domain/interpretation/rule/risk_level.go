package rule

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"

// IsRiskLevelCode 报告是否编码是旧量表风险等级值。
func IsRiskLevelCode(code string) bool {
	switch report.RiskLevel(code) {
	case report.RiskLevelNone, report.RiskLevelLow, report.RiskLevelMedium, report.RiskLevelHigh, report.RiskLevelSevere:
		return true
	default:
		return false
	}
}
