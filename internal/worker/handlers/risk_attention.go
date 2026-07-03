package handlers

import (
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
)

func isHighRiskRiskLevel(riskLevel string) bool {
	return domainAssessment.IsHighRisk(domainAssessment.RiskLevel(riskLevel))
}

func isHighRiskV2Level(level *eventoutcome.ResultLevel) bool {
	return eventoutcome.LevelIsHighRisk(level)
}

func attentionRiskLevelFromV2(level *eventoutcome.ResultLevel) string {
	return eventoutcome.AttentionRiskLevel(level)
}
