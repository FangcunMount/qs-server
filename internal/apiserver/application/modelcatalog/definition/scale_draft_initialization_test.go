package definition

import (
	"context"
	"strings"
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestInitializeScaleDraftDoesNotRequireOrInventPublishConclusions(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	model, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{Code: "M5-QR-DRAFT", Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault, Title: "Dedicated technical draft", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := InitializeScaleDefinition(model, now.Add(time.Minute)); err != nil {
		t.Fatalf("empty draft must be creatable before authoring conclusions: %v", err)
	}
	if !model.IsDraft() || model.DefinitionV2 == nil || len(model.DefinitionV2.Conclusions) != 0 || !model.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatal("initialization invented publish content or changed draft status")
	}
	if _, err := (RuntimeMaterializer{}).MaterializeScale(model); err == nil || !strings.Contains(err.Error(), "risk conclusion") {
		t.Fatalf("empty draft became publishable runtime: %v", err)
	}
	issues := (ScaleDefinitionHandler{}).ValidateForPublish(context.Background(), model)
	rejected := false
	for _, issue := range issues {
		if issue.Code == "definition_v2.decision.invalid" {
			rejected = true
		}
	}
	if !rejected {
		t.Fatalf("publication validation lost decision requirement: %+v", issues)
	}
}
