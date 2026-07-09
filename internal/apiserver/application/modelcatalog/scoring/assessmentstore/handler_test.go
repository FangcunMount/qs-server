package assessmentstore

import (
	"context"
	"encoding/json"
	"testing"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/snapshot"
)

func TestDefinitionHandlerPrepareForSaveMaterializesDefinitionV2(t *testing.T) {
	payload, err := json.Marshal(&scalesnapshot.ScaleSnapshot{
		Code:         "SCL_DEF",
		ScaleVersion: "1.0.0",
		Title:        "Scale",
		Factors: []scalesnapshot.FactorSnapshot{
			{Code: "F1", Title: "Factor 1", QuestionCodes: []string{"Q1"}},
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	handler := DefinitionHandler{}
	result, issues, err := handler.PrepareForSave(context.Background(), nil, appdefinition.SaveInput{
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("PrepareForSave() error = %v", err)
	}
	if len(issues) > 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
	if result.DefinitionV2 == nil || len(result.DefinitionV2.Measure.Factors) != 1 {
		t.Fatalf("DefinitionV2 = %#v, want one factor", result.DefinitionV2)
	}
	if result.Payload.Format != domain.PayloadFormatAssessmentScaleV1 {
		t.Fatalf("payload format = %q", result.Payload.Format)
	}
}

func TestDefinitionHandlerSupportsScaleIdentity(t *testing.T) {
	handler := DefinitionHandler{}
	if !handler.Supports(domain.Identity{Kind: domain.KindScale}) {
		t.Fatal("Supports(scale) = false, want true")
	}
	if handler.Supports(domain.Identity{Kind: domain.KindTypology}) {
		t.Fatal("Supports(typology) = true, want false")
	}
}
