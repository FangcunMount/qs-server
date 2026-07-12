package outcome

import (
	"testing"

	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestScoreFactUsesFrozenOutcomeMaxScoreAndName(t *testing.T) {
	max := 20.0
	execution := &domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{{
		Code: "F1", Name: "Frozen factor", Score: &domainoutcome.ScoreValue{Value: 8, Max: &max},
	}}}
	names, maxScores, _ := scoreMetadataFromRecord(nil, execution)
	projection := domainassessment.NewScaleScoreProjection(
		meta.FromUint64(1), 8, domainassessment.RiskLevelNone,
		[]domainassessment.ScaleFactorScore{domainassessment.NewScaleFactorScore(domainassessment.NewFactorCode("F1"), "projection name", 8, domainassessment.RiskLevelNone, false)},
	)
	fact := scoreFactFromProjection(projection, names, maxScores)
	if len(fact.FactorScores) != 1 || fact.FactorScores[0].FactorName != "Frozen factor" || fact.FactorScores[0].MaxScore == nil || *fact.FactorScores[0].MaxScore != 20 {
		t.Fatalf("score fact = %#v", fact)
	}
}
