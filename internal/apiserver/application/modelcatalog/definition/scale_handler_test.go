package definition

import (
	"context"
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func TestScaleDefinitionHandlerBuildsPayloadFromDefinitionV2(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{Code: "SCALE_A", Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault, Title: "Scale A", Now: now})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	definition := &modeldefinition.Definition{Measure: modeldefinition.MeasureSpec{
		Factors: []factor.Factor{{Code: "TOTAL", Title: "Total", Role: factor.FactorRoleTotal}},
		Scoring: []factor.Scoring{{FactorCode: "TOTAL", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "Q1"}}}},
	}}
	result, err := (ScaleDefinitionHandler{}).BuildSnapshotPayload(context.Background(), modelWithDefinition(model, definition))
	if err != nil {
		t.Fatalf("BuildSnapshotPayload: %v", err)
	}
	if result.PayloadFormat != domain.PayloadFormatAssessmentScaleV1 || len(result.Payload) == 0 || result.DecisionKind != domain.DecisionKindScoreRange {
		t.Fatalf("snapshot result = %#v", result)
	}
}

func modelWithDefinition(model *domain.AssessmentModel, value *modeldefinition.Definition) *domain.AssessmentModel {
	clone := *model
	clone.DefinitionV2 = value
	return &clone
}
