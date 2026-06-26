package handlers

import (
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func isHighRiskRiskLevel(riskLevel string) bool {
	return domainAssessment.IsHighRisk(domainAssessment.RiskLevel(riskLevel))
}

func isHighRiskV2Level(level *domainReport.EventResultLevel) bool {
	if level == nil {
		return false
	}
	if domainReport.IsHighSeverity(level.Severity) {
		return true
	}
	if domainReport.IsRiskLevelCode(level.Code) {
		return domainAssessment.IsHighRisk(domainAssessment.RiskLevel(level.Code))
	}
	return false
}

func attentionRiskLevelFromV2(level *domainReport.EventResultLevel) string {
	return domainReport.AttentionRiskLevel(level)
}
