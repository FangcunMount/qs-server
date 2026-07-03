package handlers

import (
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
)

func isHighRiskRiskLevel(riskLevel string) bool {
	return eventoutcome.IsHighRiskCode(riskLevel)
}

func isHighRiskV2Level(level *eventoutcome.ResultLevel) bool {
	return eventoutcome.LevelIsHighRisk(level)
}

func attentionRiskLevelFromV2(level *eventoutcome.ResultLevel) string {
	return eventoutcome.AttentionRiskLevel(level)
}
