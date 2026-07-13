package handlers

import (
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
)

func isHighRiskRiskLevel(riskLevel string) bool {
	return eventoutcome.IsHighRiskCode(riskLevel)
}

func isHighRiskOutcomeLevel(level *eventoutcome.ResultLevel) bool {
	return eventoutcome.LevelIsHighRisk(level)
}

func attentionRiskLevelFromOutcome(level *eventoutcome.ResultLevel) string {
	return eventoutcome.AttentionRiskLevel(level)
}
