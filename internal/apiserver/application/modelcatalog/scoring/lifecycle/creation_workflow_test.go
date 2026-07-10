package lifecycle

import (
	"context"
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestCreateUsesAssessmentModelRepository(t *testing.T) {
	ctx := context.Background()
	modelRepo := &authoringModelRepoStub{}
	svc := newAuthoringLifecycleService(nil, modelRepo, nil)

	got, err := svc.Create(ctx, shared.CreateScaleDTO{
		Code:           "SCL_CREATE",
		Title:          "Create Scale",
		Description:    "draft scale",
		Category:       "mental",
		Stages:         []string{"child"},
		ApplicableAges: []string{"6-12"},
		Reporters:      []string{"parent"},
		Tags:           []string{"screening"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if modelRepo.createCount != 1 {
		t.Fatalf("model repo Create calls = %d, want 1", modelRepo.createCount)
	}
	model := modelRepo.created
	if model == nil {
		t.Fatal("created model = nil")
	}
	if model.Code != "SCL_CREATE" || model.Kind != domain.KindScale || model.ProductChannel != domain.ProductChannelMedicalScale {
		t.Fatalf("created model identity = %#v, want scale medical channel", model)
	}
	if model.Definition.Format != domain.PayloadFormatAssessmentScaleV1 || len(model.Definition.Data) == 0 {
		t.Fatalf("definition payload = format %q len %d, want assessment_scale_v1 bytes", model.Definition.Format, len(model.Definition.Data))
	}
	if model.DefinitionV2 == nil {
		t.Fatal("DefinitionV2 = nil, want materialized target definition")
	}
	if !reflect.DeepEqual(model.Stages, []string{"child"}) ||
		!reflect.DeepEqual(model.ApplicableAges, []string{"6-12"}) ||
		!reflect.DeepEqual(model.Reporters, []string{"parent"}) {
		t.Fatalf("created model audience metadata stages=%v ages=%v reporters=%v",
			model.Stages, model.ApplicableAges, model.Reporters)
	}
	if got == nil || got.Code != "SCL_CREATE" || got.Title != "Create Scale" || got.Status != "draft" {
		t.Fatalf("result = %#v, want created draft scale", got)
	}
}
