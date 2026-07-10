package norming_test

import (
	"context"
	"testing"

	appdefinition "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/norming"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDefinitionHandlerPrepareForSaveUsesDefinitionV2(t *testing.T) {
	t.Parallel()

	definitionV2 := &domain.Definition{Measure: domain.MeasureSpec{
		Factors: []domain.Factor{{Code: "total", Title: "Total"}},
	}}
	payload := []byte(`{"dimensions":[{"code":"legacy-only"}]}`)
	result, issues, err := (norming.DefinitionHandler{}).PrepareForSave(context.Background(), nil, appdefinition.SaveInput{
		Payload:      payload,
		DefinitionV2: definitionV2,
	})
	if err != nil {
		t.Fatalf("PrepareForSave: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if result.DefinitionV2 != definitionV2 || result.DefinitionV2.Measure.Factors[0].Code != "total" {
		t.Fatalf("DefinitionV2 = %#v", result.DefinitionV2)
	}
	if string(result.Payload.Data) != string(payload) {
		t.Fatalf("payload = %q, want %q", result.Payload.Data, payload)
	}
}

func TestDefinitionHandlerPrepareForSaveRejectsMissingDefinitionV2(t *testing.T) {
	t.Parallel()

	_, issues, err := (norming.DefinitionHandler{}).PrepareForSave(context.Background(), nil, appdefinition.SaveInput{
		Payload: []byte(`{"dimensions":[]}`),
	})
	if err != nil {
		t.Fatalf("PrepareForSave: %v", err)
	}
	if len(issues) != 1 || issues[0].Code != "definition_v2.required" {
		t.Fatalf("issues = %#v", issues)
	}
}
